package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// appleRestoreFlag marks the one-off re-check done in the picture store
// (v2: looser title keys; v3: self-titled albums left out, and the
// matches earlier versions restored are checked again; v4: two titles in
// common are enough while no rival shares one; v5: guest and source notes
// left out of titles, and the misses the old rule kept at lookup time
// cleared; v6: one title is enough when Apple has only that album; v7: a
// self-titled part of a double title is no evidence either).
const appleRestoreFlag = "apple_homonym_restore_v7"

// homonymDrop is one Apple match the first homonym pass dropped, as its
// backup file (apple-homonyms-*.json) holds it.
type homonymDrop struct {
	MBID     string            `json:"mbid"`
	Name     string            `json:"name"`
	Store    string            `json:"store"`
	AppleID  string            `json:"apple_id"`
	Pictures []imgstore.Record `json:"pictures,omitempty"`
	Similar  json.RawMessage   `json:"similar,omitempty"`
}

// homonymResult is one re-check decision, written for the hand check:
// "restore", "reject", "kept" (an earlier restore that still passes),
// "undo" (an earlier restore that no longer does), "matched-since" (another
// match was kept since), "stale-cleared" (a miss the old rule kept at
// lookup time, cleared so the next lookup asks again) or "error" (Apple did not answer; asked again on
// the next start).
type homonymResult struct {
	MBID        string   `json:"mbid"`
	Name        string   `json:"name"`
	Store       string   `json:"store"`
	AppleID     string   `json:"apple_id"`
	Decision    string   `json:"decision"`
	Have        int      `json:"have"`
	Rival       int      `json:"rival"`
	RivalID     string   `json:"rival_id,omitempty"`
	Rivals      int      `json:"rivals"`
	AppleTitles []string `json:"apple_titles,omitempty"`
	MBTitles    []string `json:"mb_titles,omitempty"`
	RivalTitles []string `json:"rival_titles,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// homonymRestore re-checks the matches the first homonym pass dropped
// (which rejected every name another MusicBrainz artist carries) with the
// album evidence rule: one that passes is kept again and its pictures,
// similar list and top songs are marked for a refetch on the normal
// queues; the rest stay a miss.
type homonymRestore struct {
	imgs     *imgstore.Store
	rivals   func(ctx context.Context, name, except string) []string
	titles   func(ctx context.Context, mbid string) []string
	evidence func(ctx context.Context, store, appleID, mbid string, rivals []string) (imgstore.AppleEvidence, error)
	refetch  func(ctx context.Context, mbid string)
	// undo takes back an earlier restore: the match a miss again, its
	// Apple pictures and similar list removed and refetched.
	undo func(ctx context.Context, mbid string)
	// Misses kept in [staleFrom, staleTo) for a name other artists carry
	// came from the old rule at lookup time; they are cleared and the
	// artist's pictures refetched. name answers an artist's name.
	staleFrom, staleTo time.Time
	name               func(ctx context.Context, mbid string) string
	dir                string
	now                func() time.Time
	logf               func(string, ...any)
}

func (p *homonymRestore) run(ctx context.Context) (restored, rejected int, err error) {
	if p.imgs.KV(appleRestoreFlag) != "" {
		return 0, 0, nil
	}
	files, err := filepath.Glob(filepath.Join(p.dir, "apple-homonyms-*.json"))
	if err != nil {
		return 0, 0, err
	}
	sort.Strings(files)
	var drops []homonymDrop
	seen := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return 0, 0, err
		}
		var part []homonymDrop
		if err := json.Unmarshal(b, &part); err != nil {
			return 0, 0, err
		}
		for _, d := range part {
			if !seen[d.MBID] {
				seen[d.MBID] = true
				drops = append(drops, d)
			}
		}
	}
	failed := 0
	out := make([]homonymResult, 0, len(drops))
	for _, d := range drops {
		if ctx.Err() != nil {
			return restored, rejected, ctx.Err()
		}
		res := homonymResult{MBID: d.MBID, Name: d.Name, Store: d.Store, AppleID: d.AppleID}
		_, ext, _, _ := p.imgs.KeptID(imgstore.KindArtist, d.MBID, "applemusic")
		if ext != "" && ext != d.AppleID {
			res.Decision = "matched-since"
			out = append(out, res)
			continue
		}
		again := ext != "" // restored by an earlier version of this pass
		rivals := p.rivals(ctx, d.Name, d.MBID)
		res.Rivals = len(rivals)
		var ev imgstore.AppleEvidence
		var err error
		for try := range 3 {
			if ev, err = p.evidence(ctx, d.Store, d.AppleID, d.MBID, rivals); err == nil || ctx.Err() != nil {
				break
			}
			time.Sleep(time.Duration(try+1) * 2 * time.Second)
		}
		res.Have, res.Rival, res.RivalID, res.AppleTitles = ev.Have, ev.Rival, ev.RivalID, ev.Apple
		res.MBTitles = p.titles(ctx, d.MBID)
		if ev.RivalID != "" {
			res.RivalTitles = p.titles(ctx, ev.RivalID)
		}
		switch {
		case err != nil:
			res.Decision, res.Error = "error", err.Error()
			failed++
		case again && ev.OK:
			res.Decision = "kept"
		case again:
			res.Decision = "undo"
			p.undo(ctx, d.MBID)
			rejected++
		case ev.OK:
			res.Decision = "restore"
			p.imgs.KeepID(imgstore.KindArtist, d.MBID, srctag.Tag{Source: "applemusic"}, d.Store, d.AppleID)
			p.refetch(ctx, d.MBID)
			restored++
		default:
			res.Decision = "reject"
			rejected++
		}
		out = append(out, res)
	}
	if !p.staleFrom.IsZero() {
		for _, id := range p.imgs.AppleMissesBetween(ctx, p.staleFrom, p.staleTo) {
			if seen[id] {
				continue
			}
			n := p.name(ctx, id)
			if n == "" || len(p.rivals(ctx, n, id)) == 0 {
				continue
			}
			p.imgs.ForgetID(imgstore.KindArtist, id, "applemusic")
			p.refetch(ctx, id)
			out = append(out, homonymResult{MBID: id, Name: n, Decision: "stale-cleared"})
		}
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return restored, rejected, err
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		return restored, rejected, err
	}
	path := filepath.Join(p.dir, "apple-homonym-reeval-"+p.now().UTC().Format("20060102-150405")+".json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return restored, rejected, err
	}
	p.logf("apple homonyms re-check: %d restored, %d kept rejected, %d failed (of %d), results %s", restored, rejected, failed, len(drops), path)
	if failed > 0 {
		return restored, rejected, nil // asked again on the next start
	}
	return restored, rejected, p.imgs.SetKV(appleRestoreFlag, p.now().UTC().Format(time.RFC3339))
}
