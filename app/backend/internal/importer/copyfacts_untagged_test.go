package importer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// el builds one small EBML element (payload under 127 bytes).
func el(id []byte, payload ...byte) []byte {
	return append(append(append([]byte{}, id...), 0x80|byte(len(payload))), payload...)
}

func mkvFile(t *testing.T, name string, audioLangs ...string) string {
	t.Helper()
	tracks := el([]byte{0xae}, append(el([]byte{0x83}, 1), el([]byte{0x86}, []byte("V_AV1")...)...)...)
	for _, lang := range audioLangs {
		p := append(el([]byte{0x83}, 2), el([]byte{0x86}, []byte("A_AAC")...)...)
		if lang != "" {
			p = append(p, el([]byte{0x22, 0xb5, 0x9c}, []byte(lang)...)...)
		}
		tracks = append(tracks, el([]byte{0xae}, p...)...)
	}
	data := append(el([]byte{0x1a, 0x45, 0xdf, 0xa3}, el([]byte{0x42, 0x82}, []byte("matroska")...)...),
		el([]byte{0x18, 0x53, 0x80, 0x67}, el([]byte{0x16, 0x54, 0xae, 0x6b}, tracks...)...)...)
	file := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestInspectFactsUntaggedMatroskaAudio(t *testing.T) {
	cases := []struct {
		name, file string
		langs      []string
		audio      []string
		unknown    bool
		status     string
		named      []string
	}{
		{"untagged reads as eng", "Movie.2020.1080p.WEB-DL.mkv", []string{""}, []string{"en"}, false, "complete", nil},
		{"name language wins over the default", "Film.2020.ITALIAN.1080p.WEB-DL.mkv", []string{""}, []string{}, true, "partial", []string{"it"}},
		{"name says English too", "Movie.2020.ENGLISH.1080p.mkv", []string{""}, []string{"en"}, false, "complete", nil},
		{"explicit und stays unknown", "Movie.2020.1080p.mkv", []string{"und"}, []string{}, true, "partial", []string{}},
		{"tagged plus untagged", "Movie.2020.1080p.mkv", []string{"fre", ""}, []string{"en", "fr"}, false, "complete", nil},
		{"tagged plus untagged, German name", "Film.2020.GERMAN.DL.1080p.mkv", []string{"ger", ""}, []string{"de"}, true, "partial", nil},
		{"tagged eng untouched", "Movie.2020.mkv", []string{"eng"}, []string{"en"}, false, "complete", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			facts := InspectFacts(mkvFile(t, c.file, c.langs...))
			if got := stringList(facts["audio_languages"]); !slices.Equal(got, c.audio) {
				t.Fatalf("audio_languages = %v, want %v (%v)", got, c.audio, facts)
			}
			if facts["audio_unknown"] != c.unknown || facts["inspection_status"] != c.status {
				t.Fatalf("unknown=%v status=%v, want %v %v", facts["audio_unknown"], facts["inspection_status"], c.unknown, c.status)
			}
			if c.named != nil {
				if got := stringList(facts["name_languages"]); !slices.Equal(got, c.named) {
					t.Fatalf("name_languages = %v, want %v", got, c.named)
				}
			}
		})
	}
}
