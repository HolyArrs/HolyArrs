package backups

import (
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// saveSecretAt puts one secret row in, saved at the given time.
func saveSecretAt(t *testing.T, s *database.Store, at time.Time) {
	t.Helper()
	ts := database.FormatTime(at)
	err := s.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO secrets(created_at, created_by, updated_at, updated_by, kind, value) VALUES (?, 'lab', ?, 'lab', 'source_api_key', 'x')", ts, ts)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A fresh install has nothing to protect, so the card does not ask; the
// first secret brings a calm recommendation; secrets plus backups past the
// wait bring a warning; a matching kit clears it.
func TestKeyKitReminderLevels(t *testing.T) {
	m, s, c, _ := setup(t)
	m.Key = labKey(5)
	if st := m.keyKitState(t.Context()); st.Needed || st.Level != "" {
		t.Fatalf("fresh install: %+v, want no reminder", st)
	}
	saveSecretAt(t, s, c.t)
	if st := m.keyKitState(t.Context()); !st.Needed || st.Level != secrets.KitRecommend {
		t.Fatalf("first secret: %+v, want a recommendation", st)
	}
	if _, err := m.Create(t.Context(), KindManual); err != nil {
		t.Fatal(err)
	}
	if st := m.keyKitState(t.Context()); st.Level != secrets.KitRecommend {
		t.Fatalf("secret and a new backup: %+v, still a recommendation", st)
	}
	c.t = c.t.Add(secrets.KitWarnAfter + time.Hour)
	if st := m.keyKitState(t.Context()); !st.Needed || st.Level != secrets.KitWarn {
		t.Fatalf("after the wait: %+v, want a warning", st)
	}
	if err := m.setSetting(t.Context(), settingKeyKit, m.KeyPrint(), domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	if st := m.keyKitState(t.Context()); st.Needed || st.Level != "" {
		t.Fatalf("kit saved: %+v, want cleared", st)
	}
}
