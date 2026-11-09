package main

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fill must hand the asks service an Alerts sender, either the injected one
// or one built from the vault, or ask alerts vanish silently.
func TestFillWiresAsksAlerts(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	n := &alerts.Sender{Store: st}
	s := services{Store: st, Metadata: metadata.Lab(), Version: "test", Alerts: n}
	s.fill()
	if s.Asks == nil || s.Asks.Alerts != n {
		t.Fatal("asks service lacks the injected Alerts")
	}

	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	s2 := services{Store: st, Metadata: metadata.Lab(), Version: "test", Secrets: v}
	s2.fill()
	if s2.Alerts == nil || s2.Asks == nil {
		t.Fatal("no sender built from the vault")
	}
	if got, ok := s2.Asks.Alerts.(*alerts.Sender); !ok || got != s2.Alerts {
		t.Fatalf("asks Alerts = %#v, want the vault sender", s2.Asks.Alerts)
	}
}
