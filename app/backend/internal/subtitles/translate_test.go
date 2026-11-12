package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/container"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/translate/faketranslator"
)

// A made-up key, never a real account.
const fakeAIKey = "sk-made-up-translator-key-123"

// withTranslator builds a Service with a vault and the stand-in translator.
func withTranslator(t *testing.T, f *fix, ft *faketranslator.OpenAI) (*Service, *[]queued) {
	t.Helper()
	ctx := context.Background()
	v, err := secrets.Open(ctx, f.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(ft)
	t.Cleanup(srv.Close)
	s := &Service{Store: f.st, Vault: v}
	var q []queued
	s.Enqueue = func(_ context.Context, kind string, subj *domain.Subject, params map[string]any, _ string) error {
		q = append(q, queued{kind, subj.ID, params})
		return nil
	}
	ft.Key = fakeAIKey
	if _, err := s.SaveTranslator(ctx, TranslatorInput{Kind: TranslatorOpenAI, URL: srv.URL + "/v1", Model: "lab-model", Key: fakeAIKey}, domain.System); err != nil {
		t.Fatal(err)
	}
	return s, &q
}

// labMovie puts the lab file with its built-in English text track in place of
// the movie and returns that track's number.
func labMovie(t *testing.T, f *fix) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("container", "testdata", "lab-built-in.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.abs(movieRel), b, 0o644); err != nil {
		t.Fatal(err)
	}
	tracks, err := container.Probe(f.abs(movieRel))
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range tracks {
		if tr.Language == "eng" && !tr.Forced && !tr.HearingImpaired {
			return tr.Index
		}
	}
	t.Fatal("no plain English track in the lab file")
	return 0
}

func readCues(t *testing.T, p string) []subtext.Cue {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := subtext.ParseCues(string(b), subtext.SRT)
	if err != nil {
		t.Fatal(err)
	}
	return cues
}

// By hand, the built-in English track of an MKV becomes a .pt-BR.srt with the
// same lines and timings, marked as a translation with a low score. The key
// never shows in the settings answer, Events or the job result.
func TestTranslate_BuiltInTrackByHand(t *testing.T) {
	f := newFix(t)
	track := labMovie(t, f)
	ft := &faketranslator.OpenAI{}
	s, _ := withTranslator(t, f, ft)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "pt-br", TranslateFrom: "en"})
	res, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-BR", Source: SourceBuiltIn, Track: track, ByHand: true}, domain.System)
	if err != nil || res.Skipped != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if res.Path != "The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt-BR.srt" || res.Partial || res.From != "en" || res.Source != SourceBuiltIn {
		t.Fatalf("result %+v", res)
	}
	src, _ := container.ExtractText(ctx, f.abs(movieRel), track)
	got := readCues(t, f.abs(res.Path))
	if len(got) != len(src) {
		t.Fatalf("%d cues, source has %d", len(got), len(src))
	}
	for i := range got {
		if got[i].StartMS != src[i].StartMS || got[i].EndMS != src[i].EndMS || got[i].Text != "PT: "+src[i].Text {
			t.Fatalf("cue %d: %+v, source %+v", i, got[i], src[i])
		}
	}
	rows, _ := ListFor(ctx, f.st.Read(), f.movieFile)
	var row *Subtitle
	for i := range rows {
		if rows[i].Place == "beside" && rows[i].State == "present" {
			row = &rows[i]
		}
	}
	if row == nil || row.Language != "pt-br" || row.Made == nil || *row.Made != MadeTranslated || *row.Score != scoreTranslated || *row.SiteName != "lab-model" {
		t.Fatalf("row %+v", row)
	}
	st, _ := s.Look(ctx, f.movieFile, domain.System)
	if len(st.Missing) != 0 {
		t.Fatalf("still missing %+v", st.Missing)
	}
	view, _ := json.Marshal(s.Translator(ctx))
	if strings.Contains(string(view), fakeAIKey) || !strings.Contains(string(view), `"has_key":true`) {
		t.Fatalf("settings answer %s", view)
	}
	for _, e := range eventsOf(t, f.st, EventTranslated) {
		b, _ := json.Marshal(e)
		if strings.Contains(string(b), fakeAIKey) {
			t.Fatal("key in an Event")
		}
	}
	if len(eventsOf(t, f.st, EventTranslated)) != 1 {
		t.Fatal("no translated Event")
	}
}

// The timed search leaves built-in tracks to the person by default, and with
// the automatic switch on it queues the translation itself.
func TestTranslate_SearchQueuesWhenAutomatic(t *testing.T) {
	f := newFix(t)
	track := labMovie(t, f)
	s, q := withTranslator(t, f, &faketranslator.OpenAI{})
	ctx := context.Background()
	wishAll(t, f, Want{Language: "pt-br", TranslateFrom: "en"})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || len(res.Outcomes) != 1 || res.Outcomes[0].Result != "none_good_enough" || !strings.Contains(res.Outcomes[0].Reason, "by hand") || len(*q) != 0 {
		t.Fatalf("by hand only: %+v %v %+v", res.Outcomes, err, *q)
	}
	cur := s.Translator(ctx)
	if _, err := s.SaveTranslator(ctx, TranslatorInput{Kind: cur.Kind, URL: cur.URL, Model: cur.Model, BuiltInAuto: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	res, err = s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "translating" || len(*q) != 1 || (*q)[0].kind != KindTranslate {
		t.Fatalf("automatic: %+v %v %+v", res.Outcomes, err, *q)
	}
	p := (*q)[0].params
	if p["source"] != SourceBuiltIn || p["track"] != float64(track) || p["language"] != "pt-br" || p["from"] != "en" {
		t.Fatalf("params %v", p)
	}
	// The queued Job translates and saves the file.
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: p}}
	if err := s.handleTranslate(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt-BR.srt")); err != nil {
		t.Fatal(err)
	}
}

// A person's own .pt-BR.srt is never overwritten or held aside.
func TestTranslate_NeverReplacesAPersonsSubtitle(t *testing.T) {
	f := newFix(t)
	track := labMovie(t, f)
	ft := &faketranslator.OpenAI{}
	s, _ := withTranslator(t, f, ft)
	ctx := context.Background()
	own := f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt-BR.srt")
	mine := fakesite.Cue("feito à mão", 1000)
	if err := os.WriteFile(own, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-br", Source: SourceBuiltIn, Track: track, ByHand: true}, domain.System)
	if !errors.Is(err, ErrHasSubtitle) {
		t.Fatalf("by hand over a person's file: %v", err)
	}
	res, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-br", From: "en"}, domain.System)
	if err != nil || res.Skipped == "" {
		t.Fatalf("automatic over a person's file: %+v %v", res, err)
	}
	b, _ := os.ReadFile(own)
	if string(b) != mine || ft.Calls != 0 {
		t.Fatalf("file changed or translator called %d times", ft.Calls)
	}
}

// A line the translator drops is asked again; a line it never sends keeps
// its English text and the file is marked partial, every timing in place.
func TestTranslate_MissingLinesRetriedThenPartial(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	text := ""
	for i := 1; i <= 6; i++ {
		text += fakesite.Cue("line "+itoa(int64(i)), i*2000) + "\n"
	}
	en := f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt")
	if err := os.WriteFile(en, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := &faketranslator.OpenAI{Drop: map[int]int{2: 1, 5: 100}}
	s, _ := withTranslator(t, f, ft)
	res, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-br", From: "en", ByHand: true}, domain.System)
	if err != nil || !res.Partial || res.Translated != 5 || res.Lines != 6 || res.Source != SourceBeside {
		t.Fatalf("%+v %v", res, err)
	}
	got := readCues(t, f.abs(res.Path))
	if len(got) != 6 || got[1].Text != "PT: line 2" || got[4].Text != "line 5" || got[4].StartMS != 10000 || got[5].Text != "PT: line 6" {
		t.Fatalf("cues %+v", got)
	}
	rows, _ := stopgapRows(ctx, f.st.Read(), f.movieFile)
	if len(rows) != 1 || *rows[0].Made != MadeTranslatedPartial {
		t.Fatalf("rows %+v", rows)
	}
	// The translation is never used as a source for another one.
	if _, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "es", From: "pt-br", ByHand: true}, domain.System); !errors.Is(err, ErrNoSource) {
		t.Fatalf("translated from a translation: %v", err)
	}
}

// A refused key fails the Job for good, logs why, falls back to a plain
// search and holds the language back for a day; the key shows nowhere.
func TestTranslate_FailureFallsBackAndHolds(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	if err := os.WriteFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"), []byte(fakesite.Cue("hello", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := &faketranslator.OpenAI{}
	s, q := withTranslator(t, f, ft)
	ft.Key = "a-different-key" // the stored key is now refused
	wishAll(t, f, Want{Language: "pt-br", TranslateFrom: "en"})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "translating" || len(*q) != 1 {
		t.Fatalf("%+v %v", res.Outcomes, err)
	}
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: (*q)[0].params}}
	err = s.handleTranslate(ctx, r)
	var fe interface{ Final() bool }
	if err == nil || !errors.As(err, &fe) || !fe.Final() || strings.Contains(err.Error(), fakeAIKey) {
		t.Fatalf("job error %v", err)
	}
	es := eventsOf(t, f.st, EventTranslateFailed)
	if len(es) != 1 || !strings.Contains(es[0].Summary, "refused") || strings.Contains(es[0].Summary, fakeAIKey) {
		t.Fatalf("events %+v", es)
	}
	if len(*q) != 1 {
		t.Fatalf("the fallback search queued another translation: %+v", *q)
	}
	var next string
	_ = f.st.Read().QueryRow("SELECT next_try_at FROM subtitles WHERE file_id = ? AND place = 'wish' AND language = 'pt-br'", f.movieFile).Scan(&next)
	if next < database.FormatTime(now().Add(translationHold-1e9)) {
		t.Fatalf("not held for a day: %s", next)
	}
}

// A file a person made beats a machine-made one; with the site's switch off
// machine-made files are never used, and with it on they stand in, AI ones
// first, until a person's file replaces them.
func TestMachineMade_SwitchAndReplacement(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	osf := &fakesite.OpenSubs{Key: mkKey, User: mkUser, Pass: mkPass, Quota: 10, Subs: []fakesite.Sub{
		{ID: "11", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true, MachineTranslated: true, Text: fakesite.Cue("machine", 1000)},
		{ID: "12", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true, AITranslated: true, Text: fakesite.Cue("ia", 1000)},
	}}
	srv := httptest.NewServer(osf)
	t.Cleanup(srv.Close)
	v, _ := secrets.Open(ctx, f.st, make([]byte, secrets.KeySize))
	off := false
	input := func(m *bool) SiteInput {
		return SiteInput{Kind: "opensubtitles", URL: srv.URL + "/api/v1", AllowLocalNetwork: true, MachineMade: m, Limits: &SiteLimits{GapMS: 1, PerWindow: 1000, WindowSeconds: 3600}}
	}
	in := input(&off)
	in.APIKey, in.Username, in.Password = mkKey, mkUser, mkPass
	row, err := AddSite(ctx, f.st, v, in, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: f.st, Vault: v}
	wishAll(t, f, Want{Language: "fr"})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "none_good_enough" {
		t.Fatalf("switch off: %+v %v", res.Outcomes, err)
	}
	on := true
	if _, err := EditSite(ctx, f.st, v, row.ID, input(&on), domain.System); err != nil {
		t.Fatal(err)
	}
	res, err = s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "saved" || res.Outcomes[0].Made != "ai_translated" || res.Outcomes[0].Score > scoreAITranslated {
		t.Fatalf("switch on: %+v %v", res.Outcomes, err)
	}
	// An edit that leaves the switch out keeps it.
	if r, err := EditSite(ctx, f.st, v, row.ID, input(nil), domain.System); err != nil || !r.MachineMade {
		t.Fatalf("switch lost on edit: %+v %v", r, err)
	}
	// Long after it was saved, the better schedule still looks again for a
	// file made by a person, once a week.
	old := database.FormatTime(now().AddDate(0, 0, -30))
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE subtitles SET saved_at = ?, last_tried_at = NULL WHERE file_id = ? AND made IS NOT NULL", old, f.movieFile)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var better []int64
	s.Enqueue = func(_ context.Context, kind string, subj *domain.Subject, params map[string]any, _ string) error {
		if kind == KindSearch && params["better"] == true {
			better = append(better, subj.ID)
		}
		return nil
	}
	if err := s.handleBetter(ctx, &jobs.Run{}); err != nil || len(better) != 1 || better[0] != f.movieFile {
		t.Fatalf("weekly look: %v %v", better, err)
	}
	// A person's file appears and replaces the machine-made one on the next
	// better search, even though it was saved long ago.
	osf.Subs = append(osf.Subs, fakesite.Sub{ID: "13", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true, Trusted: true,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Text: fakesite.Cue("humain", 1000)})
	res, err = s.Search(ctx, f.movieFile, SearchOptions{Better: true, BetterDays: 1}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range res.Outcomes {
		found = found || (o.Result == "upgraded" && o.Made == "")
	}
	if !found {
		t.Fatalf("not replaced: %+v", res.Outcomes)
	}
	b, _ := readFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.fr.srt"))
	if !strings.Contains(b, "humain") {
		t.Fatalf("file %q", b)
	}
}

// A pt-br wish fetches a .pt-BR.srt; a pt wish is still met by one, so
// nothing met today turns missing.
func TestBrazilianPortuguese_WishAndNames(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{{ID: "br", Kind: "movie", Language: "pt-br", Title: "The Lantern", Year: 2010, ByID: true, Trusted: true,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("olá", 1000)}}}
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "pt-br"})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "saved" || res.Outcomes[0].Path != "The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt-BR.srt" {
		t.Fatalf("%+v %v", res.Outcomes, err)
	}
	wishAll(t, f, Want{Language: "pt"})
	st, err := s.Look(ctx, f.movieFile, domain.System)
	if err != nil || len(st.Missing) != 0 {
		t.Fatalf("pt wish not met by pt-BR: %+v %v", st, err)
	}
	rows, _, _ := MissingList(ctx, f.st.Read(), MissingFilter{}, 50, 0)
	for _, r := range rows {
		if r.FileID == f.movieFile {
			t.Fatalf("movie listed missing: %+v", r)
		}
	}
}

func readFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}

// A Lingarr server translates by hand the same way, named as Lingarr.
func TestTranslate_Lingarr(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	if err := os.WriteFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"), []byte(fakesite.Cue("hello", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, f.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	lg := &faketranslator.Lingarr{Key: "made-up-lingarr-key"}
	srv := httptest.NewServer(lg)
	t.Cleanup(srv.Close)
	s := &Service{Store: f.st, Vault: v}
	if _, err := s.SaveTranslator(ctx, TranslatorInput{Kind: TranslatorLingarr, URL: srv.URL, Key: lg.Key}, domain.System); err != nil {
		t.Fatal(err)
	}
	res, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-br", From: "en", ByHand: true}, domain.System)
	if err != nil || res.Partial {
		t.Fatalf("%+v %v", res, err)
	}
	got := readCues(t, f.abs(res.Path))
	if len(got) != 1 || got[0].Text != "pt-BR: hello" || got[0].StartMS != 1000 || lg.Body["targetLanguage"] != "pt-BR" {
		t.Fatalf("%+v %v", got, lg.Body)
	}
	rows, _ := stopgapRows(ctx, f.st.Read(), f.movieFile)
	if len(rows) != 1 || *rows[0].SiteName != "Lingarr" {
		t.Fatalf("%+v", rows)
	}
}

// roundTrip runs a function for every call the translator makes.
type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A person's subtitle that turns up while the translator works wins: the
// translation is dropped, and nothing is saved next to the person's file.
func TestTranslate_SubtitleArrivingMidwayWins(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	if err := os.WriteFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"), []byte(fakesite.Cue("hello", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := &faketranslator.OpenAI{}
	s, _ := withTranslator(t, f, ft)
	theirs := f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.pob.srt")
	s.TranslateHTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if err := os.WriteFile(theirs, []byte(fakesite.Cue("feito à mão", 1000)), 0o644); err != nil {
			return nil, err
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	res, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-br", From: "en", ByHand: true}, domain.System)
	if err != nil || res.Skipped == "" || ft.Calls == 0 {
		t.Fatalf("%+v %v (calls %d)", res, err, ft.Calls)
	}
	if _, err := os.Stat(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt-BR.srt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a translation was saved next to the person's file: %v", err)
	}
}

// Clearing the address turns the translator off, and its key goes with it: a
// key never waits in the vault for a translator that is gone.
func TestSaveTranslator_ClearingTheAddressDropsTheKey(t *testing.T) {
	f := newFix(t)
	s, _ := withTranslator(t, f, &faketranslator.OpenAI{})
	got, err := s.SaveTranslator(context.Background(), TranslatorInput{}, domain.System)
	if err != nil || got.Kind != "" || got.URL != "" || got.HasKey {
		t.Fatalf("%+v %v", got, err)
	}
}

// Picking a pt subtitle by hand for a pt wish replaces the pt-BR file the app
// saved for that wish, so the wish keeps one file.
func TestChoose_ReplacesTheRegionalFileOfABaseWish(t *testing.T) {
	f := newFix(t)
	br := fakesite.Sub{ID: "br", Kind: "movie", Language: "pt-br", Title: "The Lantern", Year: 2010, ByID: true, Trusted: true,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("olá", 1000)}
	site := &fakesite.Site{Subs: []fakesite.Sub{br}}
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "pt-br"})
	if res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v %v", res.Outcomes, err)
	}
	wishAll(t, f, Want{Language: "pt"})
	pt := br
	pt.ID, pt.Language, pt.Text = "pt", "pt", fakesite.Cue("olá de Lisboa", 1000)
	site.Subs = append(site.Subs, pt)
	rows, err := Sites(ctx, f.st.Read())
	if err != nil || len(rows) != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	p, err := s.Choose(ctx, f.movieFile, rows[0].Name, "pt", domain.System)
	if err != nil || p != "The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt.srt" {
		t.Fatalf("%q %v", p, err)
	}
	if _, err := os.Stat(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.pt-BR.srt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the pt-BR file stayed beside the chosen one: %v", err)
	}
}
