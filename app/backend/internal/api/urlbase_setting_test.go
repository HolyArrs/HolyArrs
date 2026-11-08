package api

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// route: GET /api/v1/settings/url-base
// route: PUT /api/v1/settings/url-base
// A base saved in Settings is cleaned, refused when it is not a plain path,
// and waits for a restart; startup reads it back.
func TestURLBaseSetting(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	u := srv.URL + "/api/v1/settings/url-base"
	if b := string(do(t, "GET", u, "", 200)); !strings.Contains(b, `"saved":""`) || !strings.Contains(b, `"restart_needed":false`) {
		t.Fatalf("get %s", b)
	}
	do(t, "PUT", u, `{"url_base":"/a/../b"}`, 400)
	if b := string(do(t, "PUT", u, `{"url_base":"holyarrs/"}`, 200)); !strings.Contains(b, `"saved":"/holyarrs"`) || !strings.Contains(b, `"restart_needed":true`) {
		t.Fatalf("put %s", b)
	}
	if got := webserver.SavedURLBase(context.Background(), s.Read()); got != "/holyarrs" {
		t.Fatalf("startup reads %q", got)
	}
	do(t, "PUT", u, `{"url_base":""}`, 200)
	if got := webserver.SavedURLBase(context.Background(), s.Read()); got != "" {
		t.Fatalf("cleared reads %q", got)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE settings SET value = '"/a//b"' WHERE grp = 'this_app' AND key = 'url_base'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := webserver.SavedURLBase(context.Background(), s.Read()); got != "" {
		t.Fatalf("a bad saved base must be none, got %q", got)
	}
}
