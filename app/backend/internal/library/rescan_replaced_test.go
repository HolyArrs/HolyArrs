package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A replaced file's name facts are dropped only when they name another
// resolution than its header.
func TestDropStaleRelease(t *testing.T) {
	cases := []struct {
		release, recorded, header string
		wantRelease, wantRecorded string
	}{
		{"Show.S01E02.1080p.WEB-DL-GRP", "WEBDL-1080p", "2160p", "Show - S01E02", ""},
		{"Show.S01E02.1080p.WEB-DL-GRP", "WEBDL-1080p", "1080p", "Show.S01E02.1080p.WEB-DL-GRP", "WEBDL-1080p"},
		{"Show.S01E02.WEB-DL-GRP", "WEBDL", "2160p", "Show.S01E02.WEB-DL-GRP", "WEBDL"},
		{"Show - S01E02", "DVD", "576p", "Show - S01E02", "DVD"},
		{"Show.S01E02.1080p.WEB-DL-GRP", "WEBDL-1080p", "", "Show.S01E02.1080p.WEB-DL-GRP", "WEBDL-1080p"},
	}
	for _, c := range cases {
		release, group := c.release, "GRP"
		f := mediafiles.File{RelativePath: "Show/Season 1/Show - S01E02.mkv", ReleaseName: &release, Group: &group}
		info := map[string]any{mediafiles.RecordedQualityKey: c.recorded}
		if c.header != "" {
			info["resolution"] = c.header
		}
		dropStaleRelease(&f, info)
		rec, _ := info[mediafiles.RecordedQualityKey].(string)
		if *f.ReleaseName != c.wantRelease || rec != c.wantRecorded {
			t.Errorf("%+v: release %q recorded %q", c, *f.ReleaseName, rec)
		}
		if dropped := c.wantRecorded == ""; dropped != (f.Group == nil) {
			t.Errorf("%+v: group %v", c, f.Group)
		}
	}
}
