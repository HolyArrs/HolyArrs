package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestWish_CleanWants(t *testing.T) {
	got, err := CleanWants([]Want{{Language: "eng"}, {Language: "es-MX", HearingImpaired: HIOnly}, {Language: "English", Forced: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Language != "en" || got[0].HearingImpaired != HINormal || got[1].Language != "es" || got[2].Language != "en" || !got[2].Forced {
		t.Fatalf("%+v", got)
	}
	for _, bad := range [][]Want{
		{{Language: "klingon"}},
		{{Language: "en"}, {Language: "eng"}},
		{{Language: "en", HearingImpaired: "loud"}},
		{{Language: "und"}},
	} {
		if _, err := CleanWants(bad); err == nil {
			t.Errorf("%+v: no error", bad)
		}
	}
	many := make([]Want, MaxWishLanguages+1)
	if _, err := CleanWants(many); !errors.Is(err, ErrTooManyWants) {
		t.Errorf("too many: %v", err)
	}
	if got, _ := CleanWants(nil); got == nil || len(got) != 0 {
		t.Errorf("nil gives %v", got)
	}
}

func TestWish_WishInheritance(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	db := f.st.Read()
	by := domain.Actor{Kind: "user", ID: 1}
	en := []Want{{Language: "en", HearingImpaired: HINormal}}
	es := []Want{{Language: "es", HearingImpaired: HIEither}}

	// With nothing set and an empty default, nothing is wanted, so nothing is
	// searched until a title has a language.
	r, err := WishFor(ctx, db, f.show, f.showVer, f.ep1)
	if err != nil || r.From != FromDefault || len(r.Wants) != 0 {
		t.Fatalf("empty default: %+v %v", r, err)
	}
	// The default reaches every title that has no wish of its own.
	s := ReadSettings(ctx, db)
	s.Languages = en
	if err := SaveSettings(ctx, f.st, s, by); err != nil {
		t.Fatal(err)
	}
	if r, _ := WishFor(ctx, db, f.movie, f.movieVer, f.movieItem); r.From != FromDefault || r.Wants[0].Language != "en" {
		t.Fatalf("default: %+v", r)
	}
	// Title beats default, Version beats title and episode beats Version.
	if err := SetWish(ctx, f.st, f.show, 0, 0, es, false, by); err != nil {
		t.Fatal(err)
	}
	if r, _ := WishFor(ctx, db, f.show, f.showVer, f.ep1); r.From != FromTitle || r.Wants[0].Language != "es" {
		t.Fatalf("title: %+v", r)
	}
	if err := SetWish(ctx, f.st, f.show, f.show4K, 0, []Want{{Language: "fr"}}, false, by); err != nil {
		t.Fatal(err)
	}
	if r, _ := WishFor(ctx, db, f.show, f.show4K, f.ep1); r.From != FromVersion || r.Wants[0].Language != "fr" {
		t.Fatalf("version: %+v", r)
	}
	if r, _ := WishFor(ctx, db, f.show, f.showVer, f.ep1); r.From != FromTitle {
		t.Fatalf("other version keeps the title's: %+v", r)
	}
	if err := SetWish(ctx, f.st, f.show, 0, f.ep2, []Want{}, false, by); err != nil {
		t.Fatal(err)
	}
	if r, _ := WishFor(ctx, db, f.show, f.show4K, f.ep2); r.From != FromItem || len(r.Wants) != 0 {
		t.Fatalf("episode with none: %+v", r)
	}
	// Setting again replaces the wish, and inherit removes it.
	if err := SetWish(ctx, f.st, f.show, 0, 0, en, false, by); err != nil {
		t.Fatal(err)
	}
	ws, _ := Wishes(ctx, db, f.show)
	if len(ws) != 3 || ws[0].Languages[0].Language != "en" {
		t.Fatalf("wishes: %+v", ws)
	}
	if err := SetWish(ctx, f.st, f.show, 0, f.ep2, nil, true, by); err != nil {
		t.Fatal(err)
	}
	if r, _ := WishFor(ctx, db, f.show, f.showVer, f.ep2); r.From != FromTitle {
		t.Fatalf("after inherit: %+v", r)
	}
	// Music has no subtitles, and Versions or episodes of another title are refused.
	if err := SetWish(ctx, f.st, f.artist, 0, 0, en, false, by); !errors.Is(err, ErrMusic) {
		t.Errorf("artist: %v", err)
	}
	if _, err := WishFor(ctx, db, f.artist, 0, 0); !errors.Is(err, ErrMusic) {
		t.Errorf("artist wish: %v", err)
	}
	if err := SetWish(ctx, f.st, f.movie, f.showVer, 0, en, false, by); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign version: %v", err)
	}
	if err := SetWish(ctx, f.st, f.movie, 0, f.ep1, en, false, by); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign episode: %v", err)
	}
	if err := SetWish(ctx, f.st, f.show, f.showVer, f.ep1, en, false, by); err == nil {
		t.Error("version and episode at once")
	}
	if err := SetWish(ctx, f.st, 9999, 0, 0, en, false, by); !errors.Is(err, ErrNotFound) {
		t.Errorf("no title: %v", err)
	}
}

func TestWish_SettingsRoundTrip(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	d := ReadSettings(ctx, f.st.Read())
	if d.PassMarkMovie != 70 || d.PassMarkEpisode != 90 || !d.UseBuiltIn || d.SearchHours != 6 || d.BetterHours != 12 || d.BetterDays != 7 || d.HISuffix != "hi" {
		t.Fatalf("defaults %+v", d)
	}
	d.PassMarkMovie, d.SearchHours, d.Better, d.HISuffix = 80, 24, false, "sdh"
	if err := SaveSettings(ctx, f.st, d, domain.System); err != nil {
		t.Fatal(err)
	}
	got := ReadSettings(ctx, f.st.Read())
	if got.PassMarkMovie != 80 || got.SearchHours != 24 || got.Better || got.HISuffix != "sdh" {
		t.Fatalf("saved %+v", got)
	}
	var enabled bool
	_ = f.st.Read().QueryRow("SELECT enabled FROM schedules WHERE kind = ?", KindBetter).Scan(&enabled)
	if enabled {
		t.Error("better schedule still on")
	}
	for _, bad := range []func(*Settings){
		func(s *Settings) { s.PassMarkMovie = 101 },
		func(s *Settings) { s.HISuffix = "deaf" },
		// The suffix ends up in file names, so only the three words pass and path
		// pieces or long text never do.
		func(s *Settings) { s.HISuffix = "../../x" },
		func(s *Settings) { s.HISuffix = "hi/../../x" },
		func(s *Settings) { s.HISuffix = strings.Repeat("h", 300) },
		func(s *Settings) { s.Languages = []Want{{Language: "en/../../x"}} },
		func(s *Settings) { s.Languages = []Want{{Language: strings.Repeat("e", 300)}} },
		func(s *Settings) { s.SearchHours = 0 },
		func(s *Settings) { s.BetterDays = 31 },
		func(s *Settings) { s.Batch = 0 },
		func(s *Settings) { s.Languages = []Want{{Language: "xx"}} },
	} {
		s := DefaultSettings()
		bad(&s)
		if err := SaveSettings(ctx, f.st, s, domain.System); err == nil {
			t.Errorf("%+v saved", s)
		}
	}
	// A bad suffix written straight into the database is never used.
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE settings SET value = '../x' WHERE grp = 'what_to_pick' AND key = 'subtitle_hi_suffix'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := ReadSettings(ctx, f.st.Read()); got.HISuffix != "hi" {
		t.Fatalf("stored bad suffix used: %q", got.HISuffix)
	}
}

func TestWish_NormLanguage(t *testing.T) {
	for in, want := range map[string]string{"eng": "en", "EN": "en", "en-US": "en", "pt_BR": "pt", "ger": "de", "deu": "de", "French": "fr",
		"und": "", "": "", "zxx": "", "chi": "zh", "zho": "zh", "pob": "pt"} {
		if got := NormLanguage(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
