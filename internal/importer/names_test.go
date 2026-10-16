package importer

import (
	"path/filepath"
	"testing"
)

func TestPlanRefusesProgramsAndSamplesByWord(t *testing.T) {
	lib := t.TempDir()
	for name, want := range map[string]string{
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.exe.mkv":    "refuse",
		"DUNE.PART.TWO.2024.1080P.WEB.H264-HOLYARRS.EXE":        "refuse",
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.dll":        "refuse",
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.jar":        "refuse",
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.apk":        "refuse",
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.sample.mkv": "refuse",
		"sample-dune.part.two.2024.1080p.mkv":                   "refuse",
		"Dune.Part.Two.2024.1080p.WEB.H264-SAMPLER.mkv":         "hardlink",
	} {
		steps := Plan([]Candidate{{Path: filepath.Join(t.TempDir(), name), Size: 2 << 30}}, target(lib))
		if steps[0].Method != want {
			t.Errorf("%s: %s %v, want %s", name, steps[0].Method, steps[0].Why, want)
		}
	}
}
