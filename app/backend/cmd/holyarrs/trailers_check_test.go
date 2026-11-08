package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// The trailers check follows trailer autoplay: on by default, off once the
// owner turns autoplay off (nothing is looked up then, so a kept failure
// could never clear), and on again with it.
func TestTrailersCheckFollowsAutoplay(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	he := &health.Engine{}
	startTrailers(buildHandlers(services{Store: s, Version: "test"}).Trailers(), he)
	set := func(v string) {
		t.Helper()
		if err := s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'autoplay_trailers', ?, 't', '{}', 't', '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`, v)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !he.Trailers(ctx).On {
		t.Fatal("off by default")
	}
	set("off")
	if he.Trailers(ctx).On {
		t.Fatal("on with autoplay off")
	}
	set("on")
	if !he.Trailers(ctx).On {
		t.Fatal("off with autoplay on")
	}
}
