package config

import (
	"slices"
	"strings"
	"testing"
)

func TestWithWriteScopes(t *testing.T) {
	tests := []struct {
		name    string
		scopes  string
		enabled bool
		want    string
	}{
		{"disabled keeps reads", DefaultOAuthScopes, false, DefaultOAuthScopes},
		{"enabled adds the write set", "read:accounts", true, "read:accounts " + DefaultOAuthWriteScopes},
		{"disabled strips write scopes from an override", "read:accounts write:transactions", false, "read:accounts"},
		{"enabled does not duplicate an override's write scope", "read:accounts write:transactions", true, "read:accounts " + DefaultOAuthWriteScopes},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := withWriteScopes(tc.scopes, tc.enabled); got != tc.want {
				t.Errorf("withWriteScopes(%q, %v) = %q, want %q", tc.scopes, tc.enabled, got, tc.want)
			}
		})
	}
}

func TestValidateProduction(t *testing.T) {
	valid := func() *Config {
		return &Config{
			Env:         Production,
			JWTSecret:   strings.Repeat("s", 32),
			DBPassword:  "strong",
			DBSSLMode:   "require",
			CORSOrigins: []string{"https://kuberan.example.com"},
		}
	}
	if err := valid().validateProduction(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := map[string]func(c *Config){
		"default JWT secret":  func(c *Config) { c.JWTSecret = "fallback-secret-key-for-dev-only" },
		"short JWT secret":    func(c *Config) { c.JWTSecret = strings.Repeat("s", 31) },
		"default DB password": func(c *Config) { c.DBPassword = "kuberan" },
		"wildcard CORS":       func(c *Config) { c.CORSOrigins = []string{"*"} },
		"no CORS origins":     func(c *Config) { c.CORSOrigins = nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := valid()
			mutate(c)
			if err := c.validateProduction(); err == nil {
				t.Error("expected a validation error")
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	if got := splitList(" 172.16.0.0/12, ,10.0.0.0/8 "); !slices.Equal(got, []string{"172.16.0.0/12", "10.0.0.0/8"}) {
		t.Errorf("splitList = %v", got)
	}
	if got := splitList(""); got != nil {
		t.Errorf("splitList(\"\") = %v, want nil", got)
	}
}
