package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// saveFileOf puts a file on disk for the kid rig's movie title and records
// it as the main file of its main Version, answering the save path.
func saveFileOf(t *testing.T, k *kidRig, title, body string) string {
	t.Helper()
	ctx := context.Background()
	id := k.ids[title]
	var v mediafiles.Version
	if err := k.st.Read().QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ?", id).Scan(&v.ID); err != nil {
		t.Fatal(err)
	}
	var slotID int64
	if err := k.st.Read().QueryRowContext(ctx, "SELECT id FROM slots WHERE version_id = ?", v.ID).Scan(&slotID); err != nil {
		t.Fatal(err)
	}
	root, err := mediafiles.LibraryPathOfVersion(ctx, k.st.Read(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	rel := title + "/" + title + ".mkv"
	if err := os.MkdirAll(filepath.Join(root, title), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &mediafiles.File{SlotIDs: []int64{slotID}, VersionID: v.ID, RelativePath: rel, Size: int64(len(body)), AddedAt: time.Now().UTC(),
		Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
	if err := k.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return "/api/v1/titles/" + itoa(id) + "/files/" + itoa(f.ID) + "/save"
}

// keyOf is the new key in an API key answer.
func keyOf(b []byte) string {
	var v struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(b, &v)
	return v.Key
}

// saveFetch sends one request as s (or with only the headers given when s is
// nil) and answers the status and body.
func saveFetch(t *testing.T, s *signedIn, base, method, path string, hdr map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, base+path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	c := http.DefaultClient
	if s != nil {
		c = s.c
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// route: GET /api/v1/titles/{id}/files/{fid}/save
// Saving a file to a device: off for everyone but the owner until given,
// never for a Kid (not even a title within their limit), an API key needs
// the read scope and the person's permission, and no plugin gets it.
func TestSaveFileRoute(t *testing.T) {
	k := newKidRig(t)
	base := k.srv.URL
	pg := saveFileOf(t, k, "Paper Kite", "kite bytes")
	r := saveFileOf(t, k, "Iron Verdict", "verdict bytes")

	if st, b := saveFetch(t, k.owner, base, "GET", pg, nil); st != 200 || b != "kite bytes" {
		t.Fatalf("owner: %d %s", st, b)
	}
	if st, b := saveFetch(t, k.owner, base, "HEAD", pg, nil); st != 200 || b != "" {
		t.Fatalf("owner HEAD: %d %s", st, b)
	}
	if st, _ := saveFetch(t, k.ana, base, "GET", pg, nil); st != 403 {
		t.Fatalf("member not given it: %d", st)
	}
	k.owner.call("PUT", "/api/v1/users/"+itoa(k.ana.id), `{"perms":["ask","save_files"]}`, 200)
	if st, b := saveFetch(t, k.ana, base, "GET", r, map[string]string{"Range": "bytes=0-6"}); st != 206 || b != "verdict" {
		t.Fatalf("member given it: %d %s", st, b)
	}

	// A Kid can't be given it, and even ticked by hand gets nothing.
	k.owner.call("PUT", "/api/v1/users/"+itoa(k.kai.id), `{"perms":["ask","save_files"]}`, 400)
	if err := k.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","save_files"]' WHERE id = ?`, k.kai.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{pg, r} {
		if st, b := saveFetch(t, k.kai, base, "GET", p, nil); st == 200 || strings.Contains(b, "bytes") {
			t.Fatalf("kid %s: %d %s", p, st, b)
		}
	}

	// API keys: the read scope and the person's own permission.
	readKey := keyOf(k.owner.call("POST", "/api/v1/auth/keys", `{"label":"read","scopes":["read"]}`, 201))
	writeKey := keyOf(k.owner.call("POST", "/api/v1/auth/keys", `{"label":"write","scopes":["write"]}`, 201))
	if st, b := saveFetch(t, nil, base, "GET", pg, map[string]string{accounts.APIKeyHeader: readKey}); st != 200 || b != "kite bytes" {
		t.Fatalf("read key: %d %s", st, b)
	}
	if st, _ := saveFetch(t, nil, base, "GET", pg, map[string]string{accounts.APIKeyHeader: writeKey}); st != 403 {
		t.Fatalf("write key: %d", st)
	}

	// A plugin is refused whatever it was given.
	tok := accounts.IssuePluginToken("saver", []string{"library.read", "library.change", "media.stream"})
	t.Cleanup(func() { accounts.RevokePluginToken("saver") })
	if st, b := saveFetch(t, nil, base, "GET", pg, map[string]string{"Authorization": "Bearer " + tok}); st != 403 || strings.Contains(b, "bytes") {
		t.Fatalf("plugin: %d %s", st, b)
	}

	// Each save started is on the audit trail, by ids: the owner's, ana's
	// (a range from the start) and the read key's.
	evs, _, err := k.st.Events(context.Background(), database.EventFilter{Kinds: []string{domain.EventFileSaved}})
	if err != nil || len(evs) != 3 {
		t.Fatalf("events: %v %d", err, len(evs))
	}
}

// With Movies off, a movie's file can't be saved either.
func TestSaveFileRouteFollowsTheMoviesPart(t *testing.T) {
	k := newKidRig(t, parts.Movies)
	p := saveFileOf(t, k, "Paper Kite", "kite bytes")
	if st, b := saveFetch(t, k.owner, k.srv.URL, "GET", p, nil); st == 200 || strings.Contains(b, "kite bytes") {
		t.Fatalf("movies off: %d %s", st, b)
	}
}
