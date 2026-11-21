package youtube

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// fileRig serves the player routes over a real server, gzip included, as
// the person named by the X-Test-User header.
type fileRig struct {
	t    *testing.T
	svc  *Service
	s    *database.Store
	root string
	srv  *httptest.Server
}

func newFileRig(t *testing.T) *fileRig {
	t.Helper()
	svc, s := addService(t, "ok")
	root := t.TempDir()
	if _, err := svc.SaveSettings(context.Background(), Settings{Folder: root, MinFreeGB: 1}, database.System); err != nil {
		t.Fatal(err)
	}
	h := &Handlers{Svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/youtube/videos/{id}/file", h.videoFile)
	mux.HandleFunc("GET /api/v1/youtube/videos/{id}/thumb", h.videoThumb)
	mux.HandleFunc("PUT /api/v1/youtube/videos/{id}/position", h.savePosition)
	mux.HandleFunc("POST /api/v1/youtube/videos/{id}/retry", h.retryVideo)
	as := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := strconv.ParseInt(r.Header.Get("X-Test-User"), 10, 64)
		c := webserver.Caller{UserID: uid, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermYouTube}}
		mux.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), c)))
	})
	srv := httptest.NewServer(webserver.Compress(as))
	t.Cleanup(srv.Close)
	return &fileRig{t: t, svc: svc, s: s, root: root, srv: srv}
}

// do sends a request as userID with extra headers and returns the answer
// with its body read.
func (r *fileRig) do(method, p string, userID int64, body string, hdr map[string]string) (*http.Response, []byte) {
	r.t.Helper()
	req, _ := http.NewRequest(method, r.srv.URL+p, strings.NewReader(body))
	req.Header.Set("X-Test-User", itoa(userID))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	// Ask for gzip by hand so the client neither adds nor undoes it.
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "identity")
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// done adds a downloaded video at rel with content and a thumbnail.
func (r *fileRig) done(userID int64, vid, rel, content string) int64 {
	r.t.Helper()
	id := addVideo(r.t, r.s, Video{UserID: userID, VideoID: vid})
	file := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(thumbOf(file), []byte("\xff\xd8\xff\xe0jpeg"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	write(r.t, r.s, func(tx *sql.Tx) error { return FinishVideo(tx, id, rel, int64(len(content)), Meta{Title: "T"}) })
	return id
}

func TestVideoFileRanges(t *testing.T) {
	t.Parallel()
	r := newFileRig(t)
	content := strings.Repeat("video bytes ", 400) // big enough to be worth gzipping
	rel := "2-ana/Chan/2024-01-01 Zażółć [abcdefghijk].mp4"
	id := r.done(2, "abcdefghijk", rel, content)
	fileURL := "/api/v1/youtube/videos/" + itoa(id) + "/file"

	resp, b := r.do("GET", fileURL, 2, "", map[string]string{"Range": "bytes=0-1"})
	if resp.StatusCode != http.StatusPartialContent || string(b) != "vi" ||
		resp.Header.Get("Content-Range") != "bytes 0-1/"+strconv.Itoa(len(content)) {
		t.Fatalf("range: %d %q %v", resp.StatusCode, b, resp.Header)
	}
	resp, b = r.do("HEAD", fileURL, 2, "", nil)
	if resp.StatusCode != http.StatusOK || len(b) != 0 || resp.Header.Get("Content-Length") != strconv.Itoa(len(content)) {
		t.Fatalf("head: %d %q %v", resp.StatusCode, b, resp.Header)
	}
	resp, b = r.do("GET", fileURL, 2, "", map[string]string{"Accept-Encoding": "gzip"})
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "" || string(b) != content ||
		resp.Header.Get("Content-Type") != "video/mp4" || resp.Header.Get("Content-Disposition") != "" {
		t.Fatalf("gzip: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = r.do("GET", fileURL+"?download=1", 2, "", nil)
	cd := resp.Header.Get("Content-Disposition")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(cd, "attachment;") ||
		!strings.Contains(cd, "filename*=utf-8''2024-01-01%20Za%C5%BC%C3%B3%C5%82%C4%87%20%5Babcdefghijk%5D.mp4") {
		t.Fatalf("download: %d %q", resp.StatusCode, cd)
	}
	resp, b = r.do("GET", "/api/v1/youtube/videos/"+itoa(id)+"/thumb", 2, "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" || !strings.HasSuffix(string(b), "jpeg") {
		t.Fatalf("thumb: %d %v", resp.StatusCode, resp.Header)
	}

	// Each of these answers 404 with nothing of the row in the body.
	gone := r.done(2, "gonegonegon", "2-ana/Chan/2024-01-02 Gone [gonegonegon].mp4", "x")
	if err := os.Remove(filepath.Join(r.root, "2-ana/Chan/2024-01-02 Gone [gonegonegon].mp4")); err != nil {
		t.Fatal(err)
	}
	nothumb := r.done(2, "nothumbnoth", "2-ana/Chan/2024-01-03 N [nothumbnoth].mp4", "x")
	if err := os.Remove(filepath.Join(r.root, "2-ana/Chan/2024-01-03 N [nothumbnoth].jpg")); err != nil {
		t.Fatal(err)
	}
	queued := addVideo(t, r.s, Video{UserID: 2, VideoID: "queuedqueue"})
	outside := r.done(2, "outsideoutx", "2-ana/x.mp4", "x")
	write(t, r.s, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE youtube_videos SET relative_path = '../x.mp4' WHERE id = ?", outside)
		return err
	})
	if err := os.WriteFile(filepath.Join(filepath.Dir(r.root), "x.mp4"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		id   int64
		user int64
		kind string
	}{
		{"file deleted", gone, 2, "file"}, {"thumb deleted", nothumb, 2, "thumb"},
		{"queued", queued, 2, "file"}, {"queued thumb", queued, 2, "thumb"},
		{"another person's", id, 3, "file"}, {"another person's thumb", id, 3, "thumb"},
		{"path leaves the folder", outside, 2, "file"}, {"thumb path leaves the folder", outside, 2, "thumb"}, {"no such row", 999999, 2, "file"}, {"bad id", 0, 2, "file"},
	} {
		p := "/api/v1/youtube/videos/" + itoa(c.id) + "/" + c.kind
		if c.id == 0 {
			p = "/api/v1/youtube/videos/x/" + c.kind
		}
		resp, b := r.do("GET", p, c.user, "", nil)
		if resp.StatusCode != http.StatusNotFound || strings.Contains(string(b), "secret") || strings.Contains(string(b), "Zażółć") ||
			strings.Contains(string(b), "Gone") || strings.Contains(string(b), "ana") {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, b)
		}
	}
}

func TestPositionMarksWatchedAtNinetyPercent(t *testing.T) {
	t.Parallel()
	r := newFileRig(t)
	id := r.done(2, "posposposp1", "2-ana/C/2024-01-01 P [posposposp1].mp4", "x")
	p := "/api/v1/youtube/videos/" + itoa(id) + "/position"
	save := func(user int64, body string, want int) (bool, Video) {
		t.Helper()
		resp, b := r.do("PUT", p, user, body, nil)
		if resp.StatusCode != want {
			t.Fatalf("%s: %d %s", body, resp.StatusCode, b)
		}
		var out struct {
			Watched bool `json:"watched"`
		}
		if want == http.StatusOK {
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatal(err)
			}
		}
		v, err := VideoOf(context.Background(), r.s.Read(), 2, id)
		if err != nil {
			t.Fatal(err)
		}
		return out.Watched, v
	}
	if w, v := save(2, `{"seconds":89,"duration":100}`, 200); w || v.WatchedAt != nil || v.PositionSeconds != 89 {
		t.Fatalf("89/100: %v %+v", w, v)
	}
	w, v := save(2, `{"seconds":90,"duration":100}`, 200)
	if !w || v.WatchedAt == nil || v.PositionSeconds != 90 {
		t.Fatalf("90/100: %v %+v", w, v)
	}
	first := *v.WatchedAt
	if w, v := save(2, `{"seconds":10,"duration":100}`, 200); !w || v.WatchedAt == nil || !v.WatchedAt.Equal(first) || v.PositionSeconds != 10 {
		t.Fatalf("10/100 after watching: %v %+v", w, v)
	}
	if _, v := save(3, `{"seconds":50,"duration":100}`, 404); v.PositionSeconds != 10 {
		t.Fatalf("another person moved it: %+v", v)
	}
	save(2, `{"seconds":"x"}`, 400)

	for _, c := range []struct {
		s, d int
		want bool
	}{{0, 0, false}, {5, 0, false}, {9, 10, true}, {8, 10, false}, {100, 100, true}, {200, 100, true}} {
		if got := watched(c.s, c.d); got != c.want {
			t.Errorf("watched(%d, %d) = %v", c.s, c.d, got)
		}
	}
}

func TestRetryOnlyFromFailed(t *testing.T) {
	t.Parallel()
	r := newFileRig(t)
	done := r.done(2, "donedonedon", "2-ana/C/2024-01-01 D [donedonedon].mp4", "x")
	if resp, b := r.do("POST", "/api/v1/youtube/videos/"+itoa(done)+"/retry", 2, "", nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("done: %d %s", resp.StatusCode, b)
	}
	failed := addVideo(t, r.s, Video{UserID: 2, VideoID: "failfailfai", MaxHeight: 720})
	write(t, r.s, func(tx *sql.Tx) error { return SetVideoState(tx, failed, StateFailed, "ERROR: Video unavailable") })
	if resp, _ := r.do("POST", "/api/v1/youtube/videos/"+itoa(failed)+"/retry", 3, "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("another person's retry: %d", resp.StatusCode)
	}
	if n := len(queuedJobs(t, r.s, KindDownload)); n != 0 {
		t.Fatalf("jobs before retry: %d", n)
	}
	resp, b := r.do("POST", "/api/v1/youtube/videos/"+itoa(failed)+"/retry", 2, "", nil)
	var got struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(b, &got) != nil || got.ID != failed || got.State != StateQueued || got.Error != "" {
		t.Fatalf("retry: %d %s", resp.StatusCode, b)
	}
	v, err := VideoOf(context.Background(), r.s.Read(), 2, failed)
	if err != nil || v.State != StateQueued || v.MaxHeight != 720 {
		t.Fatalf("row %+v %v", v, err)
	}
	jobsQueued := queuedJobs(t, r.s, KindDownload)
	params, ok := jobsQueued["youtube:2:"+itoa(failed)]
	if len(jobsQueued) != 1 || !ok || params["row_id"] != float64(failed) {
		t.Fatalf("jobs %v", jobsQueued)
	}
}

// The video and thumbnail types do not hang on the host's mime.types file
// (the shipped image has none) or on sniffing: with the extension table
// saying otherwise and bytes that sniff as text, they are still named.
// Not parallel: it changes the process-wide extension table.
func TestVideoFileTypeWithoutMimeTable(t *testing.T) {
	for ext, was := range map[string]string{".mp4": mime.TypeByExtension(".mp4"), ".jpg": mime.TypeByExtension(".jpg")} {
		if err := mime.AddExtensionType(ext, "application/x-unknown"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if was != "" {
				_ = mime.AddExtensionType(ext, was)
			}
		})
	}
	r := newFileRig(t)
	id := r.done(2, "typetypetyp", "2-ana/C/2024-01-01 T [typetypetyp].mp4", "plain text, not a video")
	for kind, want := range map[string]string{"file": "video/mp4", "thumb": "image/jpeg"} {
		resp, _ := r.do("GET", "/api/v1/youtube/videos/"+itoa(id)+"/"+kind, 2, "", nil)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != want {
			t.Errorf("%s: %d %q", kind, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	resp, _ := r.do("GET", "/api/v1/youtube/videos/999999/file", 2, "", nil)
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("404 type: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}
