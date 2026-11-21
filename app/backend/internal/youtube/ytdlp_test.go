package youtube

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/youtube/youtubetest"
)

func fakeRunner(t *testing.T, mode string) (Runner, youtubetest.Fake) {
	t.Helper()
	f := youtubetest.NewFake(t, mode)
	return Runner{Tools: Tools{YTDLP: f.Bin, FFmpeg: "/opt/ff/ffmpeg"}}, f
}

func mustLink(t *testing.T, raw string) Link {
	t.Helper()
	l, err := ParseLink(raw)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestFormatFor(t *testing.T) {
	want := "bv*[vcodec^=avc1][height<=1080]+ba[acodec^=mp4a]/bv*[height<=1080]+ba[acodec^=mp4a]/bv*[height<=1080]+ba/b[height<=1080]"
	if got := FormatFor(1080); got != want {
		t.Fatalf("FormatFor(1080) = %q", got)
	}
	if got := FormatFor(720); !strings.Contains(got, "[height<=720]") || strings.Contains(got, "1080") {
		t.Fatalf("FormatFor(720) = %q", got)
	}
	if got := FormatFor(0); got != want {
		t.Fatalf("FormatFor(0) = %q, want the 1080 default", got)
	}
}

func TestRunnerArgsNeverCarryRawInput(t *testing.T) {
	r, f := fakeRunner(t, "log")
	if _, err := r.Probe(context.Background(), mustLink(t, "https://youtu.be/dQw4w9WgXcQ?si=--exec")); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls(t)
	if len(calls) != 1 {
		t.Fatalf("calls = %q", calls)
	}
	args := calls[0]
	if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("args end %q", args)
	}
	if !slices.Equal(args[:3], []string{"--ignore-config", "--no-warnings", "--newline"}) {
		t.Fatalf("args start %q", args)
	}
	if i := slices.Index(args, "--ffmpeg-location"); i < 0 || args[i+1] != "/opt/ff/ffmpeg" {
		t.Fatalf("no --ffmpeg-location in %q", args)
	}
	for _, a := range args {
		if strings.Contains(a, "exec") || strings.Contains(a, "si=") || strings.Contains(a, "youtu.be") {
			t.Fatalf("raw input reached yt-dlp: %q", args)
		}
	}
}

func TestRunnerRebuildsURLFromKindAndID(t *testing.T) {
	r, f := fakeRunner(t, "ok")
	ctx := context.Background()
	forged := []struct {
		l    Link
		want string
	}{
		{Link{KindVideo, "dQw4w9WgXcQ", "https://evil.example/x"}, "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{Link{KindPlaylist, "PLtestlist01", "--exec=touch /tmp/x"}, "https://www.youtube.com/playlist?list=PLtestlist01"},
		{Link{KindChannel, "@Veritasium", "https://evil.example/@Veritasium"}, "https://www.youtube.com/@Veritasium/videos"},
	}
	for _, c := range forged {
		p, err := r.Probe(ctx, c.l)
		if err != nil {
			t.Fatalf("Probe(%+v): %v", c.l, err)
		}
		calls := f.Calls(t)
		args := calls[len(calls)-1]
		if n := len(args); args[n-2] != "--" || args[n-1] != c.want {
			t.Fatalf("Probe(%+v) args end %q, want -- %q", c.l, args[n-2:], c.want)
		}
		if strings.Contains(p.Link.URL, "evil") || strings.Contains(p.Link.URL, "exec") {
			t.Fatalf("Probe kept the forged URL: %q", p.Link.URL)
		}
	}
	n := len(f.Calls(t))
	if _, err := r.Entries(ctx, Link{KindChannel, "@Veritasium", "--exec=x"}, true); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls(t)[n:] {
		if u := c[len(c)-1]; !strings.HasPrefix(u, "https://www.youtube.com/@Veritasium/") || c[len(c)-2] != "--" {
			t.Fatalf("Entries args end %q", c[len(c)-2:])
		}
	}
	bad := []Link{
		{KindVideo, "--exec=xxxx", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{KindPlaylist, "PL x/../..", "https://www.youtube.com/playlist?list=PLtestlist01"},
		{KindChannel, "evil.example/x", "https://www.youtube.com/@Veritasium"},
		{KindChannel, "c/../../x", "https://www.youtube.com/@Veritasium"},
		{"other", "dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
	}
	n = len(f.Calls(t))
	for _, l := range bad {
		if _, err := r.Probe(ctx, l); !errors.Is(err, ErrNotYouTube) {
			t.Errorf("Probe(%+v) err = %v", l, err)
		}
		if _, err := r.Entries(ctx, l, false); !errors.Is(err, ErrNotYouTube) {
			t.Errorf("Entries(%+v) err = %v", l, err)
		}
	}
	if _, err := r.Entries(ctx, Link{KindVideo, "dQw4w9WgXcQ", ""}, false); !errors.Is(err, ErrNotYouTube) {
		t.Errorf("Entries(video) err = %v", err)
	}
	if got := len(f.Calls(t)); got != n {
		t.Fatalf("refused links still ran yt-dlp %d times", got-n)
	}
}

func TestRunnerWithoutFFmpegLeavesItToYTDLP(t *testing.T) {
	f := youtubetest.NewFake(t, "ok")
	r := Runner{Tools: Tools{YTDLP: f.Bin}}
	if _, err := r.Version(context.Background()); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.Calls(t)[0], "--ffmpeg-location") {
		t.Fatal("--ffmpeg-location passed without an ffmpeg")
	}
}

func TestRunnerNeedsYTDLP(t *testing.T) {
	_, err := Runner{}.Probe(context.Background(), mustLink(t, "https://youtu.be/dQw4w9WgXcQ"))
	if !errors.Is(err, ErrNoYTDLP) {
		t.Fatalf("err = %v", err)
	}
}

func TestProbe(t *testing.T) {
	r, f := fakeRunner(t, "ok")
	ctx := context.Background()

	v, err := r.Probe(ctx, mustLink(t, "https://youtu.be/dQw4w9WgXcQ"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Link.Kind != KindVideo || v.Title != "Never Gonna Give You Up" || v.ChannelName != "Rick Astley" ||
		v.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" || v.DurationSeconds != 212 || v.Count != 1 || v.LiveStatus != "not_live" ||
		len(v.Entries) != 1 || v.Entries[0].ID != "dQw4w9WgXcQ" {
		t.Fatalf("video probe = %+v", v)
	}
	if args := f.Calls(t)[0]; !slices.Contains(args, "-J") || !slices.Contains(args, "--no-playlist") || slices.Contains(args, "--flat-playlist") {
		t.Fatalf("video probe args %q", args)
	}

	p, err := r.Probe(ctx, mustLink(t, "https://www.youtube.com/playlist?list=PLtestlist01"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Test playlist" || p.ChannelName != "Some Owner" || p.Count != 3 || len(p.Entries) != 3 || !p.Entries[2].Short {
		t.Fatalf("playlist probe = %+v", p)
	}
	args := f.Calls(t)[1]
	if i := slices.Index(args, "--playlist-end"); i < 0 || args[i+1] != "5" || !slices.Contains(args, "--flat-playlist") {
		t.Fatalf("playlist probe args %q", args)
	}

	c, err := r.Probe(ctx, mustLink(t, "https://www.youtube.com/@Veritasium"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Veritasium" || c.ChannelID != "UCHnyfMqiRRG1u-2MsSQLbXA" || len(c.Entries) != 2 {
		t.Fatalf("channel probe = %+v", c)
	}
	if args := f.Calls(t)[2]; args[len(args)-1] != "https://www.youtube.com/@Veritasium/videos" {
		t.Fatalf("channel probe args %q", args)
	}
}

func TestEntriesChannelTabs(t *testing.T) {
	r, f := fakeRunner(t, "ok")
	ctx := context.Background()
	l := mustLink(t, "https://www.youtube.com/@Veritasium")
	ids := func(es []Entry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}

	es, err := r.Entries(ctx, l, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sssssssss02", "vvvvvvvvvv1", "sssssssss01", "vvvvvvvvvv2"}
	if !slices.Equal(ids(es), want) {
		t.Fatalf("entries = %q, want %q", ids(es), want)
	}
	if es[0].LiveStatus != "is_upcoming" || es[2].LiveStatus != "was_live" || es[1].Timestamp != 1700000300 || es[1].DurationSeconds != 600 {
		t.Fatalf("entries = %+v", es)
	}
	var tabs []string
	for _, c := range f.Calls(t) {
		tabs = append(tabs, c[len(c)-1])
		if i := slices.Index(c, "--extractor-args"); i < 0 || c[i+1] != "youtubetab:approximate_date" {
			t.Fatalf("no approximate_date in %q", c)
		}
	}
	if !slices.Equal(tabs, []string{"https://www.youtube.com/@Veritasium/videos", "https://www.youtube.com/@Veritasium/streams"}) {
		t.Fatalf("tabs = %q", tabs)
	}

	es, err = r.Entries(ctx, l, true)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"sssssssss02", "hhhhhhhhhh1", "vvvvvvvvvv1", "sssssssss01", "vvvvvvvvvv2"}
	if !slices.Equal(ids(es), want) || !es[1].Short {
		t.Fatalf("entries with shorts = %+v", es)
	}

	pl, err := r.Entries(ctx, mustLink(t, "https://www.youtube.com/playlist?list=PLtestlist01"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(pl), []string{"aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa3"}) {
		t.Fatalf("playlist entries = %+v", pl)
	}
}

func TestEntriesToleratesMissingTab(t *testing.T) {
	r, _ := fakeRunner(t, "nostreams")
	es, err := r.Entries(context.Background(), mustLink(t, "https://www.youtube.com/@Veritasium"), false)
	if err != nil || len(es) != 2 {
		t.Fatalf("entries = %+v, %v", es, err)
	}
}

func TestInfo(t *testing.T) {
	r, _ := fakeRunner(t, "ok")
	in, err := r.Info(context.Background(), "abcdefghijk")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2024, 3, 5, 15, 0, 0, 0, time.UTC)
	if in.Title != "A video" || in.ChannelName != "Rick Astley" || in.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" ||
		in.DurationSeconds != 213 || in.UploadedAt == nil || !in.UploadedAt.Equal(day) || in.LiveStatus != "not_live" {
		t.Fatalf("info = %+v", in)
	}
	if _, err := r.Info(context.Background(), "bad id"); err == nil {
		t.Fatal("Info took an invalid id")
	}

	r, _ = fakeRunner(t, "gone")
	if _, err := r.Info(context.Background(), "abcdefghijk"); err == nil || err.Error() != "ERROR: [youtube] abcdefghijk: Video unavailable" {
		t.Fatalf("gone err = %v", err)
	}

	r, _ = fakeRunner(t, "live")
	in, err = r.Info(context.Background(), "abcdefghijk")
	if err != nil || in.LiveStatus != "is_upcoming" {
		t.Fatalf("live info = %+v, %v", in, err)
	}
}

func TestDownloadParsesProgressAndReturnsPath(t *testing.T) {
	r, f := fakeRunner(t, "ok")
	outBase := filepath.Join(t.TempDir(), "2024-03-05 100% %(id)s [abcdefghijk]")
	var seen []Progress
	got, err := r.Download(context.Background(), "abcdefghijk", 720, outBase, func(p Progress) { seen = append(seen, p) })
	if err != nil {
		t.Fatal(err)
	}
	if got != outBase+".mp4" {
		t.Fatalf("path = %q", got)
	}
	for _, ext := range []string{".mp4", ".jpg"} {
		if _, err := os.Stat(outBase + ext); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) < 3 {
		t.Fatalf("progress = %+v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].Done <= seen[i-1].Done {
			t.Fatalf("progress not increasing: %+v", seen)
		}
	}
	if seen[0].Total != 1000 || seen[len(seen)-1].Done != 1000 {
		t.Fatalf("progress = %+v", seen)
	}
	args := f.Calls(t)[0]
	if i := slices.Index(args, "-f"); i < 0 || args[i+1] != FormatFor(720) {
		t.Fatalf("format arg in %q", args)
	}
	if args[len(args)-1] != "https://www.youtube.com/watch?v=abcdefghijk" || args[len(args)-2] != "--" {
		t.Fatalf("args end %q", args)
	}
}

func TestDownloadUsesEstimateWhenTotalUnknown(t *testing.T) {
	cases := map[string]Progress{
		"HAYT 10 NA 200.5": {10, 200},
		"HAYT 10 300 NA":   {10, 300},
		"HAYT NA NA NA":    {0, 0},
	}
	for line, want := range cases {
		if got, ok := parseProgress(line); !ok || got != want {
			t.Errorf("parseProgress(%q) = %+v, %v", line, got, ok)
		}
	}
	if _, ok := parseProgress("/some/file.mp4"); ok {
		t.Error("a path parsed as progress")
	}
}

func TestDownloadStallIsKilled(t *testing.T) {
	r, f := fakeRunner(t, "hang")
	r.Stall = time.Second
	dir := t.TempDir()
	outBase := filepath.Join(dir, "0000-00-00 x [abcdefghijk]")
	other := filepath.Join(dir, "other.part")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := r.Download(context.Background(), "abcdefghijk", 1080, outBase, nil)
	if err == nil || !strings.Contains(err.Error(), "no progress") {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("stall took %s to stop", d)
	}
	beat := filepath.Join(f.Dir, "heartbeat")
	st1, err := os.Stat(beat)
	if err != nil {
		t.Fatalf("fake never ran its loop: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if st2, err := os.Stat(beat); err != nil || !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("fake still running after the stall")
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*"))
	if !slices.Equal(left, []string{other}) {
		t.Fatalf("left behind %q", left)
	}
}

func TestDownloadErrorKeepsLastErrorLine(t *testing.T) {
	r, _ := fakeRunner(t, "fail")
	dir := t.TempDir()
	outBase := filepath.Join(dir, "0000-00-00 x [abcdefghijk]")
	_, err := r.Download(context.Background(), "abcdefghijk", 1080, outBase, nil)
	if err == nil || err.Error() != "ERROR: [youtube] x: Video unavailable" {
		t.Fatalf("err = %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
		t.Fatalf("left behind %q", left)
	}
}

func TestDownloadCancelled(t *testing.T) {
	r, _ := fakeRunner(t, "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := r.Download(ctx, "abcdefghijk", 1080, filepath.Join(t.TempDir(), "x"), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindToolsPrefersEnv(t *testing.T) {
	env := map[string]string{"HOLYARRS_YTDLP": "/config/bin/yt-dlp"}
	look := func(name string) (string, error) {
		switch {
		case name == "/nowhere/yt-dlp":
			return "", errors.New("no such file")
		case strings.HasPrefix(name, "/"):
			return name, nil
		}
		return "/usr/bin/" + name, nil
	}
	tl := FindTools(func(k string) string { return env[k] }, look)
	if tl.YTDLP != "/config/bin/yt-dlp" || tl.FFmpeg != "/usr/bin/ffmpeg" || len(tl.Missing()) != 0 {
		t.Fatalf("tools = %+v", tl)
	}
	// A set path that is not a program is missing; PATH is not tried.
	env["HOLYARRS_YTDLP"] = "/nowhere/yt-dlp"
	tl = FindTools(func(k string) string { return env[k] }, look)
	if !slices.Equal(tl.Missing(), []string{"yt-dlp"}) {
		t.Fatalf("set path pointing nowhere: %+v", tl)
	}
	tl = FindTools(func(string) string { return "" }, look)
	if tl.YTDLP != "/usr/bin/yt-dlp" {
		t.Fatalf("tools = %+v", tl)
	}
	tl = FindTools(func(string) string { return "" }, func(string) (string, error) { return "", errors.New("not found") })
	if !slices.Equal(tl.Missing(), []string{"yt-dlp", "ffmpeg"}) {
		t.Fatalf("missing = %q", tl.Missing())
	}
}

func TestVersionAndSelfUpdate(t *testing.T) {
	r, f := fakeRunner(t, "ok")
	ctx := context.Background()
	v, err := r.Version(ctx)
	if err != nil || v != "2026.10.26" {
		t.Fatalf("version = %q, %v", v, err)
	}
	out, err := r.SelfUpdate(ctx)
	if err != nil || out != "yt-dlp is up to date (2026.10.26)" {
		t.Fatalf("self-update = %q, %v", out, err)
	}
	if c := f.Calls(t); !slices.Contains(c[len(c)-1], "-U") {
		t.Fatalf("self-update args %q", c[len(c)-1])
	}

	if os.Getuid() == 0 {
		return // root writes read-only files
	}
	if err := os.Chmod(f.Bin, 0o555); err != nil {
		t.Fatal(err)
	}
	n := len(f.Calls(t))
	out, err = r.SelfUpdate(ctx)
	if err != nil || out != "" || len(f.Calls(t)) != n {
		t.Fatalf("read-only binary: %q, %v", out, err)
	}
}
