package migrate

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestCountDoneMoves(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if n, err := CountDoneMoves(ctx, st.Read()); err != nil || n != 0 {
		t.Fatalf("empty: %d %v", n, err)
	}
	now := database.FormatTime(time.Now().UTC())
	for _, s := range []string{"done", "done", "running", "undone"} {
		if err := st.Write(ctx, func(tx *sql.Tx) error {
			_, e := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,?)", now, s)
			return e
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := CountDoneMoves(ctx, st.Read()); err != nil || n != 2 {
		t.Fatalf("got %d %v", n, err)
	}
}
