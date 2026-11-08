package setup

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestOpenUntilTheWizardIsDone(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	s := New(st)

	if open, err := s.Open(ctx); err != nil || !open {
		t.Fatalf("fresh install: open=%v err=%v", open, err)
	}
	put := func(v string) {
		t.Helper()
		err := st.Write(ctx, func(tx *sql.Tx) error {
			return database.PutSetting(tx, "this_app", "first_run_done", v, database.System, time.Now())
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	put("0")
	if open, _ := s.Open(ctx); !open {
		t.Fatal("a value other than 1 counts as open")
	}
	put("1")
	if open, err := s.Open(ctx); err != nil || open {
		t.Fatalf("after done: open=%v err=%v", open, err)
	}
}
