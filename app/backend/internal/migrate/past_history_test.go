package migrate

import (
	"net/http"
	"path/filepath"
	"testing"
)

// The old app's past downloads come over: read from the API, newest first,
// keeping only downloads sent, imported and failed.
func TestRadarrHistoryAPI(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{"/api/v3/history": func(r *http.Request) any {
		if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("sortDirection") != "descending" {
			return map[string]any{"records": []any{}}
		}
		return map[string]any{"totalRecords": 3, "records": []any{
			map[string]any{"id": 9, "movieId": 1, "eventType": "downloadFolderImported", "date": "2025-05-02T10:00:00Z", "sourceTitle": "The.Matrix.1999.1080p.BluRay",
				"quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p"}}, "data": map[string]any{"size": "8000000000"}},
			map[string]any{"id": 8, "movieId": 1, "eventType": "movieFileRenamed", "date": "2025-05-01T11:00:00Z"},
			map[string]any{"id": 7, "movieId": 1, "eventType": "grabbed", "date": "2025-05-01T10:00:00Z", "sourceTitle": "The.Matrix.1999.1080p.BluRay",
				"data": map[string]any{"indexer": "Nyaa", "downloadClientName": "qBit"}},
		}}
	}})
	s := read(t, api(Radarr, u, key))
	h := findTitle(t, s, "The Matrix").History
	if len(h) != 2 || h[0].Kind != "import_done" || h[0].Quality != "Bluray-1080p" || h[0].Size != 8000000000 || h[1].Kind != "download_sent" || h[1].Source != "Nyaa" || h[1].Getter != "qBit" {
		t.Fatalf("history %+v", h)
	}
}

// An app with no history route still reads.
func TestRadarrHistoryMissingIsANote(t *testing.T) {
	u, key := radarrAPI(t)
	s := read(t, api(Radarr, u, key))
	if len(findTitle(t, s, "The Matrix").History) != 0 {
		t.Fatal("history from nowhere")
	}
}

// A Sonarr backup's History table comes over with each episode's numbers.
func TestSonarrHistoryBackup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sonarr.db")
	makeDB(t, p, append(sonarrDB(true),
		`CREATE TABLE History (Id INTEGER PRIMARY KEY, EpisodeId INTEGER, SeriesId INTEGER, SourceTitle TEXT, Date DATETIME, Quality TEXT, Data TEXT, EventType INTEGER, DownloadId TEXT)`,
		`CREATE TABLE IF NOT EXISTS Episodes (Id INTEGER PRIMARY KEY, SeriesId INTEGER, SeasonNumber INTEGER, EpisodeNumber INTEGER, EpisodeFileId INTEGER, AirDate TEXT, Title TEXT)`,
		`INSERT INTO Episodes (Id, SeriesId, SeasonNumber, EpisodeNumber, EpisodeFileId) VALUES (501, 5, 2, 5, 0)`,
		`INSERT INTO History VALUES (1, 501, 5, 'Show.S02E05.720p', '2024-03-01 10:00:00Z', '{"quality": 4}', '{"indexer":"Nyaa"}', 1, 'x')`,
		`INSERT INTO History VALUES (2, 501, 5, 'Show.S02E05.720p', '2024-03-01 11:00:00Z', '{}', '{"message":"bad file"}', 4, 'x')`,
		`INSERT INTO History VALUES (3, 501, 5, 'Show.S02E05.720p', '2024-03-01 12:00:00Z', '{}', '{}', 6, 'x')`,
	)...)
	s := read(t, file(Sonarr, p))
	var h []PastEvent
	for _, ti := range s.Titles {
		h = append(h, ti.History...)
	}
	if len(h) != 2 || h[0].Kind != "download_failed" || h[0].Reason != "bad file" || h[1].Kind != "download_sent" || h[1].Season != 2 || h[1].Episode != 5 || h[1].Source != "Nyaa" {
		t.Fatalf("history %+v", h)
	}
}

const pastCount = `SELECT count(*) FROM events WHERE group_key LIKE 'moved:%'`

func (l *moveLab) events(t *testing.T, q string) int {
	t.Helper()
	var n int
	if err := l.st.ReadEvents().QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// After the move the past downloads show in History, once, and an undo
// takes them out again.
func TestMoveBringsPastDownloads(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Titles[0].History = []PastEvent{
			{OldID: 2, Kind: "import_done", At: "2025-05-02T10:00:00Z", Release: "The.Matrix.1999.1080p", Size: 10},
			{OldID: 1, Kind: "download_sent", At: "2025-05-01T10:00:00Z", Release: "The.Matrix.1999.1080p", Source: "Nyaa", Getter: "qBit"},
			{OldID: 3, Kind: "download_failed", At: "bad date"},
		}
	})
	mv := l.move(t, Choices{})
	if n := l.events(t, pastCount); n != 2 {
		t.Fatalf("past events after move: %d", n)
	}
	if n := l.events(t, `SELECT count(*) FROM events WHERE kind = 'download_sent' AND at LIKE '2025-05-01%' AND json_extract(data, '$.moved_from') = 'Radarr' AND summary = 'Sent The.Matrix.1999.1080p to qBit'`); n != 1 {
		t.Fatalf("sent event: %d", n)
	}
	l.move(t, Choices{})
	if n := l.events(t, pastCount); n != 2 {
		t.Fatalf("moving again doubled them: %d", n)
	}
	l.undo(t, mv.ID)
	if n := l.events(t, pastCount); n != 0 {
		t.Fatalf("left after undo: %d", n)
	}
}

// A later history page failing keeps the pages already read.
func TestRadarrHistoryLaterPageFails(t *testing.T) {
	recs := []any{}
	for i := 0; i < 500; i++ {
		recs = append(recs, map[string]any{"id": 1000 - i, "movieId": 1, "eventType": "grabbed", "date": "2025-05-01T10:00:00Z", "sourceTitle": "The.Matrix.1999"})
	}
	u, key := radarrAPIWith(t, map[string]any{"/api/v3/history": func(r *http.Request) any {
		if r.URL.Query().Get("page") != "1" {
			return "not json"
		}
		return map[string]any{"records": recs}
	}})
	s := read(t, api(Radarr, u, key))
	if n := len(findTitle(t, s, "The Matrix").History); n != 500 {
		t.Fatalf("kept %d of page 1", n)
	}
}
