package updates

import (
	"context"
	"errors"
	"testing"
)

func TestModeOffByDefault(t *testing.T) {
	w := newWorld(t)
	if m, locked := w.e.Mode(context.Background()); m != ModeOff || locked {
		t.Fatalf("mode %q locked %v", m, locked)
	}
}

func TestModeSetAndEnvLocks(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	if err := w.e.SetMode(ctx, "sometimes"); err == nil {
		t.Fatal("unknown mode kept")
	}
	if err := w.e.SetMode(ctx, ModeAsk); err != nil {
		t.Fatal(err)
	}
	if m, _ := w.e.Mode(ctx); m != ModeAsk {
		t.Fatalf("mode %q", m)
	}
	t.Setenv(EnvMode, "AUTO")
	if m, locked := w.e.Mode(ctx); m != ModeAuto || !locked {
		t.Fatalf("env: mode %q locked %v", m, locked)
	}
	if err := w.e.SetMode(ctx, ModeOff); !errors.Is(err, ErrModeLocked) {
		t.Fatalf("locked mode changed: %v", err)
	}
	t.Setenv(EnvMode, "bogus")
	if m, locked := w.e.Mode(ctx); m != ModeAsk || locked {
		t.Fatalf("bad env should fall back to the setting: %q %v", m, locked)
	}
}

func TestCannotInstallWithoutKeys(t *testing.T) {
	w := newWorld(t)
	w.e.Installer = &Installer{}
	w.e.ListKeys = []PublicKey{}
	if w.e.CanInstall() {
		t.Fatal("installs offered with no pinned key")
	}
}
