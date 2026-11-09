package shows

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func animeTMDB(t *testing.T) metadata.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":16,"name":"Animation"}]}`))
		case "/3/tv/9000":
			_, _ = w.Write([]byte(`{"id":9000,"name":"Synthetic Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"original_language":"ja",
				"seasons":[{"season_number":1,"episode_count":12},{"season_number":2,"episode_count":24}]}`))
		case "/3/tv/9100":
			_, _ = w.Write([]byte(`{"id":9100,"name":"Other Cartoon","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"original_language":"ja"}`))
		case "/3/tv/9300":
			_, _ = w.Write([]byte(`{"id":9300,"name":"Kid Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-Y"}]}}`))
		case "/3/tv/9400":
			_, _ = w.Write([]byte(`{"id":9400,"name":"Grown Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"}]}}`))
		case "/3/tv/9200":
			_, _ = w.Write([]byte(`{"id":9200,"name":"Drama","genres":[],"origin_country":["US"],"original_language":"en"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// A Kid sees anime answers only for titles under their limit; a title
// above it, or with no rating, answers 403 before the server is asked.
func TestMetadataAnimeKidGate(t *testing.T) {
	t.Parallel()
	md := animeTMDB(t)
	var asked []string
	h := NewHandlers(Deps{Metadata: md, AnimeMap: func(_ context.Context, by, id string) (metadata.AnimeMap, error) {
		asked = append(asked, id)
		return metadata.AnimeMap{}, metadata.ErrNotFound
	},
		KindsOn:       func(kind string) []string { return []string{kind} },
		KidLimited:    func(http.ResponseWriter, *http.Request) bool { return false },
		LibraryRating: func(context.Context, string, string) (string, bool) { return "", false },
		Caching:       func(http.ResponseWriter, *http.Request, string) {},
		CachedDetails: func(string, string) *metadata.Details { return nil },
		Details: func(ctx context.Context, d metadata.Discoverer, kind, id string) (*metadata.Details, error) {
			det, err := d.Details(ctx, kind, id)
			return &det, err
		}})
	kid := webserver.Caller{UserID: 7, Role: webserver.RoleMember, MaxRating: "TV-Y7"}
	call := func(id string) int {
		r := httptest.NewRequest("GET", "/api/v1/metadata/show/"+id+"/anime", nil)
		r.SetPathValue("kind", "show")
		r.SetPathValue("tmdb_id", id)
		r = r.WithContext(webserver.ContextWith(r.Context(), kid))
		w := httptest.NewRecorder()
		h.Anime(w, r)
		return w.Code
	}
	if c := call("9300"); c != 200 {
		t.Fatalf("under the limit %d", c)
	}
	for _, id := range []string{"9400", "9100", "404404"} {
		if c := call(id); c != 403 {
			t.Fatalf("%s: %d", id, c)
		}
	}
	if len(asked) != 1 || asked[0] != "9300" {
		t.Fatalf("server asked %v", asked)
	}
}
