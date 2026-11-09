package config

import "testing"

func TestLoadDefaultPalette(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"unset falls back to catppuccin", "", "catppuccin"},
		{"blank falls back to catppuccin", "   ", "catppuccin"},
		{"known id is kept", "nord", "nord"},
		{"case and spaces are normalised", " Gruvbox ", "gruvbox"},
		{"unknown id warns and falls back", "neon", "catppuccin"},
		{"default is not a server palette", "default", "catppuccin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadDefaultPalette(tc.raw); got != tc.want {
				t.Errorf("loadDefaultPalette(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestLoad_DefaultPaletteFromEnv(t *testing.T) {
	t.Setenv("DEFAULT_PALETTE", "lagoon")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultPalette != "lagoon" {
		t.Errorf("DefaultPalette = %q, want lagoon", cfg.DefaultPalette)
	}

	t.Setenv("DEFAULT_PALETTE", "not-a-palette")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("a typo must not stop the server: %v", err)
	}
	if cfg.DefaultPalette != "catppuccin" {
		t.Errorf("DefaultPalette = %q, want catppuccin", cfg.DefaultPalette)
	}
}
