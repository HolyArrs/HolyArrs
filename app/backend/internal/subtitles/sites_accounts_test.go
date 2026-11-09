package subtitles

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

func TestAddic7ed_ChallengePausesSite(t *testing.T) {
	f := newFix(t)
	wishAll(t, f, Want{Language: "en"})
	ctx := context.Background()
	v, err := secrets.Open(ctx, f.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakesite.Addic7ed{User: mkUser, Pass: mkPass, Challenge: true}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	r, err := AddSite(ctx, f.st, v, SiteInput{Kind: "addic7ed", URL: srv.URL, AllowLocalNetwork: true,
		Username: mkUser, Password: mkPass}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: f.st, Vault: v}
	if _, err := s.Search(ctx, f.ep1File, SearchOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	st, reason, until := siteState(t, f, r.ID)
	if st != "paused" || reason != "blocked" || time.Until(until) < 11*time.Hour {
		t.Fatalf("%s %s %v", st, reason, until)
	}
	res, err := s.TestSite(ctx, r.ID)
	if err != nil || res.OK || !strings.Contains(res.Message, "anti-bot") || strings.Contains(res.Message, "Turn on") {
		t.Fatalf("test result %+v %v", res, err)
	}
	for _, e := range eventsOf(t, f.st, EventSitePaused) {
		if strings.Contains(e.Summary, mkPass) {
			t.Fatal("account details in event")
		}
	}
}

// Made-up account details. None of them is a real account.
const (
	mkKey  = "made-up-key-for-tests"
	mkUser = "lab-person"
	mkPass = "made-up-word-xyz"
)

func osLab() *fakesite.OpenSubs {
	return &fakesite.OpenSubs{Key: mkKey, User: mkUser, Pass: mkPass, Quota: 5, Subs: []fakesite.Sub{
		{ID: "101", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", ByID: true, Trusted: true, Text: fakesite.Cue("made up", 1000)},
	}}
}

// accountSvc builds a Service with a vault and one fake site shaped like
// OpenSubtitles.com, using the given password.
func accountSvc(t *testing.T, f *fix, site *fakesite.OpenSubs, pass string) (*Service, int64) {
	t.Helper()
	srv := httptest.NewServer(site)
	t.Cleanup(srv.Close)
	v, err := secrets.Open(context.Background(), f.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: f.st, Vault: v}
	s.Enqueue = func(context.Context, string, *domain.Subject, map[string]any, string) error { return nil }
	r, err := AddSite(context.Background(), f.st, v, SiteInput{Kind: "opensubtitles", URL: srv.URL + "/api/v1", AllowLocalNetwork: true,
		APIKey: mkKey, Username: mkUser, Password: pass, Limits: &SiteLimits{GapMS: 1, PerWindow: 1000, WindowSeconds: 3600}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return s, r.ID
}

func siteState(t *testing.T, f *fix, id int64) (state, reason string, until time.Time) {
	t.Helper()
	var r, u sql.NullString
	if err := f.st.Read().QueryRow("SELECT state, pause_reason, paused_until FROM subtitle_sites WHERE id = ?", id).Scan(&state, &r, &u); err != nil {
		t.Fatal(err)
	}
	if u.Valid {
		until, _ = time.Parse("2006-01-02T15:04:05.000Z", u.String)
		if until.IsZero() {
			until, _ = time.Parse(time.RFC3339Nano, u.String)
		}
	}
	return state, r.String, until
}

func TestOpenSubtitles_Saves(t *testing.T) {
	f := newFix(t)
	wishAll(t, f, Want{Language: "en"})
	s, id := accountSvc(t, f, osLab(), mkPass)
	res, err := s.Search(context.Background(), f.movieFile, SearchOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v", res.Outcomes)
	}
	if st, _, _ := siteState(t, f, id); st != "enabled" {
		t.Errorf("state %s", st)
	}
}

// A refused sign-in rests the site with login_refused, and the password never
// appears in its Events.
func TestOpenSubtitles_LoginRefusedPauses(t *testing.T) {
	f := newFix(t)
	wishAll(t, f, Want{Language: "en"})
	s, id := accountSvc(t, f, osLab(), "wrong-made-up-word")
	_, _ = s.Search(context.Background(), f.movieFile, SearchOptions{}, domain.System)
	st, reason, until := siteState(t, f, id)
	if st != "paused" || reason != "login_refused" || time.Until(until) < 11*time.Hour {
		t.Fatalf("%s %s %v", st, reason, until)
	}
	var n int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM events WHERE data LIKE ? OR summary LIKE ?", "%wrong-made-up%", "%wrong-made-up%").Scan(&n)
	if n != 0 {
		t.Error("password in events")
	}
}

// With no downloads left, the site rests until the reset time it gave.
func TestOpenSubtitles_QuotaRests(t *testing.T) {
	f := newFix(t)
	wishAll(t, f, Want{Language: "en"})
	site := osLab()
	site.Quota = 0
	s, id := accountSvc(t, f, site, mkPass)
	_, _ = s.Search(context.Background(), f.movieFile, SearchOptions{}, domain.System)
	st, reason, until := siteState(t, f, id)
	if d := time.Until(until); st != "paused" || reason != "download_limit" || d < 2*time.Hour || d > 3*time.Hour+time.Minute {
		t.Fatalf("%s %s %v", st, reason, d)
	}
}

func TestAccount_Kinds(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	v, _ := secrets.Open(ctx, f.st, make([]byte, secrets.KeySize))
	if _, err := AddSite(ctx, f.st, v, SiteInput{Kind: "opensubtitles", APIKey: mkKey}, domain.System); err != ErrNeedsAccount {
		t.Errorf("opensubtitles without a sign-in: %v", err)
	}
	if r, err := AddSite(ctx, f.st, v, SiteInput{Kind: "addic7ed", Username: "a", Password: "b"}, domain.System); err != nil || r.Limits.GapMS != 3000 || r.Limits.WindowSeconds != 86400 {
		t.Errorf("addic7ed: %+v %v", r, err)
	}
	if _, err := AddSite(ctx, f.st, v, SiteInput{Kind: "addic7ed", Username: "a"}, domain.System); err != ErrNeedsAccount {
		t.Errorf("addic7ed without a password: %v", err)
	}
	r, err := AddSite(ctx, f.st, v, SiteInput{Kind: "podnapisi"}, domain.System)
	if err != nil || r.State != "disabled" {
		t.Errorf("podnapisi starts off: %+v %v", r, err)
	}
	on := true
	r, err = AddSite(ctx, f.st, v, SiteInput{Kind: "podnapisi", Name: "Podnapisi on", Enabled: &on}, domain.System)
	if err != nil || r.State != "enabled" {
		t.Errorf("podnapisi turned on: %+v %v", r, err)
	}
	r, err = AddSite(ctx, f.st, v, SiteInput{Kind: "subdl", APIKey: mkKey}, domain.System)
	if err != nil || !r.HasAccount || r.URL != "https://api.subdl.com/api/v1" {
		t.Errorf("subdl: %+v %v", r, err)
	}
}
