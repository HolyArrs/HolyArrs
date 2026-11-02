package herovideo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

const artist = "a74b1b7f-71a5-4011-9441-d0b5e4122711"

const (
	drakePage   = "UCdrakedrakedrakedrakedr"
	kenshiPage  = "UCkenshikenshikenshikens"
	officialCh  = "UCofficialofficialoffici"
	topicCh     = "UCtopictopictopictopicto"
	wikidataURL = "https://www.wikidata.org/wiki/Q123"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// frame pictures: 120x90 grey JPEGs.
func jpegOf(fill func(x, y int) uint8) []byte { return jpegAt(95, fill) }

func jpegAt(quality int, fill func(x, y int) uint8) []byte {
	img := image.NewGray(image.Rect(0, 0, 120, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 120; x++ {
			img.SetGray(x, y, color.Gray{Y: fill(x, y)})
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: quality})
	return b.Bytes()
}

func flat(v uint8) []byte { return jpegOf(func(int, int) uint8 { return v }) }

func shade(x, y int) uint8 { return uint8(x*2 + y/3) }

var (
	gradient = jpegOf(shade)
	// The three frames of each kind of upload.
	frameSets = map[string][3][]byte{
		"video": {flat(20), flat(220), gradient}, // a real video: frames far apart
		// a cover over audio: the same picture, each frame encoded anew
		"still":   {jpegAt(95, shade), jpegAt(90, shade), jpegAt(85, shade)},
		"unclear": {flat(100), flat(108), flat(104)}, // between the two thresholds
		// frames that say nothing: YouTube's black stand-in, or one file three times
		"blank": {flat(0), flat(1), flat(0)},
		"same":  {gradient, gradient, gradient},
	}
)

// fake is YouTube Music, the Data API, the channel feeds, YouTube's
// pictures and Wikidata in one test server.
type fake struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	hits     map[string]int
	searches []string
	videoIDs [][]string
	// search answers per country (gl): fixture names ("" for none).
	search map[string]string
	pages  map[string][]byte
	// music, when set, is every YouTube Music answer's status; slow makes
	// them wait longer than the client.
	music   int
	slow    bool
	api     int // the Data API's status when set
	feed    int // every feed's status when set (YouTube refusing a datacenter address)
	feeds   map[string][]byte
	claims  map[string][]string
	shapes  map[string]string // tall, short, live, gone (default: a 16:9 video of 200 s)
	frames  map[string]string // still, unclear, gone, error (default: video)
	uploads map[string][]string
	sheets  []byte
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, hits: map[string]int{}, search: map[string]string{"US": "music_search_us.json", "JP": "music_search_jp.json"},
		pages:  map[string][]byte{drakePage: fixture(t, "music_artist.json"), kenshiPage: fixture(t, "music_artist.json"), "VLOLAKvideos": fixture(t, "music_videos.json")},
		feeds:  map[string][]byte{officialCh: fixture(t, "feed.xml"), topicCh: fixture(t, "feed_topic.xml")},
		claims: map[string][]string{}, shapes: map[string]string{}, frames: map[string]string{}, uploads: map[string][]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) hit(k string) {
	f.mu.Lock()
	f.hits[k]++
	f.mu.Unlock()
}

func (f *fake) count(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[k]
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	music, slow, apiStatus := f.music, f.slow, f.api
	f.mu.Unlock()
	switch p := r.URL.Path; {
	case strings.HasPrefix(p, "/youtubei/v1/"):
		var body struct {
			Query, Params, BrowseID string
			Context                 struct {
				Client struct{ ClientName, GL, HL string }
			}
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method != http.MethodPost || body.Context.Client.ClientName != "WEB_REMIX" {
			f.t.Errorf("YouTube Music call %s %+v", r.Method, body.Context.Client)
		}
		if slow {
			time.Sleep(300 * time.Millisecond)
		}
		if music != 0 {
			f.hit("music_error")
			w.WriteHeader(music)
			return
		}
		if strings.HasSuffix(p, "/search") {
			f.hit("search")
			f.mu.Lock()
			f.searches = append(f.searches, body.Context.Client.GL+"/"+body.Context.Client.HL+" "+body.Query)
			name := f.search[body.Context.Client.GL]
			f.mu.Unlock()
			if body.Params != artistsOnly {
				f.t.Errorf("search params %q", body.Params)
			}
			if name == "" {
				name = "music_search_none.json"
			}
			w.Write(fixture(f.t, name))
			return
		}
		f.hit("browse")
		f.mu.Lock()
		b, ok := f.pages[body.BrowseID]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Write(b)
	case p == "/yt/videos":
		f.hit("videos")
		if r.Header.Get("X-Goog-Api-Key") != "test-key" || strings.Contains(r.URL.RawQuery, "test-key") {
			f.t.Errorf("key not in a header only: %s", r.URL.RawQuery)
		}
		q := r.URL.Query()
		if q.Get("part") != "contentDetails,player" || q.Get("maxWidth") != "1000" {
			f.t.Errorf("videos call %s", r.URL.RawQuery)
		}
		if apiStatus != 0 {
			w.WriteHeader(apiStatus)
			return
		}
		ids := strings.Split(q.Get("id"), ",")
		f.mu.Lock()
		f.videoIDs = append(f.videoIDs, ids)
		f.mu.Unlock()
		var items []map[string]any
		for _, id := range ids {
			f.mu.Lock()
			kind := f.shapes[id]
			f.mu.Unlock()
			w, h, d := "1000", "563", "PT3M20S"
			switch kind {
			case "gone":
				continue
			case "tall":
				w, h = "316", "562"
			case "short":
				d = "PT45S"
			case "live":
				d = "P0D"
			}
			items = append(items, map[string]any{"id": id, "contentDetails": map[string]any{"duration": d}, "player": map[string]any{"embedWidth": w, "embedHeight": h}})
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	case p == "/yt/playlistItems":
		f.hit("playlistItems")
		ch := "UC" + strings.TrimPrefix(r.URL.Query().Get("playlistId"), "UU")
		f.mu.Lock()
		ids := f.uploads[ch]
		f.mu.Unlock()
		var items []map[string]any
		for _, id := range ids {
			items = append(items, map[string]any{"snippet": map[string]any{"title": "Upload " + id, "resourceId": map[string]any{"videoId": id}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	case p == "/feeds/videos.xml":
		ch := r.URL.Query().Get("channel_id")
		f.hit("feed:" + ch)
		f.mu.Lock()
		b, ok := f.feeds[ch]
		status := f.feed
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Write(b)
	case strings.HasPrefix(p, "/vi/"):
		parts := strings.Split(strings.TrimPrefix(p, "/vi/"), "/")
		id := parts[0]
		f.hit("frame:" + id)
		f.mu.Lock()
		kind := f.frames[id]
		f.mu.Unlock()
		switch kind {
		case "gone":
			w.WriteHeader(404)
			return
		case "error":
			w.WriteHeader(500)
			return
		case "":
			kind = "video"
		}
		n := map[string]int{"1.jpg": 0, "2.jpg": 1, "3.jpg": 2}[parts[1]]
		w.Write(frameSets[kind][n])
	case p == "/sheet.jpg":
		f.hit("sheet")
		w.Write(f.sheets)
	case p == "/w/api.php":
		f.hit("wikidata")
		item := r.URL.Query().Get("entity")
		if r.URL.Query().Get("action") != "wbgetclaims" || r.URL.Query().Get("property") != "P2397" {
			f.t.Errorf("wikidata %s", r.URL.RawQuery)
		}
		var claims []map[string]any
		f.mu.Lock()
		for _, ch := range f.claims[item] {
			claims = append(claims, map[string]any{"rank": "normal", "mainsnak": map[string]any{"datavalue": map[string]any{"value": ch, "type": "string"}}})
		}
		f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"claims": map[string]any{"P2397": claims}})
	default:
		w.WriteHeader(404)
	}
}

// fakeClock is the tests' time.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type env struct {
	f      *fake
	s      *Service
	clk    *fakeClock
	limits *imgstore.Limits
	subj   imgstore.Subject
	deep   []string
}

func newEnv(t *testing.T, subj imgstore.Subject, opts func(*Options)) *env {
	t.Helper()
	e := &env{f: newFake(t), clk: &fakeClock{t: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}, subj: subj}
	e.limits = imgstore.NewLimits(nil)
	e.limits.Now = e.clk.now
	o := Options{HTTP: e.f.srv.Client(), UserAgent: "test", Music: e.f.srv.URL, API: e.f.srv.URL + "/yt", Feeds: e.f.srv.URL,
		Thumbs: e.f.srv.URL, Wikidata: e.f.srv.URL, Key: "test-key", Limits: e.limits, Now: e.clk.now, Logf: t.Logf,
		Artist: func(context.Context, string) imgstore.Subject { return e.subj },
		Deep:   func(id string) { e.deep = append(e.deep, id) }}
	if opts != nil {
		opts(&o)
	}
	s, err := Open(filepath.Join(t.TempDir(), "v.sqlite"), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e.s = s
	return e
}

var drake = imgstore.Subject{Known: true, Name: "Drake", Country: "CA"}

func (e *env) answer(t *testing.T) Answer {
	t.Helper()
	b, ok := e.s.Get(context.Background(), artist)
	var a Answer
	if !ok || json.Unmarshal(b, &a) != nil {
		t.Fatalf("no answer stored: %s", b)
	}
	return a
}

func ids(a Answer) string {
	var out []string
	for _, v := range a.Videos {
		out = append(out, v.YouTubeID)
	}
	return strings.Join(out, ",")
}

func (e *env) verdict(t *testing.T, id string) string {
	t.Helper()
	var v string
	_ = e.s.db.QueryRow(`SELECT verdict FROM verdicts WHERE video=?`, id).Scan(&v)
	return v
}

func TestMusicSearchAndBrowseParsing(t *testing.T) {
	var search, page, list any
	for name, out := range map[string]*any{"music_search_us.json": &search, "music_artist.json": &page, "music_videos.json": &list} {
		if err := json.Unmarshal(fixture(t, name), out); err != nil {
			t.Fatal(err)
		}
	}
	got := searchArtists(search)
	if len(got) != 3 || got[0] != (musicArtist{"Drake Bell", "UCbellbellbellbellbellbe"}) || got[1] != (musicArtist{"DRAKE", drakePage}) || got[2].Name != "Kenshi Yonezu" {
		t.Fatalf("artists %+v", got)
	}
	shelf, more := videoShelf(page)
	if more != "VLOLAKvideos" || len(shelf) != 3 {
		t.Fatalf("shelf %+v more %q", shelf, more)
	}
	if shelf[0] != (cand{ID: "guestVideo1", Title: "Guest Spot", Line: "Rihanna & DRAKE", Label: "MUSIC_VIDEO_TYPE_OMV"}) || shelf[2].Label != labelUserUpload {
		t.Fatalf("shelf %+v", shelf)
	}
	vids := listVideos(list)
	if len(vids) != 7 {
		t.Fatalf("list %+v", vids)
	}
	if vids[0] != (cand{ID: "ownVideo001", Title: "Hit Song", Line: "DRAKE", Label: "MUSIC_VIDEO_TYPE_OMV", Seconds: 225}) {
		t.Fatalf("first %+v", vids[0])
	}
	if vids[1].Line != "DRAKE & Future" || vids[1].Seconds != 250 || vids[2].Seconds != 30 {
		t.Fatalf("second, third %+v %+v", vids[1], vids[2])
	}
	if n := clock("1:02:03"); n != 3723 {
		t.Fatalf("clock %d", n)
	}
}

// The whole order on one artist: YouTube Music's shelf then its list, the
// artist's own videos first; user uploads, audio titles and teasers
// dropped for free; stills dropped; an unclear one kept and queued.
func TestCollectFromYouTubeMusic(t *testing.T) {
	e := newEnv(t, drake, nil)
	e.f.frames["stillVid001"] = "still"
	e.f.frames["unclearVid1"] = "unclear"
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	a := e.answer(t)
	if got := ids(a); got != "ownVideo001,ownVideo002,unclearVid1,guestVideo1" {
		t.Fatalf("list %s", got)
	}
	if a.ArtistMBID != artist || a.Videos[0].Source != SourceYouTubeMusic || a.Videos[0].Title != "Hit Song" || a.Videos[0].Duration != 200 {
		t.Fatalf("first %+v", a.Videos[0])
	}
	for id, want := range map[string]string{"fanUpload01": VerdictUser, "audioVid001": VerdictTitle, "teaserVid01": VerdictShort,
		"stillVid001": VerdictStill, "unclearVid1": VerdictPending, "ownVideo001": VerdictVideo, "liveVideo01": "", "songTrack01": ""} {
		if got := e.verdict(t, id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	if len(e.deep) != 1 || e.deep[0] != "unclearVid1" {
		t.Fatalf("storyboard checks queued %v", e.deep)
	}
	if e.f.count("frame:fanUpload01")+e.f.count("frame:audioVid001")+e.f.count("frame:teaserVid01") != 0 {
		t.Fatal("videos dropped for free were still checked")
	}
	if e.f.count("videos") != 1 || len(e.f.videoIDs[0]) != 5 {
		t.Fatalf("videos calls %d %v", e.f.count("videos"), e.f.videoIDs)
	}
	if n, with := e.s.Count(ctx); n != 1 || with != 1 {
		t.Fatalf("count %d %d", n, with)
	}
}

func TestLabelFilterDropsUserUploads(t *testing.T) {
	e := newEnv(t, drake, nil)
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ids(e.answer(t)), "fanUpload01") || e.verdict(t, "fanUpload01") != VerdictUser {
		t.Fatalf("user upload kept: %s", ids(e.answer(t)))
	}
}

func TestShapeAndLengthFilter(t *testing.T) {
	e := newEnv(t, drake, nil)
	e.f.shapes["ownVideo002"] = "tall"
	e.f.shapes["guestVideo1"] = "short"
	e.f.shapes["unclearVid1"] = "live"
	e.f.shapes["stillVid001"] = "gone"
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	if got := ids(e.answer(t)); got != "ownVideo001,unclearVid1" {
		t.Fatalf("list %s", got)
	}
	for id, want := range map[string]string{"ownVideo002": VerdictTall, "guestVideo1": VerdictShort, "stillVid001": VerdictGone} {
		if got := e.verdict(t, id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	// A live or upcoming video has no length yet: kept, its shape unchecked.
	var shape int
	_ = e.s.db.QueryRow(`SELECT shape FROM verdicts WHERE video='unclearVid1'`).Scan(&shape)
	if shape != 0 || e.s.Stats(context.Background())["listed_shape_unchecked"] != 1 {
		t.Fatalf("live video shape %d, stats %v", shape, e.s.Stats(context.Background()))
	}
}

// One videos call per 50 videos, 1 point each from the shared quota.
func TestShapeCheckOneCallPer50(t *testing.T) {
	e := newEnv(t, drake, func(o *Options) { o.Max = 60 })
	var items []string
	for i := range 60 {
		id := fmt.Sprintf("manyVideo%02d", i)
		items = append(items, `{"musicResponsiveListItemRenderer": {"flexColumns": [{"musicResponsiveListItemFlexColumnRenderer": {"text": {"runs": [{"text": "Song `+id+`"}]}}}, {"musicResponsiveListItemFlexColumnRenderer": {"text": {"runs": [{"text": "Drake"}]}}}], "playlistItemData": {"videoId": "`+id+`"}}}`)
	}
	e.f.pages["VLOLAKvideos"] = []byte(`{"contents": {"twoColumnBrowseResultsRenderer": {"secondaryContents": {"sectionListRenderer": {"contents": [{"musicPlaylistShelfRenderer": {"contents": [` + strings.Join(items, ",") + `]}}]}}}}}`)
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("videos") != 2 || len(e.f.videoIDs[0]) != 50 || len(e.f.videoIDs[1]) != 12 {
		t.Fatalf("calls %d: %d and %d ids", e.f.count("videos"), len(e.f.videoIDs[0]), len(e.f.videoIDs[len(e.f.videoIDs)-1]))
	}
	if used := e.limits.Status()["youtube"].Used; used != 2 {
		t.Fatalf("quota used %d", used)
	}
}

func TestTitleFilter(t *testing.T) {
	for title, bad := range map[string]bool{
		"Song (Official Audio)": true, "Song (Audio)": true, "Song [Audio]": true, "Song (Official Visualizer)": true,
		"Song - Visualiser": true, "Song (Lyric Video)": true, "Song Lyrics": true, "Song (Official Video)": false,
		"Lucid Dreams (Directed by Cole Bennett) - Lyrical Lemonade": false, "Audiovisual Live": false,
	} {
		if got := badTitle.MatchString(title); got != bad {
			t.Errorf("%q: %v", title, got)
		}
	}
}

func TestStillCheckThresholds(t *testing.T) {
	score := func(kind string) float64 {
		var imgs []image.Image
		for _, b := range frameSets[kind] {
			img, _, err := image.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			imgs = append(imgs, img)
		}
		return FrameScore(imgs)
	}
	if s := score("still"); s >= StillBelow || verdictOf(s) != VerdictStill {
		t.Fatalf("still scored %.2f", s)
	}
	if s := score("video"); s <= VideoAbove || verdictOf(s) != VerdictVideo {
		t.Fatalf("video scored %.2f", s)
	}
	if s := score("unclear"); s < StillBelow || s > VideoAbove || verdictOf(s) != VerdictPending {
		t.Fatalf("unclear scored %.2f", s)
	}
	// Through the service: frames that say nothing go to the storyboard
	// check instead of counting as a still.
	e := newEnv(t, drake, nil)
	for id, kind := range map[string]string{"ownVideo001": "still", "ownVideo002": "blank", "guestVideo1": "same", "stillVid001": "video", "unclearVid1": "unclear"} {
		e.f.frames[id] = kind
	}
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"ownVideo001": VerdictStill, "ownVideo002": VerdictPending, "guestVideo1": VerdictPending, "stillVid001": VerdictVideo, "unclearVid1": VerdictPending} {
		if got := e.verdict(t, id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	if got := ids(e.answer(t)); got != "ownVideo002,stillVid001,unclearVid1,guestVideo1" {
		t.Fatalf("list %s", got)
	}
}

// YouTube Music has no such artist: the Wikidata channel is a " - Topic"
// channel and is skipped; the MusicBrainz channel's uploads are used (its
// feed, then one playlistItems call as the feed holds too few).
func TestTopicChannelsSkipped(t *testing.T) {
	sigur := imgstore.Subject{Known: true, Name: "Sigur Rós", Country: "IS", Links: []string{wikidataURL, "https://www.youtube.com/channel/" + officialCh}}
	e := newEnv(t, sigur, nil)
	e.f.search = map[string]string{}
	e.f.claims["Q123"] = []string{topicCh}
	e.f.uploads[officialCh] = []string{"chanVideo03"}
	e.f.uploads[topicCh] = []string{"topicTrack1"}
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	a := e.answer(t)
	if got := ids(a); got != "chanVideo01,chanVideo03,chanVideo02" {
		t.Fatalf("list %s", got)
	}
	if a.Videos[0].Source != SourceMusicBrainz || a.Videos[0].Title != "Sigur Rós - Glósóli (Official Video)" {
		t.Fatalf("first %+v", a.Videos[0])
	}
	if e.f.count("playlistItems") != 1 || e.verdict(t, "chanShort01") != VerdictTall || e.verdict(t, "chanAudio01") != VerdictTitle {
		t.Fatalf("playlistItems %d, short %q, audio %q", e.f.count("playlistItems"), e.verdict(t, "chanShort01"), e.verdict(t, "chanAudio01"))
	}
	if strings.Contains(ids(a), "topicTrack1") {
		t.Fatal("topic channel used")
	}
	var topic int
	_ = e.s.db.QueryRow(`SELECT topic FROM channels WHERE channel=?`, topicCh).Scan(&topic)
	if topic != 1 {
		t.Fatal("topic channel not marked")
	}
	// The daily feed reads the official channel only.
	e.clk.add(FeedEvery)
	if due, _ := e.s.Due(ctx, 10); len(due) != 1 {
		t.Fatalf("feed not due: %v", due)
	}
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("feed:"+topicCh) != 1 || e.f.count("feed:"+officialCh) != 2 {
		t.Fatalf("feeds read: topic %d, official %d", e.f.count("feed:"+topicCh), e.f.count("feed:"+officialCh))
	}
}

func TestWikidataChannelUsedFirst(t *testing.T) {
	sigur := imgstore.Subject{Known: true, Name: "Sigur Rós", Links: []string{"https://www.youtube.com/channel/" + topicCh, wikidataURL}}
	e := newEnv(t, sigur, nil)
	e.f.search = map[string]string{}
	e.f.claims["Q123"] = []string{officialCh}
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	if a := e.answer(t); len(a.Videos) == 0 || a.Videos[0].Source != SourceWikidata {
		t.Fatalf("answer %+v", a)
	}
}

func TestHomeCountryRetry(t *testing.T) {
	e := newEnv(t, imgstore.Subject{Known: true, Name: "米津玄師", Country: "JP"}, nil)
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.f.searches, "|"); got != "US/en 米津玄師|JP/ja 米津玄師" {
		t.Fatalf("searches %s", got)
	}
	if a := e.answer(t); len(a.Videos) == 0 || a.Videos[0].Source != SourceYouTubeMusic {
		t.Fatalf("answer %+v", a)
	}
	// No second search for a region or a missing country.
	for _, c := range []string{"", "XW", "US"} {
		e := newEnv(t, imgstore.Subject{Known: true, Name: "米津玄師", Country: c}, nil)
		_ = e.s.Collect(context.Background(), artist)
		if len(e.f.searches) != 1 {
			t.Fatalf("country %q: searches %v", c, e.f.searches)
		}
	}
}

func Test30DayRefresh(t *testing.T) {
	e := newEnv(t, drake, nil)
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	frames := e.f.count("frame:ownVideo001")
	e.clk.add(RefreshEvery - time.Hour)
	if due, _ := e.s.Due(ctx, 10); len(due) != 0 {
		t.Fatalf("due before 30 days: %v", due)
	}
	e.clk.add(time.Hour)
	if due, _ := e.s.Due(ctx, 10); len(due) != 1 || due[0] != artist {
		t.Fatalf("not due after 30 days: %v", due)
	}
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("search") != 2 {
		t.Fatalf("searched %d times", e.f.count("search"))
	}
	// Verdicts are kept for good: known videos are not checked again.
	if e.f.count("frame:ownVideo001") != frames || e.f.count("videos") != 1 {
		t.Fatalf("known videos checked again: frames %d, videos calls %d", e.f.count("frame:ownVideo001"), e.f.count("videos"))
	}
	if got := e.answer(t).FetchedAt; !got.Equal(e.clk.now()) {
		t.Fatalf("fetched at %v", got)
	}
}

// Past the day's quota no videos call is made: the videos are listed
// unchecked and checked once the quota is back.
func TestQuotaGuard(t *testing.T) {
	e := newEnv(t, drake, nil)
	e.limits.LoadQuota(e.clk.now().In(mustPacific(t)).Format("2006-01-02"), imgstore.YouTubeDailyUnits)
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("videos") != 0 {
		t.Fatal("videos call past the quota")
	}
	if got := ids(e.answer(t)); got != "ownVideo001,ownVideo002,stillVid001,unclearVid1,guestVideo1" {
		t.Fatalf("list %s", got)
	}
	if n := e.s.Stats(ctx)["listed_shape_unchecked"]; n != 5 {
		t.Fatalf("unchecked %d", n)
	}
	e.clk.add(2 * time.Hour)
	if due, _ := e.s.Due(ctx, 10); len(due) != 0 {
		t.Fatalf("due while the quota is spent: %v", due)
	}
	e.clk.add(recheckEvery)
	due, _ := e.s.Due(ctx, 10)
	if len(due) != 1 {
		t.Fatalf("unchecked videos not due the next day: %v", due)
	}
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("videos") != 1 || e.s.Stats(ctx)["listed_shape_unchecked"] != 0 {
		t.Fatalf("videos calls %d, stats %v", e.f.count("videos"), e.s.Stats(ctx))
	}
}

func mustPacific(t *testing.T) *time.Location {
	l, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Without a key the shape check is skipped (no call), the rest still runs.
func TestNoKeySkipsShapeCheck(t *testing.T) {
	e := newEnv(t, drake, func(o *Options) { o.Key = "" })
	if err := e.s.Collect(context.Background(), artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("videos") != 0 || len(e.answer(t).Videos) != 5 {
		t.Fatalf("videos calls %d, list %s", e.f.count("videos"), ids(e.answer(t)))
	}
	if due, _ := e.s.Due(context.Background(), 10); len(due) != 0 {
		t.Fatal("unchecked videos due without a key")
	}
}

func TestTransientErrorsNotStored(t *testing.T) {
	ctx := context.Background()
	for _, status := range []int{500, 503} {
		e := newEnv(t, drake, nil)
		e.f.music = status
		if err := e.s.Collect(ctx, artist); err == nil {
			t.Fatalf("%d: no error", status)
		}
		if _, ok := e.s.Get(ctx, artist); ok {
			t.Fatalf("%d stored as an answer", status)
		}
	}
	// A 429 pauses YouTube Music: the next job waits without a call.
	e := newEnv(t, drake, nil)
	e.f.music = 429
	if err := e.s.Collect(ctx, artist); err == nil {
		t.Fatal("429: no error")
	}
	if err := e.s.Collect(ctx, artist); !errors.Is(err, ErrPaused) || e.f.count("music_error") != 1 {
		t.Fatalf("after a 429: %v, %d calls", err, e.f.count("music_error"))
	}
	if _, ok := e.s.Get(ctx, artist); ok {
		t.Fatal("429 stored as an answer")
	}
	// A timeout.
	e = newEnv(t, drake, func(o *Options) { o.HTTP = &http.Client{Timeout: 100 * time.Millisecond} })
	e.f.slow = true
	if err := e.s.Collect(ctx, artist); err == nil {
		t.Fatal("timeout: no error")
	}
	if _, ok := e.s.Get(ctx, artist); ok {
		t.Fatal("timeout stored as an answer")
	}
	// The Data API failing, then a frame: nothing stored; once both answer
	// the list is built, the videos judged before not checked again.
	e = newEnv(t, drake, nil)
	e.f.api = 500
	if err := e.s.Collect(ctx, artist); err == nil {
		t.Fatal("videos 500: no error")
	}
	e.f.api = 0
	e.f.frames["ownVideo002"] = "error"
	if err := e.s.Collect(ctx, artist); err == nil {
		t.Fatal("frame 500: no error")
	}
	if _, ok := e.s.Get(ctx, artist); ok {
		t.Fatal("frame failure stored as an answer")
	}
	before := e.f.count("frame:ownVideo001")
	delete(e.f.frames, "ownVideo002")
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if len(e.answer(t).Videos) != 5 || e.f.count("frame:ownVideo001") != before {
		t.Fatalf("list %s, frames %d then %d", ids(e.answer(t)), before, e.f.count("frame:ownVideo001"))
	}
}

// Only a real empty result is stored as one.
func TestRealEmptyStored(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, imgstore.Subject{Known: true, Name: "Nobody Known"}, nil)
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if a := e.answer(t); len(a.Videos) != 0 {
		t.Fatalf("answer %+v", a)
	}
	if n, with := e.s.Count(ctx); n != 1 || with != 0 {
		t.Fatalf("count %d %d", n, with)
	}
	// YouTube Music switched off and no backup: nothing stored.
	e = newEnv(t, imgstore.Subject{Known: true, Name: "Drake"}, func(o *Options) { o.On = func(n string) bool { return n != "youtube_music" } })
	if err := e.s.Collect(ctx, artist); !errors.Is(err, ErrPaused) {
		t.Fatalf("off: %v", err)
	}
	if _, ok := e.s.Get(ctx, artist); ok {
		t.Fatal("switched-off source stored as no videos")
	}
	// The artist answer not stored yet: the job waits.
	e = newEnv(t, imgstore.Subject{}, nil)
	if err := e.s.Collect(ctx, artist); !errors.Is(err, ErrNoArtist) {
		t.Fatalf("no artist answer: %v", err)
	}
}

func TestKillSwitches(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, drake, func(o *Options) { o.On = func(n string) bool { return n != "youtube" } })
	if err := e.s.Collect(ctx, artist); err != nil || e.f.count("search") != 0 {
		t.Fatalf("youtube off: %v, %d searches", err, e.f.count("search"))
	}
	e = newEnv(t, drake, func(o *Options) { o.On = func(n string) bool { return n != "youtube_frames" && n != "youtube_api" } })
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("videos") != 0 || e.f.count("frame:ownVideo001") != 0 || len(e.answer(t).Videos) != 5 {
		t.Fatalf("checks ran while off: videos %d, frames %d, list %s", e.f.count("videos"), e.f.count("frame:ownVideo001"), ids(e.answer(t)))
	}
}

// A channel-listed artist: a new upload in the daily feed joins the list
// first; only it is checked.
func TestDailyFeedAddsNewUploads(t *testing.T) {
	sigur := imgstore.Subject{Known: true, Name: "Sigur Rós", Links: []string{"https://www.youtube.com/channel/" + officialCh}}
	e := newEnv(t, sigur, nil)
	e.f.search = map[string]string{}
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	e.f.feeds[officialCh] = bytes.Replace(fixture(t, "feed.xml"), []byte("<entry>"), []byte(`<entry><yt:videoId>chanVideo04</yt:videoId><title>Sigur Rós - New Song</title><link rel="alternate" href="https://www.youtube.com/watch?v=chanVideo04"/></entry><entry>`), 1)
	e.clk.add(FeedEvery)
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if got := ids(e.answer(t)); got != "chanVideo04,chanVideo01,chanVideo02" {
		t.Fatalf("list %s", got)
	}
	if e.f.count("frame:chanVideo01") != 3 || e.f.count("frame:chanVideo04") != 3 || e.f.count("videos") != 2 {
		t.Fatalf("frames %d %d, videos calls %d", e.f.count("frame:chanVideo01"), e.f.count("frame:chanVideo04"), e.f.count("videos"))
	}
}

// An artist found on YouTube Music: a new upload in its official channel's
// feed reads its YouTube Music page again; an upload seen before does not.
func TestDailyFeedRereadsYouTubeMusic(t *testing.T) {
	subj := drake
	subj.Links = []string{"https://www.youtube.com/channel/" + officialCh}
	e := newEnv(t, subj, nil)
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("browse") != 2 || e.f.count("feed:"+officialCh) != 0 {
		t.Fatalf("browse %d, feed %d", e.f.count("browse"), e.f.count("feed:"+officialCh))
	}
	e.clk.add(FeedEvery)
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("feed:"+officialCh) != 1 || e.f.count("browse") != 4 {
		t.Fatalf("new uploads: feed %d, browse %d", e.f.count("feed:"+officialCh), e.f.count("browse"))
	}
	e.clk.add(FeedEvery)
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if e.f.count("feed:"+officialCh) != 2 || e.f.count("browse") != 4 {
		t.Fatalf("nothing new: feed %d, browse %d", e.f.count("feed:"+officialCh), e.f.count("browse"))
	}
}

func TestMainActFirst(t *testing.T) {
	for _, tc := range []struct {
		c    cand
		main bool
	}{
		{cand{Line: "DRAKE & Future"}, true},
		{cand{Line: "Rihanna & DRAKE"}, false},
		{cand{Title: "Drake - Song (Official Video)"}, true},
		{cand{Title: "Rihanna, Drake - Work"}, false},
		{cand{Title: "Song (Official Video)"}, true},
		{cand{Title: "Work (feat. Drake)"}, false},
		{cand{Title: "Drake - Song (feat. Future)"}, true},
	} {
		if got := mainAct(tc.c, "Drake"); got != tc.main {
			t.Errorf("%+v: %v", tc.c, got)
		}
	}
}

func TestHeatmapReadOnceAndPausedOnError(t *testing.T) {
	calls := map[string]int{}
	e := newEnv(t, drake, func(o *Options) {
		o.HeatmapMax = 2
		o.Heatmap = func(_ context.Context, id string) (float64, float64, bool, error) {
			calls[id]++
			return 42, 200, id == "ownVideo001", nil
		}
	})
	ctx := context.Background()
	for range 2 {
		if err := e.s.Collect(ctx, artist); err != nil {
			t.Fatal(err)
		}
	}
	a := e.answer(t)
	if a.Videos[0].Start == nil || *a.Videos[0].Start != 42 || a.Videos[1].Start != nil || len(calls) != 2 || calls["ownVideo001"] != 1 {
		t.Fatalf("starts %+v, calls %v", a.Videos[:2], calls)
	}
	n := 0
	e = newEnv(t, drake, func(o *Options) {
		o.Heatmap = func(context.Context, string) (float64, float64, bool, error) {
			n++
			return 0, 0, false, context.DeadlineExceeded
		}
	})
	for range 2 {
		if err := e.s.Collect(ctx, artist); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 || len(e.answer(t).Videos) != 5 {
		t.Fatalf("heatmap tried %d times after a failure", n)
	}
}

// The storyboard check settles an unclear video: a still leaves the list.
func TestDeepCheckSettlesUnclear(t *testing.T) {
	var e *env
	e = newEnv(t, drake, func(o *Options) {
		o.VideoJSON = func(_ context.Context, id string) ([]byte, error) {
			return []byte(`{"formats":[{"format_id":"sb0","width":48,"columns":5,"rows":5,"fragments":[{"url":"` + e.f.srv.URL + `/sheet.jpg"}]},{"format_id":"18"}]}`), nil
		}
	})
	// One sheet of 25 identical tiles: no motion.
	tile := jpegOf(func(x, y int) uint8 { return uint8((x % 24) * 10) })
	e.f.sheets = tile
	e.f.frames["unclearVid1"] = "unclear"
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ids(e.answer(t)), "unclearVid1") {
		t.Fatal("unclear video not kept for now")
	}
	if err := e.s.DeepCheck(ctx, "unclearVid1"); err != nil {
		t.Fatal(err)
	}
	if e.verdict(t, "unclearVid1") != VerdictStill || strings.Contains(ids(e.answer(t)), "unclearVid1") || e.f.count("sheet") != 1 {
		t.Fatalf("verdict %q, list %s", e.verdict(t, "unclearVid1"), ids(e.answer(t)))
	}
	// A failed check is tried again a day later by the daily pass.
	e.f.frames["stillVid001"] = "unclear"
	_, _ = e.s.db.Exec(`DELETE FROM verdicts WHERE video='stillVid001'`)
	if err := e.s.rebuild(ctx, artist); err != nil {
		t.Fatal(err)
	}
	e.s.o.VideoJSON = func(context.Context, string) ([]byte, error) { return nil, errors.New("bot check") }
	if err := e.s.DeepCheck(ctx, "stillVid001"); err != nil {
		t.Fatal(err)
	}
	e.clk.add(recheckEvery)
	// While yt-dlp is paused no check is queued.
	e.s.pauseYTDLP()
	if _, deep := e.s.Due(ctx, 10); len(deep) != 0 {
		t.Fatalf("deep due while paused %v", deep)
	}
	e.clk.add(31 * time.Minute)
	if _, deep := e.s.Due(ctx, 10); len(deep) != 1 || deep[0] != "stillVid001" {
		t.Fatalf("deep due %v", deep)
	}
}

// YouTube refuses feed reads from a datacenter address at times (404 or
// 5xx): for a channel-listed artist that is never "no videos". With a key
// the uploads playlist stands in; without one nothing is stored and feed
// reads pause.
func TestFeedFailureNeverStoredAsEmpty(t *testing.T) {
	sigur := imgstore.Subject{Known: true, Name: "Sigur Rós", Links: []string{"https://www.youtube.com/channel/" + officialCh}}
	ctx := context.Background()
	e := newEnv(t, sigur, func(o *Options) { o.Key = "" })
	e.f.search = map[string]string{}
	e.f.feed = 404
	if err := e.s.Collect(ctx, artist); err == nil {
		t.Fatal("feed refused: no error")
	}
	if _, ok := e.s.Get(ctx, artist); ok {
		t.Fatal("a refused feed stored as no videos")
	}
	if err := e.s.Collect(ctx, artist); !errors.Is(err, ErrPaused) || e.f.count("feed:"+officialCh) != 1 {
		t.Fatalf("feed reads not paused: %v, %d reads", err, e.f.count("feed:"+officialCh))
	}
	e = newEnv(t, sigur, nil)
	e.f.search = map[string]string{}
	e.f.feed = 500
	e.f.uploads[officialCh] = []string{"chanVideo03"}
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if got := ids(e.answer(t)); got != "chanVideo03" {
		t.Fatalf("list from the uploads playlist %s", got)
	}
}

// A daily feed that cannot be read leaves the list as it is and is read
// again the next day.
func TestDailyFeedFailureKeepsList(t *testing.T) {
	sigur := imgstore.Subject{Known: true, Name: "Sigur Rós", Links: []string{"https://www.youtube.com/channel/" + officialCh}}
	e := newEnv(t, sigur, nil)
	e.f.search = map[string]string{}
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	before := ids(e.answer(t))
	e.f.feed = 404
	e.clk.add(FeedEvery)
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if got := ids(e.answer(t)); got != before {
		t.Fatalf("list changed: %s, was %s", got, before)
	}
	if due, _ := e.s.Due(ctx, 10); len(due) != 0 {
		t.Fatalf("feed due again the same day: %v", due)
	}
}
