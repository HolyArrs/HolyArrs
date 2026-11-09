package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

func TestWireUpdateChecksMapsState(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"tag_name": "v9.0.0"}`))
	}))
	t.Cleanup(srv.Close)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	he := &health.Engine{}
	uc := &updates.Checker{Store: s, Version: "v1.0.0", ReleaseFeed: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }}
	WireUpdateChecks(he, uc)
	ctx := context.Background()
	latest, newer, ran := he.UpdateCheck(ctx)
	if latest != "v9.0.0" || !newer || !ran {
		t.Fatalf("UpdateCheck = %q %v %v", latest, newer, ran)
	}
	st := he.UpdateState(ctx)
	if !st.On || st.Latest != "v9.0.0" || !st.Newer || st.Every != 24*time.Hour || st.CheckedAt == nil || st.Feed != srv.URL {
		t.Fatalf("UpdateState = %+v", st)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", "update_check", "off", domain.System, now)
	}); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	now = now.Add(48 * time.Hour)
	if _, _, ran := he.UpdateCheck(ctx); ran {
		t.Fatal("ran with the check off")
	}
	if he.UpdateState(ctx).On {
		t.Fatal("state says on")
	}
	if calls.Load() != before {
		t.Fatal("feed asked with the check off")
	}
}
