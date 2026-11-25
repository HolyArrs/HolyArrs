package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Brought is one Downloader the old apps used, added here as soon as they
// were read, so the welcome Downloader step shows it already connected.
type Brought struct {
	ID int64 `json:"id"`
	// App is the old app it was found in ("sonarr").
	App    string `json:"app"`
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail"`
	// Kind is the client ("sabnzbd").
	Kind string `json:"kind"`
	// Needs is what the old app hid and a new row waits for: "key",
	// "password" or "token"; empty when nothing is missing.
	Needs string `json:"needs,omitempty"`
}

// BringDownloaders adds the Downloaders of the read apps right away, one per
// address (a client several apps share is added once, with each app's
// category), as a small finished import of its own so Undo works and a full
// import later finds them here. Each keeps the category its old app used; a
// client that does not answer yet is still added. Move keys are kept.
func (m *Mover) BringDownloaders(ctx context.Context, snaps []int64, by domain.Actor) ([]Brought, error) {
	// Two reads finishing together must not both miss the other's new client.
	bringMu.Lock()
	defer bringMu.Unlock()
	p, err := LoadPlan(ctx, m.Store, snaps, Choices{})
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, g := range p.Groups {
		if g.Name == GroupDownloaders {
			items = g.Items
		}
	}
	out := []Brought{}
	if len(items) == 0 {
		return out, nil
	}
	id, err := startQuickMove(ctx, m.Store, by.ID, snaps, m.now())
	if err != nil {
		return nil, err
	}
	r := &run{m: m, ctx: ctx, move: id, moveAt: m.now(), by: by, people: map[string]int64{}, personStable: map[string]bool{}, personMarked: map[string]bool{},
		rep: &Report{Move: id, Apps: p.Apps, At: m.now(), Lines: []Line{}, Counts: map[string]int{}}}
	for _, it := range items {
		res, detail := r.one(GroupDownloaders, it)
		needs := ""
		if res == ResCreated {
			needs = it.Needs
		}
		r.line(GroupDownloaders, it, res, detail, needs)
		b := Brought{App: strings.Fields(it.From + " ")[0], Name: it.Name, Result: res, Detail: detail, Kind: it.Downloader.Adapter}
		if needs != "" {
			b.Needs = strings.TrimPrefix(strings.TrimPrefix(needs, "your "), "secret ")
		}
		if h := r.sameDownloaderRow(it.Downloader); h != nil {
			b.ID, b.Name = h.ID, h.Name
		}
		out = append(out, b)
	}
	r.rep.Next = next(r.rep)
	if quickMoveEmpty(ctx, m.Store, id, r.rep) {
		// Nothing was added or changed: keep no empty import in the history.
		return out, dropQuickMove(ctx, m.Store, id)
	}
	return out, finishQuickMove(ctx, m.Store, id, r.rep, m.now())
}

var bringMu sync.Mutex

// quickMoveEmpty reports whether a quick move added and changed nothing.
func quickMoveEmpty(ctx context.Context, st *database.Store, id int64, rep *Report) bool {
	if rep.Counts[ResCreated] > 0 || rep.Counts[ResChanged] > 0 {
		return false
	}
	for _, l := range rep.Lines {
		if strings.Contains(l.Detail, " now ") {
			return false
		}
	}
	n, err := madeCount(ctx, st, id)
	return err == nil && n == 0
}

// sameDownloaderRow is the Downloader sameDownloader names, or nil.
func (r *run) sameDownloaderRow(d *DownloaderPlan) *domain.Downloader {
	have, err := database.Select[domain.Downloader](r.ctx, r.m.Store.Read(), "downloaders", "removed = 0 AND adapter = ? ORDER BY id", d.Adapter)
	if err != nil {
		return nil
	}
	want := domain.Downloader{Adapter: d.Adapter, URL: d.URL}
	for i := range have {
		if domain.SameClient(have[i], want) {
			return &have[i]
		}
	}
	return nil
}

// fillEmptyMaps gives the Downloader already here the old apps' folder maps
// when it has none yet; set ones are never changed. It says what it did, or "".
// The change is marked so Undo empties the maps again.
func (r *run) fillEmptyMaps(key string, d *DownloaderPlan, maps []domain.PathMap) string {
	if len(maps) == 0 {
		return ""
	}
	have := r.sameDownloaderRow(d)
	if have == nil || len(have.PathMap) > 0 {
		return ""
	}
	if r.dry {
		return "it will get the folder maps " + mapList(maps)
	}
	have.PathMap = maps
	after, _ := json.Marshal(maps)
	if err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		if err := database.Update(tx, "downloaders", have, r.by); err != nil {
			return err
		}
		return r.markTx(tx, key+"#maps", "downloader_maps", have.ID, "", string(after))
	}); err != nil {
		return ""
	}
	return "it now has the folder maps " + mapList(maps)
}

// undoDownloaderMaps empties the folder maps an import filled in, unless they
// were changed since.
func (m *Mover) undoDownloaderMaps(ctx context.Context, x madeRow, by domain.Actor) (string, string) {
	var have domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", x.row, &have); err != nil || have.Removed {
		return ResKept, "already gone"
	}
	cur, _ := json.Marshal(have.PathMap)
	if string(cur) != x.after {
		return ResKept, keptChanged
	}
	have.PathMap = []domain.PathMap{}
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "downloaders", &have, by) }); err != nil {
		return ResFailed, err.Error()
	}
	return ResRemoved, "the folder maps the import filled in are gone"
}

// downloaderAgain fills in what a Downloader imported before still lacks (its
// kinds' categories, then empty folder maps) and says what it did, or "".
func (r *run) downloaderAgain(it Item, id int64) string {
	added := r.addKindCategoriesTo(it.Downloader, id)
	maps, _ := safeMaps(it.Downloader.PathMap, r.m.Store.Dir())
	if filled := r.fillEmptyMaps(it.Key, it.Downloader, maps); filled != "" {
		added = strings.TrimPrefix(added+"; "+filled, "; ")
	}
	return added
}
