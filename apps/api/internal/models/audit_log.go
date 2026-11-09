package models

// AuditLog records sensitive user operations for security and compliance.
// UserID is nil for events with no signed-in user (e.g. a failed login), and
// ResourceID is nil when the resource has no UUID; see auditService.Log.
type AuditLog struct {
	Base
	UserID       *string `gorm:"type:uuid;index" json:"user_id"`
	Action       string  `gorm:"not null" json:"action"`
	ResourceType string  `gorm:"not null" json:"resource_type"`
	ResourceID   *string `gorm:"type:uuid" json:"resource_id"`
	IPAddress    string  `json:"ip_address"`
	Changes      string  `json:"changes,omitempty"`
	// Source is where the event came from: "web" (the default) or "mcp" for a
	// change made by a connected app. ClientID is that app's OAuth client.
	Source   string `gorm:"type:varchar(20);not null;default:web" json:"source"`
	ClientID string `gorm:"type:varchar(255);not null;default:''" json:"client_id"`
}

// Audit log sources.
const (
	AuditSourceWeb = "web"
	AuditSourceMCP = "mcp"
)
