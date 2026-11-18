package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

func TestCleanupSettings_DefaultIsOff(t *testing.T) {
	f := newFix(t)
	s := ReadSettings(context.Background(), f.st.Read())
	if s.Cleanup == nil || len(s.Cleanup) != 0 {
		t.Fatalf("default cleanup %+v", s.Cleanup)
	}
	for _, lang := range []string{"en", "pt-br", "", "xx"} {
		if s.CleanupFor(lang).Any() {
			t.Errorf("%q: a step is on by default", lang)
		}
	}
}

func TestCleanupSettings_RoundTripAndLookup(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	s := DefaultSettings()
	s.Cleanup = map[string]subtext.CleanupOptions{
		"*":       {FixOverlaps: true},
		"English": {RemoveAds: true, RemoveHI: true},
		"pt":      {CommonFixes: true},
	}
	if err := SaveSettings(ctx, f.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	got := ReadSettings(ctx, f.st.Read())
	if len(got.Cleanup) != 3 || !got.Cleanup["en"].RemoveHI || !got.Cleanup["*"].FixOverlaps {
		t.Fatalf("saved %+v", got.Cleanup)
	}
	for lang, want := range map[string]subtext.CleanupOptions{
		"en":    {RemoveAds: true, RemoveHI: true},
		"eng":   {RemoveAds: true, RemoveHI: true},
		"pt-br": {CommonFixes: true}, // no row of its own: its base language's
		"pt":    {CommonFixes: true},
		"fr":    {FixOverlaps: true}, // no row: the one for every language
		"":      {FixOverlaps: true},
	} {
		if o := got.CleanupFor(lang); o != want {
			t.Errorf("%q: %+v, want %+v", lang, o, want)
		}
	}
}

func TestCleanupSettings_Refused(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	many := map[string]subtext.CleanupOptions{"*": {}}
	for _, l := range languages {
		many[l.two] = subtext.CleanupOptions{}
	}
	for l := range 40 {
		many[string(rune('a'+l%26))+string(rune('a'+l/26))+"x"] = subtext.CleanupOptions{}
	}
	for name, rows := range map[string]map[string]subtext.CleanupOptions{
		"unknown language": {"klingon": {RemoveAds: true}},
		"path in key":      {"en/../../x": {}},
		"same language":    {"en": {}, "eng": {RemoveHI: true}},
		"too many rows":    many,
	} {
		s := DefaultSettings()
		s.Cleanup = rows
		if err := SaveSettings(ctx, f.st, s, domain.System); err == nil {
			t.Errorf("%s: saved", name)
		}
	}
	if _, err := CleanCleanup(map[string]subtext.CleanupOptions{"xx": {}}); !errors.Is(err, ErrBadLanguage) {
		t.Errorf("unknown language: %v", err)
	}
	// A bad row written straight into the database is never used.
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('what_to_pick', 'subtitle_cleanup', '{"klingon":{"remove_hi":true}}', '', '{}', '', '{}')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := ReadSettings(ctx, f.st.Read()); len(got.Cleanup) != 0 {
		t.Fatalf("stored bad cleanup used: %+v", got.Cleanup)
	}
}

// route: PUT /api/v1/settings/subtitles
func TestCleanupSettings_PutReplacesOrKeeps(t *testing.T) {
	f := newFix(t)
	m := http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	put := func(body string) Settings {
		t.Helper()
		code, out := call(m, "PUT", "/api/v1/settings/subtitles", body)
		if code != http.StatusOK {
			t.Fatalf("PUT %s: %d %s", body, code, out)
		}
		var s Settings
		if err := json.Unmarshal([]byte(out), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := put(`{"cleanup": {"en": {"remove_ads": true}, "fr": {"remove_hi": true}}}`)
	if len(s.Cleanup) != 2 || !s.Cleanup["en"].RemoveAds {
		t.Fatalf("%+v", s.Cleanup)
	}
	// Leaving cleanup out keeps the rows.
	if s = put(`{"pass_mark_movie": 75}`); len(s.Cleanup) != 2 || s.PassMarkMovie != 75 {
		t.Fatalf("%+v", s)
	}
	// Sending cleanup replaces the rows whole, so a row can be taken away.
	if s = put(`{"cleanup": {"en": {"remove_tags": true}}}`); len(s.Cleanup) != 1 || s.Cleanup["en"].RemoveAds || !s.Cleanup["en"].RemoveTags {
		t.Fatalf("%+v", s.Cleanup)
	}
	if s = put(`{"cleanup": {}}`); len(s.Cleanup) != 0 {
		t.Fatalf("%+v", s.Cleanup)
	}
	if code, _ := call(m, "PUT", "/api/v1/settings/subtitles", `{"cleanup": {"xx": {}}}`); code != http.StatusBadRequest {
		t.Fatalf("bad language: %d", code)
	}
}
