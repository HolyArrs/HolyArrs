package downloads

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// importRow records one Import of the rig's download in a state.
func importRow(t *testing.T, x *dl41, state string) {
	t.Helper()
	exec41(t, x.s, `INSERT INTO imports(created_at, created_by, updated_at, updated_by, download_id, from_path, kind, mode, state)
		VALUES ('t', '{}', 't', '{}', ?, '/data/downloads', 'movie', 'auto', ?)`, x.dlID, state)
}

func ownRow(t *testing.T, x *dl41) DownloadRow {
	t.Helper()
	for _, r := range view(t, x.srv.URL+"/api/v1/downloads/view").Downloads {
		if r.ID == x.dlID {
			return r
		}
	}
	t.Fatal("the download is not on the page")
	return DownloadRow{}
}

// A refused import never reads as in the library, whether the Slot kept the
// copy it had or went looking for another one.
func TestDownloadsViewRefusedImportIsNotDone(t *testing.T) {
	t.Parallel()
	for slot, text := range map[mediafiles.SlotState]string{
		mediafiles.SlotInLibrary: "The files were not what the name said; the copy already in the library stays.",
		mediafiles.SlotSearching: "The files were not what the name said; HolyArrs looks for another copy.",
		mediafiles.SlotWanted:    "The files were not what the name said; HolyArrs looks for another copy.",
	} {
		x := seed41(t, slot, "finished")
		importRow(t, x, "rejected")
		if r := ownRow(t, x); r.Phase != "import_refused" || r.Label != "Import refused" || r.Text != text {
			t.Fatalf("slot %s: phase %q label %q text %q", slot, r.Phase, r.Label, r.Text)
		}
	}
}

// An import done after a refused one (a person let it in) is in the library.
func TestDownloadsViewImportDoneAfterARefusalIsDone(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "finished")
	importRow(t, x, "rejected")
	importRow(t, x, "done")
	if r := ownRow(t, x); r.Phase != "done" || r.Text != "In the library." {
		t.Fatalf("phase %q text %q", r.Phase, r.Text)
	}
}

// Missing or still-copying files read as automatic work, not as a failed
// import that needs a person to review it.
func TestDownloadsViewWaitingForFiles(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotNeedsADecision, "finished")
	exec41(t, x.s, "UPDATE slots SET last_reason = ? WHERE id = ?", "the file could not be placed: the file is not in the download folder any more", x.slotID)
	r := ownRow(t, x)
	if r.Phase != "waiting_to_import" || r.Label != "Waiting to import" || r.CanImport || r.Text != "Waiting for the files to arrive; HolyArrs will check again automatically." {
		t.Fatalf("phase %q label %q can_import %v text %q", r.Phase, r.Label, r.CanImport, r.Text)
	}
}

// A download whose Release is gone (its Source was deleted) is named
// by what the Downloader held, else by its Title, never by a bare hash.
func TestDownloadsViewNamesADownloadWhoseReleaseIsGone(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotRemoved, "finished")
	exec41(t, x.s, "UPDATE downloads SET release_id = NULL")
	if r := ownRow(t, x); r.Name != "Salt Choir" {
		t.Fatalf("no files seen: name %q", r.Name)
	}
	exec41(t, x.s, `UPDATE downloads SET files_seen = '[{"name":"Salt.Choir.2020.1080p.WEB.H264-HOLYARRS.mkv","size":10}]'`)
	if r := ownRow(t, x); r.Name != "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS" {
		t.Fatalf("one file: name %q", r.Name)
	}
	exec41(t, x.s, `UPDATE downloads SET files_seen = '[{"name":"Salt.Choir.2020.1080p-GRP/salt.mkv","size":10},{"name":"Salt.Choir.2020.1080p-GRP/salt.nfo","size":1}]'`)
	if r := ownRow(t, x); r.Name != "Salt.Choir.2020.1080p-GRP" {
		t.Fatalf("a folder: name %q", r.Name)
	}
}

func TestSeenName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":                        "",
		"a.mkv":                   "a",
		"Show.S01/ep1.mkv":        "Show.S01",
		`Show.S01\ep1.mkv`:        "Show.S01",
		"/Show.S01/ep1.mkv":       "Show.S01",
		"..":                      "",
		"noext":                   "noext",
		"Movie.2020.1080p-GRP.x":  "Movie.2020.1080p-GRP",
		"Movie.2020.WEB.H264-GRP": "Movie.2020.WEB.H264-GRP",
		"Salt.Choir.2020":         "Salt.Choir.2020",
	} {
		var files []domain.SeenFile
		if in != "" {
			files = []domain.SeenFile{{Name: in}}
		}
		if got := seenName(files); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := seenName([]domain.SeenFile{{Name: "sample.mkv"}, {Name: "movie.mkv"}}); got != "" {
		t.Errorf("loose files with no folder: %q", got)
	}
}

// A download a person removed leaves the Downloads list (History keeps its
// row), and the Downloader still listing it for a moment must not bring it
// back as a client row.
func TestDownloadsViewHidesARemovedDownload(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "cleaned")
	outside := `{"hash":"1212121212121212121212121212121212121212","name":"Other.App","category":"linux","size":500,"progress":0.5,"state":"downloading","added_on":1700000100}`
	pointQB(t, x, qbList(t, `[{"hash":"ABCDABCDABCDABCDABCDABCDABCDABCDABCDABCD","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":1,"upspeed":9,"state":"uploading"},`+outside+`]`).URL)
	feedStore(t, x.dl)
	v := view(t, x.srv.URL+"/api/v1/downloads/view")
	if len(v.Downloads) != 1 || v.Downloads[0].Hash != "1212121212121212121212121212121212121212" || v.OutsideTotal != 1 {
		t.Fatalf("rows %d outside %d: %+v", len(v.Downloads), v.OutsideTotal, v.Downloads)
	}
	var state string
	if err := x.s.Read().QueryRow("SELECT state FROM downloads WHERE id = ?", x.dlID).Scan(&state); err != nil || state != "cleaned" {
		t.Fatalf("the row for History: %q %v", state, err)
	}
}

// A removed download hides the Downloader's copy only while the removal is
// new or still being retried. A job added again later is the person's own
// torrent and reads as an outside row. The wait counts from cleaned_at, so a
// later write to the row does not start it again.
func TestDownloadsViewShowsARemovedTorrentAddedAgain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		age     time.Duration
		pending any
		clear   any
		want    int
	}{
		{"just removed", time.Minute, nil, nil, 0},
		{"still being removed", time.Hour, 1, nil, 0},
		{"clear still waiting", time.Hour, nil, database.FormatTime(time.Now()), 0},
		// A torrent added again inside the wait stays hidden for the rest of it.
		{"added again inside the wait", 9 * time.Minute, nil, nil, 0},
		{"added again after the wait", 11 * time.Minute, nil, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := seed41(t, mediafiles.SlotInLibrary, "cleaned")
			exec41(t, x.s, "UPDATE downloads SET cleaned_at = ?, removal_pending = ?, clear_retry_at = ? WHERE id = ?",
				database.FormatTime(time.Now().Add(-tc.age)), tc.pending, tc.clear, x.dlID)
			pointQB(t, x, qbList(t, `[{"hash":"ABCDABCDABCDABCDABCDABCDABCDABCDABCDABCD","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":1,"state":"uploading"}]`).URL)
			feedStore(t, x.dl)
			v := view(t, x.srv.URL+"/api/v1/downloads/view")
			if len(v.Downloads) != tc.want || v.OutsideTotal != tc.want {
				t.Fatalf("rows %d outside %d: %+v", len(v.Downloads), v.OutsideTotal, v.Downloads)
			}
			if tc.want == 1 {
				r := v.Downloads[0]
				if !r.ClientItem || !r.Outside || r.Title != "" || !strings.EqualFold(r.Hash, "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd") {
					t.Fatalf("the added-again job: %+v", r)
				}
			}
		})
	}
}

// The wait that hides a removed download's client copy counts from cleaned_at:
// a later write to the row must not start it again.
func TestHidesClientCopy(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	pending := int64(0)
	at := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	for _, tc := range []struct {
		name string
		d    knownDownload
		want bool
	}{
		{"a removal still pending", knownDownload{RemovalPending: &pending, CleanedAt: at(time.Hour)}, true},
		{"a clear still waiting", knownDownload{ClearRetryAt: at(-time.Hour), CleanedAt: at(time.Hour)}, true},
		{"removed a moment ago, written to since", knownDownload{CleanedAt: at(5 * time.Minute), UpdatedAt: now}, true},
		{"removed long ago, written to since", knownDownload{CleanedAt: at(11 * time.Minute), UpdatedAt: now}, false},
		// A row cleaned before cleaned_at existed falls back to its last write.
		{"no cleaned_at, a moment ago", knownDownload{UpdatedAt: now.Add(-5 * time.Minute)}, true},
		{"no cleaned_at, long ago", knownDownload{UpdatedAt: now.Add(-11 * time.Minute)}, false},
	} {
		if got := hidesClientCopy(tc.d, now); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// GET /api/v1/downloads is the raw Downloads list: it leaves out removed
// downloads like the screen does, and still lists live ones.
func TestListDownloadsLeavesOutCleaned(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "cleaned")
	srv := httptest.NewServer(ListDownloads(x.s))
	t.Cleanup(srv.Close)
	var out struct {
		Downloads []domain.Download `json:"downloads"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL, "", 200), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Downloads) != 0 {
		t.Fatalf("cleaned rows: %+v", out.Downloads)
	}
	exec41(t, x.s, "UPDATE downloads SET state = 'finished'")
	if err := json.Unmarshal(do(t, "GET", srv.URL, "", 200), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Downloads) != 1 || out.Downloads[0].State != "finished" {
		t.Fatalf("live rows: %+v", out.Downloads)
	}
}
