package migrate

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// a SABnzbd or qBittorrent read with a typed key arrives at the typed
// address with that key, may reach the local network when that address is
// local, and is not paused; one with no kept key stays paused.
func TestImportedDownloaderCarriesTypedKey(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	v, err := secrets.Open(ctx, l.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	l.mv.Secrets = v
	const sabKey, qbitPass = "sab-test-key", "qbit-test-pass"
	if err := secretsfile.SaveMoveCredential(ctx, l.st, v, string(SABnzbd), secretsfile.MoveCredential{Address: "http://192.168.1.20:8585/", APIKey: sabKey, AllowLocalNetwork: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := secretsfile.SaveMoveCredential(ctx, l.st, v, string(QBittorrent), secretsfile.MoveCredential{Address: "http://10.0.0.5:8090", Username: "admin", Password: qbitPass, AllowLocalNetwork: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	r := &run{m: l.mv, ctx: ctx, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	r.move = newTestMove(t, l)
	items := []Item{
		{Key: "downloader:sab", Name: "SABnzbd", Downloader: &DownloaderPlan{Adapter: "sabnzbd", URL: "http://192.168.1.20:8585"}},
		{Key: "downloader:qbit", Name: "qBittorrent", Downloader: &DownloaderPlan{Adapter: "qbittorrent", URL: "http://10.0.0.5:8090", Username: "admin"}},
		{Key: "downloader:other", Name: "Other SAB", Downloader: &DownloaderPlan{Adapter: "sabnzbd", URL: "http://192.168.1.21:8080"}},
	}
	for _, it := range items {
		if got, why := r.one(GroupDownloaders, it); got != ResCreated {
			t.Fatalf("%s %s: %s", it.Name, got, why)
		}
	}
	check := func(name, want string, on bool) {
		t.Helper()
		var ref *int64
		var state string
		var local bool
		if err := l.st.Read().QueryRow("SELECT secret_ref, state, allow_local_network FROM downloaders WHERE name = ?", name).Scan(&ref, &state, &local); err != nil {
			t.Fatal(err)
		}
		if want == "" {
			if ref != nil || state != "disabled" {
				t.Fatalf("%s: key %v state %s, want no key and paused", name, ref, state)
			}
			return
		}
		if ref == nil {
			t.Fatalf("%s has no key", name)
		}
		if got, _ := v.Get(ctx, *ref); got != want {
			t.Fatalf("%s holds another key", name)
		}
		if state == "disabled" || local != on {
			t.Fatalf("%s: state %s local %v", name, state, local)
		}
	}
	check("SABnzbd", sabKey, true)
	check("qBittorrent", qbitPass, true)
	check("Other SAB", "", false)

	// Undo still removes a Downloader that only holds the key the import
	// carried over, and that key goes with it.
	var x madeRow
	var ref int64
	if err := l.st.Read().QueryRow("SELECT m.key, m.what, m.before, m.after, m.fingerprint, m.row_id, m.move_id, d.secret_ref FROM move_made m JOIN downloaders d ON d.id = m.row_id WHERE m.what = 'downloader' AND d.secret_ref IS NOT NULL ORDER BY d.secret_ref DESC LIMIT 1").
		Scan(&x.key, &x.what, &x.before, &x.after, &x.fp, &x.row, &x.move, &ref); err != nil {
		t.Fatal(err)
	}
	// The owner retypes the key: the carried row is dropped and a new one may
	// take the same id. Undo must then keep the Downloader and the new key.
	var reused int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		// Newer rows (saved move logins) go too, so the next id is ref again.
		if _, err := tx.Exec("DELETE FROM secrets WHERE id >= ?", ref); err != nil {
			return err
		}
		var err error
		reused, err = v.PutTx(tx, "downloader_secret", "typed-later", database.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if reused != ref {
		t.Fatalf("test setup: id not reused (%d vs %d)", reused, ref)
	}
	if res, why := l.mv.undoOne(ctx, x, domain.System); res != ResKept {
		t.Fatalf("undo with retyped key at reused id: %s %s", res, why)
	}
	if got, _ := v.Get(ctx, ref); got != "typed-later" {
		t.Fatal("typed key was removed")
	}
	// Put a fresh carried mark back to check the normal path.
	if err := l.st.Read().QueryRow("SELECT value FROM secrets WHERE id = ?", ref).Scan(&x.after); err != nil {
		t.Fatal(err)
	}
	x.after = carriedKeyMark(ref, x.after)
	if res, why := l.mv.undoOne(ctx, x, domain.System); res != ResRemoved {
		t.Fatalf("undo: %s %s", res, why)
	}
	var left int
	if err := l.st.Read().QueryRow("SELECT count(*) FROM secrets WHERE id = ?", ref).Scan(&left); err != nil || left != 0 {
		t.Fatalf("carried key left behind: %d %v", left, err)
	}
}

func newTestMove(t *testing.T, l *moveLab) int64 {
	t.Helper()
	var id int64
	if err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
		if err == nil {
			id, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
