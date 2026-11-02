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

// singleTitleFlag marks the one-off single-title re-check done.
const singleTitleFlag = "single_title_v1"

// singleResult is one re-check decision, written before anything changes:
// "keep" (the one title in common is rare), "drop" (it is not: the match
// becomes a miss and the source's pictures go, so the next source or the
// placeholder shows) or "changed-since" (the kept id is no longer the one
// weighed).
type singleResult struct {
	MBID         string            `json:"mbid"`
	Name         string            `json:"name"`
	Source       string            `json:"source"`
	Store        string            `json:"store,omitempty"`
	ID           string            `json:"id"`
	Decision     string            `json:"decision"`
	Title        string            `json:"title,omitempty"`
	TitleHolders int               `json:"title_holders"`
	Have         int               `json:"have"`
	Rival        int               `json:"rival"`
	SrcTitles    []string          `json:"source_titles,omitempty"`
	Pictures     []imgstore.Record `json:"pictures,omitempty"`
	Similar      json.RawMessage   `json:"similar,omitempty"`
	sharedWith   []string
}

// singleTitlePass weighs again, with the rare-title rule, every match the
// latest homonym re-check and shared-id pass kept on one title in common.
type singleTitlePass struct {
	imgs     *imgstore.Store
	rivals   func(ctx context.Context, name, except string) []string
	evidence func(ctx context.Context, source, store, id, mbid string, rivals []string) (imgstore.AppleEvidence, error)
	similar  func(mbid, source string) []byte // removes and answers the list; nil: none
	refetch  func(ctx context.Context, mbid string)
	dir      string
	now      func() time.Time
	logf     func(string, ...any)
}

// latest reads the newest file matching pattern in dir into out (false
// when there is none).
func latest(dir, pattern string, out any) (bool, error) {
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(files) == 0 {
		return false, err
	}
	sort.Strings(files)
	b, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(b, out)
}

func (p *singleTitlePass) run(ctx context.Context) (kept, dropped int, err error) {
	if p.imgs.KV(singleTitleFlag) != "" {
		return 0, 0, nil
	}
	var todo []singleResult
	var apple []homonymResult
	if _, err := latest(p.dir, "apple-homonym-reeval-*.json", &apple); err != nil {
		return 0, 0, err
	}
	for _, r := range apple {
		if (r.Decision == "restore" || r.Decision == "kept") && r.Have == 1 {
			todo = append(todo, singleResult{MBID: r.MBID, Name: r.Name, Source: "applemusic", Store: r.Store, ID: r.AppleID})
		}
	}
	var shared []sharedResult
	if _, err := latest(p.dir, "shared-ids-*.json", &shared); err != nil {
		return 0, 0, err
	}
	for _, r := range shared {
		if r.Decision == "keep" && r.Have == 1 {
			todo = append(todo, singleResult{MBID: r.MBID, Name: r.Name, Source: r.Source, Store: r.Store, ID: r.ID, sharedWith: r.SharedWith})
		}
	}
	for i := range todo {
		r := &todo[i]
		if _, ext, _, _ := p.imgs.KeptID(imgstore.KindArtist, r.MBID, r.Source); ext != r.ID {
			r.Decision = "changed-since"
			continue
		}
		set := map[string]bool{}
		for _, o := range r.sharedWith {
			set[o] = true
		}
		for _, o := range p.rivals(ctx, r.Name, r.MBID) {
			set[o] = true
		}
		rivals := make([]string, 0, len(set))
		for o := range set {
			rivals = append(rivals, o)
		}
		sort.Strings(rivals)
		ev, err := p.evidence(ctx, r.Source, r.Store, r.ID, r.MBID, rivals)
		if err != nil {
			// Nothing has changed yet: the whole pass is asked again on
			// the next start.
			return 0, 0, err
		}
		r.Title, r.TitleHolders, r.Have, r.Rival, r.SrcTitles = ev.Single, ev.SingleHolders, ev.Have, ev.Rival, ev.Apple
		if ev.OK {
			r.Decision = "keep"
			kept++
			continue
		}
		r.Decision = "drop"
		dropped++
		for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
			if rec, ok := p.imgs.Record(ctx, kind, r.MBID); ok && rec.Provider == r.Source {
				r.Pictures = append(r.Pictures, rec)
			}
		}
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return 0, 0, err
	}
	path := filepath.Join(p.dir, "single-title-"+p.now().UTC().Format("20060102-150405")+".json")
	write := func() error {
		b, err := json.MarshalIndent(todo, "", " ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, b, 0o600)
	}
	// The backup is written before anything changes.
	if err := write(); err != nil {
		return 0, 0, err
	}
	for i := range todo {
		r := &todo[i]
		if r.Decision != "drop" {
			continue
		}
		p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: r.Source}, "", "")
		if r.Source == "applemusic" && p.similar != nil {
			if body := p.similar(r.MBID, "applemusic"); body != nil {
				r.Similar = body
				if err := write(); err != nil {
					return kept, dropped, err
				}
			}
		}
		for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
			p.imgs.DropFrom(ctx, kind, r.MBID, r.Source)
		}
		p.refetch(ctx, r.MBID)
	}
	p.logf("single-title re-check: %d kept, %d dropped (of %d), results %s", kept, dropped, len(todo), path)
	return kept, dropped, p.imgs.SetKV(singleTitleFlag, p.now().UTC().Format(time.RFC3339))
}
