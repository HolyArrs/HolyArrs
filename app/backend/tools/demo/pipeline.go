package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// placeholderMark starts every file seed writes; reset deletes only files
// that start with it.
const placeholderMark = "HolyArrs demo placeholder"

// payload is what the stand-in Downloader "downloads" for one Release.
type payload struct {
	Files []string // relative to the Release folder
	Fail  bool     // the download fails instead
	// State, when set, leaves the download unfinished in that Downloader
	// state (queued, downloading, paused) at Progress, with no files.
	State    string
	Progress float64
	Speed    int64
}

// fakeEngine stands in for the lab qBittorrent while seed runs: a Release
// sent to it is done at once, its files are tiny placeholders in the save
// folder, and the app's own Downloader manager and importer take it from
// there.
type fakeEngine struct {
	save string
	mu   sync.Mutex
	plan map[string]payload
	jobs map[string]downloads.Job
	made []string
}

func newFakeEngine(save string) *fakeEngine {
	return &fakeEngine{save: save, plan: map[string]payload{}, jobs: map[string]downloads.Job{}}
}

func (f *fakeEngine) Health(context.Context) (downloads.Health, error) {
	return downloads.Health{Version: "demo", At: time.Now()}, nil
}

func (f *fakeEngine) Add(_ context.Context, r downloads.AddRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.InfoHash
	if id == "" {
		id = hashOf(r.Name)
	}
	p, ok := f.plan[r.Name]
	if !ok {
		p = payload{Files: []string{r.Name + ".mkv"}}
	}
	root := filepath.Join(f.save, r.Name)
	j := downloads.Job{ID: id, Name: r.Name, Category: r.Category, SavePath: f.save, ContentPath: filepath.ToSlash(root), Progress: 1, State: downloads.StateFinished}
	if p.Fail {
		j.State, j.Progress, j.Detail = downloads.StateFailed, 0.37, "error"
	}
	if p.State != "" {
		// Believable numbers: each release moves at its own pace, near the
		// speed asked for, with its own swarm.
		n := int(id[0]) + int(id[1])
		if p.Speed > 0 {
			p.Speed = p.Speed * int64(70+n%70) / 100
			j.UpSpeed = p.Speed * int64(4+n%9) / 100
			j.Seeds, j.Peers = 6+n%90, 2+n%24
		} else if p.State == downloads.StateDownloading {
			j.Peers = n % 2 // stalled: almost nobody to talk to
		}
		j.State, j.Progress, j.Speed, j.Size = p.State, p.Progress, p.Speed, int64(2+n%14)<<30
		if p.Speed > 0 {
			j.ETA = int64(float64(j.Size) * (1 - p.Progress) / float64(p.Speed))
		}
	}
	if !p.Fail && p.State == "" {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		f.made = append(f.made, root+string(filepath.Separator))
		for i, name := range p.Files {
			full := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return "", err
			}
			size := 3072 + i*512
			buf := make([]byte, size)
			copy(buf, placeholderMark+": not a real download.\n")
			if err := os.WriteFile(full, buf, 0o644); err != nil {
				return "", err
			}
			f.made = append(f.made, full)
			j.Files = append(j.Files, downloads.File{Name: filepath.ToSlash(filepath.Join(r.Name, name)), Size: int64(size), Progress: 1})
			j.Size += int64(size)
		}
	}
	f.jobs[id] = j
	return id, nil
}

func (f *fakeEngine) List(context.Context, string) ([]downloads.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]downloads.Job, 0, len(f.jobs))
	for _, j := range f.jobs {
		out = append(out, wobble(j))
	}
	return out, nil
}

func (f *fakeEngine) Get(_ context.Context, id string) (downloads.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return j, downloads.ErrNotFound
	}
	return wobble(j), nil
}

// wobble moves a downloading job's speeds a little with the clock, so a
// graph drawn from repeated reads is not a flat line.
func wobble(j downloads.Job) downloads.Job {
	if j.State != downloads.StateDownloading || j.Speed == 0 {
		return j
	}
	t := float64(time.Now().UnixMilli())/1000 + float64(j.ID[2])
	k := 1 + 0.18*math.Sin(t/3.1) + 0.07*math.Sin(t/1.3)
	j.Speed = int64(float64(j.Speed) * k)
	j.UpSpeed = int64(float64(j.UpSpeed) * (2 - k))
	return j
}

func (f *fakeEngine) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.jobs, id)
	return nil
}

func hashOf(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// mark is the highest id in each table that a case's rows come after.
type mark struct {
	events, releases, downloads, imports, files int64
}

func (w *world) markNow(ctx context.Context) mark {
	var m mark
	db := w.s.Read()
	_ = db.QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM releases").Scan(&m.releases)
	_ = db.QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM downloads").Scan(&m.downloads)
	_ = db.QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM imports").Scan(&m.imports)
	_ = db.QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM files").Scan(&m.files)
	_ = w.s.ReadEvents().QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM events").Scan(&m.events)
	return m
}

// stamp is a span of rows made in one go and when it should look like it
// happened.
type stamp struct {
	from, to mark
	at       time.Time
	step     time.Duration
}

// releaseFor stores one Release found for Slots, as a search stores it
// (judged, with its parsed name and a short verdict).
func (w *world) releaseFor(ctx context.Context, slotIDs []int64, name string, size int64, outcome string, points int) (int64, error) {
	var srcID int64
	if err := w.s.Read().QueryRowContext(ctx, "SELECT id FROM sources WHERE protocol = 'torrent' ORDER BY id LIMIT 1").Scan(&srcID); err != nil {
		return 0, fmt.Errorf("no torrent Source to file the Release under: %w", err)
	}
	p := releasename.Parse(name, releasename.Hints{Size: size})
	parsed := map[string]any{}
	if b, err := json.Marshal(p); err == nil {
		_ = json.Unmarshal(b, &parsed)
	}
	mine := map[string]any{"outcome": outcome, "passed": true, "points": points, "rank": 1}
	bySlot := map[string]any{}
	for _, id := range slotIDs {
		bySlot[strconv.FormatInt(id, 10)] = mine
	}
	hash := hashOf(name)
	seed, peers := 40+int(size%300), 3+int(size%17)
	pub := time.Now().Add(-6 * time.Hour)
	rel := &domain.Release{SourceID: srcID, Name: name, LinkRef: "http://fake-indexer/demo/" + strings.ToLower(dotted(name)) + ".torrent",
		Protocol: "torrent", Size: size, PublishedAt: &pub, Seeders: &seed, Peers: &peers, InfoHash: &hash, Flags: p.Flags, Parsed: parsed,
		MatchedSlotIDs: slotIDs, Verdict: map[string]any{"outcome": outcome, "passed": true, "points": points, "rank": 1, "by_slot": bySlot},
		FoundBy: "auto_search", State: "judged"}
	if rel.Flags == nil {
		rel.Flags = []string{}
	}
	var id int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = database.Insert(tx, "releases", rel, w.by)
		return err
	})
	if err == nil {
		w.m.Releases = append(w.m.Releases, id)
	}
	return id, err
}

// chooseAuto settles a Release as the Slot's pick the way a search does:
// the Slot goes through searching to chosen and the Release is chosen.
func (w *world) chooseAuto(ctx context.Context, titleID, slotID, relID int64, name string, judged int) error {
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotSearching, "searching", w.by); err != nil {
			return err
		}
		if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotChosen, "chose "+name, w.by); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", relID, slotID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE releases SET state = 'chosen', hold_until = NULL, verdict = json_set(verdict, '$.pick_slot_ids', json(?)) WHERE id = ?`,
			fmt.Sprintf("[%d]", slotID), relID)
		return err
	})
	if err != nil {
		return err
	}
	return w.s.AddEvent(ctx, &domain.Event{Actor: w.by, Kind: "release_chosen", TitleID: &titleID, SlotID: &slotID,
		Summary: fmt.Sprintf("Chose %s out of %d Releases", name, judged), Data: map[string]any{"release_id": relID},
		GroupKey: fmt.Sprintf("slot:%d", slotID)})
}

// hold holds a Release for a Slot until a time, the way a search does when
// the Pick rules ask to wait for a better copy.
func (w *world) hold(ctx context.Context, titleID, slotID, relID int64, name string, until time.Time) error {
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotSearching, "searching", w.by); err != nil {
			return err
		}
		if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotHeld, "holding "+name, w.by); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET held_release_id = ?, next_try_at = ? WHERE id = ?", relID, database.FormatTime(until), slotID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE releases SET state = 'held', hold_until = ?, verdict = json_set(verdict, '$.outcome', 'held', '$.pick_slot_ids', json(?)) WHERE id = ?`,
			database.FormatTime(until), fmt.Sprintf("[%d]", slotID), relID)
		return err
	})
	if err != nil {
		return err
	}
	return w.s.AddEvent(ctx, &domain.Event{Actor: w.by, Kind: "release_held", TitleID: &titleID, SlotID: &slotID,
		Summary: "Holding " + name, Data: map[string]any{"release_id": relID}, GroupKey: fmt.Sprintf("slot:%d", slotID)})
}

// grab sends a chosen Release to the stand-in Downloader, lets the app's
// Downloader manager see it finish (or fail) and imports what finished.
func (w *world) grab(ctx context.Context, slotID, relID int64, name string, p payload) error {
	w.eng.mu.Lock()
	w.eng.plan[name] = p
	w.eng.mu.Unlock()
	if _, err := w.dm.Grab(ctx, slotID, relID, w.by); err != nil {
		return fmt.Errorf("grab: %w", err)
	}
	w.finished = nil
	if _, err := w.dm.SyncOnce(ctx); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	for _, id := range w.finished {
		if _, err := w.im.Import(ctx, id, false, w.by); err != nil {
			return fmt.Errorf("import: %w", err)
		}
	}
	return nil
}

// slotOf finds a movie's Slot in a Version (by label).
func (w *world) slotOf(ctx context.Context, titleID int64, label string) (int64, error) {
	var id int64
	err := w.s.Read().QueryRowContext(ctx, `SELECT s.id FROM slots s JOIN versions v ON v.id = s.version_id JOIN items i ON i.id = s.item_id
		WHERE i.title_id = ? AND v.label = ? ORDER BY s.id LIMIT 1`, titleID, label).Scan(&id)
	return id, err
}

// episodeSlots finds the main Version Slots of a show's episodes.
func (w *world) episodeSlots(ctx context.Context, titleID int64, season int, eps ...int) ([]int64, error) {
	q := `SELECT s.id FROM slots s JOIN versions v ON v.id = s.version_id JOIN items i ON i.id = s.item_id
		WHERE i.title_id = ? AND v.is_main = 1 AND json_extract(i.numbers, '$.season') = ?`
	args := []any{titleID, season}
	if len(eps) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(eps)), ",")
		q += " AND json_extract(i.numbers, '$.episode') IN (" + ph + ")"
		for _, e := range eps {
			args = append(args, e)
		}
	}
	q += " AND s.state IN ('wanted', 'better_wanted') ORDER BY json_extract(i.numbers, '$.episode')"
	rows, err := w.s.Read().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// activity makes the last weeks' History through the app's own pick,
// Downloader and import code: grabs and imports, upgrades, a 4K copy,
// downloads that need a decision, a failed one, Held and Turned down
// Releases. Each case is done once; times are set afterwards.
func (w *world) activity(ctx context.Context) error {
	db := w.s.Read()
	ds, err := database.Select[domain.Downloader](ctx, db, "downloaders", "protocol = 'torrent' AND removed = 0 ORDER BY priority, id")
	if err != nil {
		return err
	}
	if len(ds) == 0 {
		w.skip("activity: no torrent Downloader")
		return nil
	}
	w.dm.Sleep = func(context.Context, time.Duration) error { return nil }
	for _, d := range ds {
		if _, err := w.dm.Check(ctx, d); err != nil {
			return fmt.Errorf("stand-in Downloader: %w", err)
		}
	}
	now := time.Now().UTC()
	day := 24 * time.Hour
	type kase struct {
		key string
		at  time.Time
		run func() error
	}
	var cases []kase
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	gb := int64(1 << 30)
	n := 0
	for _, e := range demoMovies {
		e := e
		id := w.m.Titles["movie:"+e.TMDB]
		if id == 0 {
			continue
		}
		var t mediafiles.Title
		if err := database.Get(ctx, db, "titles", id, &t); err != nil {
			return err
		}
		m := movieFromTitle(t)
		n++
		at := ago(time.Duration(19-n%18)*day + time.Duration(n*37)*time.Minute)
		switch e.Plan {
		case planGrab, planUpgrade:
			q := e.Quality
			if e.Plan == planUpgrade {
				q = "bluray1080"
			}
			name := movieRelease(m, q)
			cases = append(cases, kase{"grab " + name, at, func() error {
				slot, err := w.slotOf(ctx, id, "Main")
				if err != nil {
					return err
				}
				rel, err := w.releaseFor(ctx, []int64{slot}, name, 9*gb+int64(n)*gb/7, "chosen", 40)
				if err != nil {
					return err
				}
				if err := w.chooseAuto(ctx, id, slot, rel, name, 6+n%9); err != nil {
					return err
				}
				return w.grab(ctx, slot, rel, name, payload{Files: []string{name + ".mkv", "Sample/" + strings.ToLower(dotted(m.Name)) + "-sample.mkv", name + ".nfo"}})
			}})
		case planFailed:
			name := movieRelease(m, e.Quality)
			cases = append(cases, kase{"failed " + name, ago(2*day + 3*time.Hour), func() error {
				slot, err := w.slotOf(ctx, id, "Main")
				if err != nil {
					return err
				}
				rel, err := w.releaseFor(ctx, []int64{slot}, name, 7*gb, "chosen", 35)
				if err != nil {
					return err
				}
				if err := w.chooseAuto(ctx, id, slot, rel, name, 5); err != nil {
					return err
				}
				return w.grab(ctx, slot, rel, name, payload{Fail: true})
			}})
		case planHeld:
			name := movieRelease(m, e.Quality)
			wait := time.Duration(2+n%4)*24*time.Hour + time.Duration(n%11)*time.Hour
			cases = append(cases, kase{"held " + name, ago(3 * time.Hour), func() error {
				slot, err := w.slotOf(ctx, id, "Main")
				if err != nil {
					return err
				}
				rel, err := w.releaseFor(ctx, []int64{slot}, name, 10*gb, "held", 25)
				if err != nil {
					return err
				}
				return w.hold(ctx, id, slot, rel, name, now.Add(wait))
			}})
		case planTurned:
			names := []string{dotted(m.Name) + "." + strconv.Itoa(m.Year) + ".1080p.HDCAM.x264-NOGRP",
				dotted(m.Name) + "." + strconv.Itoa(m.Year) + ".MULTi.1080p.BluRay.x264-UKDHD"}
			reasons := []string{"camera copy, not a real release", "wrong language (French audio only)"}
			cases = append(cases, kase{"turned " + m.Name, ago(time.Duration(4+n%5) * day), func() error {
				slot, err := w.slotOf(ctx, id, "Main")
				if err != nil {
					return err
				}
				for i, name := range names {
					rel, err := w.releaseFor(ctx, []int64{slot}, name, 2*gb+int64(i)*gb, "", 5)
					if err != nil {
						return err
					}
					if err := w.sr.TurnDown(ctx, rel, reasons[i], w.by); err != nil {
						return err
					}
				}
				return nil
			}})
		}
		if e.FourK == "have" {
			name := movieRelease(m, "uhd")
			cases = append(cases, kase{"4k " + name, ago(time.Duration(6+n%7)*day + 2*time.Hour), func() error {
				slot, err := w.slotOf(ctx, id, "4K")
				if err != nil {
					return err
				}
				rel, err := w.releaseFor(ctx, []int64{slot}, name, 58*gb, "chosen", 60)
				if err != nil {
					return err
				}
				if err := w.chooseAuto(ctx, id, slot, rel, name, 3); err != nil {
					return err
				}
				return w.grab(ctx, slot, rel, name, payload{Files: []string{name + ".mkv"}})
			}})
		}
	}
	// Show episodes: single episodes and one season pack.
	for _, sc := range []struct {
		tmdb    string
		season  int
		eps     []int
		quality string
		days    int
	}{{"100088", 2, []int{3}, "web1080", 5}, {"95396", 2, []int{6}, "web2160", 3}, {"66732", 4, nil, "web1080", 9}, {"2316", 6, []int{4}, "web1080", 1}} {
		sc := sc
		id := w.m.Titles["show:"+sc.tmdb]
		if id == 0 {
			continue
		}
		var t mediafiles.Title
		if err := database.Get(ctx, db, "titles", id, &t); err != nil {
			return err
		}
		key := fmt.Sprintf("episodes %s S%02d %v", t.Name, sc.season, sc.eps)
		cases = append(cases, kase{key, ago(time.Duration(sc.days)*day + 7*time.Hour), func() error {
			slots, err := w.episodeSlots(ctx, id, sc.season, sc.eps...)
			if err != nil || len(slots) == 0 {
				return fmt.Errorf("no looking episodes (%v)", err)
			}
			name := fmt.Sprintf("%s.S%02d.%s", dotted(t.Name), sc.season, qualityTags[sc.quality])
			files := []string{}
			if len(sc.eps) == 1 {
				name = fmt.Sprintf("%s.S%02dE%02d.%s", dotted(t.Name), sc.season, sc.eps[0], qualityTags[sc.quality])
				files = append(files, name+".mkv")
			} else {
				for i := range slots {
					files = append(files, fmt.Sprintf("%s.S%02dE%02d.%s.mkv", dotted(t.Name), sc.season, i+1, qualityTags[sc.quality]))
				}
			}
			rel, err := w.releaseFor(ctx, slots, name, int64(len(slots))*2*gb, "chosen", 30)
			if err != nil {
				return err
			}
			if len(slots) == 1 {
				if err := w.chooseAuto(ctx, id, slots[0], rel, name, 7); err != nil {
					return err
				}
			} else if _, err := w.sr.Choose(ctx, rel, 0, false, w.by); err != nil {
				return fmt.Errorf("choose pack: %w", err)
			}
			return w.grab(ctx, slots[0], rel, name, payload{Files: files})
		}})
	}
	sort.SliceStable(cases, func(i, j int) bool { return cases[i].at.Before(cases[j].at) })
	for _, c := range cases {
		if w.m.Steps[c.key] {
			continue
		}
		before := w.markNow(ctx)
		err := c.run()
		w.m.Steps[c.key] = true
		if err != nil {
			w.skip("%s: %v", c.key, err)
			continue
		}
		w.stamps = append(w.stamps, stamp{from: before, to: w.markNow(ctx), at: c.at, step: 3 * time.Minute})
		w.say("   %s", c.key)
	}
	for _, p := range w.eng.made {
		w.m.Paths = append(w.m.Paths, p)
	}
	w.eng.made = nil
	return nil
}
