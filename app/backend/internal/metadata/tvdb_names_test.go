package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// TVDB names a show in its original language. Like Sonarr, a show
// with an English name is named by it (with its English synopsis); the
// original name, every other language's name and the aliases become other
// names, so word-only sites and local trackers find its releases.
func TestTVDBShowOtherNames(t *testing.T) {
	cases := []struct {
		name, lang, translations, texts, aliases, english string
		wantName, wantOverview                            string
		want                                              []string
		translationAsked                                  bool
	}{
		// The texts come with the details: no extra request.
		{"夜桜さんちの大作戦", "jpn", `["jpn","eng","fra"]`,
			`{"nameTranslations":[{"language":"jpn","name":"夜桜さんちの大作戦"},{"language":"eng","name":"Mission: Yozakura Family","aliases":["Mission Yozakura Family"]},{"language":"fra","name":"Mission : Famille Yozakura"}],"overviewTranslations":[{"language":"eng","overview":"A spy family."}]}`,
			`[{"language":"jpn","name":"Yozakura-san Chi no Daisakusen"}]`, ``,
			"Mission: Yozakura Family", "A spy family.",
			[]string{"夜桜さんちの大作戦", "Yozakura-san Chi no Daisakusen", "Mission Yozakura Family", "Mission : Famille Yozakura"}, false},
		// The romaji names, aliases in the original language, come right
		// after the original name, ahead of English spellings and other
		// languages, so a word search of a few names asks for them.
		{"氷剣の魔術師が世界を統べる", "jpn", `["jpn","eng","fra","deu"]`,
			`{"nameTranslations":[{"language":"jpn","name":"氷剣の魔術師が世界を統べる","aliases":["Hyouken no Majutsushi ga Sekai wo Suberu"]},{"language":"eng","name":"The Iceblade Sorcerer Shall Rule the World","aliases":["The Iceblade Magician Rules Over the World"]},{"language":"fra","name":"Le Sorcier de glace"},{"language":"deu","name":"Der Eisklingenmagier"}]}`,
			`[{"language":"eng","name":"Iceblade Sorcerer"},{"language":"jpn","name":"Hyouken no Majutsushi ga Sekai o Suberu"}]`, ``,
			"The Iceblade Sorcerer Shall Rule the World", "orig",
			[]string{"氷剣の魔術師が世界を統べる", "Hyouken no Majutsushi ga Sekai wo Suberu", "Hyouken no Majutsushi ga Sekai o Suberu", "The Iceblade Magician Rules Over the World", "Le Sorcier de glace", "Der Eisklingenmagier", "Iceblade Sorcerer"}, false},
		{"오징어 게임", "kor", `["kor","eng","deu","spa"]`,
			`{"nameTranslations":[{"language":"eng","name":"Squid Game"},{"language":"deu","name":"Squid Game"},{"language":"spa","name":"El juego del calamar"}]}`,
			`[]`, ``, "Squid Game", "orig", []string{"오징어 게임", "El juego del calamar"}, false},
		{"Слово пацана", "rus", `["rus"]`, ``, `[{"language":"eng","name":"The Boy's Word"}]`, ``,
			"Слово пацана", "orig", []string{"The Boy's Word"}, false},
		{"الحشاشين", "ara", `["ara","eng"]`, `{"nameTranslations":[{"language":"eng","name":"The Assassins"}]}`, `[]`, ``,
			"The Assassins", "orig", []string{"الحشاشين"}, false},
		{"पंचायत", "hin", `["hin","eng"]`, `{"nameTranslations":[{"language":"eng","name":"Panchayat"}]}`, `[]`, ``,
			"Panchayat", "orig", []string{"पंचायत"}, false},
		{"เด็กใหม่", "tha", `["tha","eng"]`, `{"nameTranslations":[{"language":"eng","name":"Girl from Nowhere"}]}`, `[]`, ``,
			"Girl from Nowhere", "orig", []string{"เด็กใหม่"}, false},
		{"三体", "zho", `["zho","eng"]`, `{"nameTranslations":[{"language":"eng","name":"Three-Body"}]}`, `[]`, ``,
			"Three-Body", "orig", []string{"三体"}, false},
		{"Le Bureau des Légendes", "fra", `["fra","eng","deu"]`,
			`{"nameTranslations":[{"language":"eng","name":"The Bureau"},{"language":"deu","name":"Büro der Legenden"}]}`, `[]`, ``,
			"The Bureau", "orig", []string{"Le Bureau des Légendes", "Büro der Legenden"}, false},
		{"Dark", "deu", `["deu","eng"]`, `{"nameTranslations":[{"language":"eng","name":"Dark"}]}`, `[]`, ``,
			"Dark", "orig", nil, false},
		{"La casa de papel", "spa", `["spa","eng"]`, `{"nameTranslations":[{"language":"eng","name":"Money Heist"}]}`, `[]`, ``,
			"Money Heist", "orig", []string{"La casa de papel"}, false},
		// An English show keeps its name; other languages are other names.
		{"Lab Show", "eng", `["eng","fra"]`, `{"nameTranslations":[{"language":"eng","name":"Lab Show"},{"language":"fra","name":"Le Show"}]}`, `[]`, ``,
			"Lab Show", "orig", []string{"Le Show"}, false},
		// No texts with the details: the English name is one more request.
		{"夜桜さんちの大作戦", "jpn", `["jpn","eng"]`, ``, `[{"language":"jpn","name":"Yozakura-san Chi no Daisakusen"}]`,
			`{"data":{"name":"Mission: Yozakura Family","overview":"A spy family.","aliases":["Mission Yozakura Family"],"language":"eng"}}`,
			"Mission: Yozakura Family", "A spy family.", []string{"夜桜さんちの大作戦", "Yozakura-san Chi no Daisakusen", "Mission Yozakura Family"}, true},
		// The English name fails: the show keeps its name and the aliases.
		{"夜桜さんちの大作戦", "jpn", `["eng"]`, ``, `[{"language":"jpn","name":"Yozakura-san Chi no Daisakusen"}]`, `broken`,
			"夜桜さんちの大作戦", "orig", []string{"Yozakura-san Chi no Daisakusen"}, true},
	}
	for _, tc := range cases {
		asked := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v4/login":
				_, _ = w.Write([]byte(`{"data":{"token":"s"}}`))
			case r.URL.Path == "/v4/series/424019/extended":
				texts := ""
				if tc.texts != "" && r.URL.Query().Get("meta") == "translations" {
					texts = `,"translations":` + tc.texts
				}
				_, _ = w.Write([]byte(`{"data":{"id":424019,"name":"` + tc.name + `","overview":"orig","year":"2024","originalLanguage":"` + tc.lang +
					`","nameTranslations":` + tc.translations + `,"aliases":` + tc.aliases + texts + `}}`))
			case r.URL.Path == "/v4/series/424019/translations/eng":
				asked = true
				if tc.english == "broken" {
					w.WriteHeader(500)
					return
				}
				_, _ = w.Write([]byte(tc.english))
			case strings.HasPrefix(r.URL.Path, "/v4/series/424019/episodes/"):
				_, _ = w.Write([]byte(`{"data":{"episodes":[]},"links":{"next":null}}`))
			default:
				w.WriteHeader(404)
			}
		}))
		p := &TVDB{BaseURL: srv.URL + "/v4", Key: func(context.Context) (string, error) { return fakeTVDBKey, nil }, Gap: time.Microsecond}
		s, err := p.Show(context.Background(), "tvdb:424019")
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if s.Name != tc.wantName || s.Overview != tc.wantOverview || !slices.Equal(s.OtherNames, tc.want) || asked != tc.translationAsked {
			t.Fatalf("%s: name %q overview %q other names %q (want %q %q %q), translation asked %v", tc.name, s.Name, s.Overview, s.OtherNames, tc.wantName, tc.wantOverview, tc.want, asked)
		}
	}
}
