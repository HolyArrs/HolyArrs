package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// exactIDsFlag marks the one-off exact-id pass done. v2: the barcode
// recheck and the kept misses came after v1, so a server that finished v1
// runs the pass once more for them.
const exactIDsFlag = "exact_ids_v2"

// viaChecked marks a miss the exact steps found nothing for.
const viaChecked = "checked"

// exactLine is one decision of the exact-id pass, one JSON line each,
// written before the change it describes: the old and new id, how the
// match is now found, and what was removed.
type exactLine struct {
	MBID     string            `json:"mbid"`
	Name     string            `json:"name"`
	Source   string            `json:"source"`
	Old      string            `json:"old"`
	New      string            `json:"new"`
	Via      string            `json:"via"`
	Outcome  string            `json:"outcome"` // same, changed, kept, dropped
	Pictures []imgstore.Record `json:"pictures,omitempty"`
	Similar  json.RawMessage   `json:"similar,omitempty"`
}

// exactIDsPass puts every kept Apple Music, Deezer and Spotify artist match
// not yet classified through the matching order: MusicBrainz link, release
// barcodes, the linked Wikidata item, then the strict name rule (with
// same-name artists, album evidence and a rare lone title). A match the
// order confirms keeps its id and notes how it was found; one it moves
// takes the new id; a name match that fails becomes a miss. Changed
// matches lose that source's pictures (and Apple similar list) and are
// refetched on the normal queues. Rows are classified one by one, so a
// restart goes on where it stopped.
type exactIDsPass struct {
	imgs     *imgstore.Store
	brief    func(ctx context.Context, mbid string) (mbread.Brief, bool)
	rivals   func(ctx context.Context, name, except string) []string
	prefetch func(ctx context.Context, qids []string) // keeps Wikidata answers ahead, 50 a call
	exact    func(ctx context.Context, source, mbid, name, qid, country string) (id, store, how string, err error)
	evidence func(ctx context.Context, source, store, id, mbid string, rivals []string) (imgstore.AppleEvidence, error)
	similar  func(mbid, source string) []byte
	refetch  func(ctx context.Context, mbid string)
	dir      string
	now      func() time.Time
	logf     func(string, ...any)
	wait     time.Duration // pause before asking a paused source again (default a minute)
}

// paused reports an error that only asks to wait (a source paused, or
// rate limited, for now).
func paused(err error) bool {
	return errors.Is(err, imgstore.ErrSourcePaused) || strings.Contains(err.Error(), "paused") || strings.Contains(err.Error(), "429")
}

func (p *exactIDsPass) run(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{}
	if p.imgs.KV(exactIDsFlag) != "" {
		return counts, nil
	}
	if p.wait == 0 {
		p.wait = time.Minute
	}
	if _, err := p.barcodeRecheck(ctx); err != nil {
		return counts, err
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return counts, err
	}
	path := filepath.Join(p.dir, "exact-ids-"+p.now().UTC().Format("20060102-150405")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return counts, err
	}
	defer f.Close()
	line := func(l exactLine) error {
		b, err := json.Marshal(l)
		if err != nil {
			return err
		}
		_, err = f.Write(append(b, '\n'))
		return err
	}
	left := 0
	for _, source := range []string{"applemusic", "deezer", "spotify"} {
		rows, err := p.imgs.UnverifiedRows(ctx, source)
		if err != nil {
			return counts, err
		}
		for start := 0; start < len(rows); start += 200 {
			chunk := rows[start:min(len(rows), start+200)]
			briefs := map[string]mbread.Brief{}
			var qids []string
			for _, r := range chunk {
				if b, ok := p.brief(ctx, r.MBID); ok {
					briefs[r.MBID] = b
					if q := imgstore.LinkedQID(b.Links); q != "" {
						qids = append(qids, q)
					}
				}
			}
			if len(qids) > 0 && p.prefetch != nil {
				p.prefetch(ctx, qids)
			}
			for _, r := range chunk {
				if ctx.Err() != nil {
					return counts, ctx.Err()
				}
				b, ok := briefs[r.MBID]
				if !ok {
					left++
					continue
				}
				l, err := p.decide(ctx, source, r, b)
				if err != nil {
					p.logf("exact ids: %s %s: %v", source, r.MBID, err)
					left++
					continue
				}
				counts[source+" "+l.Via+" "+l.Outcome]++
				if err := p.apply(ctx, r, l, line); err != nil {
					return counts, err
				}
			}
			p.logf("exact ids: %s %d of %d done", source, min(len(rows), start+200), len(rows))
		}
	}
	// Misses go through the exact steps only: a name never makes a match
	// the earlier checks refused.
	for _, source := range []string{"applemusic", "deezer", "spotify"} {
		rows, err := p.imgs.UnverifiedMisses(ctx, source)
		if err != nil {
			return counts, err
		}
		for start := 0; start < len(rows); start += 200 {
			chunk := rows[start:min(len(rows), start+200)]
			briefs := map[string]mbread.Brief{}
			var qids []string
			for _, r := range chunk {
				if b, ok := p.brief(ctx, r.MBID); ok {
					briefs[r.MBID] = b
					if q := imgstore.LinkedQID(b.Links); q != "" {
						qids = append(qids, q)
					}
				}
			}
			if len(qids) > 0 && p.prefetch != nil {
				p.prefetch(ctx, qids)
			}
			for _, r := range chunk {
				if ctx.Err() != nil {
					return counts, ctx.Err()
				}
				b, ok := briefs[r.MBID]
				if !ok {
					left++
					continue
				}
				id, store, how, err := p.exactWaiting(ctx, source, r.MBID, b)
				if err != nil {
					p.logf("exact ids: %s miss %s: %v", source, r.MBID, err)
					left++
					continue
				}
				l := exactLine{MBID: r.MBID, Name: b.Name, Source: source, New: id, Via: viaName(how), Outcome: "found"}
				if id == "" {
					l.Via, l.Outcome = "none", "missing"
					p.imgs.SetVia(imgstore.KindArtist, r.MBID, source, viaChecked)
				} else if source == "applemusic" && store == "" {
					// The storefront is found by the next lookup, in order.
					p.imgs.ForgetID(imgstore.KindArtist, r.MBID, source)
					p.refetch(ctx, r.MBID)
				} else {
					st := store
					if source != "applemusic" {
						st = how
					}
					p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: source}, st, id)
					p.imgs.SetVia(imgstore.KindArtist, r.MBID, source, l.Via)
					p.refetch(ctx, r.MBID)
				}
				counts[source+" "+l.Via+" "+l.Outcome]++
				if err := line(l); err != nil {
					return counts, err
				}
			}
			p.logf("exact ids: %s misses %d of %d done", source, min(len(rows), start+200), len(rows))
		}
	}
	p.logf("exact ids: %v, %d left for the next start, results %s", counts, left, path)
	if left > 0 {
		return counts, nil
	}
	return counts, p.imgs.SetKV(exactIDsFlag, p.now().UTC().Format(time.RFC3339))
}

// decide runs one row through the order (waiting out a paused source).
func (p *exactIDsPass) decide(ctx context.Context, source string, r imgstore.AppleArtistRow, b mbread.Brief) (exactLine, error) {
	l := exactLine{MBID: r.MBID, Name: b.Name, Source: source, Old: r.AppleID}
	if link := imgstore.LinkedID(b.Links, source); link != "" {
		l.New, l.Via, l.Outcome = link, imgstore.ViaLink, "same"
		if link != r.AppleID {
			l.Outcome = "changed"
		}
		return l, nil
	}
	id, _, how, err := p.exactWaiting(ctx, source, r.MBID, b)
	if err != nil {
		return l, err
	}
	if id != "" {
		l.New, l.Via, l.Outcome = id, viaName(how), "same"
		if id != r.AppleID {
			l.Outcome = "changed"
		}
		return l, nil
	}
	l.Via = imgstore.ViaName
	rivals := p.rivals(ctx, b.Name, r.MBID)
	if len(rivals) == 0 {
		l.New, l.Outcome = r.AppleID, "kept"
		return l, nil
	}
	for {
		ev, err := p.evidence(ctx, source, r.Store, r.AppleID, r.MBID, rivals)
		if err != nil {
			if !paused(err) {
				return l, err
			}
			select {
			case <-ctx.Done():
				return l, ctx.Err()
			case <-time.After(p.wait):
			}
			continue
		}
		if ev.OK {
			l.New, l.Outcome = r.AppleID, "kept"
		} else {
			l.Outcome = "dropped"
		}
		return l, nil
	}
}

// viaName is the via name of a Match* how.
func viaName(how string) string {
	switch how {
	case imgstore.MatchBarcode:
		return imgstore.ViaBarcode
	case imgstore.MatchWikidata:
		return imgstore.ViaWikidata
	}
	return imgstore.ViaName
}

// apply writes the line (with what goes, before it goes) and makes the
// change.
func (p *exactIDsPass) apply(ctx context.Context, r imgstore.AppleArtistRow, l exactLine, line func(exactLine) error) error {
	if l.Outcome == "same" || l.Outcome == "kept" {
		p.imgs.SetVia(imgstore.KindArtist, r.MBID, l.Source, l.Via)
		return line(l)
	}
	for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
		if rec, ok := p.imgs.Record(ctx, kind, r.MBID); ok && rec.Provider == l.Source {
			l.Pictures = append(l.Pictures, rec)
		}
	}
	if l.Source == "applemusic" && p.similar != nil {
		// The similar list is read and removed here; the line keeps it.
		l.Similar = p.similar(r.MBID, "applemusic")
	}
	if err := line(l); err != nil {
		return err
	}
	switch {
	case l.Outcome == "dropped":
		p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: l.Source}, "", "")
	case l.Source == "applemusic":
		// The storefront is found again by the next lookup, in this order.
		p.imgs.ForgetID(imgstore.KindArtist, r.MBID, l.Source)
	default:
		how := map[string]string{imgstore.ViaLink: imgstore.MatchLink, imgstore.ViaBarcode: imgstore.MatchBarcode, imgstore.ViaWikidata: imgstore.MatchWikidata}[l.Via]
		p.imgs.KeepID(imgstore.KindArtist, r.MBID, srctag.Tag{Source: l.Source}, how, l.New)
		p.imgs.SetVia(imgstore.KindArtist, r.MBID, l.Source, l.Via)
	}
	for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
		p.imgs.DropFrom(ctx, kind, r.MBID, l.Source)
	}
	p.refetch(ctx, r.MBID)
	return nil
}

// exactWaiting asks the exact steps, waiting out a paused source.
func (p *exactIDsPass) exactWaiting(ctx context.Context, source, mbid string, b mbread.Brief) (id, store, how string, err error) {
	for {
		id, store, how, err = p.exact(ctx, source, mbid, b.Name, imgstore.LinkedQID(b.Links), b.Country)
		if err == nil || !paused(err) {
			return id, store, how, err
		}
		select {
		case <-ctx.Done():
			return "", "", "", ctx.Err()
		case <-time.After(p.wait):
		}
	}
}

// barcodeRecheckFlag marks the undo of the first barcode rule done.
const barcodeRecheckFlag = "exact_ids_barcode_recheck_v1"

// barcodeRecheck undoes what the first barcode rule changed (it took an
// album's first artist, wrong for a duo's or an alias's album): every
// match it moved gets its old id back and its pictures refetched, and every
// row found by a barcode is classified again under the name-checked rule.
func (p *exactIDsPass) barcodeRecheck(ctx context.Context) (int, error) {
	if p.imgs.KV(barcodeRecheckFlag) != "" {
		return 0, nil
	}
	files, err := filepath.Glob(filepath.Join(p.dir, "exact-ids-*.jsonl"))
	if err != nil {
		return 0, err
	}
	restored := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return restored, err
		}
		for _, raw := range strings.Split(string(b), "\n") {
			var l exactLine
			if raw == "" || json.Unmarshal([]byte(raw), &l) != nil || l.Via != imgstore.ViaBarcode || l.Outcome != "changed" || l.Old == "" {
				continue
			}
			_, ext, _, found := p.imgs.KeptID(imgstore.KindArtist, l.MBID, l.Source)
			if found && ext != l.New && ext != "" {
				continue // matched otherwise since
			}
			switch l.Source {
			case "applemusic":
				p.imgs.KeepID(imgstore.KindArtist, l.MBID, srctag.Tag{Source: l.Source}, "us", l.Old)
			default:
				p.imgs.KeepID(imgstore.KindArtist, l.MBID, srctag.Tag{Source: l.Source}, imgstore.MatchSearch, l.Old)
			}
			p.imgs.SetVia(imgstore.KindArtist, l.MBID, l.Source, "")
			for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
				p.imgs.DropFrom(ctx, kind, l.MBID, l.Source)
			}
			p.refetch(ctx, l.MBID)
			restored++
		}
	}
	n, err := p.imgs.ClearVia(ctx, imgstore.ViaBarcode)
	if err != nil {
		return restored, err
	}
	p.logf("exact ids: first barcode rule undone: %d matches restored, %d barcode rows to classify again", restored, n)
	return restored, p.imgs.SetKV(barcodeRecheckFlag, p.now().UTC().Format(time.RFC3339))
}
