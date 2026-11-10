package services

import (
	"encoding/json"
	"testing"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestAppearancePatch_UnmarshalRejectsUnknownKeys(t *testing.T) {
	var p AppearancePatch
	if err := json.Unmarshal([]byte(`{"palette":"nord","theme":"x"}`), &p); err == nil {
		t.Fatal("expected an unknown key to be rejected")
	}
	if err := json.Unmarshal([]byte(`{"palette":"nord","show_cents":false}`), &p); err != nil {
		t.Fatalf("known keys must decode: %v", err)
	}
	if p.Palette == nil || *p.Palette != "nord" || p.ShowCents == nil || *p.ShowCents {
		t.Errorf("unexpected decode: %+v", p)
	}
	if p.Mode != nil {
		t.Error("absent keys must stay nil")
	}
}

func TestMergeAppearance(t *testing.T) {
	str := func(s string) *string { return &s }
	yes := true
	base := models.Appearance{Palette: "nord", Mode: "dark", ChartStyle: "dither"}

	got := MergeAppearance(base, &AppearancePatch{Palette: str("gruvbox"), ShowCents: &yes})
	if got.Palette != "gruvbox" || got.Mode != "dark" || got.ChartStyle != "dither" {
		t.Errorf("only sent keys change: %+v", got)
	}
	if got.ShowCents == nil || !*got.ShowCents {
		t.Errorf("show_cents not applied: %+v", got)
	}
	if base.Palette != "nord" || base.ShowCents != nil {
		t.Error("the base value must not be mutated")
	}
	if MergeAppearance(base, nil) != base {
		t.Error("a nil patch is a no-op")
	}
}

func TestUpdateProfileSettings_Appearance(t *testing.T) {
	str := func(s string) *string { return &s }
	no := false
	setup := func(t *testing.T) (UserServicer, *models.User) {
		t.Helper()
		db := testutil.SetupTestDB(t)
		t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
		svc := NewUserService(db, NewFXService(db, testFXStaleness))
		return svc, testutil.CreateTestUser(t, db)
	}
	get := func(t *testing.T, svc UserServicer, id string) models.Appearance {
		t.Helper()
		u, err := svc.GetUserByID(id)
		testutil.AssertNoError(t, err)
		return u.Appearance
	}

	t.Run("new users start with empty appearance", func(t *testing.T) {
		svc, user := setup(t)
		if got := get(t, svc, user.ID); got != (models.Appearance{}) {
			t.Errorf("expected zero appearance, got %+v", got)
		}
	})

	t.Run("patches merge instead of replacing", func(t *testing.T) {
		svc, user := setup(t)
		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			Appearance: &AppearancePatch{Palette: str("nord"), Mode: str("light")},
		}))
		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			Appearance: &AppearancePatch{Mode: str("dark"), ShowCents: &no},
		}))

		got := get(t, svc, user.ID)
		if got.Palette != "nord" || got.Mode != "dark" || got.ShowCents == nil || *got.ShowCents {
			t.Errorf("merge result wrong: %+v", got)
		}
	})

	t.Run("choosing default is stored and keeps other keys", func(t *testing.T) {
		svc, user := setup(t)
		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			Appearance: &AppearancePatch{Palette: str("nord"), ChartStyle: str("dither")},
		}))
		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			Appearance: &AppearancePatch{Palette: str("default")},
		}))
		got := get(t, svc, user.ID)
		if got.Palette != "default" || got.ChartStyle != "dither" {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("appearance combines with other settings in one update", func(t *testing.T) {
		svc, user := setup(t)
		hide := true
		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			HideBalances: &hide,
			Appearance:   &AppearancePatch{Density: str("compact")},
		}))
		u, err := svc.GetUserByID(user.ID)
		testutil.AssertNoError(t, err)
		if !u.HideBalances || u.Appearance.Density != "compact" {
			t.Errorf("both changes must apply: hide=%v appearance=%+v", u.HideBalances, u.Appearance)
		}
	})

	t.Run("unknown values are rejected and nothing is stored", func(t *testing.T) {
		svc, user := setup(t)
		hide := true
		for name, patch := range map[string]*AppearancePatch{
			"palette":     {Palette: str("neon")},
			"mode":        {Mode: str("sepia")},
			"chart_style": {ChartStyle: str("fancy")},
			"density":     {Density: str("tiny")},
		} {
			t.Run(name, func(t *testing.T) {
				err := svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{HideBalances: &hide, Appearance: patch})
				testutil.AssertAppError(t, err, "INVALID_INPUT")
			})
		}
		u, err := svc.GetUserByID(user.ID)
		testutil.AssertNoError(t, err)
		if u.HideBalances || u.Appearance != (models.Appearance{}) {
			t.Errorf("a rejected update must not apply: hide=%v appearance=%+v", u.HideBalances, u.Appearance)
		}
	})

	t.Run("an empty patch alone is not an update", func(t *testing.T) {
		svc, user := setup(t)
		err := svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{Appearance: &AppearancePatch{}})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})
}
