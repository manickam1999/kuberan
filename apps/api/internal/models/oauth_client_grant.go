package models

import (
	"slices"
	"strings"
	"time"
)

// OAuthClientGrant records what a user has consented to for one OAuth client
// (plan 022). It replaces the global trusted_oauth_clients table: grants are
// per user and per scope, so a client trusted for read:* is never silently
// auto-accepted for a scope the user has not reviewed (e.g. write:*).
//
// ApprovedScopes is what the user granted; ReviewedScopes is everything the
// consent screen has shown them (approved or declined). AutoAccept is the
// "remember this application" choice. A revoked grant keeps its row so the
// MCP server can refuse writes from tokens issued before the revocation.
type OAuthClientGrant struct {
	Base
	UserID         string `gorm:"type:uuid;not null;uniqueIndex:idx_oauth_client_grants_user_client,where:deleted_at IS NULL" json:"user_id"`
	ClientID       string `gorm:"type:varchar(255);not null;uniqueIndex:idx_oauth_client_grants_user_client,where:deleted_at IS NULL" json:"client_id"`
	ClientName     string `gorm:"type:varchar(255);not null;default:''" json:"client_name"`
	ApprovedScopes string `gorm:"type:text;not null;default:''" json:"-"`
	ReviewedScopes string `gorm:"type:text;not null;default:''" json:"-"`
	AutoAccept     bool   `gorm:"not null;default:false" json:"auto_accept"`
	// ClientHost is the host the app's authorization code is sent to
	// (claude.ai); "" until the user's next consent.
	ClientHost string     `gorm:"type:varchar(255);not null;default:''" json:"client_host"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// TableName pins the table name so GORM does not split "OAuth" into "o_auth";
// it must match the SQL migration (oauth_client_grants).
func (OAuthClientGrant) TableName() string {
	return "oauth_client_grants"
}

// Approved returns the granted scopes.
func (g *OAuthClientGrant) Approved() []string {
	return strings.Fields(g.ApprovedScopes)
}

// Reviewed returns every scope the consent screen has shown the user.
func (g *OAuthClientGrant) Reviewed() []string {
	return strings.Fields(g.ReviewedScopes)
}

// HasApproved reports whether scope is in the granted set.
func (g *OAuthClientGrant) HasApproved(scope string) bool {
	return slices.Contains(g.Approved(), scope)
}

// IsRevoked reports whether the user has disconnected this client.
func (g *OAuthClientGrant) IsRevoked() bool {
	return g.RevokedAt != nil
}
