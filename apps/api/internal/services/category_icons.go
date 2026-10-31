package services

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	apperrors "kuberan/internal/errors"
)

// categoryIconNames is the curated set of named category icons (Lucide icon
// names). A category icon is either one of these or a single emoji. Keep in
// sync with CATEGORY_ICONS in apps/web/src/lib/category-icons.ts.
var categoryIconNames = map[string]struct{}{
	"baby": {}, "banknote": {}, "bike": {}, "book": {}, "briefcase": {},
	"building-2": {}, "bus": {}, "car": {}, "coffee": {}, "coins": {},
	"credit-card": {}, "droplet": {}, "dumbbell": {}, "film": {}, "fuel": {},
	"gamepad-2": {}, "gift": {}, "globe": {}, "graduation-cap": {},
	"hand-coins": {}, "heart-pulse": {}, "home": {}, "house": {}, "landmark": {},
	"laptop": {}, "music": {}, "party-popper": {}, "paw-print": {},
	"piggy-bank": {}, "pill": {}, "plane": {}, "receipt": {}, "scissors": {},
	"shirt": {}, "shopping-bag": {}, "shopping-cart": {}, "smartphone": {},
	"sprout": {}, "tag": {}, "train-front": {}, "trending-up": {}, "tv": {},
	"utensils": {}, "wallet": {}, "wifi": {}, "wrench": {}, "zap": {},
}

// maxEmojiRunes bounds an emoji icon. ZWJ sequences such as a family emoji
// run to about 7 code points; 10 leaves room without admitting short words.
const maxEmojiRunes = 10

// CategoryIconNames returns the named icons a category may use, sorted.
func CategoryIconNames() []string {
	names := make([]string, 0, len(categoryIconNames))
	for n := range categoryIconNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ValidateCategoryIcon returns INVALID_INPUT unless icon is empty, one of the
// named icons, or a single emoji. Free text is rejected because the web app,
// the bot and the MCP server can only render those two forms.
func ValidateCategoryIcon(icon string) error {
	if icon == "" || isCategoryIconName(icon) || isEmojiIcon(icon) {
		return nil
	}
	return apperrors.WithMessage(apperrors.ErrInvalidInput,
		"icon must be a single emoji or one of: "+strings.Join(CategoryIconNames(), ", "))
}

func isCategoryIconName(icon string) bool {
	_, ok := categoryIconNames[icon]
	return ok
}

// isEmojiIcon reports whether icon looks like one emoji: every code point is a
// symbol or an emoji joiner/modifier, with at least one symbol. Go's unicode
// tables have no Extended_Pictographic property or grapheme segmentation, so
// this approximates both; it rejects letters, digits, spaces and punctuation,
// which is what matters for rendering.
func isEmojiIcon(icon string) bool {
	if utf8.RuneCountInString(icon) > maxEmojiRunes {
		return false
	}
	hasSymbol := false
	for _, r := range icon {
		switch {
		case unicode.IsSymbol(r):
			hasSymbol = true
		case r == 0x200D, // zero-width joiner
			r == 0xFE0E, r == 0xFE0F, // variation selectors
			r >= 0xE0020 && r <= 0xE007F: // tag characters (subdivision flags)
		default:
			return false
		}
	}
	return hasSymbol
}
