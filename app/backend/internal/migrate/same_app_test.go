package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func sameAppHandlers(t *testing.T) (*webserver.Router, *database.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Secrets: v, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	return r, st, owner
}

type sameAppAnswer struct {
	Snapshot Saved   `json:"snapshot"`
	Replaced []int64 `json:"replaced"`
	Linked   []struct {
		Kind     Kind    `json:"kind"`
		Snapshot Saved   `json:"snapshot"`
		Replaced []int64 `json:"replaced"`
	} `json:"linked"`
}

func readAt(t *testing.T, r *webserver.Router, owner context.Context, kind, u, key string) sameAppAnswer {
	t.Helper()
	body := `{"kind":"` + kind + `","url":"` + u + `","key":"` + key + `","allow_local_network":true}`
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/read", strings.NewReader(body)).WithContext(owner))
	if rec.Code != http.StatusCreated {
		t.Fatalf("read %s: %d %s", u, rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secretMark) {
		t.Fatalf("a key reached the answer: %s", rec.Body.String())
	}
	var out sameAppAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func kept(t *testing.T, st *database.Store, kind Kind) []Saved {
	t.Helper()
	list, err := List(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	var out []Saved
	for _, s := range list {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

// The same Radarr typed at two addresses (same key) counts once: the second
// read replaces the first and names it, so the page updates that row.
func TestSameAppTwoAddressesCountsOnce(t *testing.T) {
	r, st, owner := sameAppHandlers(t)
	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_K", nil)
	other := strings.Replace(ru, "127.0.0.1", "localhost", 1)
	first := readAt(t, r, owner, "radarr", ru, rk)
	second := readAt(t, r, owner, "radarr", other, rk)
	if len(second.Replaced) != 1 || second.Replaced[0] != first.Snapshot.ID {
		t.Fatalf("replaced %v, want [%d]", second.Replaced, first.Snapshot.ID)
	}
	if got := kept(t, st, Radarr); len(got) != 1 || got[0].ID != second.Snapshot.ID {
		t.Fatalf("kept %+v, want only the new read", got)
	}
}

// Another Radarr (another key) at another address is a second copy.
func TestSameAppOtherKeyStays(t *testing.T) {
	r, st, owner := sameAppHandlers(t)
	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_A", nil)
	ou, ok := radarrAPIKeyed(t, "S3CR3T_RADARR_B", nil)
	readAt(t, r, owner, "radarr", ru, rk)
	if got := readAt(t, r, owner, "radarr", ou, ok); len(got.Replaced) != 0 {
		t.Fatalf("replaced %v, want none", got.Replaced)
	}
	if got := kept(t, st, Radarr); len(got) != 2 {
		t.Fatalf("kept %d radarr reads, want 2", len(got))
	}
}

// A Radarr typed by hand, then the same Radarr reached through Overseerr
// under another address: the linked read replaces the typed one.
func TestSameAppViaOverseerr(t *testing.T) {
	r, st, owner := sameAppHandlers(t)
	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_K", nil)
	typed := readAt(t, r, owner, "radarr", strings.Replace(ru, "127.0.0.1", "localhost", 1), rk)
	su := seerrWithArrs(t, "S3CR3T_SEERR_K", []any{}, []any{arrEntry(ru, rk)})
	got := readAt(t, r, owner, "overseerr", su, "S3CR3T_SEERR_K")
	if len(got.Linked) != 1 || len(got.Linked[0].Replaced) != 1 || got.Linked[0].Replaced[0] != typed.Snapshot.ID {
		t.Fatalf("linked %+v, want the typed read %d replaced", got.Linked, typed.Snapshot.ID)
	}
	if n := len(kept(t, st, Radarr)); n != 1 {
		t.Fatalf("kept %d radarr reads, want 1", n)
	}
}
