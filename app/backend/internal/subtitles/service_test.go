package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

type queued struct {
	kind   string
	id     int64
	params map[string]any
}

// svc builds a Service on the fixture with the lab's fake site added.
func svc(t *testing.T, f *fix, site *fakesite.Site) (*Service, *[]queued) {
	t.Helper()
	srv := httptest.NewServer(site)
	t.Cleanup(srv.Close)
	s := &Service{Store: f.st}
	var q []queued
	s.Enqueue = func(_ context.Context, kind string, subj *domain.Subject, params map[string]any, _ string) error {
		q = append(q, queued{kind, subj.ID, params})
		return nil
	}
	if _, err := AddSite(context.Background(), f.st, nil, SiteInput{Kind: "fake", URL: srv.URL, AllowLocalNetwork: true,
		Limits: &SiteLimits{GapMS: 1, PerWindow: 1000, WindowSeconds: 3600}}, domain.System); err != nil {
		t.Fatal(err)
	}
	return s, &q
}

func wishAll(t *testing.T, f *fix, ws ...Want) {
	t.Helper()
	set := ReadSettings(context.Background(), f.st.Read())
	set.Languages = ws
	if err := SaveSettings(context.Background(), f.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
}

func (f *fix) abs(rel string) string { return filepath.Join(f.lib, filepath.FromSlash(rel)) }

const movieRel = "The Lantern (2010)/The Lantern (2010) 1080p BluRay.mkv"

// relInLib reports whether v is a path relative to the Library folder.
func relInLib(v any) bool {
	p, ok := v.(string)
	return ok && p != "" && !filepath.IsAbs(p) && !strings.Contains(p, `\`) && !strings.HasPrefix(p, "/")
}

func eventsOf(t *testing.T, st *database.Store, kind string) []domain.Event {
	t.Helper()
	es, _, err := st.Events(context.Background(), database.EventFilter{Kinds: []string{kind}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// A lab file with built-in English and forced Spanish is not searched for
// those languages, but French is searched.
func TestSearch_BuiltInCountsTowardTheWish(t *testing.T) {
	f := newFix(t)
	b, err := os.ReadFile(filepath.Join("container", "testdata", "lab-built-in.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.abs(movieRel), b, 0o644); err != nil {
		t.Fatal(err)
	}
	site := fakesite.Lab()
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"}, Want{Language: "es", Forced: true})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Status.Missing) != 0 || site.Searches != 0 || len(res.Outcomes) != 0 {
		t.Fatalf("searched for what is built in: %+v, %d searches", res, site.Searches)
	}
	rows, _ := ListFor(ctx, f.st.Read(), f.movieFile)
	if len(rows) != 3 || rows[0].Place != "built_in" {
		t.Fatalf("rows %+v", rows)
	}
	// When built-in subtitles do not count, English is searched and saved.
	set := ReadSettings(ctx, f.st.Read())
	set.UseBuiltIn = false
	_ = SaveSettings(ctx, f.st, set, domain.System)
	res, _ = s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if site.Searches != 1 || len(res.Outcomes) != 2 || res.Outcomes[0].Result != "saved" {
		t.Fatalf("not counted: %+v", res.Outcomes)
	}
}

// A lab movie gets a subtitle file named with its language, plus an Event.
func TestService_SaveNamesAndEvents(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"}, Want{Language: "fr", HearingImpaired: HIEither})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 2 || res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v", res.Outcomes)
	}
	want := "The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"
	if res.Outcomes[0].Path != want {
		t.Fatalf("path %q", res.Outcomes[0].Path)
	}
	b, err := os.ReadFile(f.abs(want))
	if err != nil || !strings.Contains(string(b), "-->") {
		t.Fatalf("file %q %v", b, err)
	}
	if len(eventsOf(t, f.st, EventSaved)) != 2 {
		t.Error("saved events")
	}
	// The wish is now fully met, so a second search asks no site.
	st, _ := s.Look(ctx, f.movieFile, domain.System)
	if len(st.Missing) != 0 {
		t.Fatalf("missing after save: %+v", st.Missing)
	}
	// No temporary files are left behind.
	ents, _ := os.ReadDir(filepath.Dir(f.abs(want)))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".holyarrs-sub") {
			t.Errorf("left %s", e.Name())
		}
	}
	for _, c := range []struct {
		hi, forced bool
		sfx, want  string
	}{{false, false, "hi", "a/M.en.srt"}, {true, false, "hi", "a/M.en.hi.srt"}, {true, false, "sdh", "a/M.en.sdh.srt"}, {false, true, "hi", "a/M.en.forced.srt"}} {
		if got := SubtitleName("a/M.mkv", "en", c.hi, c.forced, c.sfx, "srt"); got != c.want {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
}

// Weak subtitles are skipped and the wish is retried later, a zipped answer
// is unpacked, and a found file is never overwritten.
func TestService_PassMarkZipAndFoundFiles(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{
		{ID: "weak", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.720p.WEB.x264-OTHER", Format: "srt", Text: fakesite.Cue("x", 1000)},
		{ID: "es", Kind: "movie", Language: "es", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("hola", 1000), Zipped: true},
	}}
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"}, Want{Language: "es"}, Want{Language: "de"})
	// A German file a person placed there counts as present.
	found := "The Lantern (2010)/The Lantern (2010) 1080p BluRay.de.srt"
	if err := os.WriteFile(f.abs(found), []byte(fakesite.Cue("hallo", 0)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 2 || res.Outcomes[0].Result != "none_good_enough" || res.Outcomes[1].Result != "saved" {
		t.Fatalf("%+v", res.Outcomes)
	}
	if _, err := os.Stat(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.es.srt")); err != nil {
		t.Fatal("zip not saved")
	}
	var state string
	var next sql.NullString
	_ = f.st.Read().QueryRow("SELECT state, next_try_at FROM subtitles WHERE place = 'wish' AND language = 'en'").Scan(&state, &next)
	if state != "none_good_enough" || !next.Valid {
		t.Fatalf("wish row %s %v", state, next)
	}
	if len(eventsOf(t, f.st, EventNoneGood)) != 1 {
		t.Error("none-good event")
	}
	// The found German file is untouched and recorded as found.
	if b, _ := os.ReadFile(f.abs(found)); !strings.Contains(string(b), "hallo") {
		t.Error("found file changed")
	}
	// Saving over the name of a found file is refused.
	fc, _ := s.load(ctx, f.movieFile)
	_, err = s.save(ctx, fc, &Subtitle{FileID: f.movieFile, Language: "de", Place: "beside", Origin: "fetched", State: "present", Sync: map[string]any{}},
		fakesite.Cue("x", 0), "srt", DefaultSettings(), nil, domain.System)
	if err != ErrNotOurs {
		t.Fatalf("over a found file: %v", err)
	}
}

// A better subtitle replaces the saved one, which moves to the holding folder.
func TestService_BetterSubtitle(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{
		{ID: "ok", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-OTHER", Format: "srt", Text: fakesite.Cue("first", 1000)},
	}}
	s, q := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	res, _ := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if res.Outcomes[0].Result != "saved" || res.Outcomes[0].Score != 72 {
		t.Fatalf("%+v", res.Outcomes)
	}
	// The schedule picks it up because 72 is under 99 and it was saved today.
	if err := s.handleBetter(ctx, &jobs.Run{}); err != nil {
		t.Fatal(err)
	}
	if len(*q) != 1 || (*q)[0].params["better"] != true {
		t.Fatalf("queued %+v", *q)
	}
	// Nothing better turns up, so the subtitle is kept.
	res, _ = s.Search(ctx, f.movieFile, SearchOptions{Better: true, BetterDays: 7}, domain.System)
	if res.Outcomes[0].Result != "kept" {
		t.Fatalf("%+v", res.Outcomes)
	}
	site.Subs = append(site.Subs, fakesite.Sub{ID: "best", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("second", 1000)})
	res, _ = s.Search(ctx, f.movieFile, SearchOptions{Better: true, BetterDays: 7}, domain.System)
	if res.Outcomes[0].Result != "upgraded" || res.Outcomes[0].Score != 97 {
		t.Fatalf("%+v", res.Outcomes)
	}
	b, _ := os.ReadFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"))
	if !strings.Contains(string(b), "second") {
		t.Fatal("not replaced")
	}
	held, _ := filepath.Glob(filepath.Join(f.lib, ".holyarrs-replaced", "*", "The Lantern (2010)", "*.en.srt"))
	if len(held) != 1 {
		t.Fatalf("held %v", held)
	}
	var n int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE state = 'replaced'").Scan(&n)
	if n != 1 || len(eventsOf(t, f.st, EventUpgraded)) != 1 {
		t.Fatalf("replaced rows %d", n)
	}
}

// A failing site rests and is not asked again until the rest ends.
func TestService_SitePause(t *testing.T) {
	f := newFix(t)
	site := fakesite.Lab()
	site.Fail = 503
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	rows, _ := Sites(ctx, f.st.Read())
	if rows[0].State != "paused" || *rows[0].PauseReason != "down" || rows[0].Failures != 1 {
		t.Fatalf("%+v", rows[0])
	}
	if len(eventsOf(t, f.st, EventSitePaused)) != 1 {
		t.Error("paused event")
	}
	res, _ := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if len(res.Asked) != 0 {
		t.Fatalf("asked a resting site: %v", res.Asked)
	}
	// After the rest it is asked again, and a good answer clears the rest.
	site.Fail = 0
	now = func() time.Time { return time.Now().UTC().Add(time.Hour) }
	t.Cleanup(func() { now = func() time.Time { return time.Now().UTC() } })
	res, _ = s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if len(res.Asked) != 1 || res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v", res)
	}
	rows, _ = Sites(ctx, f.st.Read())
	if rows[0].State != "enabled" || rows[0].Failures != 0 {
		t.Fatalf("%+v", rows[0])
	}
}

// Timing fixes and turning a subtitle down.
func TestService_SyncAndTurnDown(t *testing.T) {
	f := newFix(t)
	site := fakesite.Lab()
	s, q := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	res, _ := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	rel := res.Outcomes[0].Path
	rows, _ := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "place = 'beside' AND state = 'present'")
	nr, err := s.Sync(ctx, rows[0].ID, subtext.Fix{OffsetMS: 2000}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.abs(rel))
	if !strings.Contains(string(b), "00:00:03,000 --> 00:00:04,500") {
		t.Fatalf("not moved: %q", b)
	}
	if nr.Sync["offset_ms"] != int64(2000) || len(eventsOf(t, f.st, EventSynced)) != 1 {
		t.Fatalf("%+v", nr.Sync)
	}
	if _, err := s.Sync(ctx, rows[0].ID, subtext.Fix{OffsetMS: 1}, domain.System); err != ErrNotBeside {
		t.Fatalf("old row: %v", err)
	}
	if _, err := s.Sync(ctx, nr.ID, subtext.Fix{}, domain.System); err == nil {
		t.Fatal("empty fix")
	}
	if err := s.TurnDown(ctx, nr.ID, "wrong film", domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.abs(rel)); !os.IsNotExist(err) {
		t.Fatal("still there")
	}
	if len(*q) != 1 {
		t.Fatalf("no new search queued: %+v", *q)
	}
	// Events carry paths inside the Library folder.
	if es := eventsOf(t, f.st, EventTurnedDown); len(es) != 1 || !relInLib(es[0].Data["moved_to"]) {
		t.Fatalf("turned-down event %+v", es)
	}
	// The next search does not pick the turned-down subtitle again.
	res, _ = s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if res.Outcomes[0].Result != "none_good_enough" {
		t.Fatalf("%+v", res.Outcomes)
	}
}

// A placed import queues a search. A better copy holds the old File's
// subtitles aside and queues the new File. The missing schedule finds Files
// never looked at and wishes that are due.
func TestService_EventsAndSchedules(t *testing.T) {
	f := newFix(t)
	s, q := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	slot := f.slotOf(t, f.movieFile)
	s.OnEvent(domain.Event{Kind: "import_done", SlotID: &slot})
	s.OnEvent(domain.Event{Kind: "import_rejected", SlotID: &slot})
	if len(*q) != 1 || (*q)[0].id != f.movieFile {
		t.Fatalf("%+v", *q)
	}
	// With no wish set nothing is queued; after that every File is.
	*q = nil
	if err := s.handleMissing(ctx, &jobs.Run{}); err != nil || len(*q) != 0 {
		t.Fatalf("%v %+v", err, *q)
	}
	wishAll(t, f, Want{Language: "en"})
	_ = s.handleMissing(ctx, &jobs.Run{})
	if len(*q) != 3 {
		t.Fatalf("queued %+v", *q)
	}
	// Files looked at under the same wish and fully met are not queued again.
	for _, id := range []int64{f.movieFile, f.ep1File, f.ep2File} {
		if _, err := s.Search(ctx, id, SearchOptions{}, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	*q = nil
	_ = s.handleMissing(ctx, &jobs.Run{})
	if len(*q) != 0 {
		t.Fatalf("queued again %+v", *q)
	}
	// A better copy moves the old File's saved subtitle to the holding folder.
	var newFile int64
	err := f.st.Write(ctx, func(tx *sql.Tx) error {
		ts := database.FormatTime(now())
		res, err := tx.Exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by)
			VALUES (?, ?, ?, ?, ?, ?, 'The Lantern (2010)/The Lantern (2010) 2160p.mkv', ?, 'imported', 'copy')`, ts, sysActor, ts, sysActor, "["+itoa(slot)+"]", f.movieVer, ts)
		if err != nil {
			return err
		}
		newFile, _ = res.LastInsertId()
		_, err = tx.Exec("UPDATE files SET state = 'replaced' WHERE id = ?", f.movieFile)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	*q = nil
	s.OnEvent(domain.Event{Kind: "file_replaced", Data: map[string]any{"old_file_id": float64(f.movieFile), "new_file_id": float64(newFile)}})
	if len(*q) != 1 || (*q)[0].id != newFile {
		t.Fatalf("%+v", *q)
	}
	held, _ := filepath.Glob(filepath.Join(f.lib, ".holyarrs-replaced", "*", "The Lantern (2010)", "*.en.srt"))
	if hs := eventsOf(t, f.st, EventHeldWithVideo); len(held) != 1 || len(hs) != 1 || !relInLib(hs[0].Data["moved_to"]) {
		t.Fatalf("held %v", held)
	}
	// The missing list shows wanted subtitles that are not present.
	if err := SetWish(ctx, f.st, f.show, 0, 0, []Want{{Language: "es"}}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Look(ctx, f.ep1File, domain.System); err != nil {
		t.Fatal(err)
	}
	list, total, err := MissingList(ctx, f.st.Read(), MissingFilter{Language: "es"}, 50, 0)
	if err != nil || total != 1 || list[0].Title != "Quiet Harbour" || *list[0].Episode != 2 {
		t.Fatalf("%+v %d %v", list, total, err)
	}
}

func TestService_MusicAndOddFiles(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	if _, err := s.Search(context.Background(), 9999, SearchOptions{}, domain.System); err != ErrNotFound {
		t.Errorf("no file: %v", err)
	}
	if _, ok := besideName("Movie", "Movie.en.hi.srt"); !ok {
		t.Error("hi name")
	}
	for _, bad := range []string{"Movie.srt", "Movie.xx.srt", "Other.en.srt", "Movie.en.weird.srt", "Movie.en.txt"} {
		if _, ok := besideName("Movie", bad); ok {
			t.Errorf("%s read", bad)
		}
	}
	h, _ := besideName("Movie", "movie.ES.forced.ass")
	if h.Language != "es" || !h.Forced {
		t.Errorf("%+v", h)
	}
}

func (f *fix) slotOf(t *testing.T, fileID int64) int64 {
	t.Helper()
	var id int64
	if err := f.st.Read().QueryRow("SELECT id FROM slots WHERE file_id = ?", fileID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// At most MaxCandidatesPerSite candidates from one site are scored.
func TestService_SearchCapsCandidatesPerSite(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{}
	for i := range MaxCandidatesPerSite + 50 {
		site.Subs = append(site.Subs, fakesite.Sub{ID: "s" + itoa(int64(i)), Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010,
			Release: "The.Lantern.2010.720p.WEB.x264-OTHER", Format: "srt", Text: fakesite.Cue("x", 1000)})
	}
	s, _ := svc(t, f, site)
	wishAll(t, f, Want{Language: "en"})
	res, err := s.Search(context.Background(), f.movieFile, SearchOptions{Show: true}, domain.System)
	if err != nil || len(res.Outcomes) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if n := len(res.Outcomes[0].Choices); n != MaxCandidatesPerSite {
		t.Fatalf("%d choices scored, want %d", n, MaxCandidatesPerSite)
	}
}

func fetchedRow(fileID int64, lang string) *Subtitle {
	return &Subtitle{FileID: fileID, Language: lang, Place: "beside", Origin: "fetched", State: "present", Sync: map[string]any{}}
}

// When the final rename of a save fails, the file it held aside returns to
// its name and its row stays present.
func TestService_SaveFailurePutsHeldFileBack(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)
	rel, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("first", 0), "srt", DefaultSettings(), nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	renameInto = func(from, to string) error { return errors.New("disk full") }
	defer func() { renameInto = os.Rename }()
	if _, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("second", 0), "srt", DefaultSettings(), nil, domain.System); err == nil {
		t.Fatal("no error")
	}
	if b, _ := os.ReadFile(f.abs(rel)); !strings.Contains(string(b), "first") {
		t.Fatalf("held file not put back: %q", b)
	}
	var n int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE place = 'beside' AND state = 'present' AND relative_path = ?", rel).Scan(&n)
	if n != 1 {
		t.Fatalf("%d present rows", n)
	}
}

// A save cut off after its file is in place but before its row is written is
// finished by the next look. The file is marked fetched, never found, so a
// later better subtitle may still replace it.
func TestService_SaveCutOffBeforeRowIsRecoveredOnLook(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)
	saveCrash = func() error { return errors.New("power cut") }
	rel, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("first", 0), "srt", DefaultSettings(), nil, domain.System)
	saveCrash = nil
	if err == nil {
		t.Fatal("no crash")
	}
	rel = SubtitleName(fc.file.RelativePath, "en", false, false, "hi", "srt")
	if _, err := os.Stat(f.abs(rel)); err != nil {
		t.Fatal("file not in place after the cut")
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	var origin string
	_ = f.st.Read().QueryRow("SELECT origin FROM subtitles WHERE place = 'beside' AND state = 'present' AND relative_path = ?", rel).Scan(&origin)
	if origin != "fetched" {
		t.Fatalf("origin %q, want fetched", origin)
	}
	fc, _ = s.load(ctx, f.movieFile)
	if _, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("second", 0), "srt", DefaultSettings(), nil, domain.System); err != nil {
		t.Fatalf("our own file was not overwritable: %v", err)
	}
}

func TestService_ReplacementCutOffAfterInstallRecoversNewSubtitle(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)
	rel, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("first", 0), "srt", DefaultSettings(), nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	old, err := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "file_id = ? AND state = 'present' AND relative_path = ?", f.movieFile, rel)
	if err != nil || len(old) != 1 {
		t.Fatalf("old rows %+v: %v", old, err)
	}
	saveCrash = func() error { return errors.New("power cut after install") }
	t.Cleanup(func() { saveCrash = nil })
	_, err = s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("second", 0), "srt", DefaultSettings(), &old[0], domain.System)
	saveCrash = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f.abs(rel))
	if err != nil || !strings.Contains(string(got), "second") {
		t.Fatalf("installed subtitle = %q, %v; want second", got, err)
	}
	var present, replaced, pending int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE file_id = ? AND state = 'present' AND relative_path = ?", f.movieFile, rel).Scan(&present)
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE file_id = ? AND state = 'replaced'", f.movieFile).Scan(&replaced)
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitle_saves WHERE file_id = ?", f.movieFile).Scan(&pending)
	if present != 1 || replaced != 1 || pending != 0 {
		t.Fatalf("present %d, replaced %d, pending %d", present, replaced, pending)
	}
}

func TestService_FileMatchesProofRejectsWrongSizeAndLinks(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.srt")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	proof := bytesProof([]byte("expected subtitle"))
	if fileMatchesProof(large, proof) {
		t.Fatal("a large wrong-size target matched the journal proof")
	}
	real := filepath.Join(dir, "real.srt")
	if err := os.WriteFile(real, []byte("expected subtitle"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.srt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if fileMatchesProof(link, proof) {
		t.Fatal("a linked target matched the journal proof")
	}
}

func TestService_SaveCutOffAfterHoldRestoresOldSubtitleOnLook(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)
	rel, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("first", 0), "srt", DefaultSettings(), nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	old, err := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "file_id = ? AND place = 'beside' AND state = 'present' AND relative_path = ?", f.movieFile, rel)
	if err != nil || len(old) != 1 {
		t.Fatalf("old rows %+v: %v", old, err)
	}
	saveHoldCrash = func(int) error { return errors.New("power cut after hold") }
	t.Cleanup(func() { saveHoldCrash = nil })
	_, err = s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("second", 0), "srt", DefaultSettings(), &old[0], domain.System)
	saveHoldCrash = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if _, err := os.Stat(f.abs(rel)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new subtitle was installed before the cut: %v", err)
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f.abs(rel))
	if err != nil || !strings.Contains(string(got), "first") {
		t.Fatalf("old subtitle was not restored: %q, %v", got, err)
	}
	var present, pending int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE file_id = ? AND place = 'beside' AND state = 'present' AND relative_path = ?", f.movieFile, rel).Scan(&present)
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitle_saves WHERE file_id = ?", f.movieFile).Scan(&pending)
	if present != 1 || pending != 0 {
		t.Fatalf("present rows %d, pending saves %d", present, pending)
	}
}

func TestService_SaveCutOffMidHoldDoesNotMistakeOldTargetForNewSubtitle(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)
	hi := fetchedRow(f.movieFile, "en")
	hi.HearingImpaired = true
	hiRel, err := s.save(ctx, fc, hi, fakesite.Cue("hearing impaired old", 0), "srt", DefaultSettings(), nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	plainRel, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("plain old", 0), "srt", DefaultSettings(), nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	old, err := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "file_id = ? AND state = 'present' AND relative_path = ?", f.movieFile, hiRel)
	if err != nil || len(old) != 1 {
		t.Fatalf("old rows %+v: %v", old, err)
	}
	saveHoldCrash = func(held int) error {
		if held == 1 {
			return errors.New("power cut after first hold")
		}
		return nil
	}
	t.Cleanup(func() { saveHoldCrash = nil })
	_, err = s.save(ctx, fc, fetchedRow(f.movieFile, "en"), fakesite.Cue("new", 0), "srt", DefaultSettings(), &old[0], domain.System)
	saveHoldCrash = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ rel, text string }{{hiRel, "hearing impaired old"}, {plainRel, "plain old"}} {
		got, err := os.ReadFile(f.abs(tc.rel))
		if err != nil || !strings.Contains(string(got), tc.text) {
			t.Fatalf("%s = %q, %v; want original %q", tc.rel, got, err, tc.text)
		}
	}
	var present, pending int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE file_id = ? AND place = 'beside' AND state = 'present'", f.movieFile).Scan(&present)
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitle_saves WHERE file_id = ?", f.movieFile).Scan(&pending)
	if present != 2 || pending != 0 {
		t.Fatalf("present rows %d, pending saves %d", present, pending)
	}
}

// A save error sent on Events and the live stream shows only paths inside the
// Library folder, never where that folder is on disk.
func TestService_FailedEventHidesLibraryFolder(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)
	err := &os.PathError{Op: "rename", Path: filepath.Join(fc.lib, "The Lantern (2010)", "x.en.srt"), Err: errors.New("access denied")}
	s.failed(ctx, fc, Want{Language: "en"}, "Lab", err, domain.System)
	es := eventsOf(t, f.st, EventFailed)
	if len(es) != 1 {
		t.Fatalf("events %+v", es)
	}
	msg, _ := es[0].Data["error"].(string)
	if strings.Contains(msg, fc.lib) || strings.Contains(es[0].Summary, fc.lib) || !strings.Contains(msg, "x.en.srt") {
		t.Fatalf("error %q summary %q", msg, es[0].Summary)
	}
}
