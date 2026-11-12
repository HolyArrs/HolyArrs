package parts

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Sources is a part of its own ("I add downloads myself"): it turns off and
// on without touching the media parts, needs none of them, and none needs
// it. Downloads and imports are core and keep running while it is off.
func TestSourcesPartSwitchesOnItsOwn(t *testing.T) {
	if !Valid(Sources) || Sources.Label() != "Sources" {
		t.Fatalf("Sources is not a registered part: %q", Sources.Label())
	}
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	c := &Control{Store: s, Reg: jobs.NewRegistry()}
	if err := c.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.On(Sources) {
		t.Fatal("Sources is off on a fresh install")
	}
	if err := c.Set(ctx, Sources, false, domain.System); err != nil {
		t.Fatalf("turning Sources off: %v", err)
	}
	if c.On(Sources) || !c.On(Movies) || !c.On(Shows) || !c.On(Music) || !c.On(Subtitles) || !c.On(Wishlist) {
		t.Fatalf("off now: %v", c.Off())
	}
	if !c.OwnerOn(Core) || !c.OwnerOn(Media) || c.KindOff("import_download") || c.KindOff("grab") {
		t.Fatal("downloads and imports stopped with Sources")
	}
	// Turning every media part off is still refused or allowed as before:
	// Sources plays no part in it.
	for _, p := range []Part{Wishlist, Subtitles, Music, Shows, Movies} {
		if err := c.Set(ctx, p, false, domain.System); err != nil {
			t.Fatalf("turning %s off with Sources off: %v", p, err)
		}
	}
	if err := c.Set(ctx, Sources, true, domain.System); err != nil {
		t.Fatalf("turning Sources on with every media part off: %v", err)
	}
	// The switch is kept for the next start.
	if err := c.Set(ctx, Sources, false, domain.System); err != nil {
		t.Fatal(err)
	}
	again := &Control{Store: s, Reg: jobs.NewRegistry()}
	if err := again.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if again.On(Sources) {
		t.Fatal("Sources came back on after a restart")
	}
}
