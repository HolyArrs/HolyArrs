package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// collisionDeezer is a fake Deezer with two artists named "Drake": 246791
// (the rapper, albums Views and Scorpion) and 555 (another Drake, album
// Simbiosis), listed first by the search.
func collisionDeezer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/artist":
			_, _ = w.Write([]byte(`{"data":[{"id":555,"name":"Drake","picture_xl":"https://img.test/other-drake.jpg"},
				{"id":246791,"name":"Drake","picture_xl":"https://img.test/drake.jpg"}]}`))
		case "/artist/555":
			_, _ = w.Write([]byte(`{"id":555,"name":"Drake","picture_xl":"https://img.test/other-drake.jpg"}`))
		case "/artist/246791":
			_, _ = w.Write([]byte(`{"id":246791,"name":"Drake","picture_xl":"https://img.test/drake.jpg","nb_fan":100}`))
		case "/artist/7":
			_, _ = w.Write([]byte(`{"id":7,"name":"No Pic","picture_xl":"https://img.test/images/artist//1000x1000.jpg"}`))
		case "/artist/555/albums":
			_, _ = w.Write([]byte(`{"data":[{"id":1,"title":"Simbiosis"}]}`))
		case "/artist/246791/albums":
			_, _ = w.Write([]byte(`{"data":[{"id":2,"title":"Views"},{"id":3,"title":"Scorpion"},{"id":4,"title":"Take Care (Deluxe)"}]}`))
		case "/artist/555/top", "/artist/246791/top":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/w/api.php":
			if r.URL.Query().Get("entity") == "Q33240" {
				_, _ = w.Write([]byte(`{"claims":{"P18":[{"rank":"normal","mainsnak":{"datavalue":{"value":"Drake July 2016.jpg"}}}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"claims":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestParseArtistURLs(t *testing.T) {
	ids := parseArtistURLs([]string{"https://www.wikidata.org/wiki/Q33240", "https://www.deezer.com/artist/246791",
		"https://www.deezer.com/album/1", "https://open.spotify.com/artist/x"})
	if ids.Deezer != 246791 || ids.Wikidata != "Q33240" {
		t.Fatalf("ids %+v", ids)
	}
	if ids := parseArtistURLs([]string{"https://www.deezer.com/en/artist/12/"}); ids.Deezer != 12 {
		t.Fatalf("ids %+v", ids)
	}
}

// TestPictureByIDsNameCollision: the picture comes from the Deezer id the
// MusicBrainz links name, never from another artist with the same name.
func TestPictureByIDsNameCollision(t *testing.T) {
	srv := collisionDeezer(t)
	d := &ArtistImages{BaseURL: srv.URL, WikidataBase: srv.URL + "/w/api.php", Gap: time.Millisecond, Timeout: time.Second}
	ctx := context.Background()
	if p := d.PictureByIDs(ctx, ArtistIDs{Deezer: 246791}); p != "https://img.test/drake.jpg" {
		t.Fatalf("picture %q", p)
	}
	// No Deezer picture: Wikidata's.
	p := d.PictureByIDs(ctx, ArtistIDs{Deezer: 7, Wikidata: "Q33240"})
	if !strings.HasPrefix(p, CommonsThumbBase+"/") || !strings.HasSuffix(p, "/600px-Drake_July_2016.jpg") {
		t.Fatalf("wikidata picture %q", p)
	}
	// Nothing by id: nothing, never a name search.
	if p := d.PictureByIDs(ctx, ArtistIDs{}); p != "" {
		t.Fatalf("picture without ids %q", p)
	}
}

func TestCommonsThumb(t *testing.T) {
	// md5("Drake_July_2016.jpg") starts with the folder letters.
	got := commonsThumb("Drake July 2016.jpg", 600)
	if !strings.HasPrefix(got, CommonsThumbBase+"/") || strings.Count(got, "Drake_July_2016.jpg") != 2 {
		t.Fatalf("thumb %q", got)
	}
	if commonsThumb("Logo.svg", 600) != "" || commonsThumb("", 600) != "" {
		t.Fatal("only photos")
	}
}

// TestProfileForNameCollision: with no Deezer link, a name search needs
// the exact name and album titles that agree with MusicBrainz.
func TestProfileForNameCollision(t *testing.T) {
	srv := collisionDeezer(t)
	d := &ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond, Timeout: time.Second}
	ctx := context.Background()
	p, err := d.ProfileFor(ctx, "Drake", 0, []string{"Views", "Scorpion", "Take Care", "Nothing Was the Same"})
	if err != nil || p.ID != 246791 || p.Picture != "https://img.test/drake.jpg" {
		t.Fatalf("profile %+v %v", p, err)
	}
	// Albums no candidate shares: no profile at all.
	if p, _ := d.ProfileFor(ctx, "Drake", 0, []string{"Something Else", "Another", "Third"}); p.ID != 0 {
		t.Fatalf("unsure match kept: %+v", p)
	}
	// The linked id wins over the search.
	if p, _ := d.ProfileFor(ctx, "Drake", 555, nil); p.ID != 555 {
		t.Fatalf("by id %+v", p)
	}
}
