package releasename

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// corpusRow is one release name with the value every quality field should
// read as (testdata/release_corpus.json). An empty value means the field is
// not checked for that name.
type corpusRow struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	Resolution string `json:"resolution"`
	Quality    string `json:"quality"`
	Codec      string `json:"codec"`
	HDR        string `json:"hdr"`
	Audio      string `json:"audio"`
	Edition    string `json:"edition"`
	Group      string `json:"group"`
	Note       string `json:"note"`
}

// corpusSource maps the corpus quality word to the parser's source.
var corpusSource = map[string]string{
	"webdl": "web_dl", "webrip": "web_rip", "hdrip": "web_rip",
	"bluray": "bluray", "bdrip": "bluray", "remux": "remux", "brdisk": "disc_image",
	"hdtv": "tv", "sdtv": "tv", "pdtv": "tv", "dsr": "tv",
	"dvd": "dvd", "dvdrip": "dvd", "vhsrip": "dvd", "ldrip": "dvd",
	"cam": "cam", "telesync": "telesync", "telecine": "telecine",
	"dvdscr": "screener", "r5": "regional", "workprint": "workprint",
}

var corpusAudio = map[string]string{
	"ddp": "eac3", "dd+": "eac3", "eac3": "eac3", "dd": "ac3", "ac3": "ac3", "aac": "aac",
	"dts": "dts", "dts-hd ma": "dts_hd_ma", "dts-hd hra": "dts_hd_hra", "dts-x": "dts_x",
	"dts-es": "dts_es", "truehd": "truehd", "flac": "flac", "opus": "opus", "mp3": "mp3", "lpcm": "lpcm",
}

var corpusEditions = []string{"directors cut", "final cut", "special edition", "open matte", "criterion",
	"extended", "theatrical", "remastered", "imax", "unrated", "anniversary"}

// corpusMiss lists every field of row that the parser reads wrong.
func corpusMiss(row corpusRow) []string {
	r := Parse(row.Name, Hints{Show: row.Kind == "anime"})
	var miss []string
	bad := func(field, got, want string) {
		miss = append(miss, fmt.Sprintf("%s=%q want %q", field, got, want))
	}
	if w := corpusSource[row.Quality]; row.Quality != "" && r.Source.Value != w {
		bad("source", r.Source.Value, w)
	}
	if row.Resolution != "" && strconv.Itoa(r.Resolution.Value) != row.Resolution {
		bad("resolution", strconv.Itoa(r.Resolution.Value), row.Resolution)
	}
	if row.Codec != "" && r.Codec.Value != row.Codec {
		bad("codec", r.Codec.Value, row.Codec)
	}
	want := strings.ReplaceAll(strings.ReplaceAll(row.HDR, "hdr10+", "hdr10plus"), "sdr", "")
	if got := strings.Join(r.HDR.Value, "+"); got != want {
		bad("hdr", got, want)
	}
	if row.Audio != "" {
		a := row.Audio
		if strings.Contains(a, "atmos") != r.Audio.Atmos {
			bad("atmos", strconv.FormatBool(r.Audio.Atmos), a)
		}
		a = strings.TrimSpace(strings.Replace(a, "atmos", "", 1))
		ch := ""
		if i := strings.LastIndexByte(a, ' '); i >= 0 && strings.Contains(a[i+1:], ".") {
			a, ch = a[:i], a[i+1:]
		} else if strings.Contains(a, ".") {
			a, ch = "", a
		}
		a = strings.Join(strings.Fields(a), " ")
		if a != "" && r.Audio.Format != corpusAudio[a] {
			bad("audio", r.Audio.Format, corpusAudio[a])
		}
		if ch != "" && r.Audio.Channels != ch {
			bad("channels", r.Audio.Channels, ch)
		}
	}
	got := strings.ToLower(strings.Join(r.Edition, " "))
	for _, e := range corpusEditions {
		if strings.Contains(row.Edition, e) != strings.Contains(got, e) {
			bad("edition", got, row.Edition)
			break
		}
	}
	for flag, word := range map[string]string{"hardcoded_subs": "hardcoded subs", "three_d": "3d", "hybrid": "hybrid"} {
		if strings.Contains(row.Edition, word) && !r.Has(flag) {
			bad("flag", strings.Join(r.Flags, ","), flag)
		}
	}
	if row.Group != "" && r.Group.Value != row.Group {
		bad("group", r.Group.Value, row.Group)
	}
	return miss
}

// TestReleaseCorpus reads every video name of the corpus and checks each
// quality field. Music names are read by the music parser and skipped here,
// as are names noted "known:" (a miss owned by another reader).
func TestReleaseCorpus(t *testing.T) {
	data, err := os.ReadFile("testdata/release_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []corpusRow
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	pass, total := 0, 0
	for _, row := range rows {
		if row.Kind == "music" || strings.HasPrefix(row.Note, "known:") {
			continue
		}
		total++
		if m := corpusMiss(row); len(m) > 0 {
			t.Errorf("%s: %s", row.Name, strings.Join(m, "; "))
			continue
		}
		pass++
	}
	t.Logf("corpus: %d/%d video names read right", pass, total)
	if total < 300 {
		t.Errorf("corpus has %d video names, want 300 or more", total)
	}
}
