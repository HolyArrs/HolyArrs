package migrate

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/subtitles"
)

type moveLab struct {
	st     *store.Store
	q      *jobs.Queue
	mv     *Mover
	movies string // the movies folder as this server sees it
	file   string // the one media file
	snaps  []int64
	rules  []PathRule
	parts  map[string]bool
	people map[string]string // the owner's per-person choices
}

func newMoveLab(t *testing.T) *moveLab { return newMoveLabWith(t, nil) }

// newMoveLabWith lets a test change the three Snapshots before they are kept.
func newMoveLabWith(t *testing.T, change func(radarr, bazarr, seerr *Snapshot)) *moveLab {
	t.Helper()
	st, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	// The owner, so moved asks of unknown askers have someone.
	err = st.Write(ctx, func(tx *sql.Tx) error {
		_, err := store.Insert(tx, "users", &domain.User{Name: "owner", Role: "owner", PasswordHash: "x", State: "active", Perms: "[]"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	l := &moveLab{st: st, movies: filepath.Join(root, "films"), parts: map[string]bool{}, people: map[string]string{"person:seerr:1": "owner"}}
	l.file = filepath.Join(l.movies, "The Matrix (1999)", "The.Matrix.1999.1080p.mkv")
	if err := os.MkdirAll(filepath.Dir(l.file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.file, []byte("not really a movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := &metadata.Static{Movies: []metadata.Movie{{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}}}}
	l.q = jobs.New(st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	lib := &library.Service{Store: st, Queue: l.q, Metadata: md}
	l.mv = &Mover{Store: st, Library: lib, Queue: l.q, PartOn: func(p string) bool { return !l.parts[p] }}
	l.mv.Register(l.q)
	l.rules = []PathRule{{From: "/movies", To: filepath.ToSlash(l.movies)}}

	radarr := &Snapshot{Kind: Radarr, Version: "6.4.4",
		Folders:         []Folder{{Path: "/movies"}},
		Titles:          []Title{{SourceID: 1, Kind: "movie", Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"}, Path: "/movies/The Matrix (1999)", Monitored: false, ProfileID: 2}},
		QualityProfiles: []Profile{{ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}},
		Exclusions:      []Exclusion{{IDs: map[string]string{"tmdb": "999"}, Name: "Bad Movie"}},
		Indexers:        []Provider{{Name: "Nyaa", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/1/"}, NeedsReentry: []string{"apiKey"}}},
		DownloadClients: []Provider{{Name: "qBit", Kind: "QBittorrent", Enabled: true, Priority: 1, Settings: map[string]any{"host": "qbit", "port": int64(8080), "username": "admin"}, NeedsReentry: []string{"password"}}},
		Notifications:   []Provider{{Name: "Discord", Kind: "Discord", Enabled: true, NeedsReentry: []string{"webHookUrl"}}},
		Naming:          map[string]any{"renameMovies": false, "standardMovieFormat": "{Movie Title} ({Release Year}) {Quality Full}"},
	}
	bazarr := &Snapshot{Kind: Bazarr, Subtitles: &SubtitleSetup{Languages: []string{"en", "es"}, Providers: []Provider{{Name: "podnapisi", Kind: "podnapisi"}}}}
	seerr := &Snapshot{Kind: Seerr,
		Users:    []User{{SourceID: 1, Name: "luis", Admin: true, Owner: true}, {SourceID: 2, Name: "Ana", Permissions: []string{"REQUEST"}, MovieLimit: 3, MovieDays: 7}},
		Requests: []Request{{SourceID: 7, Kind: "movie", IDs: map[string]string{"tmdb": "604"}, State: "pending", By: 2, ByName: "Ana", Created: "2026-09-01T10:00:00Z"}},
	}
	if change != nil {
		change(radarr, bazarr, seerr)
	}
	for _, s := range []*Snapshot{radarr, bazarr, seerr} {
		s.Via = ViaAPI
		s.finish()
		saved, err := Save(ctx, st, 1, s)
		if err != nil {
			t.Fatal(err)
		}
		l.snaps = append(l.snaps, saved.ID)
	}
	return l
}

func (l *moveLab) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := l.st.Read().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func (l *moveLab) plan(t *testing.T) *Plan {
	t.Helper()
	p, err := LoadPlan(context.Background(), l.st, l.snaps, Choices{PathRules: l.rules, People: l.people})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// move starts a move and waits for its Job.
func (l *moveLab) move(t *testing.T, ch Choices) Move {
	t.Helper()
	ctx := context.Background()
	if ch.PathRules == nil {
		ch.PathRules = l.rules
	}
	if ch.People == nil {
		ch.People = l.people
	}
	p, err := LoadPlan(ctx, l.st, l.snaps, ch)
	if err != nil {
		t.Fatal(err)
	}
	mv, err := l.mv.Start(ctx, l.snaps, ch, PlanHash(p), domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer l.q.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := GetMove(ctx, l.st, mv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == "done" || got.State == "failed" {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the move never finished")
	return Move{}
}

// undo queues the undo Job and waits for it.
func (l *moveLab) undo(t *testing.T, id int64) Move {
	t.Helper()
	ctx := context.Background()
	if _, err := l.mv.StartUndo(ctx, id, domain.Actor{Kind: "user", ID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := l.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer l.q.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := GetMove(ctx, l.st, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == "undone" || got.UndoState == "failed" {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the undo never finished")
	return Move{}
}

func lineOf(t *testing.T, r *Report, key string) Line {
	t.Helper()
	for _, l := range r.Lines {
		if l.Key == key {
			return l
		}
	}
	t.Fatalf("no line %q in %+v", key, r.Lines)
	return Line{}
}

func TestDryRunChangesNothing(t *testing.T) {
	l := newMoveLab(t)
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	for key, want := range map[string]string{
		"folder:" + filepath.ToSlash(l.movies): ResCreate, "title:movie:tmdb:603": ResAdopt, "source:nyaa": ResCreate,
		"downloader:http://qbit:8080": ResCreate, "alert:discord": ResCreate, "naming:radarr": ResChange, "subtitles:languages": ResChange,
		"subtitle_site:podnapisi": ResCreate, "person:seerr:1": ResMatched, "person:seerr:2": ResCreate, "ask:seerr:7": ResCreate,
		"never_add:movie:tmdb:999": ResCreate, "pick_rules:1:radarr:2": ResMatched,
	} {
		if got := lineOf(t, rep, key); got.Result != want {
			t.Errorf("%s: %s (%s), want %s", key, got.Result, got.Detail, want)
		}
	}
	if len(rep.NeedsInput) == 0 || len(rep.Next) == 0 {
		t.Fatalf("needs %v next %v", rep.NeedsInput, rep.Next)
	}
	for _, table := range []string{"titles", "library_folders", "sources", "downloaders", "alerts", "asks", "never_add", "subtitle_sites", "move_made"} {
		if n := l.count(t, "SELECT count(*) FROM "+table); n != 0 {
			t.Errorf("dry run wrote %d rows to %s", n, table)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM users"); n != 1 {
		t.Errorf("dry run made people: %d", n)
	}
	// Without the path rule the folder is not found here, and says why.
	l.rules = nil
	rep = l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, "folder:/movies"); got.Result != ResNotFound || !strings.Contains(got.Detail, "path rule") {
		t.Fatalf("folder %+v", got)
	}
}

func TestMoveAppliesOnceAndUndoes(t *testing.T) {
	l := newMoveLab(t)
	before, err := os.Stat(l.file)
	if err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" || mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	rep := mv.Report
	if got := lineOf(t, rep, "title:movie:tmdb:603"); got.Result != ResAdopted {
		t.Fatalf("title %+v", got)
	}
	// The title is adopted in place, Keep up to date off, on the 4K preset.
	var keep bool
	var rules string
	if err := l.st.Read().QueryRow(`SELECT t.keep_up_to_date, p.name FROM titles t JOIN versions v ON v.title_id = t.id JOIN pick_rules p ON p.id = v.pick_rules_id
		WHERE json_extract(t.source_ids, '$.tmdb') = '603'`).Scan(&keep, &rules); err != nil {
		t.Fatal(err)
	}
	if keep || rules != "4K" {
		t.Fatalf("keep %v rules %s", keep, rules)
	}
	if n := l.count(t, "SELECT count(*) FROM files WHERE origin = 'adopted'"); n != 1 {
		t.Fatalf("adopted files %d", n)
	}
	after, err := os.Stat(l.file)
	if err != nil || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("the media file changed: %v", err)
	}
	// Paused things wait for their secret; people are turned-off placeholders.
	if n := l.count(t, "SELECT count(*) FROM sources WHERE name = 'Nyaa' AND state = 'disabled' AND secret_ref IS NULL"); n != 1 {
		t.Fatal("source not paused")
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE state = 'disabled' AND category = 'holyarrs' AND username = ''"); n != 1 {
		t.Fatal("downloader not paused")
	}
	if n := l.count(t, "SELECT count(*) FROM alerts WHERE enabled = 0 AND kind = 'discord'"); n != 1 {
		t.Fatal("alert not off")
	}
	var ana int64
	if err := l.st.Read().QueryRow("SELECT id FROM users WHERE name = 'Ana' AND state = 'disabled' AND password_hash = '' AND role = 'member'").Scan(&ana); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, "SELECT count(*) FROM user_ask_settings WHERE user_id = ? AND per_week IS NULL AND movies_per_week = 3", ana); n != 1 {
		t.Fatal("limit not moved")
	}
	if n := l.count(t, "SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id = a.id WHERE a.asked_by = ? AND a.state = 'asked' AND p.counted = 0", ana); n != 1 {
		t.Fatal("ask not moved to Ana")
	}
	if n := l.count(t, "SELECT count(*) FROM never_add WHERE json_extract(source_ids, '$.tmdb') = '999'"); n != 1 {
		t.Fatal("never add")
	}
	ns, _ := naming.Read(context.Background(), l.st.Read())
	if ns.RenameOnImport || ns.FilePattern != "{title} ({year}) {quality}" {
		t.Fatalf("naming %+v", ns)
	}
	if langs := subtitles.ReadSettings(context.Background(), l.st.Read()).Languages; len(langs) != 2 || langs[1].Language != "es" {
		t.Fatalf("subtitle languages %+v", langs)
	}
	if !strings.Contains(rep.Markdown(), "The Matrix") {
		t.Fatal("report text")
	}

	// A second move adds nothing twice.
	counts := map[string]int{}
	tables := []string{"titles", "library_folders", "sources", "downloaders", "alerts", "asks", "never_add", "subtitle_sites", "users"}
	for _, tb := range tables {
		counts[tb] = l.count(t, "SELECT count(*) FROM "+tb)
	}
	again := l.move(t, Choices{})
	for _, tb := range tables {
		if n := l.count(t, "SELECT count(*) FROM "+tb); n != counts[tb] {
			t.Errorf("second move: %s went from %d to %d", tb, counts[tb], n)
		}
	}
	if again.Report.Counts[ResCreated]+again.Report.Counts[ResAdopted]+again.Report.Counts[ResChanged] != 0 {
		t.Fatalf("second move made things: %+v", again.Report.Counts)
	}

	// Undo removes what the first move made, never the file.
	ctx := context.Background()
	if u := l.undo(t, mv.ID); u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	if _, err := os.Stat(l.file); err != nil {
		t.Fatalf("undo touched the media file: %v", err)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed'"); n != 0 {
		t.Fatal("title still in the Library")
	}
	for _, tb := range []string{"sources", "downloaders", "alerts", "asks", "never_add", "subtitle_sites"} {
		if n := l.count(t, "SELECT count(*) FROM "+tb); n != 0 {
			t.Errorf("undo left %d rows in %s", n, tb)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM users"); n != 1 {
		t.Fatalf("undo left people: %d", n)
	}
	if ns, _ := naming.Read(ctx, l.st.Read()); !ns.RenameOnImport || ns.FilePattern != naming.DefaultFilePattern {
		t.Fatalf("naming not back: %+v", ns)
	}
	if _, err := l.mv.StartUndo(ctx, mv.ID, domain.Actor{Kind: "user", ID: 1}); err != ErrUndone {
		t.Fatalf("undo twice: %v", err)
	}
	// After the undo a move brings the title back.
	third := l.move(t, Choices{})
	if got := lineOf(t, third.Report, "title:movie:tmdb:603"); got.Result != ResAdopted {
		t.Fatalf("after undo %+v", got)
	}
}

func TestMoveRespectsPartsAndSkips(t *testing.T) {
	l := newMoveLab(t)
	l.parts["movies"] = true
	l.parts["subtitles"] = true
	mv := l.move(t, Choices{Skip: []string{GroupAlerts}})
	for key, want := range map[string]string{"title:movie:tmdb:603": ResSkipped, "subtitles:languages": ResSkipped, "alert:discord": ResSkipped,
		"ask:seerr:7": ResSkipped, "source:nyaa": ResCreated} {
		if got := lineOf(t, mv.Report, key); got.Result != want {
			t.Errorf("%s: %s, want %s", key, got.Result, want)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM titles"); n != 0 {
		t.Fatal("movies part off, yet a movie came over")
	}
}

func TestStartRefusesWhileBusyAndBadInput(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	hash := func(ch Choices) string {
		p, err := LoadPlan(ctx, l.st, l.snaps, ch)
		if err != nil {
			return ""
		}
		return PlanHash(p)
	}
	if _, err := l.mv.Start(ctx, nil, Choices{}, "x", by); err != ErrNoSnapshotPicked {
		t.Fatalf("no snapshots: %v", err)
	}
	if _, err := l.mv.Start(ctx, l.snaps, Choices{PathRules: []PathRule{{From: "/a/../b", To: "/c"}}}, "x", by); err == nil {
		t.Fatal("a climbing path rule was taken")
	}
	if _, err := l.mv.Start(ctx, []int64{999}, Choices{}, "x", by); err != ErrNoSnapshot {
		t.Fatalf("missing snapshot: %v", err)
	}
	if _, err := l.mv.Start(ctx, l.snaps, Choices{}, "", by); err != ErrPlanChanged {
		t.Fatalf("no plan hash: %v", err)
	}
	if _, err := l.mv.Start(ctx, l.snaps, Choices{}, hash(Choices{Skip: []string{GroupAlerts}}), by); err != ErrPlanChanged {
		t.Fatalf("hash of another plan: %v", err)
	}
	if _, err := l.mv.Start(ctx, l.snaps, Choices{}, hash(Choices{}), by); err != nil {
		t.Fatal(err)
	}
	if _, err := l.mv.Start(ctx, l.snaps, Choices{}, hash(Choices{}), by); err != ErrMoveBusy {
		t.Fatalf("second while queued: %v", err)
	}
	ms, err := ListMoves(ctx, l.st)
	if err != nil || len(ms) != 1 {
		t.Fatalf("moves %+v %v", ms, err)
	}
	if _, err := l.mv.StartUndo(ctx, ms[0].ID, by); err != ErrMoveBusy {
		t.Fatalf("undo while queued: %v", err)
	}
	if _, err := GetMove(ctx, l.st, 999); err != ErrNoMove {
		t.Fatalf("missing move: %v", err)
	}
}
