package imgstore

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// silhouette is a near-blank default picture: a grey square with a darker
// disk, the way a source draws "no photo".
func silhouette(w int) image.Image {
	img := flat(w, w, 200).(*image.NRGBA)
	r := w / 4
	for y := range w {
		for x := range w {
			if dx, dy := x-w/2, y-w/2; dx*dx+dy*dy < r*r {
				i := img.PixOffset(x, y)
				img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 120, 120, 120
			}
		}
	}
	return img
}

func pngOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// An artist photo that is a blank default picture, or byte for byte one of
// the artist's album covers, is refused at store time and the chain tries
// the next source; Purge finds the ones stored before and drops them.
func TestArtistPhotoPlaceholderAndCoverRefused(t *testing.T) {
	pics := map[string][]byte{
		"/blank.png": pngOf(t, silhouette(500)),
		"/cover.png": pngOf(t, picture(500, 500, 7)),
		"/real.png":  pngOf(t, picture(500, 500, 9)),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := pics[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	const artist, album = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	chain := map[string][]string{} // kind|id -> picture paths, in chain order
	s, err := Open(Options{Dir: t.TempDir(), HTTP: srv.Client(), Logf: func(string, ...any) {},
		Chain: func(kind, id string) []Step {
			var steps []Step
			for _, p := range chain[kind+"|"+id] {
				steps = append(steps, Step{Name: p, Get: func(context.Context) ([]Candidate, error) {
					return []Candidate{{Provider: strings.Trim(p, "/.png"), URL: srv.URL + p}}, nil
				}})
			}
			return steps
		},
		AlbumsOf: func(_ context.Context, id string) []string {
			if id == artist {
				return []string{album}
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	chain[KindAlbum+"|"+album] = []string{"/cover.png"}
	if err := s.Fetch(ctx, KindAlbum, album); err != nil {
		t.Fatal(err)
	}
	chain[KindArtist+"|"+artist] = []string{"/blank.png", "/cover.png", "/real.png"}
	if err := s.Fetch(ctx, KindArtist, artist); err != nil {
		t.Fatal(err)
	}
	if r, ok := s.Record(ctx, KindArtist, artist); !ok || r.Provider != "real" {
		t.Fatalf("artist photo from %q, want the real one after the blank and the cover", r.Provider)
	}

	// Pictures stored before the check: Purge finds and drops them, with a
	// backup of each file.
	const other = "33333333-3333-4333-8333-333333333333"
	chain[KindArtist+"|"+other] = []string{"/real.png"}
	if err := s.Fetch(ctx, KindArtist, other); err != nil {
		t.Fatal(err)
	}
	blank, err := EncodeWebP(Resize(silhouette(500).(*image.NRGBA), 500))
	if err != nil {
		t.Fatal(err)
	}
	cover, err := os.ReadFile(s.SlotPath(KindAlbum, album, SlotCard))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.SlotPath(KindArtist, other, SlotCard), blank, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.SlotPath(KindArtist, artist, SlotCard), cover, 0o644); err != nil {
		t.Fatal(err)
	}
	backup := t.TempDir()
	var requeued []string
	got, err := s.PurgeBadArtistPhotos(ctx, backup, func(id string) { requeued = append(requeued, id) })
	if err != nil || got.Placeholders != 1 || got.Covers != 1 {
		t.Fatalf("purge %+v %v", got, err)
	}
	if _, ok := s.Record(ctx, KindArtist, other); ok {
		t.Fatal("blank photo still stored")
	}
	if len(requeued) != 2 {
		t.Fatalf("requeued %v", requeued)
	}
	files, _ := filepath.Glob(filepath.Join(backup, "*", "*"))
	if len(files) < 2 {
		t.Fatalf("backup files %v", files)
	}
	if _, err := os.Stat(filepath.Join(backup, "rows.json")); err != nil {
		t.Fatalf("backup rows: %v", err)
	}
}
