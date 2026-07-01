package models

// TrustedOAuthClient records an OAuth client that a user has approved via
// trust-on-first-use (TOFU) consent. Once a client_id is present here, the
// consent step auto-accepts for that client instead of prompting again.
type TrustedOAuthClient struct {
	Base
	ClientID string `gorm:"type:varchar(255);uniqueIndex;not null" json:"client_id"`
	Name     string `gorm:"type:varchar(255);not null;default:''" json:"name"`
}

// TableName pins the table name so GORM does not split "OAuth" into
// "o_auth"; it must match the SQL migration (trusted_oauth_clients).
func (TrustedOAuthClient) TableName() string {
	return "trusted_oauth_clients"
}
