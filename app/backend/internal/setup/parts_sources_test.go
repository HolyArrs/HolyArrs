package setup

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// Turning Sources off asks with its own words: searches stop, while the
// Downloaders and imports carry on. No screen hides with it.
func TestSourcesOffAsksWithItsOwnWords(t *testing.T) {
	parts.Register()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var asked []string
	h := &PartHandlers{Parts: &parts.Control{Store: st, Reg: jobs.NewRegistry()},
		Actor: func(*http.Request) domain.Actor { return domain.System },
		Confirm: func(_ http.ResponseWriter, _ *http.Request, what string) bool {
			asked = append(asked, what)
			return false
		},
		Event: func(*http.Request, string, string, string) {}}
	m := http.NewServeMux()
	h.Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	for _, p := range []string{"sources", "music"} {
		resp, err := http.Post(srv.URL+"/api/v1/parts/"+p+"/off", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if len(asked) != 2 {
		t.Fatalf("asked %q", asked)
	}
	if !strings.Contains(asked[0], "Downloaders and imports keep working") || strings.Contains(asked[0], "screens hide") {
		t.Fatalf("Sources asks %q", asked[0])
	}
	if !strings.Contains(asked[1], "screens hide") {
		t.Fatalf("Music asks %q", asked[1])
	}
}
