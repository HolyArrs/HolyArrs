package importer

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestUntaggedAudioFallsBackToNameLanguage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Show.S01.ITA.1080p.WEB-DL.AAC2.0.H.264-ESTA")
	file := filepath.Join(dir, "Show.S01E01.Le.donne.ITA.1080p.WEB-DL.AAC2.0.H.264-ESTA.mkv")
	if got := NameLanguages(file); !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("file name: %v", got)
	}
	if got := NameLanguages(filepath.Join(dir, "Show.S01E01.1080p.mkv")); !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("folder name: %v", got)
	}
	facts := map[string]any{"audio_languages": []string{}, "name_languages": []string{"it"}}
	if got := InspectedLanguages(facts); !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("fallback: %v", got)
	}
	if got := InspectedLanguages(map[string]any{"audio_languages": []any{"en"}}); !reflect.DeepEqual(got, []string{"en"}) {
		t.Fatalf("tagged audio must win: %v", got)
	}
	// One tagged English track plus one untagged track, name says ITA.ENG.
	facts = map[string]any{"audio_languages": []any{"en"}, "audio_unknown": true, "name_languages": []any{"it", "en"}}
	if got := InspectedLanguages(facts); !reflect.DeepEqual(got, []string{"en", "it"}) {
		t.Fatalf("mixed tagged and untagged: %v", got)
	}
	if AudioMeets(map[string]any{"audio_languages": []string{}, "name_languages": []string{"it"}}, []string{"it"}) {
		t.Fatal("name language must not count as audio proof")
	}
	if got := InspectFacts(file)["name_languages"]; !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("InspectFacts on missing file: %v", got)
	}
}
