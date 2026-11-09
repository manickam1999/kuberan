package appearance

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestResolveDefault(t *testing.T) {
	tests := []struct {
		raw    string
		want   string
		wantOK bool
	}{
		{"", DefaultPalette, true},
		{"nord", "nord", true},
		{"gruvbox", "gruvbox", true},
		{"neon", DefaultPalette, false},
		{"default", DefaultPalette, false}, // "default" is a user choice, not a server palette
	}
	for _, tc := range tests {
		got, ok := ResolveDefault(tc.raw)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("ResolveDefault(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestResolve_Layers(t *testing.T) {
	tests := []struct {
		name, user, server, want string
	}{
		{"user choice wins", "gruvbox", "nord", "gruvbox"},
		{"default falls to the server", "default", "nord", "nord"},
		{"missing key falls to the server", "", "nord", "nord"},
		{"unknown user value falls to the server", "neon", "nord", "nord"},
		{"no server default falls to catppuccin", "", "", DefaultPalette},
		{"bad server default falls to catppuccin", "default", "neon", DefaultPalette},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.user, tc.server); got != tc.want {
				t.Errorf("Resolve(%q, %q) = %q, want %q", tc.user, tc.server, got, tc.want)
			}
		})
	}
}

func TestPaletteChoice(t *testing.T) {
	for _, id := range PaletteIDs() {
		if !IsPalette(id) || !IsPaletteChoice(id) {
			t.Errorf("%q should be a palette", id)
		}
	}
	if IsPalette(PaletteDefault) {
		t.Error(`"default" is not a palette id`)
	}
	if !IsPaletteChoice(PaletteDefault) {
		t.Error(`"default" is a valid stored choice`)
	}
	if IsPaletteChoice("neon") || IsPaletteChoice("") {
		t.Error("unknown and empty values must be rejected")
	}
	if !slices.Contains(PaletteIDs(), DefaultPalette) {
		t.Error("the built-in default must be in the list")
	}
}

// TestPaletteIDsMatchWebRegistry keeps the Go list and the web registry
// (apps/web/src/lib/palettes.ts) identical, in the same order.
func TestPaletteIDsMatchWebRegistry(t *testing.T) {
	path := filepath.Join("..", "..", "..", "web", "src", "lib", "palettes.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web palette registry: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*id:\s*"([a-z0-9-]+)"`)
	var web []string
	for _, m := range re.FindAllSubmatch(src, -1) {
		web = append(web, string(m[1]))
	}
	if len(web) == 0 {
		t.Fatal("no palette ids found in palettes.ts")
	}
	if !slices.Equal(web, PaletteIDs()) {
		t.Errorf("palette ids differ\n web (%s): %v\n  go: %v", path, web, PaletteIDs())
	}
}
