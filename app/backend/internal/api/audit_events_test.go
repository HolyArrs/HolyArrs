package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Audit Events are left out of History, its count and its kind filters for
// people without users or settings rights and for read-scope keys.
func TestAuditEventsHiddenFromHistoryReaders(t *testing.T) {
	a, _ := signinRig(t, nil)
	ctx := context.Background()
	for _, k := range []string{"signed_in", "sign_in_failed", "settings_changed", "secrets_imported", "import_done"} {
		if err := a.s.Store.AddEvent(ctx, &domain.Event{Kind: k, Summary: k}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(who auth.Who, query string) (int, int) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/events"+query, nil)
		r = r.WithContext(auth.ContextWith(r.Context(), who))
		a.Handler().ServeHTTP(w, r)
		var out struct {
			Events []json.RawMessage `json:"events"`
			Total  int               `json:"total"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return len(out.Events), out.Total
	}
	member := auth.Who{UserID: 2, Role: "member", Perms: []auth.Perm{auth.PermManageLibrary}}
	readKey := auth.Who{UserID: 1, Role: auth.RoleOwner, Via: "api_key", Scopes: []string{auth.ScopeRead}}
	settings := auth.Who{UserID: 3, Role: "member", Perms: []auth.Perm{auth.PermSettings}}
	owner := auth.Who{UserID: 1, Role: auth.RoleOwner}
	for name, c := range map[string]struct {
		who         auth.Who
		query       string
		rows, total int
	}{
		"member":          {member, "", 1, 1},
		"member filtered": {member, "?kind=sign*,settings_changed", 0, 0},
		"read key":        {readKey, "", 1, 1},
		"settings person": {settings, "", 5, 5},
		"owner":           {owner, "", 5, 5},
		"owner filtered":  {owner, "?kind=sign*", 2, 2},
	} {
		if n, total := list(c.who, c.query); n != c.rows || total != c.total {
			t.Errorf("%s: %d rows, total %d", name, n, total)
		}
	}
}
