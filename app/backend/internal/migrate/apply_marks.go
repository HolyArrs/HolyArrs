package migrate

import (
	"context"
	"database/sql"
	"strings"
)

// rowQuerier reads one row, in or outside a transaction.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// markFingerprintKind is the kind of marked row whose fingerprint a mark of
// kind what is checked against.
func markFingerprintKind(what string) string {
	if what == "music_policy" {
		return "title"
	}
	if what == "ask_claim" {
		return "ask"
	}
	return what
}

// sameMarkedCreation reports whether two fingerprints name the same
// created_at.
func sameMarkedCreation(marked, current string) bool {
	mi := strings.IndexByte(marked, 31)
	ci := strings.IndexByte(current, 31)
	return mi > 0 && ci > 0 && marked[:mi] == current[:ci]
}

// natural names, per kind of marked row, its table and the column that
// with created_at fingerprints it: SQLite gives a deleted row's id to the
// next row, so undo deletes a row only while its fingerprint still holds.
var natural = map[string][2]string{
	"ask": {"asks", "name"}, "user": {"users", "name"}, "alert": {"alerts", "name"}, "subtitle_site": {"subtitle_sites", "name"},
	"source": {"sources", "name"}, "downloader": {"downloaders", "name"}, "never_add": {"never_add", "name"},
	"title": {"titles", "kind"}, "folder": {"library_folders", "path"}, "outside_list": {"outside_lists", "name"},
	"pick_rules": {"pick_rules", "name"}, "own_trait": {"own_traits", "name"}, "turned_down": {"releases", "name"},
}

// markWith is markTx as an after-insert step of another package's save.
func (r *run) markWith(key, what string) func(tx *sql.Tx, id int64) error {
	return func(tx *sql.Tx, id int64) error { return r.markTx(tx, key, what, id, "", "") }
}
