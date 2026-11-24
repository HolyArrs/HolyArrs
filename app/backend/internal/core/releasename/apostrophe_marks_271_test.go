package releasename

import "testing"

// An apostrophe typed as an acute accent ("Schitt´s", common on
// German and Spanish keyboards) or a full-width mark must drop like ' does,
// so the release "Schitts.Creek" still matches the library name.
func TestApostropheMarks271(t *testing.T) {
	for _, lib := range []string{"Schitt's Creek", "Schitt’s Creek", "Schitt´s Creek", "Schitt＇s Creek", "Schitt`s Creek", "Schittʼs Creek"} {
		for _, rel := range []string{"Schitts.Creek.S01E01.1080p.WEB.h264-GRP", "Schitt.s.Creek.S01E01.1080p.WEB-GRP", "Schitt's.Creek.S01E01.1080p.WEB-GRP"} {
			if want := CleanName(Parse(rel, Hints{}).Title.Value); !SameName(lib, want) {
				t.Errorf("lib %q does not match release %q (clean %q, forms %q)", lib, rel, want, NameForms(lib))
			}
		}
	}
}
