package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// sendLib is a Library with one show (main and 4K Versions, three
// episodes) and one movie, a Library folder per kind, and a tag on each.
type sendLib struct {
	show, movie      int64
	mainVer, wideVer int64
	movieVer         int64
	slots            map[string]int64 // "e1", "e2", "e3", "4k1", "movie"
	tag              int64
}

func (h *hookRig) exec(q string, args ...any) int64 {
	h.t.Helper()
	var id int64
	if err := h.st.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(q, args...)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	}); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *hookRig) library() sendLib {
	h.t.Helper()
	l := sendLib{slots: map[string]int64{}}
	var rules int64
	_ = h.st.Read().QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rules)
	stamp := pinStamp
	showDir, movieDir := h.t.TempDir(), h.t.TempDir()
	sf := h.exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, '{}', ?, '{}', ?, 'show', 'Shows', 'ok', '[]')`, stamp, stamp, showDir)
	mf := h.exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, '{}', ?, '{}', ?, 'movie', 'Movies', 'ok', '[]')`, stamp, stamp, movieDir)
	l.show = h.exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, keep_up_to_date)
		VALUES (?, '{}', ?, '{}', 'show', 'Lab Show', 'lab show', 2025, '{"tmdb":"100","tvdb":"200"}', 'tmdb', 'import', 'active', 1)`, stamp, stamp)
	part := h.exec(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, number, name, keep_up_to_date) VALUES (?, '{}', ?, '{}', ?, 'season', 1, 'Season 1', 1)`, stamp, stamp, l.show)
	l.mainVer = h.exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
		VALUES (?, '{}', ?, '{}', ?, 'Main', ?, ?, 'Lab Show', 1)`, stamp, stamp, l.show, rules, sf)
	l.wideVer = h.exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
		VALUES (?, '{}', ?, '{}', ?, '4K', ?, ?, 'Lab Show 4K', 0)`, stamp, stamp, l.show, rules, sf)
	for e := 1; e <= 3; e++ {
		it := h.exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind, numbers, name, air_date, keep_up_to_date)
			VALUES (?, '{}', ?, '{}', ?, ?, 'episode', ?, ?, '2025-01-06T02:00:00Z', 1)`, stamp, stamp, l.show, part, fmt.Sprintf(`{"season":1,"episode":%d}`, e),
			map[int]any{1: "Pilot", 2: nil, 3: "Third"}[e])
		l.slots[fmt.Sprintf("e%d", e)] = h.exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'wanted')`, stamp, stamp, it, l.mainVer)
		if e == 1 {
			l.slots["4k1"] = h.exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'wanted')`, stamp, stamp, it, l.wideVer)
		}
	}
	l.movie = h.exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, keep_up_to_date)
		VALUES (?, '{}', ?, '{}', 'movie', 'Lab Movie', 'lab movie', 2024, '{"tmdb":"693134"}', 'tmdb', 'import', 'active', 1)`, stamp, stamp)
	l.movieVer = h.exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
		VALUES (?, '{}', ?, '{}', ?, 'Main', ?, ?, 'Lab Movie (2024)', 1)`, stamp, stamp, l.movie, rules, mf)
	mi := h.exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind) VALUES (?, '{}', ?, '{}', ?, 'movie')`, stamp, stamp, l.movie)
	l.slots["movie"] = h.exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'wanted')`, stamp, stamp, mi, l.movieVer)
	l.tag = h.exec(`INSERT INTO compat_tags (label, created_at) VALUES ('alice', ?)`, stamp)
	h.exec("INSERT INTO compat_title_tags (tag_id, title_id) VALUES (?, ?), (?, ?)", l.tag, l.show, l.tag, l.movie)
	return l
}

// placed records an import: one plan step per file, each placed or not and
// replacing an older copy or not, with each placed file put on its Slots.
type placeStep struct {
	slots    []int64
	version  int64
	path     string
	placed   bool
	replaces bool
}

func (h *hookRig) placed(steps ...placeStep) int64 {
	h.t.Helper()
	imp := &mediafiles.Import{Kind: "episodes", Mode: "auto", State: "done"}
	for _, s := range steps {
		st := mediafiles.PlanStep{TargetSlotIDs: s.slots, TargetPath: s.path, Method: "hardlink", Why: []string{}}
		res := mediafiles.PlaceResult{Outcome: "failed"}
		if s.replaces {
			old := int64(1)
			st.ReplacesFileID = &old
		}
		if s.placed {
			res.Outcome = "placed"
			f := &mediafiles.File{SlotIDs: s.slots, VersionID: s.version, RelativePath: s.path, Size: 5 << 20, AddedAt: time.Now(),
				Quality: mediafiles.Quality{Resolution: 1080, Source: "web_dl"}, Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported",
				PlacedBy: "hardlink", Role: "main", State: "present"}
			if err := h.st.Write(context.Background(), func(tx *sql.Tx) error {
				id, err := database.Insert(tx, "files", f, domain.System)
				if err != nil {
					return err
				}
				for _, sl := range s.slots {
					if _, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", id, sl); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				h.t.Fatal(err)
			}
		}
		imp.Plan, imp.Result = append(imp.Plan, st), append(imp.Result, res)
	}
	var id int64
	if err := h.st.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = database.Insert(tx, "imports", imp, domain.System)
		return err
	}); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *hookRig) addHook(side, name, url, secret string, change func(m map[string]any)) int64 {
	h.t.Helper()
	var m map[string]any
	_ = json.Unmarshal([]byte(pulsarrHook(0, name, url, secret)), &m)
	if change != nil {
		change(m)
	}
	b, _ := json.Marshal(m)
	st, body := h.do(side, "POST", "notification", string(b))
	if st != 201 {
		h.t.Fatalf("add hook: %d %s", st, body)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(body), &out)
	return int64(out["id"].(float64))
}

// importEvent stores an import Event and waits for the hook calls it makes.
func (h *hookRig) importEvent(s *hookSender, title, importID int64) {
	h.t.Helper()
	if err := h.st.AddEvent(context.Background(), &domain.Event{Kind: "import_done", TitleID: &title, Data: map[string]any{"import_id": importID}}); err != nil {
		h.t.Fatal(err)
	}
	s.wait()
}

func (h *hookRig) sender(allowed bool) *hookSender {
	s := &hookSender{store: h.st, vault: h.vault, backoff: []time.Duration{time.Millisecond, time.Millisecond},
		allowed: func(context.Context, string) bool { return allowed }}
	s.watch(context.Background())
	return s
}

// One episode file imported sends one import event, shaped as Pulsarr
// reads it, with the hook's secret header. An upgrade sends nothing, a
// file of another Version sends nothing, and a step that was not placed
// sends nothing.
func TestHookEvents_ShowImport(t *testing.T) {
	h := newHookRig(t)
	l := h.library()
	rc := newReceiver(t)
	h.addHook("sonarr", "pulsarr", rc.srv.URL+"/v1/notifications/webhook?instanceId=x", "s3cret", nil)
	tests := len(rc.got())
	s := h.sender(true)
	imp := h.placed(
		placeStep{slots: []int64{l.slots["e1"]}, version: l.mainVer, path: "Lab Show/Season 01/Lab.Show.S01E01.mkv", placed: true},
		placeStep{slots: []int64{l.slots["e2"], l.slots["e3"]}, version: l.mainVer, path: "Lab Show/Season 01/Lab.Show.S01E02E03.mkv", placed: true, replaces: true},
		placeStep{slots: []int64{l.slots["e3"]}, version: l.mainVer, path: "Lab Show/Season 01/x.mkv"},
	)
	h.importEvent(s, l.show, imp)
	calls := rc.got()[tests:]
	if len(calls) != 1 {
		t.Fatalf("want one import event, got %d: %+v", len(calls), calls)
	}
	c := calls[0]
	if c.method != "POST" || c.uri != "/v1/notifications/webhook?instanceId=x" || c.header.Get("X-Pulsarr-Secret") != "s3cret" || c.header.Get("Content-Type") != "application/json" {
		t.Fatalf("call: %s %s %v", c.method, c.uri, c.header)
	}
	b := c.body
	if b["eventType"] != "Download" || b["instanceName"] != AppName || b["isUpgrade"] != false || b["movie"] != nil {
		t.Fatalf("event: %v", b)
	}
	se := b["series"].(map[string]any)
	if se["id"].(float64) != float64(l.show) || se["title"] != "Lab Show" || se["tvdbId"].(float64) != 200 || fmt.Sprint(se["tags"]) != "[alice]" {
		t.Fatalf("series: %v", se)
	}
	eps := b["episodes"].([]any)
	if len(eps) != 1 {
		t.Fatalf("episodes: %v", eps)
	}
	ep := eps[0].(map[string]any)
	if ep["seasonNumber"].(float64) != 1 || ep["episodeNumber"].(float64) != 1 || ep["title"] != "Pilot" || ep["airDateUtc"] != "2025-01-06T02:00:00Z" {
		t.Fatalf("episode: %v", ep)
	}
	f := b["episodeFile"].(map[string]any)
	if f["relativePath"] != "Season 01/Lab.Show.S01E01.mkv" || f["quality"] != "WEBDL-1080p" || f["qualityVersion"].(float64) != 1 || f["size"].(float64) != 5<<20 || f["id"].(float64) <= 0 {
		t.Fatalf("file: %v", f)
	}
	// A file for the 4K Version is not the title's main copy.
	h.importEvent(s, l.show, h.placed(placeStep{slots: []int64{l.slots["4k1"]}, version: l.wideVer, path: "Lab Show 4K/Season 01/e1.mkv", placed: true}))
	if n := len(rc.got()) - tests; n != 1 {
		t.Fatalf("4K import sent %d events", n-1)
	}
}

// With the upgrade switch on too, an upgrade is sent, marked as one, with
// every episode its file holds; an episode with no name gets empty text.
func TestHookEvents_UpgradeSwitch(t *testing.T) {
	h := newHookRig(t)
	l := h.library()
	rc := newReceiver(t)
	h.addHook("sonarr", "pulsarr", rc.srv.URL+"/hook", "s", func(m map[string]any) { m["onUpgrade"] = true })
	tests := len(rc.got())
	s := h.sender(true)
	h.importEvent(s, l.show, h.placed(placeStep{slots: []int64{l.slots["e2"], l.slots["e3"]}, version: l.mainVer, path: "Lab Show/Season 01/e2e3.mkv", placed: true, replaces: true}))
	calls := rc.got()[tests:]
	if len(calls) != 1 || calls[0].body["isUpgrade"] != true {
		t.Fatalf("upgrade: %+v", calls)
	}
	eps := calls[0].body["episodes"].([]any)
	if len(eps) != 2 || eps[0].(map[string]any)["episodeNumber"].(float64) != 2 || eps[0].(map[string]any)["title"] != "" {
		t.Fatalf("episodes: %v", eps)
	}
	if title, ok := eps[0].(map[string]any)["title"]; !ok || title != "" {
		t.Fatalf("an episode with no name must carry empty text: %v", eps[0])
	}
}

// A movie import goes to the Radarr side's hooks only, as a movie object.
func TestHookEvents_MovieImport(t *testing.T) {
	h := newHookRig(t)
	l := h.library()
	son, rad := newReceiver(t), newReceiver(t)
	h.addHook("sonarr", "pulsarr", son.srv.URL+"/hook", "s", nil)
	h.addHook("radarr", "pulsarr", rad.srv.URL+"/hook?instanceId=r", "m0vie", nil)
	sonTests, radTests := len(son.got()), len(rad.got())
	s := h.sender(true)
	h.importEvent(s, l.movie, h.placed(placeStep{slots: []int64{l.slots["movie"]}, version: l.movieVer, path: "Lab Movie (2024)/Lab Movie (2024).mkv", placed: true}))
	if n := len(son.got()) - sonTests; n != 0 {
		t.Fatalf("the Sonarr side got %d movie events", n)
	}
	calls := rad.got()[radTests:]
	if len(calls) != 1 || calls[0].header.Get("X-Pulsarr-Secret") != "m0vie" {
		t.Fatalf("radarr calls: %+v", calls)
	}
	b := calls[0].body
	mv := b["movie"].(map[string]any)
	if b["eventType"] != "Download" || b["instanceName"] != AppName || mv["title"] != "Lab Movie" || mv["tmdbId"].(float64) != 693134 ||
		mv["id"].(float64) != float64(l.movie) || fmt.Sprint(mv["tags"]) != "[alice]" || b["series"] != nil {
		t.Fatalf("movie event: %v", b)
	}
}

// Nothing is sent while the side may not send (the arr API is off or the
// side has no key), to a hook with the import switch off, to a hook limited
// to tags the title does not carry, or for an Event that is not an import.
func TestHookEvents_NotSent(t *testing.T) {
	h := newHookRig(t)
	l := h.library()
	rc := newReceiver(t)
	h.addHook("sonarr", "off", rc.srv.URL+"/off", "s", func(m map[string]any) { m["onDownload"] = false })
	other := h.exec(`INSERT INTO compat_tags (label, created_at) VALUES ('bob', ?)`, pinStamp)
	h.addHook("sonarr", "bob only", rc.srv.URL+"/bob", "s", func(m map[string]any) { m["tags"] = []any{fmt.Sprint(other)} })
	tests := len(rc.got())
	s := h.sender(true)
	h.importEvent(s, l.show, h.placed(placeStep{slots: []int64{l.slots["e1"]}, version: l.mainVer, path: "Lab Show/Season 01/e1.mkv", placed: true}))
	if err := h.st.AddEvent(context.Background(), &domain.Event{Kind: "download_sent", TitleID: &l.show, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	s.wait()
	if n := len(rc.got()) - tests; n != 0 {
		t.Fatalf("%d calls that should not go out: %+v", n, rc.got()[tests:])
	}
	// With the side not allowed to send, a hook that is on stays quiet.
	h2 := newHookRig(t)
	l2 := h2.library()
	h2.addHook("sonarr", "on", rc.srv.URL+"/on", "s", nil)
	tests = len(rc.got())
	h2.importEvent(h2.sender(false), l2.show, h2.placed(placeStep{slots: []int64{l2.slots["e1"]}, version: l2.mainVer, path: "Lab Show/Season 01/e1.mkv", placed: true}))
	if n := len(rc.got()) - tests; n != 0 {
		t.Fatalf("%d calls while the side may not send", n)
	}
}

// A call that gets no answer or a server error is tried again; a refusal
// such as 401 is not.
func TestHookEvents_Retries(t *testing.T) {
	h := newHookRig(t)
	l := h.library()
	rc := newReceiver(t)
	h.addHook("sonarr", "pulsarr", rc.srv.URL+"/hook", "s", nil)
	tests := len(rc.got())
	s := h.sender(true)
	n := 0
	step := func() int64 {
		n++
		return h.placed(placeStep{slots: []int64{l.slots["e1"]}, version: l.mainVer, path: fmt.Sprintf("Lab Show/Season 01/e1.%d.mkv", n), placed: true})
	}
	rc.mu.Lock()
	rc.seq = []int{503, 200}
	rc.mu.Unlock()
	h.importEvent(s, l.show, step())
	if n := len(rc.got()) - tests; n != 2 {
		t.Fatalf("503 then 200: %d calls", n)
	}
	tests = len(rc.got())
	rc.mu.Lock()
	rc.seq = []int{401, 200}
	rc.mu.Unlock()
	h.importEvent(s, l.show, step())
	if n := len(rc.got()) - tests; n != 1 {
		t.Fatalf("401: %d calls", n)
	}
	if strings.Contains(fmt.Sprint(rc.got()), "apikey") {
		t.Fatal("a key reached the hook")
	}
}
