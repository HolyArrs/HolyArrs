package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/translate/faketranslator"
)

// stubPluginSite stands in for a plugin's subtitle site.
type stubPluginSite struct {
	mu      sync.Mutex
	offers  []sites.Candidate
	asked   []sites.Query
	made    []sites.Candidate
	text    string
	err     error
	onMake  func()
	searchE error
	// gone is how many makes find the plugin not running, as while it
	// starts; held, when set, gets the first make, which then waits for its
	// Job to stop, as an hour-long make does when HolyArrs stops.
	gone int
	held chan struct{}
}

func (p *stubPluginSite) Search(_ context.Context, q sites.Query) ([]sites.Candidate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, q)
	out := make([]sites.Candidate, len(p.offers))
	for i, o := range p.offers {
		o.Link = itoa(q.FileID) // as the app's adapter does
		out[i] = o
	}
	return out, p.searchE
}

func (p *stubPluginSite) Download(ctx context.Context, c sites.Candidate) ([]byte, error) {
	p.mu.Lock()
	p.made = append(p.made, c)
	on, err, text, held := p.onMake, p.err, p.text, p.held
	p.held = nil
	if err == nil && held == nil && p.gone > 0 {
		p.gone--
		err = fmt.Errorf("%w (starting)", sites.ErrPluginGone)
	}
	p.mu.Unlock()
	if held != nil {
		close(held)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if on != nil {
		on()
	}
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// withPluginSite adds a plugin subtitle site served by p and returns its row.
func withPluginSite(t *testing.T, f *fix, p *stubPluginSite, limits *SiteLimits) SiteRow {
	t.Helper()
	old := sites.PluginSite
	t.Cleanup(func() { sites.PluginSite = old })
	sites.PluginSite = func(id string) sites.Site {
		if id != "ws" {
			t.Errorf("plugin %q", id)
		}
		return p
	}
	row, err := AddSite(context.Background(), f.st, nil, SiteInput{Kind: "plugin", URL: "plugin://ws", Limits: limits}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func pluginStub() *stubPluginSite {
	// The plugin claims a person made it; HolyArrs never believes that.
	return &stubPluginSite{offers: []sites.Candidate{{Key: "audio-1", Language: "fr", Format: "srt"}}, text: fakesite.Cue("bonjour", 1000)}
}

const movieFr = "The Lantern (2010)/The Lantern (2010) 1080p BluRay.fr.srt"

// A plugin site's offer is machine-made, whatever the plugin says, and a
// search queues its own Job for it instead of waiting: the plugin is never
// asked to make anything while the search runs.
func TestPluginSite_SearchQueuesMakeAndNeverWaits(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := pluginStub()
	s, q := svc(t, f, &fakesite.Site{})
	row := withPluginSite(t, f, p, nil)
	if !row.MachineMade || row.URL != "plugin://ws" {
		t.Fatalf("row %+v", row)
	}
	wishAll(t, f, Want{Language: "fr"})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil || len(res.Outcomes) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	o := res.Outcomes[0]
	if o.Result != "making" || o.Made != sites.Transcribed || o.Site != "Plugin" {
		t.Fatalf("outcome %+v", o)
	}
	if len(p.made) != 0 {
		t.Fatalf("the search waited on the plugin: %d makes", len(p.made))
	}
	if len(p.asked) != 1 || p.asked[0].FileID != f.movieFile {
		t.Fatalf("asked %+v", p.asked)
	}
	if len(*q) != 1 || (*q)[0].kind != KindMake || (*q)[0].id != f.movieFile {
		t.Fatalf("queued %+v", *q)
	}
	var state string
	if err := f.st.Read().QueryRow("SELECT state FROM subtitles WHERE file_id = ? AND place = 'wish' AND language = 'fr'", f.movieFile).Scan(&state); err != nil || state != "none_good_enough" {
		t.Fatalf("wish %q %v", state, err)
	}
	if es := eventsOf(t, f.st, EventNoneGood); len(es) != 0 {
		t.Fatalf("none-good event while making: %+v", es)
	}
	// A search with NoMake, as after a failed make, never queues one.
	*q = nil
	if res, err = s.Search(ctx, f.movieFile, SearchOptions{NoMake: true}, domain.System); err != nil || res.Outcomes[0].Result != "none_good_enough" || len(*q) != 0 {
		t.Fatalf("NoMake: %+v %v %+v", res.Outcomes, err, *q)
	}
}

// A file a person made that passes always wins over a plugin's offer.
func TestPluginSite_PersonsFileWins(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{{ID: "p1", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true, Trusted: true,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("humain", 1000)}}}
	s, q := svc(t, f, site)
	withPluginSite(t, f, pluginStub(), nil)
	wishAll(t, f, Want{Language: "fr"})
	res, err := s.Search(context.Background(), f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "saved" || res.Outcomes[0].Made != "" || len(*q) != 0 {
		t.Fatalf("%+v %v %+v", res.Outcomes, err, *q)
	}
}

// A plugin's file is written from this very File, so its offer scores as a
// full match for it, movie or episode, though the hook sends no title, year,
// episode or release name. It is saved at the transcribed cap all the same,
// and a passing file made by a person still replaces it on the next search.
func TestPluginSite_ScoredAsThisFile(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	site := &fakesite.Site{}
	s, q := svc(t, f, site)
	withPluginSite(t, f, pluginStub(), nil)
	wishAll(t, f, Want{Language: "fr"})
	for _, id := range []int64{f.movieFile, f.ep1File} {
		res, err := s.Search(ctx, id, SearchOptions{Show: true}, domain.System)
		if err != nil || len(res.Outcomes) != 1 || len(res.Outcomes[0].Choices) != 1 {
			t.Fatalf("file %d: %+v %v", id, res, err)
		}
		// Listed at the score it is saved with; its parts show the full match.
		sc := res.Outcomes[0].Choices[0].Score
		raw := 0
		for _, p := range sc.Parts {
			if !p.Matched && p.Name != "trusted" {
				t.Fatalf("file %d: %s not matched in %+v", id, p.Name, sc.Parts)
			}
			raw += p.Points
		}
		if sc.Refused != "" || sc.Total != scoreTranscribed || raw < 98 {
			t.Fatalf("file %d: score %+v (parts %d)", id, sc, raw)
		}
	}

	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || len(*q) != 1 {
		t.Fatalf("%v %+v", err, *q)
	}
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: (*q)[0].params}}
	if err := s.handleMake(ctx, r); err != nil {
		t.Fatal(err)
	}
	if r.Result()["score"] != float64(scoreTranscribed) {
		t.Fatalf("result %+v", r.Result())
	}
	rows, _ := stopgapRows(ctx, f.st.Read(), f.movieFile)
	if len(rows) != 1 || *rows[0].Score != scoreTranscribed {
		t.Fatalf("rows %+v", rows)
	}

	site.Subs = []fakesite.Sub{{ID: "p1", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true, Trusted: true,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("humain", 1000)}}
	*q = nil
	// As the weekly look for a file made by a person searches.
	res, err := s.Search(ctx, f.movieFile, SearchOptions{Better: true, BetterDays: 7}, domain.System)
	if err != nil || len(res.Outcomes) != 1 || res.Outcomes[0].Result != "upgraded" || res.Outcomes[0].Made != "" || len(*q) != 0 {
		t.Fatalf("%+v %v %+v", res.Outcomes, err, *q)
	}
	if b, _ := readFile(f.abs(res.Outcomes[0].Path)); !strings.Contains(b, "humain") {
		t.Fatalf("file %q", b)
	}
	if rows, _ := stopgapRows(ctx, f.st.Read(), f.movieFile); len(rows) != 0 {
		t.Fatalf("the transcription is still kept: %+v", rows)
	}
}

// In the choose list a plugin's offer shows, and is ranked, at the score it
// would be saved with: under a passing file made by a person that matches
// less of the File, over one that does not pass.
func TestPluginSite_ChooseListAtSavedScore(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{
		{ID: "pass", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true,
			Release: "The.Lantern.2010.1080p.BluRay.x264-OTHER", Format: "srt", Text: fakesite.Cue("humain", 1000)},
		{ID: "low", Kind: "movie", Language: "fr", Title: "The Lantern", Release: "The.Lantern.720p.WEB", Format: "srt", Text: fakesite.Cue("bas", 1000)},
	}}
	s, _ := svc(t, f, site)
	withPluginSite(t, f, pluginStub(), nil)
	wishAll(t, f, Want{Language: "fr"})
	res, err := s.Search(context.Background(), f.movieFile, SearchOptions{Show: true}, domain.System)
	if err != nil || len(res.Outcomes) != 1 || len(res.Outcomes[0].Choices) != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	cs := res.Outcomes[0].Choices
	pass := ReadSettings(context.Background(), f.st.Read()).PassMarkMovie
	if cs[0].Key != "pass" || cs[0].MachineMade != "" || cs[0].Score.Total < pass || cs[0].Score.Total >= 98 {
		t.Fatalf("first %+v %+v", cs[0].Key, cs[0].Score)
	}
	if cs[1].MachineMade != sites.Transcribed || cs[1].Score.Total != scoreTranscribed {
		t.Fatalf("second %+v %+v", cs[1].Key, cs[1].Score)
	}
	if cs[2].Key != "low" || cs[2].Score.Total >= scoreTranscribed {
		t.Fatalf("third %+v %+v", cs[2].Key, cs[2].Score)
	}
}

// With the owner's translator set up and a source to translate from,
// translation still goes first; the plugin site waits for a later search.
func TestPluginSite_TranslationGoesFirst(t *testing.T) {
	f := newFix(t)
	if err := os.WriteFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"), []byte(fakesite.Cue("hello", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, q := withTranslator(t, f, &faketranslator.OpenAI{})
	withPluginSite(t, f, pluginStub(), nil)
	wishAll(t, f, Want{Language: "fr", TranslateFrom: "en"})
	res, err := s.Search(context.Background(), f.movieFile, SearchOptions{}, domain.System)
	if err != nil || res.Outcomes[0].Result != "translating" || len(*q) != 1 || (*q)[0].kind != KindTranslate {
		t.Fatalf("%+v %v %+v", res.Outcomes, err, *q)
	}
}

// The make Job saves the plugin's file beside the video, marked as written
// from the audio and scored low, so a person's file replaces it later.
func TestMake_SavesMachineMade(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := pluginStub()
	s, q := svc(t, f, &fakesite.Site{})
	withPluginSite(t, f, p, nil)
	wishAll(t, f, Want{Language: "fr"})
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || len(*q) != 1 {
		t.Fatalf("%v %+v", err, *q)
	}
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: (*q)[0].params}}
	if err := s.handleMake(ctx, r); err != nil {
		t.Fatal(err)
	}
	if len(p.made) != 1 || p.made[0].Key != "audio-1" || p.made[0].Link != itoa(f.movieFile) {
		t.Fatalf("made %+v", p.made)
	}
	if r.Result()["path"] != movieFr {
		t.Fatalf("result %+v", r.Result())
	}
	if b, _ := readFile(f.abs(movieFr)); !strings.Contains(b, "bonjour") {
		t.Fatalf("file %q", b)
	}
	rows, _ := stopgapRows(ctx, f.st.Read(), f.movieFile)
	if len(rows) != 1 || *rows[0].Made != sites.Transcribed || *rows[0].Score > scoreTranscribed || *rows[0].SiteName != "Plugin" || rows[0].Origin != "fetched" {
		t.Fatalf("rows %+v", rows)
	}
	if es := eventsOf(t, f.st, EventSaved); len(es) != 1 || !strings.Contains(es[0].Summary, "from Plugin") {
		t.Fatalf("events %+v", es)
	}
	// Scores: written from the audio sits under translated, over AI-translated.
	if stopgapScore(sites.Transcribed, 99) != scoreTranscribed || !(scoreAITranslated < scoreTranscribed && scoreTranscribed < scoreTranslated) {
		t.Fatal("score order")
	}
	if madeRank(sites.Transcribed) >= madeRank(sites.AITranslated) || madeRank(sites.AITranslated) >= madeRank(sites.MachineTranslated) {
		t.Fatal("rank order")
	}
}

// A subtitle that arrives while the plugin works wins: the made file is
// dropped and the person's file is left alone.
func TestMake_SubtitleArrivedMeanwhile(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := pluginStub()
	mine := fakesite.Cue("le mien", 1000)
	p.onMake = func() { _ = os.WriteFile(f.abs(movieFr), []byte(mine), 0o644) }
	s, _ := svc(t, f, &fakesite.Site{})
	row := withPluginSite(t, f, p, nil)
	wishAll(t, f, Want{Language: "fr"})
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	res, err := s.Make(ctx, f.movieFile, MakeRequest{Want: Want{Language: "fr"}, SiteID: row.ID, SiteKey: "audio-1", SiteLanguage: "fr"}, domain.System)
	if err != nil || res.Skipped == "" || res.Path != "" || len(p.made) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if b, _ := readFile(f.abs(movieFr)); b != mine {
		t.Fatalf("file %q", b)
	}
	// Asked again now, it skips before the plugin is even asked.
	if res, err = s.Make(ctx, f.movieFile, MakeRequest{Want: Want{Language: "fr"}, SiteID: row.ID, SiteKey: "audio-1", SiteLanguage: "fr"}, domain.System); err != nil || res.Skipped == "" || len(p.made) != 1 {
		t.Fatalf("again: %+v %v made %d", res, err, len(p.made))
	}
}

// A plugin that fails ends the Job for good, falls back to a search that
// queues no new make and holds the language back a day. A plugin that is
// only restarting is tried again, with no hold.
func TestMake_FailureAndRestart(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := pluginStub()
	s, q := svc(t, f, &fakesite.Site{})
	withPluginSite(t, f, p, nil)
	wishAll(t, f, Want{Language: "fr"})
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || len(*q) != 1 {
		t.Fatalf("%v %+v", err, *q)
	}
	params := (*q)[0].params
	run := func() error {
		*q = nil
		return s.handleMake(ctx, &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: params}})
	}
	nextTry := func() time.Time {
		var at string
		if err := f.st.Read().QueryRow("SELECT next_try_at FROM subtitles WHERE file_id = ? AND place = 'wish' AND language = 'fr'", f.movieFile).Scan(&at); err != nil {
			t.Fatal(err)
		}
		tm, _ := time.Parse(database.TimeLayout, at)
		return tm
	}

	// The plugin is not running: the make is queued again a minute on, under
	// the other key, and nothing is held back or reported.
	p.err = fmt.Errorf("%w (stopped)", sites.ErrPluginGone)
	key := makeKey(f.movieFile, Want{Language: "fr"})
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: params, DedupeKey: &key}}
	*q = nil
	if err := s.handleMake(ctx, r); err != nil || len(*q) != 1 || r.Result()["waiting"] == nil {
		t.Fatalf("plugin gone: %v queued %+v result %+v", err, *q, r.Result())
	}
	again := (*q)[0]
	if again.kind != KindMake || again.key != key+":wait" || again.at.Sub(now()) > makeRetryEvery || again.at.Sub(now()) < makeRetryEvery-time.Second {
		t.Fatalf("queued again %+v", again)
	}
	since, _ := again.params["wait_since"].(string)
	if since == "" || nextTry().After(now().Add(12*time.Hour)) || len(eventsOf(t, f.st, EventFailed)) != 0 {
		t.Fatalf("since %q next %v", since, nextTry())
	}
	// The next try keeps when the wait began and takes the first key back.
	k2 := key + ":wait"
	r = &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: again.params, DedupeKey: &k2}}
	*q = nil
	if err := s.handleMake(ctx, r); err != nil || len(*q) != 1 || (*q)[0].key != key || (*q)[0].params["wait_since"] != since {
		t.Fatalf("second wait: %v %+v", err, *q)
	}
	// A day on, it gives up as a failed make does.
	old := makeParams(MakeRequest{Want: Want{Language: "fr"}, SiteID: int64(params["site_id"].(float64)), SiteKey: "audio-1", SiteLanguage: "fr",
		WaitSince: database.FormatTime(now().Add(-makeWaitLimit - time.Minute))})
	*q = nil
	err := s.handleMake(ctx, &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: old}})
	var fe interface{ Final() bool }
	if err == nil || !errors.As(err, &fe) || !fe.Final() || !strings.Contains(err.Error(), "within a day") || len(*q) != 0 {
		t.Fatalf("a day on: %v %+v", err, *q)
	}
	if nextTry().Before(now().Add(23 * time.Hour)) {
		t.Fatalf("not held after a day: %v", nextTry())
	}

	p.err = errors.New("the Whisper server did not answer")
	err = run()
	if err == nil || !errors.As(err, &fe) || !fe.Final() || !strings.Contains(err.Error(), "Whisper") {
		t.Fatalf("failure: %v", err)
	}
	if len(*q) != 0 {
		t.Fatalf("the fallback search queued %+v", *q)
	}
	if nextTry().Before(now().Add(23 * time.Hour)) {
		t.Fatalf("not held: %v", nextTry())
	}
	if es := eventsOf(t, f.st, EventFailed); len(es) != 1 || !strings.Contains(es[0].Summary, "Whisper") {
		t.Fatalf("events %+v", es)
	}
}

// A forced offer for a forced want is made as forced, not refused.
// HolyArrs stops while a plugin makes a subtitle. On the next start the
// queue runs the make again, before the plugin polls again; the make waits
// for it, a minute at a time, and saves the subtitle once it is back, with
// no one searching or choosing the offer again.
func TestMake_RestartMidMakeResumesOnItsOwn(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	oldEvery := makeRetryEvery
	makeRetryEvery = 20 * time.Millisecond
	t.Cleanup(func() { makeRetryEvery = oldEvery })
	p := pluginStub()
	started := make(chan struct{})
	p.held = started
	withPluginSite(t, f, p, &SiteLimits{GapMS: 1, PerWindow: 1000, WindowSeconds: 3600, DownloadsToDay: 1})
	wishAll(t, f, Want{Language: "fr"})
	opts := jobs.Options{Workers: 1, PollInterval: 10 * time.Millisecond}

	first := &Service{Store: f.st}
	q1 := jobs.New(f.st, opts)
	first.Register(q1)
	if err := q1.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if res, err := first.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || res.Outcomes[0].Result != "making" {
		t.Fatalf("%+v %v", res, err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the make never reached the plugin")
	}
	q1.Stop() // HolyArrs stops mid-make

	// The next start: the plugin is not polling yet for the first two tries.
	p.mu.Lock()
	p.gone = 2
	p.mu.Unlock()
	next := &Service{Store: f.st}
	q2 := jobs.New(f.st, opts)
	next.Register(q2)
	if err := q2.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q2.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for {
		rows, _ := stopgapRows(ctx, f.st.Read(), f.movieFile)
		if len(rows) == 1 {
			if *rows[0].Made != sites.Transcribed || *rows[0].Score != scoreTranscribed {
				t.Fatalf("saved %+v", rows[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the make did not resume on its own")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b, _ := readFile(f.abs(movieFr)); !strings.Contains(b, "bonjour") {
		t.Fatalf("file %q", b)
	}
	p.mu.Lock()
	made := len(p.made)
	p.mu.Unlock()
	if made != 4 { // held, two while starting, then made
		t.Fatalf("the plugin was asked %d times", made)
	}
	// Only the make the plugin finished counted against the daily cap of 1.
	var used int
	if err := f.st.Read().QueryRow("SELECT downloads_today FROM subtitle_sites WHERE kind = 'plugin'").Scan(&used); err != nil || used != 1 {
		t.Fatalf("downloads counted %d %v", used, err)
	}
	var states []string
	rows, err := f.st.Read().Query("SELECT state || ':' || COALESCE(progress_note, '') FROM jobs WHERE kind = ? ORDER BY id", KindMake)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var st string
		_ = rows.Scan(&st)
		states = append(states, st)
	}
	rows.Close()
	if len(states) != 3 || states[0] != "done:restarted after a stop" || states[1] != "done:" || states[2] != "done:" {
		t.Fatalf("make Jobs %v", states)
	}
}

func TestMake_ForcedOffer(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := &stubPluginSite{offers: []sites.Candidate{{Key: "signs", Language: "fr", Forced: true, Format: "srt"}}, text: fakesite.Cue("panneau", 1000)}
	s, q := svc(t, f, &fakesite.Site{})
	withPluginSite(t, f, p, nil)
	wishAll(t, f, Want{Language: "fr", Forced: true})
	if res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || res.Outcomes[0].Result != "making" || len(*q) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: (*q)[0].params}}
	if err := s.handleMake(ctx, r); err != nil || r.Result()["path"] != "The Lantern (2010)/The Lantern (2010) 1080p BluRay.fr.forced.srt" {
		t.Fatalf("%v %+v", err, r.Result())
	}
}

// A plugin site turned off, or one at its daily cap, makes nothing.
func TestMake_SiteOffAndDailyCap(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := pluginStub()
	s, _ := svc(t, f, &fakesite.Site{})
	row := withPluginSite(t, f, p, &SiteLimits{GapMS: 1, PerWindow: 1000, WindowSeconds: 3600, DownloadsToDay: 1})
	wishAll(t, f, Want{Language: "fr"})
	req := MakeRequest{Want: Want{Language: "fr"}, SiteID: row.ID, SiteKey: "audio-1", SiteLanguage: "fr", ByHand: true}
	// A make that finds the plugin not running uses none of the day's cap.
	p.gone = 2
	for range 2 {
		if _, err := s.Make(ctx, f.movieFile, req, domain.System); !errors.Is(err, sites.ErrPluginGone) {
			t.Fatalf("plugin gone: %v", err)
		}
	}
	if res, err := s.Make(ctx, f.movieFile, req, domain.System); err != nil || res.Path != movieFr {
		t.Fatalf("first: %+v %v", res, err)
	}
	if _, err := s.Make(ctx, f.movieFile, req, domain.System); err == nil || !strings.Contains(err.Error(), "used up") || len(p.made) != 3 {
		t.Fatalf("over the cap: %v, made %d", err, len(p.made))
	}
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE subtitle_sites SET state = 'disabled' WHERE id = ?", row.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if res, err := s.Make(ctx, f.movieFile, req, domain.System); err != nil || res.Skipped == "" || len(p.made) != 3 {
		t.Fatalf("off: %+v %v", res, err)
	}
}

// A person who picks a plugin site's offer gets a make Job, and the made
// file replaces the subtitle the app saved, marked as their pick.
func TestChoose_PluginOfferQueuesMake(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := pluginStub()
	site := &fakesite.Site{Subs: []fakesite.Sub{{ID: "p1", Kind: "movie", Language: "fr", Title: "The Lantern", Year: 2010, ByID: true, Trusted: true,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("humain", 1000)}}}
	s, q := svc(t, f, site)
	withPluginSite(t, f, p, nil)
	wishAll(t, f, Want{Language: "fr"})
	if res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil || res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v %v", res, err)
	}
	path, err := s.Choose(ctx, f.movieFile, "Plugin", "audio-1", domain.System)
	if err != nil || path != "" || len(*q) != 1 || (*q)[0].kind != KindMake || (*q)[0].params["by_hand"] != true || len(p.made) != 0 {
		t.Fatalf("choose: %q %v %+v", path, err, *q)
	}
	r := &jobs.Run{Job: jobs.Job{Subject: &domain.Subject{Type: "file", ID: f.movieFile}, Params: (*q)[0].params}}
	if err := s.handleMake(ctx, r); err != nil || r.Result()["path"] != movieFr {
		t.Fatalf("make: %v %+v", err, r.Result())
	}
	if b, _ := readFile(f.abs(movieFr)); !strings.Contains(b, "bonjour") {
		t.Fatalf("file %q", b)
	}
	var origin string
	if err := f.st.Read().QueryRow("SELECT origin FROM subtitles WHERE file_id = ? AND relative_path = ? AND state = 'present'", f.movieFile, movieFr).Scan(&origin); err != nil || origin != "by_hand" {
		t.Fatalf("origin %q %v", origin, err)
	}
}

// Only kind plugin takes a plugin address, and it takes nothing else; an
// edit that leaves the address out keeps it.
func TestPluginSite_Address(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	for _, in := range []SiteInput{{Kind: "plugin"}, {Kind: "plugin", URL: "https://ws.example"}, {Kind: "plugin", URL: "plugin://ws/x"}, {Kind: "subdl", URL: "plugin://ws", APIKey: "k"}} {
		if _, err := AddSite(ctx, f.st, nil, in, domain.System); err == nil {
			t.Errorf("%+v: want an error", in)
		}
	}
	row, err := AddSite(ctx, f.st, nil, SiteInput{Kind: "plugin", URL: "plugin://ws", Name: "Whisper"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	if r, err := EditSite(ctx, f.st, nil, row.ID, SiteInput{Name: "Whisper", MachineMade: &off}, domain.System); err != nil || r.URL != "plugin://ws" || r.MachineMade {
		t.Fatalf("edit: %+v %v", r, err)
	}
}
