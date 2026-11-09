// Package appearance holds the UI appearance vocabulary the API shares with
// the web app: the palette list and the allowed values of each preference.
//
// The palette ids are mirrored by apps/web/src/lib/palettes.ts; a Go test
// (palettes_test.go) reads that file and fails when the two lists differ.
package appearance

import "slices"

// DefaultPalette is the palette used when neither the user nor the server
// (DEFAULT_PALETTE) has picked one.
const DefaultPalette = "catppuccin"

// PaletteDefault is the palette value meaning "follow the server default".
const PaletteDefault = "default"

// paletteIDs is the one palette list. Keep in sync with PALETTES in
// apps/web/src/lib/palettes.ts (enforced by a test).
var paletteIDs = []string{
	"gold", "cobalt", "saffron", "graphite", "lagoon", "nord", "catppuccin", "gruvbox",
}

// PaletteIDs returns a copy of the palette ids.
func PaletteIDs() []string {
	return slices.Clone(paletteIDs)
}

// IsPalette reports whether id is a known palette id (not "default").
func IsPalette(id string) bool {
	return slices.Contains(paletteIDs, id)
}

// IsPaletteChoice reports whether v is a valid stored palette preference: a
// palette id or "default".
func IsPaletteChoice(v string) bool {
	return v == PaletteDefault || IsPalette(v)
}

// ResolveDefault validates a DEFAULT_PALETTE value. An empty value yields
// DefaultPalette with ok=true; an unknown one yields DefaultPalette with
// ok=false so the caller can warn.
func ResolveDefault(raw string) (id string, ok bool) {
	if raw == "" {
		return DefaultPalette, true
	}
	if IsPalette(raw) {
		return raw, true
	}
	return DefaultPalette, false
}

// Resolve applies the resolution order: the user's choice, then the server
// default, then DefaultPalette. Any value that is not a known palette id
// (including "default" and "") falls through to the next layer.
func Resolve(userChoice, serverDefault string) string {
	if IsPalette(userChoice) {
		return userChoice
	}
	if IsPalette(serverDefault) {
		return serverDefault
	}
	return DefaultPalette
}
