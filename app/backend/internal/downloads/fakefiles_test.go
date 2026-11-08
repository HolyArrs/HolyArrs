package downloads

import (
	"context"
	"testing"
)

const gb = int64(1) << 30

func TestFakeFiles(t *testing.T) {
	cases := []struct {
		files    []File
		protocol string
		fake     bool
	}{
		{[]File{{Name: "Dune/Dune.mkv", Size: gb}, {Name: "Dune/Dune.en.srt"}, {Name: "Dune/Dune.nfo"}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.zip"}, {Name: "Dune/password.txt"}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.rar"}, {Name: "Dune/Dune.r00"}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.rar"}, {Name: "Dune/Dune.r00"}}, "usenet", false},
		{[]File{{Name: "Dune/Dune.part1.rar", Size: gb}, {Name: "Dune/Dune.part2.rar", Size: gb}, {Name: "Dune/Dune.nfo", Size: 900}, {Name: "Dune/Sample/dune-sample.mkv", Size: 2 << 20}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.rar", Size: gb}, {Name: "Dune/Dune.r00", Size: gb}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.rar", Size: gb}, {Name: "Dune/Dune.r00", Size: gb}, {Name: "Dune/Get it.url"}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.7z.001", Size: gb}, {Name: "Dune/Dune.7z.002", Size: gb}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.zip", Size: gb}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.zip", Size: 5 << 20}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.part1.rar", Size: gb}, {Name: "Dune/Dune.part2.rar", Size: gb}, {Name: "Dune/password.txt", Size: 20}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.mkv", Size: gb}, {Name: "Dune/Downloaded from X.url"}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.mkv", Size: gb}, {Name: "Dune/Site.webloc"}}, "torrent", false},
		{[]File{{Name: "Dune/Get the full movie.url"}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.zip"}, {Name: "Dune/Get the full movie.url"}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.mkv", Size: gb}, {Name: "Dune/password.txt"}}, "torrent", true},
		{[]File{{Name: "Dune/Dune.mkv", Size: gb}, {Name: "Dune/Codec.exe"}}, "torrent", true},
		{[]File{{Name: "Album/01 Song.dsf", Size: 40 << 20}, {Name: "Album/02 Song.dff", Size: 40 << 20}, {Name: "Album/Artwork.zip"}}, "torrent", false},
		{[]File{{Name: "Movie/VIDEO_TS/VTS_01_1.VOB", Size: gb}, {Name: "Movie/covers.rar"}}, "torrent", false},
		{[]File{{Name: "Movie/BDMV/STREAM/00001.m2ts", Size: gb}, {Name: "Movie/extras.rar"}}, "torrent", false},
		{[]File{{Name: "Movie/Movie.iso", Size: gb}, {Name: "Movie/covers.rar"}}, "torrent", false},
		{[]File{{Name: "Movie/Movie.mpg", Size: gb}, {Name: "Movie/covers.zip"}}, "torrent", false},
		{[]File{{Name: "Album/01.tak", Size: 40 << 20}, {Name: "Album/scans.7z"}}, "torrent", false},
		{[]File{{Name: "Album/01.aiff", Size: 40 << 20}, {Name: "Album/02.wma", Size: 40 << 20}, {Name: "Album/03.mka", Size: 40 << 20}, {Name: "Album/scans.zip"}}, "torrent", false},
		{[]File{{Name: "Album/01 Song.flac", Size: 40 << 20}, {Name: "Album/cover.zip"}}, "torrent", false},
		{[]File{{Name: "Album/Album.lnk"}}, "torrent", true},
		{[]File{{Name: "Movie/Movie.iso", Size: 0}, {Name: "Movie/covers.rar", Size: 5 << 20}}, "torrent", true},
		{[]File{{Name: "Movie/VIDEO_TS/VTS_01_1.VOB", Size: 10 << 10}, {Name: "Movie/covers.rar", Size: 5 << 20}}, "torrent", true},
		{[]File{{Name: "Movie/Movie.iso", Size: 5 * gb}, {Name: "Movie/covers.rar", Size: 5 << 20}}, "torrent", false},
		{[]File{{Name: "Dune/Dune.mkv", Size: 3 << 20}, {Name: "Dune/Get it.url"}, {Name: "Dune/Dune.zip", Size: 4 * gb}}, "torrent", true},
		{[]File{{Name: "Album/01 Song.flac", Size: 1 << 10}, {Name: "Album/cover.zip"}}, "torrent", true},
	}
	for i, c := range cases {
		if got := len(fakeFiles(c.files, c.protocol)) > 0; got != c.fake {
			t.Errorf("case %d: fake %v, want %v (%v)", i, got, c.fake, fakeFiles(c.files, c.protocol))
		}
	}
}

func TestFakeFilesFailWhileDownloading(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"progress": 0.1, "state": "downloading"})
	w.fake.files[hash] = []map[string]any{{"name": "Dune/Dune.Part.Two.2024.1080p.zip", "size": 100}, {"name": "Dune/PASSWORD.txt", "size": 1}}
	_, _ = m.SyncOnce(context.Background())
	wantFailedPath(t, w, id, "fake files instead of the media: PASSWORD.txt, Dune.Part.Two.2024.1080p.zip", true)
}

func TestShortcutBesideMediaIsNoted(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"progress": 0.1, "state": "downloading"})
	w.fake.files[hash] = []map[string]any{{"name": "Dune/Dune.Part.Two.2024.1080p.mkv", "size": int64(4 << 30)}, {"name": "Dune/Downloaded from X.url", "size": 100}}
	_, _ = m.SyncOnce(context.Background())
	if d := w.download(t, id); d.State == "failed" {
		t.Fatalf("download failed: %v", *d.StateReason)
	}
	var n int
	if err := w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'download_shortcut' AND summary LIKE '%Downloaded from X.url%'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("shortcut events %d %v", n, err)
	}
	w.fake.set(hash, map[string]any{"progress": 0.5, "state": "downloading"})
	_, _ = m.SyncOnce(context.Background())
	_ = w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'download_shortcut'").Scan(&n)
	if n != 1 {
		t.Fatalf("noted %d times", n)
	}
}

func TestShortcutOnlyFails(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"progress": 0.1, "state": "downloading"})
	w.fake.files[hash] = []map[string]any{{"name": "Dune/Watch Dune online.url", "size": 100}}
	_, _ = m.SyncOnce(context.Background())
	wantFailedPath(t, w, id, "Watch Dune online.url", true)
}
