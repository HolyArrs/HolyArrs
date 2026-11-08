package discover

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A title qualifies only with a textless backdrop at least 1920 wide and a PNG logo.
func TestPickSigninShot(t *testing.T) {
	t.Parallel()
	en := "en"
	b := metadata.ImageBase + "/w780/"
	logo := []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l.png", Lang: &en}}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Lang: &en, Width: 3840, Height: 2160}}, Logos: logo}); ok {
		t.Fatal("a titled backdrop qualified")
	}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Width: 1280, Height: 720}}, Logos: logo}); ok {
		t.Fatal("a small backdrop qualified")
	}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Width: 1920, Height: 1080}}}); ok {
		t.Fatal("a title without a logo qualified")
	}
	s, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "s.jpg", Width: 1920, Height: 1080}, {Path: b + "w.jpg", Width: 3840, Height: 2160}}, Logos: logo})
	if !ok || s.Backdrop.URL != metadata.ImageBase+"/original/w.jpg" || s.Backdrop.W != 3840 {
		t.Fatalf("want the widest original: %+v", s)
	}
	s, _ = pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "sq.jpg", Width: 3840, Height: 2400}, {Path: b + "hd.jpg", Width: 2560, Height: 1440}}, Logos: logo})
	if s.Backdrop.URL != metadata.ImageBase+"/original/hd.jpg" {
		t.Fatalf("want the 16:9 one: %+v", s)
	}
}
