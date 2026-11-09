package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// safeItem is one Release a fake Source answers with: its name, size and when it was posted.
type safeItem struct {
	name string
	size int64
	pub  time.Time
}

// TestReleaseSafety_MovieEarlyAndTinyRefused checks that a movie Release posted before the movie came out, and one far too small, are refused with plain reasons while the real one is chosen.
func TestReleaseSafety_MovieEarlyAndTinyRefused(t *testing.T) {
	out := time.Now().UTC().AddDate(0, -1, 0)
	early, tiny, real := "Dune.Part.Two.2024.2160p.WEB-DL.H265-EARLY", "Dune.Part.Two.2024.2160p.WEB-DL.H265-TINY", "Dune.Part.Two.2024.1080p.WEB-DL.H264-HOLYARRS"
	url := safeServer(t, []safeItem{{early, 9 << 30, out.AddDate(0, 0, -10)}, {tiny, 5 << 20, out.AddDate(0, 0, 5)}, {real, 6 << 30, out.AddDate(0, 0, 5)}})
	f := seed(t, url)
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET cinema_date = ?, digital_date = ?", out.Format(time.RFC3339Nano), out.AddDate(0, 0, 20).Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	if j := runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	wantRefused(t, f, early, "posted_early")
	wantRefused(t, f, tiny, "too_small")
	if r := relByName(t, f, real); r.State != "chosen" {
		t.Fatalf("the real Release is %s %v", r.State, reasonCodes(r))
	}
}

// TestReleaseSafety_EpisodeEarlyAndTinyRefused checks that an episode Release posted before it aired, and a tiny one, are refused while the real one is chosen.
func TestReleaseSafety_EpisodeEarlyAndTinyRefused(t *testing.T) {
	aired := time.Now().UTC().AddDate(0, 0, -7)
	early, tiny, real := "Copper.Valley.S02E01.1080p.WEB-DL.H.264-EARLY", "Copper.Valley.S02E01.1080p.WEB-DL.H.264-TINY", "Copper.Valley.S02E01.1080p.WEB-DL.H.264-QUILL"
	url := safeServer(t, []safeItem{{early, 1200 << 20, aired.AddDate(0, 0, -3)}, {tiny, 2 << 20, aired.AddDate(0, 0, 1)}, {real, 1200 << 20, aired.AddDate(0, 0, 1)}})
	f := seedShow(t, url, nil)
	sr := &Searcher{Store: f.s}
	if j := runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E01"]}, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	wantRefused(t, f.fixture, early, "posted_early")
	wantRefused(t, f.fixture, tiny, "too_small")
	if r := relByName(t, f.fixture, real); r.State != "chosen" {
		t.Fatalf("the real Release is %s %v", r.State, reasonCodes(r))
	}
}

// TestReleaseSafety_AlbumEarlyAndTinyRefused checks that an album Release posted before the album came out, and one too small for its tracks, are refused while the real one is chosen.
// With the switches off the early one counts again.
func TestReleaseSafety_AlbumEarlyAndTinyRefused(t *testing.T) {
	out := time.Date(2019, 5, 1, 0, 0, 0, 0, time.UTC)
	early, tiny, real := "Velvet Cartography - Low Tide Atlas (2019) [FLAC] [WEB]", "Velvet Cartography - Low Tide Atlas (2019) [MP3 320] [WEB]", flac
	url := safeServer(t, []safeItem{{early, 500 << 20, out.AddDate(0, -1, 0)}, {tiny, 1 << 20, out.AddDate(0, 0, 2)}, {real, 400 << 20, out.AddDate(0, 0, 2)}})
	f := seedArtist(t, url)
	sr := &Searcher{Store: f.s}
	if j := f.albumJob(t, sr, lta, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	wantRefused(t, f.fixture, early, "posted_early")
	wantRefused(t, f.fixture, tiny, "too_small")
	if r := relByName(t, f.fixture, real); r.State != "chosen" {
		t.Fatalf("the real Release is %s %v", r.State, reasonCodes(r))
	}
}

// TestReleaseSafety_EarlyCheckOff checks that the switches turn the early and size checks off.
func TestReleaseSafety_EarlyCheckOff(t *testing.T) {
	aired := time.Now().UTC().AddDate(0, 0, -7)
	early := "Copper.Valley.S02E01.1080p.WEB-DL.H.264-EARLY"
	f := seedShow(t, safeServer(t, []safeItem{{early, 1200 << 20, aired.AddDate(0, 0, -3)}}), nil)
	setSetting(t, f.fixture, SettingRefuseEarly, "false")
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E01"]}, nil)
	if r := relByName(t, f.fixture, early); r.State != "chosen" {
		t.Fatalf("with the check off: %s %v", r.State, reasonCodes(r))
	}
}

// safeServer answers every search with items and says in its caps that it serves movies, TV and music.
func safeServer(t *testing.T, items []safeItem) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><caps><limits max="50" default="25"/><searching>
<search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep,tvdbid"/>
<movie-search available="yes" supportedParams="q,imdbid,tmdbid"/><music-search available="yes" supportedParams="q,artist,album"/>
</searching><categories><category id="2000" name="Movies"/><category id="3000" name="Audio"/><category id="5000" name="TV"/></categories></caps>`))
			return
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
		for i, it := range items {
			fmt.Fprintf(&b, `<item><title>%s</title><pubDate>%s</pubDate><enclosure url="http://src/%d.torrent" length="%d" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="%d"/></item>`, xmlEscape(it.name), it.pub.UTC().Format(time.RFC1123Z), i, it.size, 90-i)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	})
	return srv.URL + "/api"
}

// wantRefused checks that the Release called name was refused for the given rule.
func wantRefused(t *testing.T, f fixture, name, rule string) {
	t.Helper()
	r := relByName(t, f, name)
	if r.State == "chosen" || !hasCode(r, "hard.safety:"+rule) {
		t.Fatalf("%s: %s %v", name, r.State, reasonCodes(r))
	}
}
