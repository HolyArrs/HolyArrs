package downloads

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Push updates for the Downloads page (setting downloads.sync = on): the
// background reader's publisher calls PublishDL with one client's state
// before and after a window, and this file turns that into keyed row
// patches on /live topic "dl".
//
// Who receives what:
//   - managers: every row of the client (HolyArrs' own rows get their live
//     numbers; outside rows get whole rows when new, changed fields after);
//   - everyone else: only HolyArrs' own rows tied to titles they asked for,
//     live numbers only. Outside torrents never reach them.
//
// A non-manager with a stream open gets a patch for every window of the
// client, empty when none of their rows moved, so their rev chain has no
// holes; the browser drops nothing and reads its page again on any gap.

// LiveKey is a row's key in patches: "<downloader id>:<external id>".
func LiveKey(downloaderID int64, externalID string) string {
	return strconv.FormatInt(downloaderID, 10) + ":" + strings.ToLower(externalID)
}

// ownNumbers are the fields of a HolyArrs row a patch may move: the client's
// live numbers. State, phase and words come from the Manager's own writes,
// which reach the page through the "download" event and a page read.
func ownNumbers(j Job) map[string]any {
	m := map[string]any{
		"progress": j.Progress, "speed": j.Speed, "up_speed": j.UpSpeed, "seeds": j.Seeds, "peers": j.Peers,
		"seeds_total": j.SeedsTotal, "peers_total": j.PeersTotal, "ratio": j.Ratio, "eta": nil, "availability": nil,
	}
	if j.Availability != nil && j.Progress < 1 {
		m["availability"] = *j.Availability
	}
	if j.ETA > 0 {
		m["eta"] = j.ETA
	}
	return m
}

// rowFields is a row as the page reads it, field by field.
func rowFields(row DownloadRow) map[string]any {
	b, _ := json.Marshal(row)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	delete(m, "updated_at")
	return m
}

// changed lists the fields of cur that differ from prev; a field gone from
// cur reads as null.
func changed(prev, cur map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range cur {
		if p, ok := prev[k]; !ok || !reflect.DeepEqual(p, v) {
			out[k] = v
		}
	}
	for k := range prev {
		if _, ok := cur[k]; !ok {
			out[k] = nil
		}
	}
	return out
}

// dlOwn is what the page's own rows are, for one patch.
type dlOwn struct {
	keys  map[string]bool           // keys of the newest own rows (the page's list)
	users map[string]map[int64]bool // key -> people whose asks it serves
}

// ownRows reads which keys of client id are HolyArrs' own rows on the page
// and, when anyone but a manager watches, whose asks each serves.
func (a *Web) ownRows(ctx context.Context, id int64, keys []string, people bool) dlOwn {
	o := dlOwn{keys: map[string]bool{}, users: map[string]map[int64]bool{}}
	db := a.read()
	// The same list the view shows: the newest 100 downloads.
	if rs, err := db.QueryContext(ctx, "SELECT downloader_id, external_id FROM downloads ORDER BY id DESC LIMIT 100"); err == nil {
		for rs.Next() {
			var did int64
			var ext string
			if rs.Scan(&did, &ext) == nil && did == id && ext != "" {
				o.keys[LiveKey(did, ext)] = true
			}
		}
		rs.Close()
	}
	if !people || len(keys) == 0 {
		return o
	}
	ids := make([]any, 0, len(keys)+1)
	ids = append(ids, id)
	marks := make([]string, 0, len(keys))
	for _, k := range keys {
		_, ext, _ := strings.Cut(k, ":")
		ids = append(ids, ext)
		marks = append(marks, "?")
	}
	rs, err := db.QueryContext(ctx, `SELECT DISTINCT lower(d.external_id), p.user_id FROM downloads d, json_each(d.slot_ids) j
		JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id JOIN asks a ON a.title_id = i.title_id JOIN ask_people p ON p.ask_id = a.id
		WHERE d.downloader_id = ? AND lower(d.external_id) IN (`+strings.Join(marks, ",")+`)`, ids...)
	if err != nil {
		return o
	}
	defer rs.Close()
	for rs.Next() {
		var ext string
		var uid int64
		if rs.Scan(&ext, &uid) == nil {
			k := LiveKey(id, ext)
			if o.users[k] == nil {
				o.users[k] = map[int64]bool{}
			}
			o.users[k][uid] = true
		}
	}
	return o
}

// PublishDL is the publisher's Build: one patch per audience for client id
// going from prev (at fromRev) to cur (at toRev).
func (a *Web) PublishDL(ctx context.Context, id int64, fromRev, toRev uint64, prev, cur map[string]Job) {
	if a.Live == nil || !a.syncOn() {
		return
	}
	var d domain.Downloader
	if n, ok := a.downloaderNames(ctx)[id]; ok {
		d = n
	} else {
		return
	}
	viewers := a.Live.Viewers()
	ch := Diff(prev, cur)
	keyOf := func(ext string) string { return LiveKey(id, ext) }
	var moved []string
	for _, ext := range ch.Added {
		moved = append(moved, keyOf(ext))
	}
	for _, fc := range ch.Changed {
		moved = append(moved, keyOf(fc.ID))
	}
	own := a.ownRows(ctx, id, moved, len(viewers) > 0)
	outsideShown := d.Protocol == "torrent" || d.Protocol == "usenet" && a.DLSync.SABOutside(ctx)
	boot := a.Live.Boot()

	mgr := Patch{Kind: KindPatch, Boot: boot, Client: id, FromRev: fromRev, ToRev: toRev, Totals: a.dlTotals()}
	per := map[int64]*Patch{}
	for _, uid := range viewers {
		per[uid] = &Patch{Kind: KindPatch, Boot: boot, Client: id, FromRev: fromRev, ToRev: toRev}
	}
	var added []DownloadRow
	upsertOwn := func(k string, p, c Job, isNew bool) {
		f := ownNumbers(c)
		if !isNew {
			f = changed(ownNumbers(p), f)
		}
		if len(f) == 0 {
			return
		}
		f["key"] = k
		mgr.Upsert = append(mgr.Upsert, f)
		for uid := range own.users[k] {
			if pp := per[uid]; pp != nil {
				pp.Upsert = append(pp.Upsert, f)
			}
		}
	}
	for _, ext := range ch.Added {
		k, c := keyOf(ext), cur[ext]
		switch {
		case own.keys[k]:
			upsertOwn(k, Job{}, c, true)
		case outsideShown:
			row := clientRow(d, c)
			row.LiveKey = k
			added = append(added, row)
		}
	}
	if len(added) > 0 {
		a.knownTitles(ctx, id, added)
		a.linkedTitles(ctx, id, added)
		a.rowOwners(ctx, added)
		for _, row := range added {
			f := rowFields(row)
			f["key"] = row.LiveKey
			mgr.Upsert = append(mgr.Upsert, f)
		}
	}
	for _, fc := range ch.Changed {
		k, p, c := keyOf(fc.ID), prev[fc.ID], cur[fc.ID]
		switch {
		case own.keys[k]:
			upsertOwn(k, p, c, false)
		case outsideShown:
			f := changed(rowFields(clientRow(d, p)), rowFields(clientRow(d, c)))
			if len(f) > 0 {
				f["key"] = k
				mgr.Upsert = append(mgr.Upsert, f)
			}
		}
	}
	for _, ext := range ch.Removed {
		// HolyArrs' own rows leave the page through the Manager's writes.
		if k := keyOf(ext); !own.keys[k] && outsideShown {
			mgr.Remove = append(mgr.Remove, k)
		}
	}
	key := strconv.FormatInt(id, 10)
	a.Live.PublishDL(key, mgr, nil)
	uids := make([]int64, 0, len(per))
	for uid := range per {
		uids = append(uids, uid)
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	for _, uid := range uids {
		a.Live.PublishDLUser(key, per[uid], uid)
	}
}

// dlTotals adds up every client's speeds in the reader's snapshots.
func (a *Web) dlTotals() *PatchTotals {
	t := &PatchTotals{}
	for id := range a.DLSync.Store.Revs() {
		for _, j := range a.DLSync.Store.Current(id).Items {
			if j.State == StateDownloading {
				t.Speed += j.Speed
			}
			t.UpSpeed += j.UpSpeed
		}
	}
	return t
}

// dlHealthSent remembers the health last sent per client.
type dlHealthSent struct {
	mu   sync.Mutex
	last map[int64]string
}

// PublishDLHealth is the publisher's Health: a health event (managers only)
// for each client whose health word or staleness changed.
func (a *Web) PublishDLHealth(ctx context.Context) {
	if a.Live == nil || !a.syncOn() {
		return
	}
	boot := a.Live.Boot()
	sab := a.DLSync.SABOutside(ctx)
	a.dlHealth.mu.Lock()
	defer a.dlHealth.mu.Unlock()
	if a.dlHealth.last == nil {
		a.dlHealth.last = map[int64]string{}
	}
	for _, d := range a.downloaderNames(ctx) {
		if d.Removed || !(d.Protocol == "torrent" || sab && d.Protocol == "usenet") {
			continue
		}
		h := syncHealth(d, a.DLSync.Store.Current(d.ID))
		sig := h.Health + "/" + strconv.FormatBool(h.Stale)
		if a.dlHealth.last[d.ID] == sig {
			continue
		}
		a.dlHealth.last[d.ID] = sig
		p := Patch{Kind: KindHealth, Boot: boot, Client: d.ID, Health: h.Health, Name: h.Name, Stale: h.Stale}
		if !h.SeenAt.IsZero() {
			t := h.SeenAt.UTC().Truncate(time.Second)
			p.SeenAt = &t
		}
		a.Live.PublishDL("h"+strconv.FormatInt(d.ID, 10), p, nil)
	}
}
