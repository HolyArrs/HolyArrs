package migrate

import (
	"path/filepath"
	"testing"
)

// TestBlocklistIsRead: the blocklist comes from /blocklist (paged) or from a
// backup's Blocklist table.
func TestBlocklistIsRead(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{"/api/v3/blocklist": map[string]any{"page": 1, "pageSize": 1000, "totalRecords": 1, "records": []any{
		map[string]any{"movieId": 1, "sourceTitle": "The Matrix 1999 1080p WEBRip LAMA", "protocol": "torrent", "indexer": "TorrentDownload (Prowlarr)",
			"message": "Manually marked as failed", "torrentInfoHash": "ABC123", "movie": map[string]any{"title": "The Matrix"}}}}})
	s := read(t, api(Radarr, u, key))
	if len(s.Blocklist) != 1 || s.Blocklist[0] != (Blocked{TitleID: 1, Name: "The Matrix 1999 1080p WEBRip LAMA", InfoHash: "ABC123", Protocol: "torrent",
		Indexer: "TorrentDownload (Prowlarr)", Reason: "Manually marked as failed"}) {
		t.Fatalf("api %+v", s.Blocklist)
	}
	p := filepath.Join(t.TempDir(), "sonarr.db")
	makeDB(t, p, append(sonarrDB(false), `CREATE TABLE Blocklist (Id INTEGER PRIMARY KEY, SeriesId INTEGER, EpisodeIds TEXT, SourceTitle TEXT, Protocol INTEGER, Indexer TEXT, Message TEXT, TorrentInfoHash TEXT)`,
		`INSERT INTO Blocklist VALUES (1, 7, '[3]', 'Show S01E02 1080p WEB GRP', 2, 'Nyaa', 'bad', NULL)`)...)
	s = read(t, file(Sonarr, p))
	if len(s.Blocklist) != 1 || s.Blocklist[0] != (Blocked{TitleID: 7, Name: "Show S01E02 1080p WEB GRP", Protocol: "torrent", Indexer: "Nyaa", Reason: "bad"}) {
		t.Fatalf("backup %+v", s.Blocklist)
	}
}

// TestBlocklistPlan: an entry of a title that comes over is a Turned down
// item; one whose title does not is listed as not moved.
func TestBlocklistPlan(t *testing.T) {
	s := &Snapshot{Kind: Sonarr,
		Titles:    []Title{{SourceID: 7, Kind: "show", Name: "Lab Show", IDs: map[string]string{"tvdb": "81189"}}},
		Blocklist: []Blocked{{TitleID: 7, Name: "Lab Show S01E02 1080p", Protocol: "torrent"}, {TitleID: 8, Name: "Gone S01E01", Protocol: "torrent"}}}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	g := p.Group(GroupTurnedDown)
	if len(g.Items) != 1 || g.Items[0].Key != "turned_down:show:tvdb:81189:lab show s01e02 1080p" || g.Items[0].TurnDown.IDs["tvdb"] != "81189" {
		t.Fatalf("items %+v", g.Items)
	}
	if n, ok := notMoved(p, "sonarr blocklist[1]"); !ok || n.Why != "its title did not come over" {
		t.Fatalf("not moved %+v", n)
	}
}

// TestMoveTurnsDownBlocklist: a blocked release is turned down for the
// moved movie under the Source of the same indexer; undo removes it.
func TestMoveTurnsDownBlocklist(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Blocklist = []Blocked{{TitleID: 1, Name: "The.Matrix.1999.1080p.WEBRip-LAMA", InfoHash: "ABC", Protocol: "torrent", Indexer: "Nyaa (Prowlarr)", Reason: "Manually marked as failed"}}
	})
	key := "turned_down:movie:tmdb:603:the.matrix.1999.1080p.webrip-lama"
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, key); got.Result != ResCreated {
		t.Fatalf("line %+v", got)
	}
	if n := l.count(t, `SELECT count(*) FROM releases r JOIN sources s ON s.id = r.source_id WHERE r.state = 'turned_down' AND r.info_hash = 'abc'
		AND s.name = 'Nyaa' AND json_array_length(r.matched_slot_ids) = 1 AND json_extract(r.verdict, '$.turned_down_reason') = 'Manually marked as failed'`); n != 1 {
		t.Fatalf("%d turned down releases", n)
	}
	// A second move adds nothing twice.
	if again := l.move(t, Choices{}); lineOf(t, again.Report, key).Result != ResAlready {
		t.Fatalf("again %+v", lineOf(t, again.Report, key))
	}
	if u := l.undo(t, mv.ID); lineOf(t, u.Report, key).Result != ResRemoved {
		t.Fatalf("undo %+v", lineOf(t, u.Report, key))
	}
	if n := l.count(t, "SELECT count(*) FROM releases"); n != 0 {
		t.Fatalf("%d releases left", n)
	}
}
