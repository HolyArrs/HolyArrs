package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// An adopted movie (Radarr import or Add existing folder), kept up to
// date or not, gets its poster at once, with no refresh needed.
func TestAdoptStoresPoster(t *testing.T) {
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer pics.Close()
	s := open(t)
	lf := addFolder(t, s)
	if err := os.MkdirAll(filepath.Join(lf.Path, "Heat (1995)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lf.Path, "Heat (1995)", "Heat.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := &metadata.Static{Movies: []metadata.Movie{{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"}, Poster: pics.URL + "/t/p/w500/h.jpg"}}}
	svc := &Service{Store: s, Metadata: md, Now: func() time.Time { return now },
		Artwork: &ArtworkCache{Dir: t.TempDir(), Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}}
	res, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{{Folder: "Heat (1995)", TMDB: "949"}}, domain.System)
	if err != nil || len(res) != 1 || res[0].TitleID == 0 {
		t.Fatalf("adopt %+v %v", res, err)
	}
	var ti mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", res[0].TitleID, &ti); err != nil {
		t.Fatal(err)
	}
	if len(ti.Artwork) != 1 {
		t.Fatalf("artwork %v", ti.Artwork)
	}
}
