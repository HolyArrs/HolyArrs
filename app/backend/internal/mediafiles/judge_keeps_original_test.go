package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// keepsOriginalTraits sets an imported "Not Original" (as Profilarr and
// Dictionarry write it) and "Not English".
func keepsOriginalTraits(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetOwnTraits(nil) })
	SetOwnTraits([]OwnTrait{
		{Meta: database.Meta{ID: 1}, Name: "Not Original", Match: "name", Source: "import", Specs: []OwnSpec{
			{Name: "Not Original", Kind: "language", Value: "!original", Required: true},
			{Name: "Original", Kind: "language", Value: "original", Negate: true, Required: true},
		}},
		{Meta: database.Meta{ID: 2}, Name: "Not English", Match: "name", Source: "import", Specs: []OwnSpec{
			{Name: "English", Kind: "language", Value: "en", Negate: true, Required: true},
		}},
	})
}

// TestKeepsOriginalTraits: a German DL name keeps the original language,
// so imported language tests read it among the Release's languages. A
// plain German name, or audio tracks that say German only, do not.
func TestKeepsOriginalTraits(t *testing.T) {
	keepsOriginalTraits(t)
	for _, c := range []struct {
		name  string
		audio []string
		fits  bool
	}{
		{"Some.Show.S01E01.German.DL.1080p.WEB.h264-GRP", nil, false},
		{"Some.Show.S01E01.German.DL.1080p.BluRay.x264-GRP", nil, false},
		{"Some.Show.S01E01.German.1080p.WEB.h264-GRP", nil, true},
		// The file's audio tracks win over its name.
		{"Some.Show.S01E01.German.DL.1080p.WEB.h264-GRP", []string{"de"}, true},
	} {
		p := releasename.Parse(c.name, releasename.Hints{Show: true})
		p.OriginalLanguage, p.AudioLanguages = "en", c.audio
		for id, trait := range map[int64]string{1: "Not Original", 2: "Not English"} {
			if got, why := ownMatches(id, p); got != c.fits {
				t.Errorf("%s (audio %v): %q fits %v (%s), want %v", c.name, c.audio, trait, got, why, c.fits)
			}
		}
	}
}

// TestKeepsOriginalImportedProfile: rules brought from Radarr that want
// English take a German DL copy of an English film, as Radarr does, and
// still turn away a German copy that does not keep the original.
func TestKeepsOriginalImportedProfile(t *testing.T) {
	b := GoodEveryday()
	b.Language = LanguageRule{Wanted: []string{"en"}}
	b.Synced = &SyncedProfile{From: "import", App: "radarr"}
	en := &TitleRef{OriginalLanguage: "en"}
	var v Verdict
	v.language(releasename.Parse("Some.Movie.2020.German.DL.1080p.WEB.h264-GRP", releasename.Hints{}), b, en)
	if v.failed() {
		t.Fatalf("German DL of an English film refused: %+v", v.Reasons)
	}
	v = Verdict{}
	v.language(releasename.Parse("Some.Movie.2020.German.1080p.WEB.h264-GRP", releasename.Hints{}), b, en)
	if !v.failed() {
		t.Fatal("a German copy of an English film passed rules wanting English")
	}
	// A German DL of a French film keeps French, not English.
	v = Verdict{}
	v.language(releasename.Parse("Some.Movie.2020.German.DL.1080p.WEB.h264-GRP", releasename.Hints{}), b, &TitleRef{OriginalLanguage: "fr"})
	if !v.failed() {
		t.Fatal("German DL of a French film passed rules wanting English")
	}
}

// TestKeepsOriginalGermanOnly: rules that want German only, the original
// language not fine, still take German DL and still turn away an English
// show's Release with no language word, made here or imported.
func TestKeepsOriginalGermanOnly(t *testing.T) {
	en := &TitleRef{OriginalLanguage: "en"}
	for _, synced := range []*SyncedProfile{nil, {From: "import", App: "sonarr"}} {
		b := GoodEveryday()
		b.Language = LanguageRule{Wanted: []string{"de"}}
		b.Synced = synced
		var v Verdict
		v.language(releasename.Parse("Murderbot.S01E08.German.DL.1080p.WEB.h264-GRP", releasename.Hints{Show: true}), b, en)
		if v.failed() {
			t.Errorf("synced %v: German DL refused: %+v", synced, v.Reasons)
		}
		v = Verdict{}
		v.language(releasename.Parse("Murderbot.S01E08.1080p.WEB.h264-GRP", releasename.Hints{Show: true}), b, en)
		if !v.failed() {
			t.Errorf("synced %v: an English Release with no language word passed German-only rules", synced)
		}
	}
}
