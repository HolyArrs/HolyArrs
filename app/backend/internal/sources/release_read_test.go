package sources

import (
	"encoding/json"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// The view shows what the judge read, not a fresh guess.
func TestReleaseViewRead(t *testing.T) {
	for _, name := range []string{
		"Some.Movie.2019.UHDBD.2160p.REMUX.HEVC.DTS-HD.MA.7.1-GRP",
		"Some.Movie.2019.1080p.BluRay.x264-GRP",
		"Show.S01E02.720p.WEB-DL.DDP5.1.H.264-NTb",
		"Artist-Album-2020-FLAC-24BIT-GRP",
	} {
		p := releasename.Parse(name, releasename.Hints{})
		b, _ := json.Marshal(p)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		v := releaseView(domain.Release{Name: name, Parsed: m})
		if p.Resolution.Value != 0 && (v.Read == nil || v.Read.Resolution != p.Resolution.Value) {
			t.Errorf("%s: resolution %v, judge read %d", name, v.Read, p.Resolution.Value)
		}
		if v.Read != nil && v.Read.Source != p.Source.Value {
			t.Errorf("%s: source %q, judge read %q", name, v.Read.Source, p.Source.Value)
		}
	}
	if v := releaseView(domain.Release{Name: "x"}); v.Read != nil {
		t.Errorf("no stored reading should send no read, got %+v", v.Read)
	}
}
