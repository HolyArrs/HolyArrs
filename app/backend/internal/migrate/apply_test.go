package migrate

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

type moveLab struct {
	st     *database.Store
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
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	// The owner, so moved asks of unknown askers have someone.
	err = st.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "users", &domain.User{Name: "owner", Role: "owner", PasswordHash: "x", State: "active", Perms: "[]"}, domain.System)
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

// hasNext says whether one of the report's next steps starts with prefix.
func hasNext(r *Report, prefix string) bool {
	for _, n := range r.Next {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
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
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE state = 'disabled' AND category = 'holyarrs' AND username = 'admin'"); n != 1 {
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
	ns, _ := mediafiles.ReadNaming(context.Background(), l.st.Read())
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
	u := l.undo(t, mv.ID)
	if u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	// Undo lines carry the names and groups the move gave them, not raw keys.
	if u.Report == nil || len(u.Report.Lines) == 0 {
		t.Fatal("undo report has no lines")
	}
	for _, ln := range u.Report.Lines {
		if ln.Name == ln.Key || strings.Contains(ln.Name, ":") && strings.HasPrefix(ln.Name, ln.Group) {
			t.Errorf("undo line shows its key: %+v", ln)
		}
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
	if ns, _ := mediafiles.ReadNaming(ctx, l.st.Read()); !ns.RenameOnImport || ns.FilePattern != mediafiles.DefaultFilePattern {
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

// TestMoveStampsReleaseGate: a moved movie takes Radarr's release stage as
// its Release gate and the availability delay as its gate delay, and its
// gate date is worked out; this server's default stays as it is.
func TestMoveStampsReleaseGate(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Titles[0].Extra = map[string]any{"minimum_availability": "inCinemas"}
		radarr.Settings = map[string]any{"availability_delay": int64(7)}
	})
	cinema := time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC)
	l.mv.Library.Metadata = &metadata.Static{Movies: []metadata.Movie{{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}, CinemaDate: &cinema}}}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); got.Result != ResAdopted {
		t.Fatalf("title %+v", got)
	}
	var gate, opens string
	var days int
	if err := l.st.Read().QueryRow(`SELECT release_gate, gate_delay_days, coalesce(gate_opens_at, '') FROM titles WHERE json_extract(source_ids, '$.tmdb') = '603'`).Scan(&gate, &days, &opens); err != nil {
		t.Fatal(err)
	}
	if gate != "in_cinemas" || days != 7 || !strings.HasPrefix(opens, "1999-04-07") {
		t.Fatalf("gate %q delay %d opens %q", gate, days, opens)
	}
	defs, err := library.ReadGateDefaults(context.Background(), l.st.Read())
	if err != nil || defs.Gate != library.GateReleased || defs.DelayDays != 0 {
		t.Fatalf("this server's default changed: %+v %v", defs, err)
	}
}

// readWait reads the protocol block of the ready-made Pick rules name.
func (l *moveLab) readWait(t *testing.T, name string) (int64, mediafiles.ProtocolRule) {
	t.Helper()
	var id int64
	if err := l.st.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ? AND preset = 1", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var p mediafiles.PickRules
	if err := database.Get(context.Background(), l.st.Read(), "pick_rules", id, &p); err != nil {
		t.Fatal(err)
	}
	return id, p.Body.Protocol
}

// setWait changes the protocol block of Pick rules id, as a person would.
func (l *moveLab) setWait(t *testing.T, id int64, w mediafiles.ProtocolRule) {
	t.Helper()
	ctx := context.Background()
	var p mediafiles.PickRules
	if err := database.Get(ctx, l.st.Read(), "pick_rules", id, &p); err != nil {
		t.Fatal(err)
	}
	p.Body.Protocol = w
	if _, err := library.UpdatePickRules(ctx, l.st, id, library.PickRulesInput{Name: p.Name, Kinds: p.Kinds, Body: p.Body}, domain.Actor{Kind: "user", ID: 1}); err != nil {
		t.Fatal(err)
	}
}

// TestMoveDefaultWait: the dry run shows the wait change, the move sets it
// on the ready-made Pick rules (once), and undo puts the old wait back
// unless it was changed since; Pick rules whose wait was already set keep it.
func TestMoveDefaultWait(t *testing.T) {
	wait := func(radarr, _, _ *Snapshot) {
		radarr.DelayProfiles = []DelayProfile{{ID: 1, EnableUsenet: true, EnableTorrent: true, PreferredProtocol: "usenet", TorrentDelay: 120, Order: 2147483647}}
	}
	moved := mediafiles.ProtocolRule{Prefer: "usenet", WaitMinutesTorrent: 120}
	key := "pick_rules_protocol:4K"
	l := newMoveLabWith(t, wait)
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, key); got.Result != ResChange || got.Detail != "4K: prefer usenet, Usenet 0 min, torrent 120 min; used by 0 versions here now" {
		t.Fatalf("dry run %+v", got)
	}
	id, w := l.readWait(t, "4K")
	if w != seededWait {
		t.Fatalf("the dry run changed the wait: %+v", w)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, key); got.Result != ResChanged {
		t.Fatalf("move %+v", got)
	}
	if _, w = l.readWait(t, "4K"); w != moved {
		t.Fatalf("wait after the move %+v", w)
	}
	// The other ready-made rules keep theirs.
	if _, w = l.readWait(t, "Standard"); w != seededWait {
		t.Fatalf("Standard changed %+v", w)
	}
	// Moving again changes nothing.
	again := l.move(t, Choices{})
	if got := lineOf(t, again.Report, key); got.Result != ResAlready {
		t.Fatalf("second move %+v", got)
	}
	if u := l.undo(t, mv.ID); u.State != "undone" || lineOf(t, u.Report, key).Result != ResRemoved {
		t.Fatalf("undo %+v", u)
	}
	if _, w = l.readWait(t, "4K"); w != seededWait {
		t.Fatalf("wait after undo %+v", w)
	}

	// Changed since the move: undo keeps the person's wait.
	l = newMoveLabWith(t, wait)
	mv = l.move(t, Choices{})
	id, _ = l.readWait(t, "4K")
	mine := mediafiles.ProtocolRule{Prefer: "torrent", WaitMinutesUsenet: 30}
	l.setWait(t, id, mine)
	if u := l.undo(t, mv.ID); lineOf(t, u.Report, key).Result != ResKept || lineOf(t, u.Report, key).Detail != keptChanged {
		t.Fatalf("undo after a change %+v", lineOf(t, u.Report, key))
	}
	if _, w = l.readWait(t, "4K"); w != mine {
		t.Fatalf("undo changed the person's wait %+v", w)
	}

	// A wait already set here stays, in the dry run and the move.
	l = newMoveLabWith(t, wait)
	id, _ = l.readWait(t, "4K")
	l.setWait(t, id, mine)
	dry = l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, key); got.Result != ResKept {
		t.Fatalf("dry run on a set wait %+v", got)
	}
	mv = l.move(t, Choices{})
	if got := lineOf(t, mv.Report, key); got.Result != ResKept || got.Detail != "this server's Pick rules already set a wait, so it stays" {
		t.Fatalf("move on a set wait %+v", got)
	}
	if _, w = l.readWait(t, "4K"); w != mine {
		t.Fatalf("the move changed a set wait %+v", w)
	}

	// With Movies off, a wait only Radarr set is left out.
	l = newMoveLabWith(t, wait)
	l.parts["movies"] = true
	if got := lineOf(t, l.move(t, Choices{}).Report, key); got.Result != ResSkipped {
		t.Fatalf("movies off %+v", got)
	}
	if _, w = l.readWait(t, "4K"); w != seededWait {
		t.Fatalf("movies off changed the wait %+v", w)
	}
}

// TestMoveOutsideLists: Radarr lists come over paused, with their folder
// and Pick rules, once; a name taken here gets " (radarr)", or " (radarr 2)"
// when that is taken too; undo removes a
// list only while it is still paused.
func TestMoveOutsideLists(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.ImportLists = []Provider{
			{Name: "Top films", Kind: "TMDbListImport", Enabled: true, Settings: map[string]any{"listId": "8136", "link.rootfolderpath": "/movies",
				"link.qualityprofileid": int64(2), "link.searchonadd": true, "link.minimumavailability": "released"}},
			{Name: "Watch", Kind: "TraktUserImport", Enabled: true, Settings: map[string]any{"traktListType": int64(0), "username": "sean", "link.rootfolderpath": "/movies"}},
		}
	})
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	// A list here already has the first one's name.
	if _, err := library.AddFolder(ctx, l.st, library.FolderInput{Path: l.movies, Kind: "movie"}, nil, by); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"Top films", "Top films (radarr)"} {
		if _, err := requests.AddOutsideList(ctx, l.st, requests.OutsideListInput{Name: name, Kind: requests.ListTMDB, Ref: strconv.Itoa(i + 1)}, by); err != nil {
			t.Fatal(err)
		}
	}
	dry := l.mv.Run(ctx, 0, l.plan(t), true, by, nil)
	for _, k := range []string{"outside_list:tmdb_list:8136", "outside_list:trakt_list:sean/watchlist"} {
		if got := lineOf(t, dry, k); got.Result != ResCreate || !strings.Contains(got.Detail, "paused until you check it and turn it on") || !strings.Contains(got.Detail, " list ") {
			t.Fatalf("dry run %s %+v", k, got)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM outside_lists"); n != 2 {
		t.Fatalf("the dry run added lists: %d", n)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "outside_list:tmdb_list:8136"); got.Result != ResCreated {
		t.Fatalf("move %+v", got)
	}
	var on, search bool
	var gate, rules string
	if err := l.st.Read().QueryRow(`SELECT o.enabled, o.search_on_add, o.release_gate, p.name FROM outside_lists o JOIN pick_rules p ON p.id = o.pick_rules_id
		WHERE o.ref = '8136' AND o.name = 'Top films (radarr 2)'`).Scan(&on, &search, &gate, &rules); err != nil {
		t.Fatal(err)
	}
	if on || !search || gate != "released" || rules != "4K" {
		t.Fatalf("tmdb list on %v search %v gate %q rules %q", on, search, gate, rules)
	}
	if n := l.count(t, "SELECT count(*) FROM outside_lists WHERE kind = 'trakt_list' AND ref = 'sean/watchlist' AND enabled = 0"); n != 1 {
		t.Fatal("the watchlist did not come over paused")
	}
	next := strings.Join(mv.Report.Next, "\n")
	if !strings.Contains(next, "Outside lists come over paused") || !strings.Contains(next, "Add your Trakt key under Settings > Outside lists") {
		t.Fatalf("next %v", mv.Report.Next)
	}
	again := l.move(t, Choices{})
	if got := lineOf(t, again.Report, "outside_list:trakt_list:sean/watchlist"); got.Result != ResAlready {
		t.Fatalf("second move %+v", got)
	}
	// Turned on since: it stays; the paused one goes; the one made here stays.
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE outside_lists SET enabled = 1 WHERE ref = 'sean/watchlist'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	u := l.undo(t, mv.ID)
	if got := lineOf(t, u.Report, "outside_list:tmdb_list:8136"); got.Result != ResRemoved {
		t.Fatalf("undo paused list %+v", got)
	}
	if got := lineOf(t, u.Report, "outside_list:trakt_list:sean/watchlist"); got.Result != ResKept || !strings.Contains(got.Detail, "you turned it on since") {
		t.Fatalf("undo turned-on list %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM outside_lists"); n != 3 {
		t.Fatalf("lists after undo %d", n)
	}
	// With Movies off, no list comes over.
	l2 := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.ImportLists = []Provider{{Name: "Top films", Kind: "TMDbListImport", Settings: map[string]any{"listId": "8136", "link.rootfolderpath": "/movies"}}}
	})
	l2.parts["movies"] = true
	if got := lineOf(t, l2.move(t, Choices{}).Report, "outside_list:tmdb_list:8136"); got.Result != ResSkipped {
		t.Fatalf("movies off %+v", got)
	}
}

// A finished move forgets every arr key kept for "Read again".
func TestFinishedMoveForgetsKeptKeys(t *testing.T) {
	l := newMoveLab(t)
	err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
		for i, kind := range []string{"radarr", "qbittorrent"} {
			id := int64(9000 + i)
			if _, err := tx.Exec("INSERT INTO secrets (id, created_at, created_by, updated_at, updated_by, kind, value) VALUES (?, '', '', '', '', 'move_credential', 'sealed')", id); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO move_credentials (kind, secret_id, created_at, created_by) VALUES (?, ?, '', '')", kind, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move %s", mv.State)
	}
	if n := l.count(t, "SELECT count(*) FROM move_credentials"); n != 0 {
		t.Fatalf("%d keys kept after the move", n)
	}
	if n := l.count(t, "SELECT count(*) FROM secrets WHERE kind = 'move_credential'"); n != 0 {
		t.Fatalf("%d vault rows kept after the move", n)
	}
}

// A move with failed items (say the metadata server was down) keeps the
// old apps' keys so "move again" needs no retyping; a clean move forgets them.
func TestMoveWithFailuresKeepsKeys(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO secrets (id, created_at, created_by, updated_at, updated_by, kind, value) VALUES (9100, '', '', '', '', 'move_credential', 'sealed')"); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO move_credentials (kind, secret_id, created_at, created_by) VALUES ('sonarr', 9100, '', '')")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	// Pretend the move had failed items: the keys must stay.
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO secrets (id, created_at, created_by, updated_at, updated_by, kind, value) VALUES (9101, '', '', '', '', 'move_credential', 'sealed')"); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO move_credentials (kind, secret_id, created_at, created_by) VALUES ('radarr', 9101, '', '')")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	failed := &Report{Counts: map[string]int{ResFailed: 20}}
	if err := l.mv.setState(ctx, mv.ID, "done", failed); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, "SELECT count(*) FROM move_credentials"); n != 1 {
		t.Fatalf("keys after a move with failures: %d, want 1", n)
	}
	if !strings.Contains(strings.Join(next(failed), " "), "keys are kept") {
		t.Fatalf("next steps don't say the keys are kept: %v", next(failed))
	}
	if err := l.mv.setState(ctx, mv.ID, "done", &Report{Counts: map[string]int{}}); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, "SELECT count(*) FROM move_credentials"); n != 0 {
		t.Fatalf("keys after a clean move: %d", n)
	}
}
