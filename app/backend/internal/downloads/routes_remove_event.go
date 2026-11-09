package downloads

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// itemRemovedEvent writes the download_removed Event for an item taken out
// straight in a Downloader (Manager.Remove logs its own; this path skips
// it). It names the item when HolyArrs knows it.
func (a *Web) itemRemovedEvent(r *http.Request, downloaderID int64, key string, deleteFiles bool) {
	verb := "removed"
	if deleteFiles {
		verb = "removed (with its files)"
	}
	a.itemEvent(r, downloaderID, key, "download_removed", verb, map[string]any{"by_hand": true, "delete_files": deleteFiles, "hash": key})
}

// itemChangedEvent writes a download_changed Event for a change a person
// sent straight to a Downloader (pause, resume, a move, priority, Try
// again): who did it, what, and to which item.
func (a *Web) itemChangedEvent(r *http.Request, downloaderID int64, key, action, verb string) {
	a.itemEvent(r, downloaderID, key, "download_changed", verb, map[string]any{"by_hand": true, "action": action, "hash": key})
}

// itemChanged is itemChangedEvent with the Downloader id read from the path.
func (a *Web) itemChanged(r *http.Request, key, action, verb string) {
	if did, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err == nil {
		a.itemChangedEvent(r, did, key, action, verb)
	}
}

// manyChangedEvent writes one download_changed Event for a change sent to
// many items of one Downloader at once.
func (a *Web) manyChangedEvent(r *http.Request, downloaderID int64, keys []string, action, verb string, extra map[string]any) {
	if len(keys) == 1 {
		a.itemEvent(r, downloaderID, keys[0], "download_changed", verb, map[string]any{"by_hand": true, "action": action, "hash": keys[0]})
		return
	}
	ctx := context.WithoutCancel(r.Context())
	var client string
	_ = a.Store.Read().QueryRowContext(ctx, "SELECT name FROM downloaders WHERE id = ?", downloaderID).Scan(&client)
	data := map[string]any{"by_hand": true, "action": action, "count": len(keys), "downloader": client}
	for k, v := range extra {
		data[k] = v
	}
	_ = a.Store.AddEvent(ctx, &domain.Event{Actor: actor(r), Kind: "download_changed", Data: data,
		Summary: strings.TrimSpace(whoName(r) + " " + verb + " " + strconv.Itoa(len(keys)) + " items in " + client)})
}

func (a *Web) itemEvent(r *http.Request, downloaderID int64, key, kind, verb string, data map[string]any) {
	ctx := context.WithoutCancel(r.Context())
	db := a.Store.Read()
	var client, name string
	_ = db.QueryRowContext(ctx, "SELECT name FROM downloaders WHERE id = ?", downloaderID).Scan(&client)
	if client != "" {
		data["downloader"] = client
	}
	_ = db.QueryRowContext(ctx, "SELECT name FROM dl_items WHERE downloader_id = ? AND lower(key) = lower(?)", downloaderID, key).Scan(&name)
	e := &domain.Event{Actor: actor(r), Kind: kind, Data: data}
	var dlID int64
	if db.QueryRowContext(ctx, "SELECT id FROM downloads WHERE downloader_id = ? AND lower(external_id) = lower(?) ORDER BY id DESC LIMIT 1", downloaderID, key).Scan(&dlID) == nil {
		e.DownloadID = &dlID
		var rel string
		if db.QueryRowContext(ctx, "SELECT r.name FROM downloads d JOIN releases r ON r.id = d.release_id WHERE d.id = ?", dlID).Scan(&rel) == nil && name == "" {
			name = rel
		}
	}
	if name != "" {
		data["release"] = name
	} else {
		name = "an item"
	}
	where := " from "
	if kind != "download_removed" {
		where = " in "
	}
	e.Summary = strings.TrimSpace(whoName(r) + " " + verb + " " + name + where + client)
	_ = a.Store.AddEvent(ctx, e)
}
