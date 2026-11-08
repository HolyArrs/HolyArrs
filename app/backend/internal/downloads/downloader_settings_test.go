package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Hooks passed to Add run in the insert's transaction: they see the new id,
// and a failing hook leaves no Downloader behind.
func TestAddThenHookSameTx(t *testing.T) {
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	in := Input{Name: "t", Adapter: "transmission", URL: "http://10.0.0.7:9091"}
	var got int64
	if _, err := Add(ctx, s, v, in, domain.System, func(tx *sql.Tx, id int64) error {
		got = id
		_, e := tx.Exec("UPDATE downloaders SET state='disabled' WHERE id=?", id)
		return e
	}); err != nil || got == 0 {
		t.Fatal(err, got)
	}
	var state string
	if err := s.Read().QueryRow("SELECT state FROM downloaders WHERE id=?", got).Scan(&state); err != nil || state != "disabled" {
		t.Fatal(state, err)
	}
	in.Name = "t2"
	if _, err := Add(ctx, s, v, in, domain.System, func(*sql.Tx, int64) error { return errors.New("boom") }); err == nil {
		t.Fatal("hook error lost")
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM downloaders WHERE name='t2'").Scan(&n); err != nil || n != 0 {
		t.Fatal("not rolled back", n, err)
	}
	// A secret stored in the same transaction is rolled back too.
	in.Name, in.Adapter, in.URL, in.Secret = "t3", "sabnzbd", "http://10.0.0.8:8080", "key-t3"
	if _, err := Add(ctx, s, v, in, domain.System, func(*sql.Tx, int64) error { return errors.New("boom") }); err == nil {
		t.Fatal("hook error lost")
	}
	vals, _ := v.Values(ctx)
	for _, x := range vals {
		if x == "key-t3" {
			t.Fatal("secret kept after rollback")
		}
	}
}
