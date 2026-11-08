package music

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeArtists is MusicBrainz over fixed answers.
type fakeArtists struct {
	mu        sync.Mutex
	artists   map[string]metadata.Artist
	pressings map[string][]metadata.Pressing
	err       error
	// block holds SearchArtists until closed; entered says one came in;
	// members records whether each search was a member's.
	block   chan struct{}
	entered chan struct{}
	members []bool
}

func (f *fakeArtists) SearchArtists(ctx context.Context, q string) ([]metadata.Artist, error) {
	f.mu.Lock()
	f.members = append(f.members, metadata.IsMemberLookup(ctx))
	block, entered := f.block, f.entered
	f.mu.Unlock()
	if block != nil {
		entered <- struct{}{}
		<-block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := []metadata.Artist{}
	for _, a := range f.artists {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(q)) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeArtists) Artist(_ context.Context, mbid string) (metadata.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return metadata.Artist{}, f.err
	}
	if a, ok := f.artists[mbid]; ok {
		return a, nil
	}
	return metadata.Artist{}, metadata.ErrArtistNotFound
}

func (f *fakeArtists) Pressings(_ context.Context, mbid string) ([]metadata.Pressing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pressings[mbid], f.err
}

// AlbumArtist finds the album among the fixed artists' albums.
func (f *fakeArtists) AlbumArtist(_ context.Context, album string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, a := range f.artists {
		for _, al := range a.Albums {
			if al.MBID == album {
				return id, nil
			}
		}
	}
	return "", metadata.ErrArtistNotFound
}

const (
	apiArtist = "a0000000-0000-0000-0000-0000000000aa"
	apiAlbum  = "10000000-0000-0000-0000-0000000000aa"
	apiLive   = "10000000-0000-0000-0000-0000000000ab"
	apiSoon   = "10000000-0000-0000-0000-0000000000ac"
	apiRelA   = "20000000-0000-0000-0000-0000000000aa"
	apiRelB   = "20000000-0000-0000-0000-0000000000ab"
)

func apiMusic() *fakeArtists {
	tr := func(p string, n int) []metadata.Track {
		out := []metadata.Track{}
		for i := 1; i <= n; i++ {
			out = append(out, metadata.Track{ID: fmt.Sprintf("%s-%d", p, i), RecordingID: fmt.Sprintf("rec-%d", i), Disc: 1, Number: i, Position: i,
				Name: fmt.Sprintf("Song %d", i), LengthMS: 180000})
		}
		return out
	}
	return &fakeArtists{
		artists: map[string]metadata.Artist{apiArtist: {MBID: apiArtist, Name: "Lab Band", Type: "group", Status: "running", Albums: []metadata.Album{
			{MBID: apiAlbum, Name: "First Light", PrimaryType: "album", ReleaseDate: "2020-03-06"},
			{MBID: apiLive, Name: "Live Night", PrimaryType: "album", SecondaryTypes: []string{"live"}, ReleaseDate: "2021"},
			{MBID: apiSoon, Name: "Soon", PrimaryType: "ep", ReleaseDate: "2099-01-01"},
		}}},
		pressings: map[string][]metadata.Pressing{apiAlbum: {
			{MBID: apiRelA, Name: "First Light", Status: "official", Country: "XW", Date: "2020-03-06", Formats: []string{"Digital Media"}, Tracks: tr("a", 2)},
			{MBID: apiRelB, Name: "First Light", Status: "official", Country: "US", Date: "2020", Disambiguation: "deluxe", Formats: []string{"CD"}, Tracks: tr("b", 3)},
		}},
	}
}

func musicSetup(t *testing.T) (*httptest.Server, *database.Store, *library.Service, *fakeArtists) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	fa := apiMusic()
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Artists: fa}
	srv := httptest.NewServer(New(Server{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}}).Handler())
	t.Cleanup(srv.Close)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"music","label":"Music"}`, t.TempDir()), 201)
	return srv, s, lib, fa
}

// route: GET /api/v1/artists/lookup
// route: POST /api/v1/artists
// route: GET /api/v1/titles/{id}/albums
// route: GET /api/v1/albums/{id}
// route: PUT /api/v1/albums/{id}
// route: GET /api/v1/albums/{id}/tracks
// route: PUT /api/v1/titles/{id}/artist
func TestMusicAPI(t *testing.T) {
	t.Parallel()
	srv, _, lib, fa := musicSetup(t)
	u := srv.URL + "/api/v1"
	ctx := context.Background()

	do(t, "GET", u+"/artists/lookup", "", 400)
	var look struct {
		Artists []ArtistResult `json:"artists"`
		Source  string         `json:"source"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/artists/lookup?term=lab", "", 200), &look)
	if look.Source != "musicbrainz" || len(look.Artists) != 1 || look.Artists[0].MBID != apiArtist || look.Artists[0].Albums != nil || look.Artists[0].TitleID != 0 ||
		look.Artists[0].Image != "/api/v1/music-trending/art/artist/"+apiArtist {
		t.Fatalf("lookup %+v", look)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/artists/lookup?term=mbid:"+apiArtist, "", 200), &look)
	if len(look.Artists) != 1 || look.Artists[0].Name != "Lab Band" || look.Artists[0].Albums != nil {
		t.Fatalf("lookup by id %+v", look)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/artists/lookup?term=mbid:"+apiAlbum, "", 200), &look)
	if len(look.Artists) != 0 {
		t.Fatalf("unknown id %+v", look)
	}

	do(t, "POST", u+"/artists", `{}`, 400)
	do(t, "POST", u+"/artists", `{"id":"tmdb:1"}`, 400)
	do(t, "POST", u+"/artists", `{"id":"mbid:`+apiArtist+`","albums":"existing"}`, 400)
	do(t, "POST", u+"/artists", `{"id":"mbid:`+apiAlbum+`"}`, 404)
	var added struct {
		Title struct {
			ID     int64       `json:"id"`
			Kind   string      `json:"kind"`
			Slots  []any       `json:"slots"`
			Artist *ArtistInfo `json:"artist"`
		} `json:"title"`
		JobID int64 `json:"job_id"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/artists", `{"id":"mbid:`+apiArtist+`","albums":"latest","new_albums":"new"}`, 201), &added)
	ar := added.Title.Artist
	if added.Title.Kind != "artist" || added.JobID == 0 || ar == nil || ar.AlbumChoice != "latest" || ar.NewAlbums != "new" || ar.Albums != 3 ||
		ar.Included != 2 || strings.Join(ar.AlbumTypes, ",") != "album,ep" || len(added.Title.Slots) != 0 {
		t.Fatalf("added %+v", added)
	}
	id := added.Title.ID
	do(t, "POST", u+"/artists", `{"id":"mbid:`+apiArtist+`"}`, 409)
	// The refresh the add queued fetches Pressings and tracks.
	if _, err := lib.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/artists/lookup?term=lab", "", 200), &look)
	if look.Artists[0].TitleID != id {
		t.Fatalf("lookup after add %+v", look)
	}

	var albums struct {
		Albums []library.AlbumSummary `json:"albums"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/albums", u, id), "", 200), &albums)
	if len(albums.Albums) != 3 || albums.Albums[0].Name != "Soon" {
		t.Fatalf("albums %+v", albums)
	}
	var first library.AlbumSummary
	for _, a := range albums.Albums {
		if a.MBID == apiAlbum {
			first = a
		}
	}
	// latest: First Light (2020) is the newest album that is out.
	if !first.KeepUpToDate || first.Tracks != 2 || first.Missing != 2 || first.State != "missing" || first.Pressings != 2 {
		t.Fatalf("First Light %+v", first)
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/albums?included=true", u, id), "", 200), &albums)
	if len(albums.Albums) != 2 {
		t.Fatalf("included albums %+v", albums)
	}
	do(t, "GET", u+"/titles/999/albums", "", 404)

	var one struct {
		Album     library.AlbumSummary `json:"album"`
		TitleID   int64                `json:"title_id"`
		Pressings []PressingView       `json:"pressings"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/albums/%d", u, first.ID), "", 200), &one)
	if one.TitleID != id || len(one.Pressings) != 2 || !one.Pressings[0].Chosen || one.Pressings[0].MBID != apiRelA || one.Pressings[0].ChosenBy != "auto" ||
		len(one.Pressings[1].Tracks) != 3 || one.Pressings[1].Disambiguation != "deluxe" {
		t.Fatalf("album %+v", one)
	}
	do(t, "GET", u+"/albums/999", "", 404)

	var trs struct {
		Tracks []TrackView[trackSlot] `json:"tracks"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/albums/%d/tracks", u, first.ID), "", 200), &trs)
	if len(trs.Tracks) != 2 || trs.Tracks[0].Name != "Song 1" || trs.Tracks[1].Number != 2 || len(trs.Tracks[0].Slots) != 1 ||
		trs.Tracks[0].Slots[0].State != "wanted" || trs.Tracks[0].Slots[0].StateLabel != "Missing" || *trs.Tracks[0].LengthMS != 180000 {
		t.Fatalf("tracks %+v", trs)
	}

	do(t, "PUT", fmt.Sprintf("%s/albums/%d", u, first.ID), `{}`, 400)
	do(t, "PUT", fmt.Sprintf("%s/albums/%d", u, first.ID), `{"pressing_id":99999}`, 404)
	var sum library.AlbumSummary
	_ = json.Unmarshal(do(t, "PUT", fmt.Sprintf("%s/albums/%d", u, first.ID), fmt.Sprintf(`{"pressing_id":%d}`, one.Pressings[1].ID), 200), &sum)
	if sum.Tracks != 3 || !strings.Contains(sum.Pressing, "deluxe") && !strings.Contains(sum.Pressing, "US, CD, 3 tracks") {
		t.Fatalf("after choosing the deluxe Pressing %+v", sum)
	}
	_ = json.Unmarshal(do(t, "PUT", fmt.Sprintf("%s/albums/%d", u, first.ID), `{"keep_up_to_date":false}`, 200), &sum)
	if sum.KeepUpToDate || sum.State != "not_kept" {
		t.Fatalf("after Keep up to date off %+v", sum)
	}
	var live library.AlbumSummary
	for _, a := range albums.Albums {
		if a.MBID == apiLive {
			live = a
		}
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/albums", u, id), "", 200), &albums)
	for _, a := range albums.Albums {
		if a.MBID == apiLive {
			live = a
		}
	}
	do(t, "PUT", fmt.Sprintf("%s/albums/%d", u, live.ID), `{"keep_up_to_date":true}`, 409)

	do(t, "PUT", fmt.Sprintf("%s/titles/%d/artist", u, id), `{}`, 400)
	do(t, "PUT", fmt.Sprintf("%s/titles/%d/artist", u, id), `{"album_types":["mixtape"]}`, 400)
	var changed struct {
		Title struct {
			Artist *ArtistInfo `json:"artist"`
		} `json:"title"`
		JobID int64 `json:"job_id"`
	}
	_ = json.Unmarshal(do(t, "PUT", fmt.Sprintf("%s/titles/%d/artist", u, id), `{"secondary_types":["studio","live"],"new_albums":"all"}`, 200), &changed)
	if changed.JobID == 0 || changed.Title.Artist.NewAlbums != "all" || strings.Join(changed.Title.Artist.SecondaryTypes, ",") != "studio,live" {
		t.Fatalf("artist choices %+v", changed)
	}
	if _, err := lib.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/albums?included=true", u, id), "", 200), &albums)
	if len(albums.Albums) != 3 {
		t.Fatalf("live album now included %+v", albums)
	}
	do(t, "PUT", u+"/titles/1/artist", `{"new_albums":"all"}`, 200)

	// The title view and list carry the artist without listing track Slots.
	var tv struct {
		Kind   string      `json:"kind"`
		Slots  []any       `json:"slots"`
		Artist *ArtistInfo `json:"artist"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d", u, id), "", 200), &tv)
	if tv.Kind != "artist" || len(tv.Slots) != 0 || tv.Artist == nil || tv.Artist.Tracks != 3 {
		t.Fatalf("title view %+v", tv)
	}

	// Albums are searched (stage 3.3).
	do(t, "POST", fmt.Sprintf("%s/titles/%d/search", u, id), "", 202)

	// MusicBrainz busy: lookups say so plainly.
	fa.mu.Lock()
	fa.err = metadata.ErrMusicBusy
	fa.mu.Unlock()
	// (A name searched before is answered from the store: a new one is asked.)
	do(t, "GET", u+"/artists/lookup?term=lab+busy", "", 503)

	// MusicBrainz answering oddly: plain words, no status code.
	fa.mu.Lock()
	fa.err = &metadata.MusicStatusError{Code: 500}
	fa.mu.Unlock()
	if b := do(t, "GET", u+"/artists/lookup?term=lab+odd", "", 502); strings.Contains(string(b), "500") || !strings.Contains(string(b), "could not answer") {
		t.Fatalf("502 words %s", b)
	}
}

// route: GET /api/v1/settings/music
// route: PUT /api/v1/settings/music
func TestMusicSettingsAPI(t *testing.T) {
	t.Parallel()
	srv, _, _, _ := musicSetup(t)
	u := srv.URL + "/api/v1/settings/music"
	var d library.MusicDefaults
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &d)
	if d.AlbumChoice != "all" || d.PressingRule != "common" || d.CacheHours != 24 || d.MusicBrainzURL != "" {
		t.Fatalf("defaults %+v", d)
	}
	_ = json.Unmarshal(do(t, "PUT", u, `{"pressing_rule":"earliest","musicbrainz_url":"https://mb.lan/","album_types":["album","single"]}`, 200), &d)
	if d.PressingRule != "earliest" || d.MusicBrainzURL != "https://mb.lan" || strings.Join(d.AlbumTypes, ",") != "album,single" || d.AlbumChoice != "all" {
		t.Fatalf("saved %+v", d)
	}
	if d.AllowLocal {
		t.Fatal("the local network is allowed by default")
	}
	_ = json.Unmarshal(do(t, "PUT", u, `{"musicbrainz_allow_local_network":true}`, 200), &d)
	if !d.AllowLocal || d.MusicBrainzURL != "https://mb.lan" {
		t.Fatalf("allow local %+v", d)
	}
	do(t, "PUT", u, `{"musicbrainz_url":"file:///etc"}`, 400)
	do(t, "PUT", u, `{"cache_hours":0}`, 400)
	do(t, "PUT", u, `{"release_statuses":[]}`, 400)
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &d)
	if d.PressingRule != "earliest" {
		t.Fatalf("a refused save changed settings %+v", d)
	}
}

// TestMusicNotSetUp: without MusicBrainz the music routes say so.
func TestMusicNotSetUp(t *testing.T) {
	t.Parallel()
	srv, _, lib, _ := musicSetup(t)
	lib.Artists = nil
	do(t, "GET", srv.URL+"/api/v1/artists/lookup?term=lab", "", 503)
	do(t, "POST", srv.URL+"/api/v1/artists", `{"id":"mbid:`+apiArtist+`"}`, 503)
}

// trackSlot is the part of a track's slot the tracks test reads.
type trackSlot struct {
	State      string `json:"state"`
	StateLabel string `json:"state_label"`
}
