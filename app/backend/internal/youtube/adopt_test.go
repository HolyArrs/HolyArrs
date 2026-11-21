package youtube

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The adopt fixtures mirror real folders: a Pinchflat download ("NN -
// <title>" with a -thumb.jpg, an .nfo holding the id and some .en.srt) and
// yt-dlp downloads with the id in the name or in an .info.json. Pinchflat
// keeps YouTube's full-width "｜", "？" and "：" in its names.
const (
	pfName  = "01 - What is a Computer？ ｜ Crash Course： Ep 1"
	pfTitle = "What is a Computer? | Crash Course: Ep 1"
	pf2Name = "02 - Iron Condor ｜ Explained"
)

func nfo(id, title, show, aired string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<episodedetails>
  <title>` + title + `</title>
  <showtitle>` + show + `</showtitle>
  <uniqueid type="youtube" default="true">` + id + `</uniqueid>
  <plot>Something about it.</plot>
  <aired>` + aired + `</aired>
</episodedetails>
`
}

// adoptTree writes the source folder and returns it.
func adoptTree(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	files := map[string]string{
		"Science/2023-01-02 Title [abcdefghijk].mp4":  "science video",
		"Science/2023-01-02 Title [abcdefghijk].webp": "webp thumb",
		"Trading/x.mp4":                                 "trading video",
		"Trading/x.info.json":                           `{"id":"bcdefghijkl","upload_date":"20220315"}`,
		"Trading/x.jpg":                                 "trading thumb",
		"Misc/noid.mp4":                                 "no id video",
		"Misc/readme.txt":                               "not a video",
		"Computer Science/" + pfName + ".mp4":           "pinchflat video",
		"Computer Science/" + pfName + "-thumb.jpg":     "pinchflat thumb",
		"Computer Science/" + pfName + ".nfo":           nfo("cdefghijklm", "What is a Computer? | Crash Course: Ep 1", "CrashCourse", "2016-08-01 00:00:00Z"),
		"Computer Science/" + pfName + ".en.srt":        "1\n00:00:01,000 --> 00:00:02,000\nHello\n",
		"Options Spreads Tutorials/" + pf2Name + ".mp4": "condor video",
		"Options Spreads Tutorials/" + pf2Name + ".nfo": nfo("defghijklmn", "Iron Condor | Explained", "Options Guy", "2019-05-04 00:00:00Z"),
	}
	for name, body := range files {
		p := filepath.Join(src, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Date(2021, 7, 8, 9, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(src, "Trading", "x.mp4"), old, old); err != nil {
		t.Fatal(err)
	}
	return src
}

// fakeFFmpeg writes a stand-in ffmpeg: a frame or a conversion writes
// "jpeg from ffmpeg" to its output, a bare "-i" prints a duration of 100 s.
// Every run's arguments go to <dir>/ffmpeg.log, one run per line.
func fakeFFmpeg(t *testing.T) (bin, log string) {
	t.Helper()
	dir := t.TempDir()
	bin, log = filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "ffmpeg.log")
	script := fmt.Sprintf(`#!/bin/sh
echo "$*" >> %[1]q
out=""; frame=0; pipe=0
for a in "$@"; do out="$a"; [ "$a" = "-frames:v" ] && frame=1; [ "$a" = pipe:0 ] && pipe=1; done
if [ $pipe = 1 ]; then printf 'stdin: %%s\n' "$(cat)" >> %[1]q; fi
if [ $frame = 1 ]; then printf 'jpeg from ffmpeg' > "$out"; exit 0; fi
echo "  Duration: 00:01:40.00, start: 0.000000, bitrate: 23 kb/s" >&2
exit 1
`, log)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

// adoptRig is a rig with a fake ffmpeg and the adopt job.
func adoptRig(t *testing.T) (*rig, string) {
	t.Helper()
	r := newRig(t, "ok")
	ff, log := fakeFFmpeg(t)
	ytdlp := r.svc.Tools().YTDLP
	r.svc.Tools = func() Tools { return Tools{YTDLP: ytdlp, FFmpeg: ff} }
	r.q.Handle(KindAdopt, r.svc.HandleAdopt)
	return r, log
}

func (r *rig) adopt(in AdoptRequest) map[string]any {
	r.t.Helper()
	params := map[string]any{"user_id": in.UserID, "path": in.Path}
	if len(in.Playlists) > 0 {
		params["playlists"] = in.Playlists
	}
	if _, _, err := r.q.Enqueue(context.Background(), KindAdopt, nil, params, "", database.System); err != nil {
		r.t.Fatal(err)
	}
	r.drain()
	js := r.jobs(KindAdopt)
	j := js[len(js)-1]
	if j.State != "done" {
		r.t.Fatalf("adopt job %s: %+v", j.State, j.Error)
	}
	return j.Result
}

// snapshot reads every file under dir, by slash path.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPlanAdoptReadsIDs(t *testing.T) {
	t.Parallel()
	r, ffLog := adoptRig(t)
	src := adoptTree(t)
	addVideo(t, r.s, Video{UserID: anna, VideoID: "defghijklmn", State: StateDone})

	items, err := r.svc.PlanAdopt(context.Background(), AdoptRequest{UserID: anna, Path: src})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]AdoptItem{}
	for _, it := range items {
		got[it.From] = it
	}
	want := map[string]AdoptItem{
		"Science/2023-01-02 Title [abcdefghijk].mp4": {VideoID: "abcdefghijk", Folder: "Science",
			To: "7-Anna/Science/2023-01-02 Title [abcdefghijk].mp4"},
		"Trading/x.mp4": {VideoID: "bcdefghijkl", Folder: "Trading", To: "7-Anna/Trading/2022-03-15 x [bcdefghijkl].mp4"},
		"Misc/noid.mp4": {Folder: "Misc", Reason: "no YouTube id in an .nfo, an .info.json or the name"},
		"Computer Science/" + pfName + ".mp4": {VideoID: "cdefghijklm", Folder: "Computer Science",
			To: "7-Anna/CrashCourse/2016-08-01 What is a Computer_ _ Crash Course_ Ep 1 [cdefghijklm].mp4"},
		"Options Spreads Tutorials/" + pf2Name + ".mp4": {VideoID: "defghijklmn", Folder: "Options Spreads Tutorials",
			To: "7-Anna/Options Guy/2019-05-04 Iron Condor _ Explained [defghijklmn].mp4", Reason: "already in the library"},
	}
	if len(got) != len(want) {
		t.Fatalf("items %+v", items)
	}
	for from, w := range want {
		w.From = from
		if got[from] != w {
			t.Errorf("%s:\n got %+v\nwant %+v", from, got[from], w)
		}
	}
	// No lookups and nothing written.
	if calls := r.fake.Calls(t); len(calls) != 0 {
		t.Fatalf("yt-dlp ran %v", calls)
	}
	if _, err := os.Stat(ffLog); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ffmpeg ran during a plan")
	}
	if files := snapshot(t, r.root); len(files) != 0 {
		t.Fatalf("plan wrote %v", files)
	}
}

func TestPlanAdoptRefuses(t *testing.T) {
	t.Parallel()
	r, _ := adoptRig(t)
	src := adoptTree(t)
	inside := filepath.Join(r.root, "old")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, c := range []struct {
		name string
		in   AdoptRequest
		want error
	}{
		{"relative", AdoptRequest{UserID: anna, Path: "Science"}, library.ErrFolderPath},
		{"drive root", AdoptRequest{UserID: anna, Path: "/"}, library.ErrFolderPath},
		{"data folder", AdoptRequest{UserID: anna, Path: r.s.Dir()}, library.ErrFolderPath},
		{"missing", AdoptRequest{UserID: anna, Path: filepath.Join(src, "nope")}, library.ErrFolderAbsent},
		{"inside the YouTube folder", AdoptRequest{UserID: anna, Path: inside}, ErrAdoptFolder},
		{"the YouTube folder", AdoptRequest{UserID: anna, Path: r.root}, ErrAdoptFolder},
		{"no youtube", AdoptRequest{UserID: 2, Path: src}, ErrAdoptPerson},
		{"kid", AdoptRequest{UserID: kid, Path: src}, ErrAdoptPerson},
		{"nobody", AdoptRequest{UserID: 99, Path: src}, ErrAdoptPerson},
		{"not a playlist", AdoptRequest{UserID: anna, Path: src, Playlists: map[string]string{"Science": "https://youtu.be/dQw4w9WgXcQ"}}, ErrAdoptPlaylist},
		{"no folder name", AdoptRequest{UserID: anna, Path: src, Playlists: map[string]string{"": "https://www.youtube.com/playlist?list=PLtestlist01"}}, ErrAdoptPlaylist},
	} {
		if _, err := r.svc.PlanAdopt(ctx, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	// A source holding the YouTube folder.
	outer := t.TempDir()
	if err := os.Mkdir(filepath.Join(outer, "yt"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, r.s, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, keyFolder, filepath.Join(outer, "yt"), database.System, time.Now())
	})
	if _, err := r.svc.PlanAdopt(ctx, AdoptRequest{UserID: anna, Path: outer}); !errors.Is(err, ErrAdoptFolder) {
		t.Errorf("around the YouTube folder: %v", err)
	}
	write(t, r.s, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, keyFolder, "", database.System, time.Now())
	})
	if _, err := r.svc.PlanAdopt(ctx, AdoptRequest{UserID: anna, Path: src}); !errors.Is(err, ErrAdoptNoRoot) {
		t.Errorf("no YouTube folder: %v", err)
	}
}

func TestAdoptCopiesNeverMoves(t *testing.T) {
	t.Parallel()
	r, ffLog := adoptRig(t)
	src := adoptTree(t)
	before := snapshot(t, src)
	// Found on YouTube: these take its title, channel and date.
	r.putJSON("info-abcdefghijk.json", info("abcdefghijk", "Real Title", "Real Channel", 1709650800))
	r.putJSON("info-defghijklmn.json", info("defghijklmn", "Iron Condor | Explained", "Options Guy", 1556928000))
	// Gone from YouTube: these keep what the files say.
	r.put("gone-bcdefghijkl", nil)
	r.put("gone-cdefghijklm", nil)
	// A video a check once skipped is adopted into its row.
	skippedRow := addVideo(t, r.s, Video{UserID: anna, VideoID: "defghijklmn", State: StateSkipped})

	res := r.adopt(AdoptRequest{UserID: anna, Path: src})
	if fmt.Sprint(res) != "map[copied:2 failed:0 gone:2 skipped:1]" {
		t.Fatalf("result %v", res)
	}
	if after := snapshot(t, src); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("the source changed:\n%v\n%v", before, after)
	}

	science := "7-Anna/Real Channel/2024-03-05 Real Title [abcdefghijk]"
	trading := "7-Anna/Trading/2022-03-15 x [bcdefghijkl]"
	pinch := "7-Anna/CrashCourse/2016-08-01 What is a Computer_ _ Crash Course_ Ep 1 [cdefghijklm]"
	condor := "7-Anna/Options Guy/2019-05-04 Iron Condor _ Explained [defghijklmn]"
	wantFiles := map[string]string{
		science + ".mp4":  "science video",
		science + ".jpg":  "jpeg from ffmpeg", // from the .webp
		trading + ".mp4":  "trading video",
		trading + ".jpg":  "trading thumb",
		pinch + ".mp4":    "pinchflat video",
		pinch + ".jpg":    "pinchflat thumb", // the -thumb.jpg
		pinch + ".en.srt": before["Computer Science/"+pfName+".en.srt"],
		condor + ".mp4":   "condor video",
		condor + ".jpg":   "jpeg from ffmpeg", // a frame
	}
	if got := snapshot(t, r.root); fmt.Sprint(got) != fmt.Sprint(wantFiles) {
		t.Fatalf("YouTube folder:\n got %v\nwant %v", got, wantFiles)
	}
	ff, err := os.ReadFile(ffLog)
	if err != nil {
		t.Fatal(err)
	}
	// The frame is taken at 10 % of the 61 s yt-dlp gave, from the copy;
	// the .webp reaches ffmpeg on stdin. ffmpeg never opens the source.
	if !strings.Contains(string(ff), "-ss 6.10 -i "+filepath.Join(r.root, filepath.FromSlash(condor)+".mp4")) ||
		!strings.Contains(string(ff), "-i pipe:0") || !strings.Contains(string(ff), "stdin: webp thumb") || strings.Contains(string(ff), src) {
		t.Fatalf("ffmpeg runs:\n%s", ff)
	}

	vs := r.videos(anna)
	for id, w := range map[string]struct {
		rel, title, channel, day string
	}{
		"abcdefghijk": {science + ".mp4", "Real Title", "Real Channel", "2024-03-05"},
		"bcdefghijkl": {trading + ".mp4", "x", "", "2022-03-15"},
		"cdefghijklm": {pinch + ".mp4", pfTitle, "CrashCourse", "2016-08-01"},
		"defghijklmn": {condor + ".mp4", "Iron Condor | Explained", "Options Guy", "2019-05-04"},
	} {
		v := vs[id]
		if v.State != StateDone || len(v.FollowIDs) != 0 || !v.Manual || v.RelativePath != w.rel || v.Title != w.title || v.ChannelName != w.channel ||
			v.UploadedAt == nil || v.UploadedAt.UTC().Format("2006-01-02") != w.day || v.SizeBytes != int64(len(wantFiles[w.rel])) {
			t.Errorf("%s: row %+v", id, v)
		}
	}
	if vs["defghijklmn"].ID != skippedRow {
		t.Errorf("the skipped row was not reused")
	}
	if vs["bcdefghijkl"].DurationSeconds != 100 || vs["abcdefghijk"].DurationSeconds != 61 {
		t.Errorf("durations %d %d", vs["bcdefghijkl"].DurationSeconds, vs["abcdefghijk"].DurationSeconds)
	}
	if len(vs) != 4 {
		t.Fatalf("rows %v", vs)
	}
	// The job keeps counts only.
	j := r.jobs(KindAdopt)[0]
	if b, _ := json.Marshal(j); strings.Contains(string(b), "Title") || strings.Contains(string(b), "abcdefghijk") {
		t.Fatalf("job holds a title: %+v", j)
	}
	// Live events go to Anna only.
	for _, ev := range r.live.all() {
		if ev.User != anna {
			t.Fatalf("event %+v", ev)
		}
	}

	// A second run finds everything known and copies nothing.
	res = r.adopt(AdoptRequest{UserID: anna, Path: src})
	if fmt.Sprint(res) != "map[copied:0 failed:0 gone:0 skipped:5]" {
		t.Fatalf("second result %v", res)
	}
	if got := snapshot(t, r.root); fmt.Sprint(got) != fmt.Sprint(wantFiles) {
		t.Fatalf("second run changed the YouTube folder: %v", got)
	}

	// Removing an adopted video takes its subtitles too.
	if err := r.svc.RemoveVideo(context.Background(), anna, vs["cdefghijklm"].ID); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".mp4", ".jpg", ".en.srt"} {
		if _, err := os.Stat(filepath.Join(r.root, filepath.FromSlash(pinch+ext))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left after remove", ext)
		}
	}
}

func TestAdoptPlaylistFollowNoBackfill(t *testing.T) {
	t.Parallel()
	r, _ := adoptRig(t)
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Course"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"Course/01 - First.mp4": "first video",
		"Course/01 - First.nfo": nfo("aaaaaaaaaa1", "First", "Some Owner", "2020-01-01 00:00:00Z"),
	} {
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := r.adopt(AdoptRequest{UserID: anna, Path: src, Playlists: map[string]string{
		"Course":  "https://www.youtube.com/playlist?list=PLtestlist01",
		"Missing": "https://www.youtube.com/playlist?list=PLnotadopted",
	}})
	if fmt.Sprint(res) != "map[copied:1 failed:0 follows:1 gone:0 linked:0 skipped:0]" {
		t.Fatalf("result %v", res)
	}
	fs, err := FollowsOf(context.Background(), r.s.Read(), anna)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 || fs[0].Kind != KindPlaylist || fs[0].YTID != "PLtestlist01" || fs[0].Title != "Test playlist" ||
		fs[0].Backfill == nil || *fs[0].Backfill != 0 || !fs[0].Enabled || fs[0].LastCheckedAt != nil {
		t.Fatalf("follows %+v", fs)
	}
	v := r.videos(anna)["aaaaaaaaaa1"]
	if !slices.Equal(v.FollowIDs, []int64{fs[0].ID}) || v.Manual || v.State != StateDone || !strings.HasPrefix(v.RelativePath, "7-Anna/Test playlist/") {
		t.Fatalf("adopted row %+v", v)
	}
	if n := len(r.jobs(KindCheck)); n != 0 {
		t.Fatalf("adopt queued %d checks", n)
	}

	// Its first check records the rest of the playlist as skipped.
	r.check(fs[0].ID)
	vs := r.videos(anna)
	if vs["aaaaaaaaaa1"].State != StateDone || vs["aaaaaaaaaa2"].State != StateSkipped || vs["aaaaaaaaaa3"].State != StateSkipped {
		t.Fatalf("after check %v", countStates(vs))
	}
	if n := len(r.jobs(KindDownload)); n != 0 {
		t.Fatalf("check queued %d downloads", n)
	}
}

// With a real ffmpeg, a frame and a converted .png both come out as JPEG
// and the length is read. Skipped where ffmpeg is not installed.
func TestAdoptThumbsWithRealFFmpeg(t *testing.T) {
	t.Parallel()
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video, png := filepath.Join(dir, "v.mp4"), filepath.Join(dir, "p.png")
	for _, args := range [][]string{
		{"-f", "lavfi", "-i", "testsrc=duration=5:size=160x120:rate=10", "-pix_fmt", "yuv420p", video},
		{"-f", "lavfi", "-i", "testsrc=size=64x48", "-frames:v", "1", png},
	} {
		if out, err := exec.Command(ff, append([]string{"-nostdin", "-loglevel", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("ffmpeg cannot make the fixtures: %v %s", err, out)
		}
	}
	ctx := context.Background()
	if n := videoSeconds(ctx, ff, video); n != 5 {
		t.Fatalf("length %d", n)
	}
	in, err := os.Open(png)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	for name, input := range map[string][]string{"frame.jpg": {"-ss", "0.50", "-i", video}, "png.jpg": {"-i", "pipe:0"}} {
		dst := filepath.Join(dir, name)
		var stdin *os.File
		if name == "png.jpg" {
			stdin = in
		}
		if err := ffmpegJPEG(ctx, ff, dst, stdin, input...); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(dst)
		if err != nil || len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 {
			t.Fatalf("%s is not a JPEG: %v", name, err)
		}
	}
}

// Symlinks in the source folder are never followed: a linked video is not
// found, a linked .nfo or .srt is not read and a linked thumbnail gives way
// to a frame, so nothing outside the folder is read or copied.
func TestAdoptIgnoresSymlinks(t *testing.T) {
	t.Parallel()
	r, _ := adoptRig(t)
	outside := t.TempDir()
	for name, body := range map[string]string{
		"secret.jpg": "secret thumb",
		"secret.nfo": nfo("zzzzzzzzzzz", "Secret", "Secret Channel", "2001-01-01 00:00:00Z"),
		"secret.srt": "secret subtitles",
		"secret.mp4": "secret video",
	} {
		if err := os.WriteFile(filepath.Join(outside, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := t.TempDir()
	course := filepath.Join(src, "Course")
	if err := os.Mkdir(course, 0o755); err != nil {
		t.Fatal(err)
	}
	stem := filepath.Join(course, "01 - A [abcdefghijk]")
	if err := os.WriteFile(stem+".mp4", []byte("plain video"), 0o644); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		stem + "-thumb.jpg":                 "secret.jpg",
		stem + ".nfo":                       "secret.nfo",
		stem + ".en.srt":                    "secret.srt",
		filepath.Join(course, "02 - B.mp4"): "secret.mp4",
		filepath.Join(course, "02 - B.nfo"): "secret.nfo",
	} {
		if err := os.Symlink(filepath.Join(outside, target), link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(src, "Linked")); err != nil {
		t.Fatal(err)
	}
	r.put("gone-abcdefghijk", nil)

	items, err := r.svc.PlanAdopt(context.Background(), AdoptRequest{UserID: anna, Path: src})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].VideoID != "abcdefghijk" || !strings.HasPrefix(items[0].To, "7-Anna/Course/") ||
		!strings.HasSuffix(items[0].To, " A [abcdefghijk].mp4") {
		t.Fatalf("items %+v", items)
	}
	res := r.adopt(AdoptRequest{UserID: anna, Path: src})
	if fmt.Sprint(res) != "map[copied:0 failed:0 gone:1 skipped:0]" {
		t.Fatalf("result %v", res)
	}
	files := snapshot(t, r.root)
	for p, body := range files {
		if strings.Contains(body, "secret") || strings.Contains(p, "Secret") || strings.Contains(p, ".srt") {
			t.Fatalf("followed a symlink: %s = %q", p, body)
		}
	}
	if len(files) != 2 {
		t.Fatalf("files %v", files)
	}
	for p, body := range files {
		if strings.HasSuffix(p, ".jpg") && body != "jpeg from ffmpeg" {
			t.Fatalf("thumbnail %q", body)
		}
	}
	v := r.videos(anna)["abcdefghijk"]
	if v.Title != "A" || v.ChannelName != "" || len(r.videos(anna)) != 1 {
		t.Fatalf("rows %+v", r.videos(anna))
	}
}
