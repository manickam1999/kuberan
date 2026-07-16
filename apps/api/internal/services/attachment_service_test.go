package services

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/storage"
	"kuberan/internal/testutil"
)

// smallPNG returns the bytes of a valid w×h opaque PNG for use as a fake
// receipt upload.
func smallPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func defaultLimits() AttachmentLimits {
	return AttachmentLimits{MaxUploadBytes: 10 * 1024 * 1024, MaxAttachmentsPerTx: 3}
}

func TestAttachmentUpload(t *testing.T) {
	t.Run("happy path stores object and metadata", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)

		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("create tx: %v", err)
		}

		store := storage.NewMemBlobStore()
		svc := NewAttachmentService(db, store, defaultLimits())

		data := smallPNG(t, 20, 20)
		att, err := svc.Upload(user.ID, tx.ID, "  my receipt.png ", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertNoError(t, err)

		if att.ContentType != storage.ContentTypePNG {
			t.Errorf("content type = %q, want %q", att.ContentType, storage.ContentTypePNG)
		}
		if att.FileName != "my receipt.png" {
			t.Errorf("file name = %q, want sanitized %q", att.FileName, "my receipt.png")
		}
		if att.StorageKey == "" || len(att.ChecksumSHA256) != 64 {
			t.Errorf("expected opaque key and sha256 checksum, got key=%q checksum=%q", att.StorageKey, att.ChecksumSHA256)
		}
		// The bytes must actually be in the store under the opaque key.
		rc, err := store.Get(context.Background(), att.StorageKey)
		testutil.AssertNoError(t, err)
		_ = rc.Close()
	})

	t.Run("rejects oversize payload", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)

		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("create tx: %v", err)
		}

		limits := AttachmentLimits{MaxUploadBytes: 64, MaxAttachmentsPerTx: 3}
		svc := NewAttachmentService(db, storage.NewMemBlobStore(), limits)

		big := bytes.Repeat([]byte{0x01}, 128)
		_, err := svc.Upload(user.ID, tx.ID, "big.png", "image/png", int64(len(big)), bytes.NewReader(big))
		testutil.AssertAppError(t, err, "PAYLOAD_TOO_LARGE")
	})

	t.Run("rejects unsupported media type", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)

		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("create tx: %v", err)
		}

		svc := NewAttachmentService(db, storage.NewMemBlobStore(), defaultLimits())
		junk := []byte("this is a plain text file, not an image")
		_, err := svc.Upload(user.ID, tx.ID, "notes.txt", "text/plain", int64(len(junk)), bytes.NewReader(junk))
		testutil.AssertAppError(t, err, "UNSUPPORTED_MEDIA_TYPE")
	})

	t.Run("enforces per-transaction limit", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)

		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("create tx: %v", err)
		}

		limits := AttachmentLimits{MaxUploadBytes: 10 * 1024 * 1024, MaxAttachmentsPerTx: 2}
		svc := NewAttachmentService(db, storage.NewMemBlobStore(), limits)
		data := smallPNG(t, 10, 10)

		for i := 0; i < 2; i++ {
			_, err := svc.Upload(user.ID, tx.ID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
			testutil.AssertNoError(t, err)
		}
		_, err := svc.Upload(user.ID, tx.ID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertAppError(t, err, "ATTACHMENT_LIMIT")
	})

	t.Run("rejects upload to another user's transaction", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)

		owner := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, owner.ID)
		tx := &models.Transaction{UserID: owner.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("create tx: %v", err)
		}
		attacker := testutil.CreateTestUserWithEmail(t, db, "attacker@example.com")

		svc := NewAttachmentService(db, storage.NewMemBlobStore(), defaultLimits())
		data := smallPNG(t, 10, 10)
		_, err := svc.Upload(attacker.ID, tx.ID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})
}

func TestAttachmentListOpenDelete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
	if err := db.Create(tx).Error; err != nil {
		t.Fatalf("create tx: %v", err)
	}

	store := storage.NewMemBlobStore()
	svc := NewAttachmentService(db, store, defaultLimits())
	data := smallPNG(t, 12, 12)
	att, err := svc.Upload(user.ID, tx.ID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
	testutil.AssertNoError(t, err)

	t.Run("list returns the attachment", func(t *testing.T) {
		list, err := svc.List(user.ID, tx.ID)
		testutil.AssertNoError(t, err)
		if len(list) != 1 || list[0].ID != att.ID {
			t.Fatalf("expected 1 attachment %s, got %+v", att.ID, list)
		}
	})

	t.Run("open streams the stored bytes for the owner", func(t *testing.T) {
		meta, rc, err := svc.Open(user.ID, att.ID)
		testutil.AssertNoError(t, err)
		defer func() { _ = rc.Close() }()
		got, err := io.ReadAll(rc)
		testutil.AssertNoError(t, err)
		if int64(len(got)) != meta.ByteSize {
			t.Errorf("streamed %d bytes, metadata says %d", len(got), meta.ByteSize)
		}
	})

	t.Run("open rejects a non-owner", func(t *testing.T) {
		other := testutil.CreateTestUserWithEmail(t, db, "other@example.com")
		_, _, err := svc.Open(other.ID, att.ID)
		testutil.AssertAppError(t, err, "ATTACHMENT_NOT_FOUND")
	})

	t.Run("delete removes both the row and the object", func(t *testing.T) {
		key := att.StorageKey
		err := svc.Delete(user.ID, att.ID)
		testutil.AssertNoError(t, err)

		list, err := svc.List(user.ID, tx.ID)
		testutil.AssertNoError(t, err)
		if len(list) != 0 {
			t.Errorf("expected no attachments after delete, got %d", len(list))
		}
		if _, err := store.Get(context.Background(), key); err == nil {
			t.Errorf("expected object gone from store after delete")
		}
	})
}
