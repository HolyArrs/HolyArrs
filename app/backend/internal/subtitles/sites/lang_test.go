package sites

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

func TestSiteLang(t *testing.T) {
	for in, want := range map[string]string{"pt-BR": "pt-br", "pt_br": "pt-br", "POB": "pt-br", "BR_PT": "pt-br", "pt-PT": "pt", "pt": "pt", "en-US": "en", "": ""} {
		if got := siteLang(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if !langWanted([]string{"pt"}, "pt-br") || langWanted([]string{"pt-br"}, "pt") || !langWanted([]string{"pt-br"}, "pt-br") || langWanted([]string{"en"}, "") {
		t.Error("langWanted")
	}
}

// A pt-br wish asks OpenSubtitles for pt-br and gets only the Brazilian file;
// machine-made files come back flagged, never as made by a person.
func TestOpenSubtitles_BrazilAndMachineMade(t *testing.T) {
	f := osFake()
	f.Subs = append(f.Subs,
		fakesite.Sub{ID: "103", Kind: "movie", Language: "pt-PT", Title: "The Lantern", Year: 2010, Text: fakesite.Cue("x", 1000)},
		fakesite.Sub{ID: "104", Kind: "movie", Language: "pt-BR", Title: "The Lantern", Year: 2010, AITranslated: true, Text: fakesite.Cue("x", 1000)},
		fakesite.Sub{ID: "105", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, MachineTranslated: true, Text: fakesite.Cue("x", 1000)},
	)
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New("opensubtitles", Config{Name: "OS", URL: srv.URL + "/api/v1", Key: fakeKey, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: []string{"pt-br"}})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	for _, c := range got {
		if c.Language != "pt-br" {
			t.Errorf("not Brazilian: %+v", c)
		}
	}
	if got[0].MachineMade != "" || got[1].MachineMade != AITranslated {
		t.Errorf("flags %q %q", got[0].MachineMade, got[1].MachineMade)
	}
	en, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: []string{"en"}})
	if err != nil || len(en) != 2 || en[1].MachineMade != MachineTranslated {
		t.Fatalf("%+v %v", en, err)
	}
}
