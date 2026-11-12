package shows

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestPackNextRoute(t *testing.T) {
	var asked []int
	h := &SeasonHandlers{
		TitleID: func(http.ResponseWriter, *http.Request) (mediafiles.Title, bool) {
			return mediafiles.Title{Kind: "show"}, true
		},
		PackNext: func(_ context.Context, _ int64, season int, _ domain.Actor) (int64, int, error) {
			asked = append(asked, season)
			if season != 1 {
				return 0, 0, downloads.ErrNoPack
			}
			return 9, 2, nil
		},
	}
	m := http.NewServeMux()
	h.Routes(m)
	for _, c := range []struct {
		season string
		code   int
		body   string
	}{{"1", http.StatusOK, `"added":2`}, {"2", http.StatusNotFound, "no season pack"}} {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/titles/5/seasons/"+c.season+"/pack-next", nil))
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.body) {
			t.Fatalf("season %s: %d %s", c.season, rec.Code, rec.Body.String())
		}
	}
	if len(asked) != 2 {
		t.Fatalf("asked %v", asked)
	}
}
