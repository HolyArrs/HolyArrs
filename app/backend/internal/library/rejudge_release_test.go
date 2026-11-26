package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// An adopted file takes the old app's scene name while its release name is
// its own file name, and the old app's group while it has none. A release
// name HolyArrs took from its own download, a group already known and a
// file HolyArrs imported itself are kept.
func TestTakeOldRelease(t *testing.T) {
	str := func(s string) *string { return &s }
	scene := OldRelease{Name: "Show.S01E01.Pilot.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", Group: "NTb"}
	cases := []struct {
		why                  string
		f                    mediafiles.File
		r                    OldRelease
		changed              bool
		wantRelease, wantGrp string
	}{
		{"adopted with its own file name", mediafiles.File{Origin: "adopted", RelativePath: "Season 01/Show - S01E01 - Pilot WEBDL-1080p.mkv", ReleaseName: str("Show - S01E01 - Pilot WEBDL-1080p")},
			scene, true, scene.Name, "NTb"},
		{"no release name", mediafiles.File{Origin: "adopted", RelativePath: "a.mkv"}, scene, true, scene.Name, "NTb"},
		{"no scene name, only a group", mediafiles.File{Origin: "adopted", RelativePath: "x (1080p WEB-DL - HONE).mkv", ReleaseName: str("x (1080p WEB-DL - HONE)")},
			OldRelease{Group: "HONE"}, true, "x (1080p WEB-DL - HONE)", "HONE"},
		{"no scene name, group from it", mediafiles.File{Origin: "adopted", RelativePath: "a.mkv", ReleaseName: str("a")},
			OldRelease{Name: "A.2020.1080p.WEB.h264-GRP"}, true, "A.2020.1080p.WEB.h264-GRP", "GRP"},
		{"a release name HolyArrs set", mediafiles.File{Origin: "adopted", RelativePath: "a.mkv", ReleaseName: str("A.2020.REPACK.1080p.WEB.h264-OWN"), Group: str("OWN")},
			scene, false, "A.2020.REPACK.1080p.WEB.h264-OWN", "OWN"},
		{"a group already known", mediafiles.File{Origin: "adopted", RelativePath: "a-KNOWN.mkv", ReleaseName: str("a-KNOWN"), Group: str("KNOWN")},
			scene, true, scene.Name, "KNOWN"},
		{"imported by HolyArrs", mediafiles.File{Origin: "imported", RelativePath: "a.mkv", ReleaseName: str("a")}, scene, false, "a", ""},
		{"nothing recorded", mediafiles.File{Origin: "adopted", RelativePath: "a.mkv", ReleaseName: str("a")}, OldRelease{}, false, "a", ""},
	}
	for _, c := range cases {
		f := c.f
		changed := takeOldRelease(&f, c.r)
		release, group := "", ""
		if f.ReleaseName != nil {
			release = *f.ReleaseName
		}
		if f.Group != nil {
			group = *f.Group
		}
		if changed != c.changed || release != c.wantRelease || group != c.wantGrp {
			t.Errorf("%s: changed %v release %q group %q; want %v %q %q", c.why, changed, release, group, c.changed, c.wantRelease, c.wantGrp)
		}
	}
}
