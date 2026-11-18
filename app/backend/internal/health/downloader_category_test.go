package health

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// A Downloader that answers but lacks a category says so on its Links row.
func TestLinksNameAMissingCategory(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	execSQL(t, s, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Lab SABnzbd', 'sabnzbd', 'usenet', 'http://sab', 'tv', 1, '[]', 'leave', 'ok', ?)`, doctor.CategoryLine("tv"))
	e := &Engine{Store: s, Version: "test", Outside: func() []outbound.State { return nil }}
	got, err := e.Links(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].State != jobs.Failed || got[0].Reason != doctor.CategoryLine("tv") {
		t.Fatalf("%+v", got)
	}
}
