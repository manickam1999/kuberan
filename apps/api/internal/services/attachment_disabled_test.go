package services

import (
	"bytes"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/storage"
	"kuberan/internal/testutil"
)

// A server without a blob store (no STORAGE_BUCKET) must say so with a clear
// 503 rather than an internal error.
func TestAttachmentUploadWithoutStorageIsReceiptsDisabled(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	tx := &models.Transaction{UserID: user.ID, AccountID: account.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: time.Now()}
	if err := db.Create(tx).Error; err != nil {
		t.Fatalf("create tx: %v", err)
	}

	svc := NewAttachmentService(db, storage.NewDisabledBlobStore(), defaultLimits())
	data := smallPNG(t, 10, 10)
	_, err := svc.Upload(user.ID, tx.ID, "r.png", "image/png", int64(len(data)), bytes.NewReader(data))
	testutil.AssertAppError(t, err, "RECEIPTS_DISABLED")
}
