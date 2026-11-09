package health

import (
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var statusNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func check(kind, subject, state, message string) domain.HealthCheck {
	return domain.HealthCheck{Kind: kind, Subject: subject, State: state, Message: message}
}

func intp(n int) *int        { return &n }
func boolp(b bool) *bool     { return &b }
func timep(s string) *string { return &s }

func pages(a StatusAnswer) []string {
	var out []string
	for _, i := range a.Items {
		out = append(out, i.Page)
	}
	slices.Sort(out)
	return out
}

func inGroup(a StatusAnswer, g string) int {
	n := 0
	for _, i := range a.Items {
		if i.Group == g {
			n++
		}
	}
	return n
}

// TestSettingsStatusRules carries each case of the web app's former
// settings-status rules (settings-status.test.ts) over unchanged in meaning.
func TestSettingsStatusRules(t *testing.T) {
	t.Parallel()
	t.Run("all set when every known check is fine", func(t *testing.T) {
		a := SettingsStatus(StatusInput{Checks: []domain.HealthCheck{check("downloader", "qBit", "ok", "")},
			Sources: []NamedState{{Name: "A", State: "working"}}, Downloaders: []NamedState{{Name: "Q", State: "working"}}, Now: statusNow})
		if len(a.Items) != 0 || !slices.Contains(a.Known, "/settings/sources") || !slices.Contains(a.Known, "/settings/downloaders") || len(a.Unset) != 0 {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("a failing downloader counts once, from the check and the list", func(t *testing.T) {
		a := SettingsStatus(StatusInput{Checks: []domain.HealthCheck{check("downloader", "qBit", "error", "not answering")},
			Downloaders: []NamedState{{Name: "qBit", State: "not_answering"}}, Sources: []NamedState{{Name: "A", State: "working"}}, Now: statusNow})
		if len(a.Items) != 1 || a.Items[0].Page != "/settings/downloaders" || a.Items[0].Group != "copies" ||
			a.Items[0].Text != "qBit: not answering" || a.Items[0].Severity != "error" {
			t.Fatalf("%+v", a.Items)
		}
		if inGroup(a, "copies") != 1 || !slices.Contains(a.Known, "/settings/sources") {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("the metadata server down, an old backup and a failed media server", func(t *testing.T) {
		a := SettingsStatus(StatusInput{MetadataUp: boolp(false), NewestBackup: timep("2026-10-01T00:00:00Z"),
			States: []jobs.Status{{Key: "media_server:1", Kind: "media_server", Name: "Jelly", State: jobs.Failed}}, Now: statusNow})
		if got := pages(a); !slices.Equal(got, []string{"/settings/backups", "/settings/media-servers", "/settings/tmdb"}) {
			t.Fatalf("pages %v", got)
		}
		if inGroup(a, "maintenance") != 1 {
			t.Fatalf("%+v", a.Items)
		}
		want := map[string]StatusItem{
			"/settings/tmdb":          {Group: "library", Page: "/settings/tmdb", Text: metadata.Down, Severity: "warning"},
			"/settings/backups":       {Group: "maintenance", Page: "/settings/backups", Text: "No backup in the last 7 days", Severity: "warning"},
			"/settings/media-servers": {Group: "linked", Page: "/settings/media-servers", Text: "The media server Jelly failed", Severity: "error"},
		}
		for _, i := range a.Items {
			if i != want[i.Page] {
				t.Errorf("got %+v want %+v", i, want[i.Page])
			}
		}
	})
	t.Run("a recent backup is fine and no backup at all is flagged", func(t *testing.T) {
		if a := SettingsStatus(StatusInput{NewestBackup: timep("2026-10-17T00:00:00Z"), Now: statusNow}); len(a.Items) != 0 {
			t.Fatalf("%+v", a.Items)
		}
		a := SettingsStatus(StatusInput{NewestBackup: timep(""), Now: statusNow})
		if len(a.Items) != 1 || a.Items[0].Text != "No backup yet" || a.Items[0].Severity != "warning" {
			t.Fatalf("%+v", a.Items)
		}
	})
	t.Run("not set up without folders, Sources or Downloaders", func(t *testing.T) {
		a := SettingsStatus(StatusInput{Folders: intp(0), Sources: []NamedState{}, Downloaders: []NamedState{}, Now: statusNow})
		if !slices.Equal(a.Unset["library"], []string{"/settings/library-folders"}) ||
			!slices.Equal(a.Unset["copies"], []string{"/settings/sources", "/settings/downloaders"}) {
			t.Fatalf("unset %+v", a.Unset)
		}
	})
	t.Run("nothing about pages it knows nothing of", func(t *testing.T) {
		a := SettingsStatus(StatusInput{Now: statusNow})
		if len(a.Items) != 0 || len(a.Known) != 0 || len(a.Unset) != 0 {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("severities", func(t *testing.T) {
		a := SettingsStatus(StatusInput{
			Checks: []domain.HealthCheck{check("disk_space", "Movies", "warning", "low on space"), check("backups", "", "error", "")},
			States: []jobs.Status{{Key: "stage:search", Kind: "stage", Name: "search", State: jobs.Running, Stuck: true},
				{Key: "subtitle_site:2", Kind: "subtitle_site", Name: "Subs", State: jobs.Failed}},
			Sources:     []NamedState{{Name: "Ind", State: "auth_failed"}},
			UpdateNewer: boolp(true), Now: statusNow})
		want := []StatusItem{
			{Group: "library", Page: "/settings/library-folders", Text: "Movies: low on space", Severity: "warning"},
			{Group: "maintenance", Page: "/settings/backups", Text: "needs a look", Severity: "error"},
			{Group: "general", Page: "/settings/parts", Text: "The stage search is stuck", Severity: "warning"},
			{Group: "library", Page: "/settings/subtitles", Text: "The subtitle site Subs failed", Severity: "error"},
			{Group: "copies", Page: "/settings/sources", Text: "The Source Ind is not answering", Severity: "error"},
			{Group: "general", Page: "/settings/this-app", Text: "A newer HolyArrs is out", Severity: "info"},
		}
		if !slices.Equal(a.Items, want) {
			t.Fatalf("got  %+v\nwant %+v", a.Items, want)
		}
	})
}

func TestStatusFlagsKidWithLocalSkip(t *testing.T) {
	t.Parallel()
	on, off, one, none := true, false, 1, 0
	const page = "/settings/sign-in"
	for _, c := range []struct {
		skip *bool
		kids *int
		want bool
	}{{&on, &one, true}, {&off, &one, false}, {&on, &none, false}, {nil, &one, false}} {
		got := false
		for _, i := range SettingsStatus(StatusInput{SkipLocal: c.skip, Kids: c.kids}).Items {
			if i.Page == page && i.Group == "security" && i.Severity == "error" {
				got = true
			}
		}
		if got != c.want {
			t.Fatalf("skip %v kids %v: flagged %t", c.skip, c.kids, got)
		}
	}
}

// TestStatusWarnsKidWithOutsideJoin: with a Kid and outside sign-in open
// to new people, the Users page warns that newcomers get no limit.
func TestStatusWarnsKidWithOutsideJoin(t *testing.T) {
	t.Parallel()
	on, off, one, none := true, false, 1, 0
	want := StatusItem{Group: "people", Page: "/settings/users", Severity: "warning",
		Text: "New people who sign in with Plex, Jellyfin, Emby or single sign-on get no age limit; link a Kid's outside accounts to their Kid account."}
	for _, c := range []struct {
		join *bool
		kids *int
		want bool
	}{{&on, &one, true}, {&off, &one, false}, {&on, &none, false}} {
		got := slices.Contains(SettingsStatus(StatusInput{OutsideJoin: c.join, Kids: c.kids}).Items, want)
		if got != c.want {
			t.Fatalf("join %v kids %v: warned %t", *c.join, *c.kids, got)
		}
	}
}
