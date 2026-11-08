package config

import (
	"slices"
	"testing"
)

// setBaseEnv sets the variables Load requires, and clears the optional ones this
// package reads so each test starts from defaults.
func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("KUBERAN_API_URL", "http://api:8080")
	t.Setenv("PIPELINE_API_KEY", "key")
	for _, k := range []string{"LOG_LEVEL", "REQUEST_TIMEOUT", "PIPELINE_TIMEOUT", "COMPUTE_SNAPSHOTS",
		"PROCESS_FIXED_DEPOSITS", "PUBLISH_FX_RATES", "TARGET_CURRENCY"} {
		t.Setenv(k, "")
	}
}

func TestLoad_TargetCurrencyIsIgnoredWithWarning(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("TARGET_CURRENCY", "MYR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "TARGET_CURRENCY is ignored since plan 024 phase 2"
	if !slices.Contains(cfg.Warnings, want) {
		t.Errorf("Warnings = %v, want %q", cfg.Warnings, want)
	}
}

func TestLoad_NoWarningWhenTargetCurrencyUnset(t *testing.T) {
	setBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", cfg.Warnings)
	}
}

func TestLoad_PublishFXDefaultsOn(t *testing.T) {
	setBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.PublishFX {
		t.Error("PublishFX should default to true")
	}
}
