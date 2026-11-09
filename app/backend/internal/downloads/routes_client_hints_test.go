package downloads

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Usenet client's post time reaches the row; torrents never carry one.
func TestClientRowPostedAt(t *testing.T) {
	t.Parallel()
	u := domain.Downloader{ID: 2, Name: "S", Adapter: "sabnzbd", Protocol: "usenet"}
	if r := clientRow(u, Job{ID: "SABnzbd_nzo_1", PostedAt: 1700000000}); r.PostedAt == nil || r.PostedAt.Unix() != 1700000000 {
		t.Fatalf("usenet row %+v", r.PostedAt)
	}
	q := domain.Downloader{ID: 1, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent"}
	if r := clientRow(q, Job{ID: outHash, PostedAt: 1700000000}); r.PostedAt != nil {
		t.Fatalf("torrent row %+v", r.PostedAt)
	}
}

func TestClientHints(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"checkingDL": "checking", "checkingUP": "checking", "checkingResumeData": "checking", "moving": "moving", "error": "errored", "missingFiles": "missing", "stalledUP": "idle_seed", "stalledDL": "", "uploading": ""} {
		if got := clientHint(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

func TestSubfolderAndLinkGuards(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"/etc", "../up", "a/../../b", `C:\x`, `\host\share`, "c:x", "a\x00b", "..", "."} {
		if _, err := subfolder(bad); err == nil {
			t.Errorf("subfolder %q passed", bad)
		}
	}
	if s, err := subfolder("tv/new//"); err != nil || s != "tv/new" {
		t.Fatalf("good subfolder %q %v", s, err)
	}
	if joinFolder(`D:\dl`, "tv/new") != `D:\dl\tv\new` || joinFolder("/data/", "tv") != "/data/tv" {
		t.Fatal("join")
	}
	for _, bad := range []string{"magnet:?xt=urn:btih:" + outHash + "\r\nx", "https://a.test/x\n", "ftp://a.test/x", "javascript:alert(1)", "https://user@1.1.1.1/x", "magnet:", "https://a.test/a b"} {
		if err := checkLink(bad); err == nil {
			t.Errorf("link %q passed", bad)
		}
	}
	for _, ok := range []string{outHash, strings.Repeat("a", 64), "magnet:?xt=urn:btih:" + outHash, "https://1.1.1.1/x.torrent"} {
		if err := checkLink(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
}

func TestClientHintUsenetSteps(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"Verifying": "verifying", "QuickCheck": "verifying", "Repairing": "repairing", "Extracting": "unpacking", "Moving": "finishing", "Running": "script", "moving": "moving", "Downloading": ""} {
		if got := clientHint(in); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

func TestDownloaderAttemptErrorsArePublicSentinels(t *testing.T) {
	t.Parallel()
	for _, sentinel := range []error{ErrUncertain, ErrAttemptStopped, ErrPendingAttempt} {
		w := httptest.NewRecorder()
		downloaderError(w, errors.Join(sentinel, errors.New("private provider and database detail")))
		if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "private provider") || !strings.Contains(w.Body.String(), sentinel.Error()) {
			t.Fatal("attempt conflict leaked joined cause", w.Code)
		}
	}
}

// Show more past the most one answer carries stays at that most, never back
// to the first page's size (2,000 torrents, then 2,200 asked: still 2,000).
func TestOutsideLimitHoldsAtItsMost(t *testing.T) {
	t.Parallel()
	for q, want := range map[string]int{"": 200, "x": 200, "0": 200, "-5": 200, "400": 400, "2000": 2000, "2200": 2000, "99999": 2000} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/view?outside_limit="+q, nil)
		if got := outsideLimit(r); got != want {
			t.Errorf("outside_limit=%q: %d, want %d", q, got, want)
		}
	}
}
