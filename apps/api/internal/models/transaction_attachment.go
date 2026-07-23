package models

// TransactionAttachment is a receipt (image or PDF) attached to a transaction.
// The bytes live in the blob store (MinIO/S3); only metadata and an opaque
// storage key are persisted here. See plans/017-transaction-receipts.
type TransactionAttachment struct {
	Base
	UserID         string `gorm:"type:uuid;not null" json:"user_id"`
	TransactionID  string `gorm:"type:uuid;not null" json:"transaction_id"`
	StorageKey     string `gorm:"type:varchar(512);not null" json:"-"`
	FileName       string `gorm:"type:varchar(255);not null;default:''" json:"file_name"`
	ContentType    string `gorm:"type:varchar(128);not null" json:"content_type"`
	ByteSize       int64  `gorm:"type:bigint;not null" json:"byte_size"`
	ChecksumSHA256 string `gorm:"type:char(64);not null" json:"-"`
}
