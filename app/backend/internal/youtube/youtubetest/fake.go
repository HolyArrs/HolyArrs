// Package youtubetest is a stand-in yt-dlp for tests: a small sh script that
// answers from canned JSON, logs every argument list and fakes downloads.
package youtubetest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Fake is one fake yt-dlp. Bin is the program to run, ArgsLog the file each
// run's arguments are appended to and Dir the folder both live in.
type Fake struct {
	Dir, Bin, ArgsLog string
}

// endOfCall ends one run's arguments in ArgsLog.
const endOfCall = "=end="

// NewFake writes a fake yt-dlp into a fresh temp folder. Modes:
//
//	ok, log    answer from the canned JSON and download a small file
//	fail       downloads fail with "ERROR: [youtube] x: Video unavailable"
//	hang       downloads print one progress line, then go quiet for ever
//	gone       video lookups fail with "Video unavailable"
//	live       video lookups say the video is an upcoming live stream
//	nostreams  the channel has no streams tab
//	warn       a playlist lists with a warning, as when a page of it failed
//
// The JSON comes from the testdata folder next to this package, or from the
// folder in FAKE_YTDLP_DATA when that is set. A watch URL for dQw4w9WgXcQ
// answers video.json; any other video id answers info.json with its id.
func NewFake(t testing.TB, mode string) Fake {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake yt-dlp is a shell script")
	}
	_, here, _, _ := runtime.Caller(0)
	data := filepath.Join(filepath.Dir(here), "..", "testdata")
	dir := t.TempDir()
	f := Fake{Dir: dir, Bin: filepath.Join(dir, "yt-dlp"), ArgsLog: filepath.Join(dir, "args.log")}
	script := strings.NewReplacer(
		"@MODE@", quote(mode),
		"@DATA@", quote(data),
		"@DIR@", quote(dir),
		"@LOG@", quote(f.ArgsLog),
		"@END@", endOfCall,
	).Replace(fakeScript)
	if err := os.WriteFile(f.Bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

// Calls reads back every run's arguments, oldest first.
func (f Fake) Calls(t testing.TB) [][]string {
	t.Helper()
	b, err := os.ReadFile(f.ArgsLog)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	var cur []string
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if line == endOfCall {
			calls = append(calls, cur)
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return calls
}

// quote makes s one single-quoted sh word.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const fakeScript = `#!/bin/sh
MODE=@MODE@
DATA="${FAKE_YTDLP_DATA:-}"
[ -n "$DATA" ] || DATA=@DATA@
DIR=@DIR@
for a in "$@"; do printf '%s\n' "$a" >> @LOG@; done
printf '%s\n' '@END@' >> @LOG@

out=""; url=""; prev=""; op=json
for a in "$@"; do
	case "$a" in
	--version) op=version ;;
	-U) op=update ;;
	esac
	[ "$prev" = "-o" ] && out="$a" && op=download
	prev="$a"
	url="$a"
done

case "$op" in
version) echo "2026.10.26"; exit 0 ;;
update) echo "Current version: stable@2026.10.26"; echo "yt-dlp is up to date (2026.10.26)"; exit 0 ;;
esac

if [ "$op" = download ]; then
	base=$(printf '%s' "${out%".%(ext)s"}" | sed 's/%%/%/g')
	case "$MODE" in
	fail)
		: > "$base.f137.mp4.part"
		echo "[youtube] x: Downloading webpage"
		echo "ERROR: [youtube] x: Video unavailable" >&2
		exit 1 ;;
	hang)
		: > "$base.f137.mp4.part"
		: > "$base.f137.mp4.ytdl"
		: > "$base.temp.mp4"
		# A child keeps beating, like the ffmpeg yt-dlp starts, so a test
		# can tell the whole group was stopped. It beats once before the
		# progress line, so a stall kill always finds it running.
		( while :; do date > "$DIR/heartbeat"; sleep 0.05; done ) &
		until [ -s "$DIR/heartbeat" ]; do sleep 0.01; done
		echo "HAYT 10 1000 NA"
		wait ;;
	esac
	# yt-dlp prints progress on stderr when --print makes it quiet; the
	# pauses keep the order across the two streams.
	echo "HAYT 100 1000 NA"
	sleep 0.1
	echo "HAYT 500 NA 1000.0" >&2
	sleep 0.1
	echo "HAYT 1000 1000 NA"
	printf 'fake video' > "$base.mp4"
	printf 'fake thumb' > "$base.jpg"
	echo "$base.mp4"
	exit 0
fi

case "$url" in
*"watch?v="*)
	id="${url##*watch?v=}"
	if [ "$MODE" = gone ]; then echo "ERROR: [youtube] $id: Video unavailable" >&2; exit 1; fi
	if [ "$id" = dQw4w9WgXcQ ]; then cat "$DATA/video.json"; exit 0; fi
	if [ "$MODE" = live ]; then
		sed -e "s/@ID@/$id/g" -e 's/"not_live"/"is_upcoming"/' "$DATA/info.json"
	else
		sed -e "s/@ID@/$id/g" "$DATA/info.json"
	fi
	exit 0 ;;
*"playlist?list="*)
	if [ "$MODE" = warn ]; then echo "WARNING: [youtube:tab] Incomplete data received. Giving up after 3 retries" >&2; fi
	cat "$DATA/playlist.json" ;;
*/videos) cat "$DATA/channel_videos.json" ;;
*/streams)
	if [ "$MODE" = nostreams ]; then
		echo "ERROR: [youtube:tab] @Veritasium: This channel does not have a streams tab" >&2
		exit 1
	fi
	cat "$DATA/channel_streams.json" ;;
*/shorts) cat "$DATA/channel_shorts.json" ;;
*) echo "ERROR: Unsupported URL: $url" >&2; exit 1 ;;
esac
`
