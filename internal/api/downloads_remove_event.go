package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// itemRemovedEvent writes the download_removed Event for an item taken out
// straight in a Downloader (Manager.Remove logs its own; this path skips
// it). It names the item when HolyArrs knows it.
func (a *API) itemRemovedEvent(r *http.Request, downloaderID int64, key string, deleteFiles bool) {
	ctx := context.WithoutCancel(r.Context())
	db := a.s.Store.Read()
	data := map[string]any{"by_hand": true, "delete_files": deleteFiles, "hash": key}
	var client, name string
	_ = db.QueryRowContext(ctx, "SELECT name FROM downloaders WHERE id = ?", downloaderID).Scan(&client)
	if client != "" {
		data["downloader"] = client
	}
	_ = db.QueryRowContext(ctx, "SELECT name FROM dl_items WHERE downloader_id = ? AND lower(key) = lower(?)", downloaderID, key).Scan(&name)
	e := &domain.Event{Actor: actor(r), Kind: "download_removed", Data: data}
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
	verb := "removed "
	if deleteFiles {
		verb = "removed (with its files) "
	}
	e.Summary = strings.TrimSpace(whoName(r) + " " + verb + name + " from " + client)
	_ = a.s.Store.AddEvent(ctx, e)
}
