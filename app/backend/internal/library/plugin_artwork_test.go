package library

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func pngOf(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{R: uint8(x), G: 90, B: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSetPluginArtworkTable(t *testing.T) {
	s := open(t)
	var id int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune", SortName: "dune", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand",
			Artwork: []string{"poster:old.jpg", "backdrop:keep.jpg"}, Status: "released", State: "active"}
		_, err := database.Insert(tx, "titles", ti, domain.System)
		id = ti.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	svc := &Service{Store: s, Now: func() time.Time { return now }, Artwork: &ArtworkCache{Dir: dir}}
	good := pngOf(t, 300, 450)
	for _, c := range []struct {
		name, kind string
		pic        []byte
		ok         bool
	}{
		{"poster", "poster", good, true},
		{"backdrop", "backdrop", pngOf(t, 800, 450), true},
		{"bad kind", "logo", good, false},
		{"not a picture", "poster", []byte("<svg onload=alert(1)>"), false},
		{"too small", "poster", pngOf(t, 50, 50), false},
		{"empty", "poster", nil, false},
		{"huge header, never decoded", "poster", pngHeader(8000, 8000), false},
		{"wider than 4000", "poster", pngHeader(4001, 1000), false},
		{"taller than 4000", "poster", pngHeader(1000, 4001), false},
		{"too big", "poster", append(append([]byte{}, good...), make([]byte, pluginArtMax)...), false},
	} {
		err := svc.SetPluginArtwork(context.Background(), id, c.kind, c.pic, domain.System)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.ok && c.kind == "poster" && err != nil && !errors.Is(err, ErrBadArtwork) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	art, _ := mediafiles.TitleArtwork(context.Background(), s.Read(), id)
	if len(art) != 2 || art[0][:9] != "backdrop:" || art[1][:7] != "poster:" || art[1] == "poster:old.jpg" {
		t.Fatalf("artwork %v", art)
	}
	name := art[1][7:]
	if _, ok := svc.Artwork.Path(name); !ok {
		t.Fatal("stored picture not served")
	}
	// The cache trim never drops a plugin's picture.
	svc.Artwork.SetCap(1)
	svc.Artwork.trim()
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Error("trim removed a plugin's picture")
	}
}

// pngHeader is a PNG that claims w x h but has no picture data: a full
// decode would fail or eat memory, so it must be refused from the header.
func pngHeader(w, h int) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	ihdr := []byte{0, 0, 0, 13, 'I', 'H', 'D', 'R', byte(w >> 24), byte(w >> 16), byte(w >> 8), byte(w), byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h), 8, 6, 0, 0, 0}
	b.Write(ihdr)
	crc := crc32.ChecksumIEEE(ihdr[4:])
	b.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})
	return b.Bytes()
}

func TestHugePictureNeverDecoded(t *testing.T) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(pngHeader(8000, 8000)))
	if err != nil || cfg.Width != 8000 {
		t.Fatalf("test picture header: %v %v", cfg, err)
	}
	svc := &Service{Artwork: &ArtworkCache{Dir: t.TempDir()}}
	if err := svc.SetPluginArtwork(context.Background(), 1, "poster", pngHeader(8000, 8000), domain.System); !errors.Is(err, ErrBadArtwork) {
		t.Fatalf("huge picture: %v", err)
	}
}
