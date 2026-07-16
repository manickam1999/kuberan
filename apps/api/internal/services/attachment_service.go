package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/storage"
)

// attachmentService handles transaction receipt attachments: it validates
// ownership, sanitizes untrusted uploads, and coordinates the blob store with
// the metadata table. See plans/017-transaction-receipts.
type attachmentService struct {
	db     *gorm.DB
	store  storage.BlobStore
	limits AttachmentLimits
}

// NewAttachmentService creates a new AttachmentServicer backed by the given
// blob store. The concrete type is unexported to match house style.
func NewAttachmentService(db *gorm.DB, store storage.BlobStore, limits AttachmentLimits) AttachmentServicer {
	return &attachmentService{db: db, store: store, limits: limits}
}

// Upload sanitizes and stores a receipt for a transaction the user owns.
// The blob is written before the metadata row so a mid-flight failure leaves an
// orphan object (cleaned up best-effort) rather than a dangling DB row that
// points at nothing.
func (s *attachmentService) Upload(userID, txID, fileName, declaredType string, size int64, data io.Reader) (*models.TransactionAttachment, error) {
	_ = size // an advisory hint only; the real cap is enforced on the byte count below.

	// Ownership: the transaction must exist and belong to the caller.
	var tx models.Transaction
	if err := s.db.Where("id = ? AND user_id = ?", txID, userID).First(&tx).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTransactionNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Enforce the per-transaction cap (non-deleted rows only).
	var count int64
	if err := s.db.Model(&models.TransactionAttachment{}).
		Where("transaction_id = ?", txID).Count(&count).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if int(count) >= s.limits.MaxAttachmentsPerTx {
		return nil, apperrors.ErrAttachmentLimit
	}

	// Read the upload with a hard byte cap independent of the caller's hint.
	// One extra byte lets us detect an over-limit body without reading it all.
	raw, err := io.ReadAll(io.LimitReader(data, s.limits.MaxUploadBytes+1))
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if int64(len(raw)) > s.limits.MaxUploadBytes {
		return nil, apperrors.ErrPayloadTooLarge
	}

	// Sniff + sanitize: strips EXIF/GPS, defuses decompression bombs, and
	// transcodes WebP to JPEG. The declared type is never trusted.
	clean, contentType, err := storage.Normalize(raw, declaredType)
	if err != nil {
		return nil, mapNormalizeError(err)
	}

	sum := sha256.Sum256(clean)
	checksum := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("%s/%s/%s%s", userID, txID, uuid.NewString(), extensionFor(contentType))

	ctx := context.Background()
	if err := s.store.Put(ctx, key, bytes.NewReader(clean), contentType, int64(len(clean))); err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	att := &models.TransactionAttachment{
		UserID:         userID,
		TransactionID:  txID,
		StorageKey:     key,
		FileName:       sanitizeFileName(fileName),
		ContentType:    contentType,
		ByteSize:       int64(len(clean)),
		ChecksumSHA256: checksum,
	}
	if err := s.db.Create(att).Error; err != nil {
		_ = s.store.Delete(ctx, key) // best-effort orphan cleanup
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return att, nil
}

// List returns the attachment metadata for a user's transaction, oldest first.
func (s *attachmentService) List(userID, txID string) ([]models.TransactionAttachment, error) {
	var atts []models.TransactionAttachment
	if err := s.db.Where("transaction_id = ? AND user_id = ?", txID, userID).
		Order("created_at ASC").Find(&atts).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return atts, nil
}

// Open returns an attachment's metadata and a byte stream after an ownership
// check. The caller must Close the returned reader.
func (s *attachmentService) Open(userID, attachmentID string) (*models.TransactionAttachment, io.ReadCloser, error) {
	att, err := s.getOwned(userID, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.store.Get(context.Background(), att.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil, apperrors.ErrAttachmentNotFound
		}
		return nil, nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return att, rc, nil
}

// Delete soft-deletes the metadata row and best-effort removes the stored
// object. A failed object delete does not fail the request: the row is already
// gone and the orphan is reclaimable out-of-band.
func (s *attachmentService) Delete(userID, attachmentID string) error {
	att, err := s.getOwned(userID, attachmentID)
	if err != nil {
		return err
	}
	if err := s.db.Delete(att).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	_ = s.store.Delete(context.Background(), att.StorageKey) // best-effort
	return nil
}

// getOwned loads an attachment and asserts it belongs to the user.
func (s *attachmentService) getOwned(userID, attachmentID string) (*models.TransactionAttachment, error) {
	var att models.TransactionAttachment
	if err := s.db.Where("id = ? AND user_id = ?", attachmentID, userID).First(&att).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAttachmentNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &att, nil
}

// mapNormalizeError translates storage sanitization errors into client-facing
// AppErrors.
func mapNormalizeError(err error) error {
	switch {
	case errors.Is(err, storage.ErrUnsupportedMediaType):
		return apperrors.ErrUnsupportedMediaType
	case errors.Is(err, storage.ErrImageTooLarge):
		return apperrors.WithMessage(apperrors.ErrPayloadTooLarge, "image dimensions exceed the maximum allowed")
	case errors.Is(err, storage.ErrCorruptImage):
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "file could not be decoded as a valid image")
	default:
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
}

// extensionFor returns the canonical file extension for a normalized content
// type. Normalize only ever emits these three types.
func extensionFor(contentType string) string {
	switch contentType {
	case storage.ContentTypePNG:
		return ".png"
	case storage.ContentTypePDF:
		return ".pdf"
	default: // JPEG (raster output, including transcoded WebP)
		return ".jpg"
	}
}

// sanitizeFileName reduces an untrusted upload filename to a safe display value:
// base name only (no path components), control characters removed, length
// capped to the column width. The result is never used in a storage key.
func sanitizeFileName(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" || name == ".." {
		name = ""
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
