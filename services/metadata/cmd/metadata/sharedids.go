package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// sharedIDsFlag marks the one-off shared-id pass done in the picture store.
const sharedIDsFlag = "shared_ids_v1"

// sharedResult is one decision of the shared-id pass, written to the
// backup file before anything changes: "relink" (MusicBrainz links another
// id; the link replaces the kept one), "linked" (MusicBrainz links this
// id: kept), "keep" (album evidence beats the others sharing the id) or
// "drop" (no such evidence: the match becomes a miss). Pictures and the
// similar list are what was removed.
type sharedResult struct {
	MBID        string            `json:"mbid"`
	Name        string            `json:"name"`
	Source      string            `json:"source"`
	Store       string            `json:"store,omitempty"`
	ID          string            `json:"id"`
	NewID       string            `json:"new_id,omitempty"`
	Decision    string            `json:"decision"`
	SharedWith  []string          `json:"shared_with,omitempty"`
	Have        int               `json:"have"`
	Rival       int               `json:"rival"`
	RivalID     string            `json:"rival_id,omitempty"`
	SrcTitles   []string          `json:"source_titles,omitempty"`
	MBTitles    []string          `json:"mb_titles,omitempty"`
	RivalTitles []string          `json:"rival_titles,omitempty"`
	Pictures    []imgstore.Record `json:"pictures,omitempty"`
	Similar     json.RawMessage   `json:"similar,omitempty"`
}

// sharedPass re-checks the kept Apple Music, Deezer and Spotify artist
// matches once: a MusicBrainz link always wins over a kept id, and an id
// several artists hold stays only with the linked one, or with the one
// whose albums clearly fit best; the others become misses. Changed
// artists lose that source's pictures (and Apple similar list) and are
// refetched on the normal queues.
type sharedPass struct {
	imgs     *imgstore.Store
	brief    func(ctx context.Context, mbid string) (mbread.Brief, bool)
	rivals   func(ctx context.Context, name, except string) []string
	titles   func(ctx context.Context, mbid string) []string
	evidence func(ctx context.Context, source, store, id, mbid string, rivals []string) (imgstore.AppleEvidence, error)
	similar  func(mbid, source string) []byte // removes and answers the list; nil: none
	refetch  func(ctx context.Context, mbid string)
	dir      string
	now      func() time.Time
	logf     func(string, ...any)
}

// done reports the pass finished (every group answered).
func (p *sharedPass) done() bool { return p.imgs.KV(sharedIDsFlag) != "" }

func (p *sharedPass) run(ctx context.Context) (int, error) {
	if p.imgs.KV(sharedIDsFlag) != "" {
		return 0, nil
	}
	var out []sharedResult
	pending := 0 // groups a source could not answer for: asked again later
	for _, source := range []string{"applemusic", "deezer", "spotify"} {
		rows, err := p.imgs.SourceRows(ctx, source)
		if err != nil {
			return 0, err
		}
		names := map[string]string{}
		groups := map[string][]imgstore.AppleArtistRow{}
		for _, r := range rows {
			b, ok := p.brief(ctx, r.MBID)
			if !ok {
				continue
			}
			names[r.MBID] = b.Name
			if l := imgstore.LinkedID(b.Links, source); l != "" {
				if l != r.AppleID {
					out = append(out, sharedResult{MBID: r.MBID, Name: b.Name, Source: source, Store: r.Store, ID: r.AppleID, NewID: l, Decision: "relink"})
				} else {
					groups[r.AppleID] = append(groups[r.AppleID], r)
					out = append(out, sharedResult{MBID: r.MBID, Name: b.Name, Source: source, ID: r.AppleID, Decision: "linked"})
				}
				continue
			}
			groups[r.AppleID] = append(groups[r.AppleID], r)
		}
		ids := make([]string, 0, len(groups))
		for id := range groups {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			g := groups[id]
			if len(g) < 2 {
				// A lone linked match needs no line.
				out = trimLinked(out, g, source)
				continue
			}
			var gres []sharedResult
			failed := false
			for _, m := range g {
				if out = markShared(out, m.MBID, source, g); isLinked(out, m.MBID, source) {
					continue
				}
				rivals := map[string]bool{}
				for _, o := range g {
					if o.MBID != m.MBID {
						rivals[o.MBID] = true
					}
				}
				for _, o := range p.rivals(ctx, names[m.MBID], m.MBID) {
					rivals[o] = true
				}
				list := make([]string, 0, len(rivals))
				for o := range rivals {
					list = append(list, o)
				}
				sort.Strings(list)
				ev, err := p.evidence(ctx, source, m.Store, id, m.MBID, list)
				if err != nil {
					// The group stays as it is and is asked again later.
					p.logf("shared ids: %s %s: %v", source, id, err)
					failed = true
					break
				}
				res := sharedResult{MBID: m.MBID, Name: names[m.MBID], Source: source, Store: m.Store, ID: id, Decision: "drop",
					Have: ev.Have, Rival: ev.Rival, RivalID: ev.RivalID, SrcTitles: ev.Apple, MBTitles: p.titles(ctx, m.MBID)}
				for _, o := range g {
					if o.MBID != m.MBID {
						res.SharedWith = append(res.SharedWith, o.MBID)
					}
				}
				if ev.RivalID != "" {
					res.RivalTitles = p.titles(ctx, ev.RivalID)
				}
				if ev.OK {
					res.Decision = "keep"
				}
				gres = append(gres, res)
			}
			if failed {
				pending++
				continue
			}
			out = append(out, gres...)
		}
	}
	// The backup is written before anything changes.
	changed := 0
	for i := range out {
		r := &out[i]
		if r.Decision != "relink" && r.Decision != "drop" {
			continue
		}
		changed++
		for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
			if rec, ok := p.imgs.Record(ctx, kind, r.MBID); ok && rec.Provider == r.Source {
				r.Pictures = append(r.Pictures, rec)
			}
		}
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return 0, err
	}
	path := filepath.Join(p.dir, "shared-ids-"+p.now().UTC().Format("20060102-150405")+".json")
	write := func() error {
		b, err := json.MarshalIndent(out, "", " ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, b, 0o600)
	}
	if err := write(); err != nil {
		return 0, err
	}
	for i := range out {
		r := &out[i]
		switch r.Decision {
		case "relink":
			switch r.Source {
			case "deezer":
				p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: "deezer", Ref: r.NewID}, imgstore.MatchLink, r.NewID)
			case "spotify":
				p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: "spotify"}, "link", r.NewID)
			default:
				// The store is not known: the next lookup finds it by the link.
				p.imgs.ForgetID(imgstore.KindArtist, r.MBID, r.Source)
			}
		case "drop":
			p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: r.Source}, "", "")
		default:
			continue
		}
		if r.Source == "applemusic" && p.similar != nil {
			if body := p.similar(r.MBID, "applemusic"); body != nil {
				r.Similar = body
				if err := write(); err != nil {
					return i, err
				}
			}
		}
		for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
			p.imgs.DropFrom(ctx, kind, r.MBID, r.Source)
		}
		p.refetch(ctx, r.MBID)
	}
	p.logf("shared ids: %d changed (relinked or dropped) of %d checked, %d groups left for later, backup %s", changed, len(out), pending, path)
	if pending > 0 {
		return changed, nil
	}
	return changed, p.imgs.SetKV(sharedIDsFlag, p.now().UTC().Format(time.RFC3339))
}

// trimLinked removes the "linked" lines of a group that turned out not to
// be shared.
func trimLinked(out []sharedResult, g []imgstore.AppleArtistRow, source string) []sharedResult {
	for _, m := range g {
		for i := len(out) - 1; i >= 0; i-- {
			if out[i].MBID == m.MBID && out[i].Source == source && out[i].Decision == "linked" {
				out = append(out[:i], out[i+1:]...)
				break
			}
		}
	}
	return out
}

// markShared notes on a linked line who else holds the id.
func markShared(out []sharedResult, mbid, source string, g []imgstore.AppleArtistRow) []sharedResult {
	for i := range out {
		if out[i].MBID == mbid && out[i].Source == source && out[i].Decision == "linked" {
			out[i].SharedWith = nil
			for _, o := range g {
				if o.MBID != mbid {
					out[i].SharedWith = append(out[i].SharedWith, o.MBID)
				}
			}
		}
	}
	return out
}

// isLinked reports an artist whose MusicBrainz link names the shared id.
func isLinked(out []sharedResult, mbid, source string) bool {
	for _, r := range out {
		if r.MBID == mbid && r.Source == source && r.Decision == "linked" {
			return true
		}
	}
	return false
}
