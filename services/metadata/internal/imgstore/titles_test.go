package imgstore

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

func TestValidIDsAndFolders(t *testing.T) {
	for _, c := range []struct {
		kind, id string
		ok       bool
	}{{KindAlbum, albumID, true}, {KindAlbum, "603", false}, {KindTitleLogo, "movie-603", true}, {KindTitleWide, "tv-1399", true},
		{KindTitleBanner, "movie-0", false}, {KindTitleLogo, "book-1", false}, {KindPerson, "287", true}, {KindPerson, "../x", false}} {
		if ValidID(c.kind, c.id) != c.ok {
			t.Errorf("%s %s: want %v", c.kind, c.id, c.ok)
		}
	}
	s := &Store{o: Options{Dir: "/d"}}
	for id, want := range map[string]string{"movie-603": "title-logo/03/movie-603", "7": "title-logo/07/7", "42": "title-logo/42/42", "": "title-logo/00",
		albumID: "title-logo/b1/" + albumID} {
		if d := strings.ReplaceAll(s.Dir(KindTitleLogo, id), "\\", "/"); !strings.HasSuffix(d, want) {
			t.Errorf("folder of %q: %s, want .../%s", id, d, want)
		}
	}
	// Short and empty IDs never panic anywhere a picture is looked up.
	st, err := Open(Options{Dir: t.TempDir(), Chain: func(string, string) []Step { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, id := range []string{"7", "", "42"} {
		_, _, _, _ = st.Served(context.Background(), KindPerson, id, "500")
		_ = st.MakeBig(context.Background(), KindPerson, id)
		_ = st.Fetch(context.Background(), KindPerson, id)
	}
	if CommonsFile("https://upload.wikimedia.org/wikipedia/commons/thumb/a/ab/Brad_Pitt_2019_%28cropped%29.jpg/330px-Brad_Pitt_2019_%28cropped%29.jpg") != "Brad_Pitt_2019_(cropped).jpg" || CommonsFile("https://static.tvmaze.com/x.jpg") != "" {
		t.Fatal("Commons file name")
	}
}

func TestTitlePicturesFromFanart(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	logo := image.NewNRGBA(image.Rect(0, 0, 800, 310))
	logo.SetNRGBA(1, 1, color.NRGBA{255, 0, 0, 255})
	g.f.pics["/pic/800x310/tlogo.jpg"] = logo
	g.r.TitleFanart = func(_ context.Context, kind, tmdb string) ([]byte, error) {
		if kind != "movie" || tmdb != "603" {
			return nil, ErrNoPicture
		}
		return json.Marshal(map[string]any{"logo": g.f.pic(800, 310, "tlogo"), "banner": g.f.pic(1000, 185, "banner"), "backgrounds": []string{g.f.pic(3840, 2160, "bg")}})
	}
	for _, k := range []string{KindTitleLogo, KindTitleBanner, KindTitleWide} {
		if err := g.s.Fetch(ctx, k, "movie-603"); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
	if r, _ := g.s.Record(ctx, KindTitleWide, "movie-603"); r.Width != 1600 || r.Height != 900 || !r.Has(SlotTitleWide) {
		t.Fatalf("background shrunk to 1600: %+v", r)
	}
	if r, _ := g.s.Record(ctx, KindTitleBanner, "movie-603"); r.Width != 1000 || !r.Has(SlotBanner) {
		t.Fatalf("banner as it comes: %+v", r)
	}
	if r, _ := g.s.Record(ctx, KindTitleLogo, "movie-603"); r.Width != 800 || !r.Has(SlotLogo) {
		t.Fatalf("logo as it comes: %+v", r)
	}
	if err := g.s.Fetch(ctx, KindTitleLogo, "tv-1"); err != ErrNoPicture {
		t.Fatalf("no art: %v", err)
	}
}

func TestPersonPhotoFromCommonsWithAuthorAndLicense(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.r.CommonsAPI = g.f.srv.URL + "/commons"
	g.r.PersonPhoto = func(_ context.Context, tmdb string) (string, error) {
		if tmdb == "287" {
			return "Brad Pitt.jpg", nil
		}
		return "", nil
	}
	g.f.set("/commons?File:Brad Pitt.jpg", nil)
	g.f.api["/commons"] = map[string]any{"query": map[string]any{"pages": map[string]any{"1": map[string]any{"imageinfo": []any{map[string]any{
		"thumburl":    g.f.srv.URL + "/pic/500x625/500px-Brad.jpg",
		"extmetadata": map[string]any{"Artist": map[string]any{"value": `<a href="//commons.wikimedia.org/wiki/User:Jane">Jane&amp;Co</a>`}, "LicenseShortName": map[string]any{"value": "CC BY-SA 4.0"}}}}}}}}
	if err := g.s.Fetch(ctx, KindPerson, "287"); err != nil {
		t.Fatal(err)
	}
	r, _ := g.s.Record(ctx, KindPerson, "287")
	if r.Provider != "wikimedia" || r.Author != "Jane&Co" || r.License != "CC BY-SA 4.0" || !strings.Contains(r.BigURL, "/960px-") {
		t.Fatalf("record %+v", r)
	}
	if d := r.Recheck.Sub(r.Fetched); d > 31*24*time.Hour {
		t.Fatalf("Wikimedia checked monthly, got %v", d)
	}
	if err := g.s.Fetch(ctx, KindPerson, "1"); err != ErrNoPicture {
		t.Fatalf("no photo: %v", err)
	}
}
