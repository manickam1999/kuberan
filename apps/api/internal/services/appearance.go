package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"kuberan/internal/appearance"
	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// AppearancePatch is a partial update of users.appearance (PATCH /profile).
// A nil field is left unchanged; only keys present in the JSON body are
// applied. Unknown keys are rejected when decoding. The binding tags validate
// values (palette against the shared palette list).
type AppearancePatch struct {
	// Palette is "default" (follow the server default) or a palette id.
	Palette *string `json:"palette" binding:"omitempty,palette" example:"catppuccin"`
	// Mode is system, light or dark.
	Mode *string `json:"mode" binding:"omitempty,oneof=system light dark" enums:"system,light,dark"`
	// ChartStyle is clean or dither.
	ChartStyle *string `json:"chart_style" binding:"omitempty,oneof=clean dither" enums:"clean,dither"`
	// Density is comfortable or compact.
	Density *string `json:"density" binding:"omitempty,oneof=comfortable compact" enums:"comfortable,compact"`
	// ShowCents toggles cents in money displays.
	ShowCents *bool `json:"show_cents"`
}

// UnmarshalJSON decodes the patch and rejects keys that are not appearance
// preferences, so a typo is an error instead of a silent no-op.
func (p *AppearancePatch) UnmarshalJSON(data []byte) error {
	type plain AppearancePatch
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var v plain
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("appearance: %w", err)
	}
	*p = AppearancePatch(v)
	return nil
}

// IsEmpty reports whether the patch changes nothing.
func (p *AppearancePatch) IsEmpty() bool {
	return p == nil || (p.Palette == nil && p.Mode == nil && p.ChartStyle == nil &&
		p.Density == nil && p.ShowCents == nil)
}

// validate checks every present value; it backs the service for callers that
// bypass the handler's binding tags.
func (p *AppearancePatch) validate() error {
	if p == nil {
		return nil
	}
	oneOf := func(field string, v *string, allowed ...string) error {
		if v == nil {
			return nil
		}
		for _, a := range allowed {
			if *v == a {
				return nil
			}
		}
		return apperrors.WithMessage(apperrors.ErrInvalidInput, fmt.Sprintf("invalid appearance %s %q", field, *v))
	}
	if p.Palette != nil && !appearance.IsPaletteChoice(*p.Palette) {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, fmt.Sprintf("unknown appearance palette %q", *p.Palette))
	}
	if err := oneOf("mode", p.Mode, "system", "light", "dark"); err != nil {
		return err
	}
	if err := oneOf("chart_style", p.ChartStyle, "clean", "dither"); err != nil {
		return err
	}
	return oneOf("density", p.Density, "comfortable", "compact")
}

// MergeAppearance returns base with every field set in patch applied.
func MergeAppearance(base models.Appearance, patch *AppearancePatch) models.Appearance {
	if patch == nil {
		return base
	}
	if patch.Palette != nil {
		base.Palette = *patch.Palette
	}
	if patch.Mode != nil {
		base.Mode = *patch.Mode
	}
	if patch.ChartStyle != nil {
		base.ChartStyle = *patch.ChartStyle
	}
	if patch.Density != nil {
		base.Density = *patch.Density
	}
	if patch.ShowCents != nil {
		v := *patch.ShowCents
		base.ShowCents = &v
	}
	return base
}

// applyProfileChanges writes changes to the user. With an appearance patch it
// reads the stored preferences, merges the patch in Go (not with a Postgres
// jsonb || so SQLite behaves the same) and writes both in one transaction.
func (s *userService) applyProfileChanges(userID string, changes map[string]interface{}, patch *AppearancePatch) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if !patch.IsEmpty() {
			var user models.User
			if err := tx.Select("id", "appearance").Where("id = ?", userID).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperrors.ErrUserNotFound
				}
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
			changes["appearance"] = MergeAppearance(user.Appearance, patch)
		}
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(changes).Error; err != nil {
			if isUniqueViolation(err) {
				return apperrors.ErrDuplicateEmail
			}
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		return nil
	})
}
