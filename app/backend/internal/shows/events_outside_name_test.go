package shows

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// A key holder reads a deleted person's History rows as "name (deleted)".
func TestEventsNameDeletedPersonForKey(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', ?, 'member', '')`, "maria⁣2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddEvent(ctx, &database.Event{Kind: "ask_made", Actor: database.Actor{Kind: "user", ID: 2}}); err != nil {
		t.Fatal(err)
	}
	read := func(c context.Context) string {
		w := httptest.NewRecorder()
		listEvents(s, w, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(c))
		return w.Body.String()
	}
	if b := read(webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Role: webserver.RoleOwner, KeyID: 3})); strings.Contains(b, "⁣") || !strings.Contains(b, "maria (deleted)") {
		t.Fatalf("key read %s", b)
	}
	if b := read(webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Role: webserver.RoleOwner})); !strings.Contains(b, "maria⁣2") {
		t.Fatalf("app read %s", b)
	}
}

// A row that names askers by saved id gives their current names, so a deleted asker is marked
// while a new person with the same name is not. Older rows without ids get no names.
func TestEventsAskedNamesByID(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', ?, 'member', '')`, "maria⁣2"); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (3, 't', '{}', 't', '{}', 'maria', 'member', '')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*database.Event{
		{Kind: "ask_available", Summary: "Dune is ready for maria", Actor: database.Actor{Kind: "system"}, Data: map[string]any{"asked_by": []string{"maria"}, "asked_by_ids": []int64{2}}},
		{Kind: "ask_available", Summary: "Arrival is ready for maria", Actor: database.Actor{Kind: "system"}, Data: map[string]any{"asked_by": []string{"maria"}, "asked_by_ids": []int64{3}}},
		{Kind: "ask_available", Summary: "Heat is ready for maria", Actor: database.Actor{Kind: "system"}, Data: map[string]any{"asked_by": []string{"maria"}}},
	} {
		if err := s.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	read := func(c context.Context) string {
		w := httptest.NewRecorder()
		listEvents(s, w, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(c))
		return w.Body.String()
	}
	b := read(webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Role: webserver.RoleOwner}))
	if !strings.Contains(b, `"asked_names":["maria⁣2"]`) || !strings.Contains(b, `"asked_names":["maria"]`) || strings.Count(b, "asked_names") != 2 {
		t.Fatalf("app read %s", b)
	}
	if k := read(webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Role: webserver.RoleOwner, KeyID: 3})); !strings.Contains(k, `"asked_names":["maria (deleted)"]`) {
		t.Fatalf("key read %s", k)
	}
}
