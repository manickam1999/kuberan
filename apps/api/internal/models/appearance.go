package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Appearance is the user's UI preferences, stored as JSON in users.appearance.
// A missing (empty) key means "default". See plan 023 item 1.8.
type Appearance struct {
	// Palette is "default" or a palette id (internal/appearance).
	Palette string `json:"palette,omitempty"`
	// Mode is "system", "light" or "dark".
	Mode string `json:"mode,omitempty"`
	// ChartStyle is "clean" or "dither".
	ChartStyle string `json:"chart_style,omitempty"`
	// Density is "comfortable" or "compact".
	Density string `json:"density,omitempty"`
	// ShowCents toggles cents in money displays.
	ShowCents *bool `json:"show_cents,omitempty"`
}

// Value serialises the preferences to JSON text for the database. The zero
// value becomes "{}", matching the column default.
func (a Appearance) Value() (driver.Value, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("marshal appearance: %w", err)
	}
	return string(b), nil
}

// Scan reads the preferences from a JSON text/bytes column value.
func (a *Appearance) Scan(src interface{}) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*a = Appearance{}
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("scan appearance: unsupported type %T", src)
	}
	if len(raw) == 0 {
		*a = Appearance{}
		return nil
	}
	var out Appearance
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("unmarshal appearance: %w", err)
	}
	*a = out
	return nil
}

// GormDataType names the column type family.
func (Appearance) GormDataType() string { return "json" }

// GormDBDataType maps the type to jsonb on PostgreSQL and text elsewhere
// (SQLite in tests), so the same model works on both.
func (Appearance) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Name() == "postgres" {
		return "jsonb"
	}
	return "text"
}
