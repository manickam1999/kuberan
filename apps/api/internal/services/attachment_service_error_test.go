package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/storage"
	"kuberan/internal/testutil"
)

// newBrokenDB returns a gorm DB connected to a private in-memory SQLite with no
// tables migrated, so every query fails with a "no such table" error (which is
// distinct from gorm.ErrRecordNotFound). This exercises the ErrInternalServer
// DB-failure branches that a healthy DB never reaches.
func newBrokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open broken db: %v", err)
	}
	return db
}

// errReader is an io.Reader that always fails, used to exercise Upload's
// read-failure branch after ownership and cap checks have passed.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom: read failed") }

// putErrStore is a BlobStore whose Put always fails; Get/Delete are inert.
type putErrStore struct{}

func (putErrStore) Put(context.Context, string, io.Reader, string, int64) error {
	return errors.New("boom: put failed")
}
func (putErrStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, storage.ErrNotFound
}
func (putErrStore) Delete(context.Context, string) error { return nil }

// getErrStore is a BlobStore whose Get fails with a non-ErrNotFound error, used
// to exercise Open's ErrInternalServer branch (distinct from the missing-object
// ATTACHMENT_NOT_FOUND branch).
type getErrStore struct{}

func (getErrStore) Put(context.Context, string, io.Reader, string, int64) error { return nil }
func (getErrStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("boom: get failed")
}
func (getErrStore) Delete(context.Context, string) error { return nil }

// dropOnPutStore wraps a real store but drops the transaction_attachments table
// as a side effect of a successful Put. This lets the metadata Create that
// follows Put fail deterministically, exercising the orphan-cleanup branch. It
// records whether the cleanup Delete was invoked.
type dropOnPutStore struct {
	storage.BlobStore
	db      *gorm.DB
	deleted bool
}

func (s *dropOnPutStore) Put(ctx context.Context, key string, r io.Reader, ct string, size int64) error {
	if err := s.BlobStore.Put(ctx, key, r, ct, size); err != nil {
		return err
	}
	return s.db.Migrator().DropTable(&models.TransactionAttachment{})
}

func (s *dropOnPutStore) Delete(ctx context.Context, key string) error {
	s.deleted = true
	return s.BlobStore.Delete(ctx, key)
}

// seedTxn creates a user, cash account, and one transaction, returning the user
// ID and transaction ID for attachment tests.
func seedTxn(t *testing.T, db *gorm.DB) (userID, txID string) {
	t.Helper()
	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
	if err := db.Create(tx).Error; err != nil {
		t.Fatalf("create tx: %v", err)
	}
	return user.ID, tx.ID
}

// TestAttachmentUploadDBErrors covers the Upload paths that surface
// ErrInternalServer: a failing ownership lookup, a failing per-transaction count
// query, a failing body read, a failing blob Put, and a failing metadata Create
// (which must trigger best-effort orphan cleanup).
func TestAttachmentUploadDBErrors(t *testing.T) {
	data := smallPNG(t, 16, 16)

	t.Run("ownership lookup failure is an internal error", func(t *testing.T) {
		svc := NewAttachmentService(newBrokenDB(t), storage.NewMemBlobStore(), defaultLimits())
		_, err := svc.Upload("u1", "t1", "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
	})

	t.Run("count query failure is an internal error", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		userID, txID := seedTxn(t, db)
		// Ownership check will succeed, but the attachment count query then fails.
		if err := db.Migrator().DropTable(&models.TransactionAttachment{}); err != nil {
			t.Fatalf("drop table: %v", err)
		}
		svc := NewAttachmentService(db, storage.NewMemBlobStore(), defaultLimits())
		_, err := svc.Upload(userID, txID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
	})

	t.Run("body read failure is an internal error", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		userID, txID := seedTxn(t, db)
		svc := NewAttachmentService(db, storage.NewMemBlobStore(), defaultLimits())
		_, err := svc.Upload(userID, txID, "r.png", "image/png", 128, errReader{})
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
	})

	t.Run("blob put failure is an internal error", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		userID, txID := seedTxn(t, db)
		svc := NewAttachmentService(db, putErrStore{}, defaultLimits())
		_, err := svc.Upload(userID, txID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
	})

	t.Run("metadata create failure cleans up the orphaned object", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		userID, txID := seedTxn(t, db)
		store := &dropOnPutStore{BlobStore: storage.NewMemBlobStore(), db: db}
		svc := NewAttachmentService(db, store, defaultLimits())
		_, err := svc.Upload(userID, txID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
		if !store.deleted {
			t.Errorf("expected best-effort orphan cleanup Delete to be called after Create failure")
		}
	})
}

// TestAttachmentOpenStoreError covers Open's ErrInternalServer branch when the
// blob store's Get fails with an error other than ErrNotFound (the missing-object
// case is covered separately by TestAttachmentOpenMissingObject).
func TestAttachmentOpenStoreError(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	userID, txID := seedTxn(t, db)

	att := &models.TransactionAttachment{
		UserID:         userID,
		TransactionID:  txID,
		StorageKey:     userID + "/" + txID + "/obj.png",
		FileName:       "r.png",
		ContentType:    storage.ContentTypePNG,
		ByteSize:       10,
		ChecksumSHA256: "deadbeef",
	}
	if err := db.Create(att).Error; err != nil {
		t.Fatalf("create attachment row: %v", err)
	}

	svc := NewAttachmentService(db, getErrStore{}, defaultLimits())
	_, _, err := svc.Open(userID, att.ID)
	testutil.AssertAppError(t, err, "INTERNAL_ERROR")
}

// TestAttachmentListDBError covers List's ErrInternalServer branch when the
// query fails.
func TestAttachmentListDBError(t *testing.T) {
	svc := NewAttachmentService(newBrokenDB(t), storage.NewMemBlobStore(), defaultLimits())
	_, err := svc.List("u1", "t1")
	testutil.AssertAppError(t, err, "INTERNAL_ERROR")
}

// TestAttachmentOpenDeleteDBError covers the getOwned ErrInternalServer branch
// (a non-RecordNotFound lookup failure) reached through both Open and Delete.
func TestAttachmentOpenDeleteDBError(t *testing.T) {
	svc := NewAttachmentService(newBrokenDB(t), storage.NewMemBlobStore(), defaultLimits())

	t.Run("open surfaces internal error on lookup failure", func(t *testing.T) {
		_, _, err := svc.Open("u1", "a1")
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
	})

	t.Run("delete surfaces internal error on lookup failure", func(t *testing.T) {
		err := svc.Delete("u1", "a1")
		testutil.AssertAppError(t, err, "INTERNAL_ERROR")
	})
}
