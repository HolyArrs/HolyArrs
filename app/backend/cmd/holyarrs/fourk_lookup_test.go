package main

import (
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
)

// the movie lookup tells a member about the open 4K ask and
// whether the Library copy has 4K, so the title page can offer a 4K ask
// after a plain ask and on a movie already in the Library.
// route: GET /api/v1/titles/lookup
func TestLookupShowsFourKAsk(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	look := func(s *signedIn) discover.LookupView {
		t.Helper()
		var x struct {
			Movies []discover.LookupView `json:"movies"`
		}
		_ = json.Unmarshal(s.call("GET", "/api/v1/titles/lookup?term=tmdb:693134", "", 200), &x)
		if len(x.Movies) != 1 {
			t.Fatalf("lookup %+v", x)
		}
		return x.Movies[0]
	}
	plain := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	if v := look(r.ana); v.Ask == nil || !v.Ask.Mine || v.Ask4K != nil || v.Has4K {
		t.Fatalf("after plain %+v", v)
	}
	// The same member asks for 4K too: its own ask.
	uhd := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134","four_k":true}`, 201))
	if uhd.ID == plain.ID || !uhd.FourK {
		t.Fatalf("4K ask %+v", uhd)
	}
	if v := look(r.ana); v.Ask4K == nil || !v.Ask4K.Mine || v.Ask4K.ID != uhd.ID || v.Ask.ID != plain.ID {
		t.Fatalf("after 4K %+v", v)
	}
	if v := look(r.bo); v.Ask4K == nil || v.Ask4K.Mine {
		t.Fatalf("bo sees %+v", v)
	}
	// The plain copy reaches the Library; the 4K copy is still to come.
	r.owner.call("POST", "/api/v1/asks/"+itoa(plain.ID)+"/approve", `{}`, 200)
	if v := look(r.bo); v.TitleID == 0 || v.Has4K || v.Ask4K == nil {
		t.Fatalf("after plain approve %+v", v)
	}
	r.owner.call("POST", "/api/v1/asks/"+itoa(uhd.ID)+"/approve", `{}`, 200)
	if v := look(r.bo); !v.Has4K {
		t.Fatalf("after 4K approve %+v", v)
	}
}
