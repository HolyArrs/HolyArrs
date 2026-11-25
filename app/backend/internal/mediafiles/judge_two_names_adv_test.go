package mediafiles

import (
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Adversarial cases for two-name releases: real two-name forms must
// match, and sequels, remakes, short or common-word titles and same name
// with another year must not be pulled in by the two-name rule.
func TestMatchTitleTwoNamesAdversarial(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Name: "Charmed", Year: 1998, Kind: "show", Aliases: []string{"Streghe"}},
		{ID: 2, Name: "Charmed", Year: 2018, Kind: "show", Aliases: []string{"Streghe"}},
		{ID: 3, Name: "The Mummy Returns", Year: 2001, Aliases: []string{"La Mummia - Il Ritorno"}},
		{ID: 4, Name: "The Mummy", Year: 1999, Aliases: []string{"La Mummia"}},
		{ID: 5, Name: "Saw", Year: 2004, Aliases: []string{"Saw - L'enigmista"}},
		{ID: 6, Name: "Saw II", Year: 2005, Aliases: []string{"Saw II - La soluzione dell'enigma"}},
		{ID: 7, Name: "Her", Year: 2013, Aliases: []string{"Lei"}},
		{ID: 8, Name: "Us", Year: 2019, Aliases: []string{"Noi"}},
		{ID: 9, Name: "It", Year: 2017, Aliases: []string{"Es"}},
		{ID: 10, Name: "It", Year: 1990, Kind: "show", Aliases: []string{"Es"}},
		{ID: 11, Name: "Alien", Year: 1979, Aliases: []string{"Alien - L'ottavo passeggero"}},
		{ID: 12, Name: "Aliens", Year: 1986, Aliases: []string{"Aliens - Scontro finale"}},
		{ID: 13, Name: "Amélie", Year: 2001, Aliases: []string{"Le Fabuleux Destin d'Amélie Poulain", "Il favoloso mondo di Amélie"}},
		{ID: 14, Name: "Dark", Year: 2017, Kind: "show"},
		{ID: 15, Name: "Halloween", Year: 1978, Aliases: []string{"La notte delle streghe"}},
		{ID: 16, Name: "Halloween", Year: 2018, Aliases: []string{"Halloween 2018"}},
		{ID: 17, Name: "Spirited Away", Year: 2001, Aliases: []string{"La città incantata", "Sen to Chihiro no Kamikakushi", "千と千尋の神隠し"}},
		{ID: 18, Name: "Doctor Who", Year: 2005, Kind: "show", Aliases: []string{"Doctor Who (2005)"}},
		{ID: 19, Name: "Doctor Who", Year: 1963, Kind: "show"},
		{ID: 20, Name: "Money Heist", Year: 2017, Kind: "show", Aliases: []string{"La casa de papel", "La casa di carta"}},
		{ID: 21, Name: "Up", Year: 2009, Aliases: []string{"Su"}},
		{ID: 22, Name: "Lost", Year: 2004, Kind: "show", Aliases: []string{"Lost"}},
	}
	cases := []struct {
		name string
		want int64
	}{
		// must match
		{"Streghe.Charmed.1998.S01E01.1080p.WEB-DL.ITA.ENG-GRP", 1},
		{"Streghe.Charmed.2018.S01E01.1080p.WEB-DL.ITA.ENG-GRP", 2},
		{"The.Mummy.Returns.La.Mummia.Il.Ritorno.2001.1080p-GRP", 3},
		{"La.Mummia.The.Mummy.1999.1080p.BluRay.ITA.ENG-GRP", 4},
		{"La Mummia - The Mummy (1999) 1080p", 4},
		{"Saw.L.Enigmista.Saw.2004.1080p-GRP", 5},
		{"Saw.II.La.Soluzione.Dell.Enigma.Saw.II.2005.1080p-GRP", 6},
		{"Lei.Her.2013.1080p.BluRay.ITA.ENG-GRP", 7},
		{"Noi.Us.2019.1080p.ITA.ENG-GRP", 8},
		{"Es.It.2017.1080p.ITA.ENG-GRP", 9},
		{"Alien.L.Ottavo.Passeggero.Alien.1979.1080p-GRP", 11},
		{"Aliens.Scontro.Finale.Aliens.1986.1080p-GRP", 12},
		{"Il.Favoloso.Mondo.Di.Amelie.Amelie.2001.1080p-GRP", 13},
		{"Il.Favoloso.Mondo.di.Amelie.Le.Fabuleux.Destin.d.Amelie.Poulain.2001.1080p-GRP", 13},
		{"La.Notte.Delle.Streghe.Halloween.1978.1080p-GRP", 15},
		{"La.Citta.Incantata.Spirited.Away.2001.1080p-GRP", 17},
		{"La.Citta.Incantata.Sen.to.Chihiro.no.Kamikakushi.2001.1080p-GRP", 17},
		{"La.Casa.Di.Carta.Money.Heist.S01E01.1080p-GRP", 20},
		{"La.Casa.Di.Carta.La.Casa.De.Papel.S01E01.1080p-GRP", 20},
		{"Money.Heist.La.Casa.Di.Carta.S01E01.1080p-GRP", 20},
		{"Su.Up.2009.1080p.ITA.ENG-GRP", 21},
		// must not match (sequels, other years, common words, partial)
		{"La.Mummia.The.Mummy.Returns.2001.1080p-GRP", 0},
		{"Aliens.Alien.1986.1080p-GRP", 0},
		{"Alien.Aliens.1979.1080p-GRP", 0},
		{"Saw.Saw.II.2005.1080p-GRP", 0},
		{"Halloween.Halloween.2018.2018.1080p-GRP", 16},
		{"Lei.Her.Story.2013.1080p-GRP", 0},
		{"Her.Lei.Story.2013.1080p-GRP", 0},
		{"Noi.Us.And.Them.2019.1080p-GRP", 0},
		{"Es.It.Follows.2014.1080p-GRP", 0},
		{"It.Es.Ist.2017.1080p-GRP", 0},
		{"Streghe.Charmed.Ones.S01E01.1080p-GRP", 0},
		{"Charmed.Charmed.S01E01.1080p-GRP", 0},
		{"Lost.Lost.S01E01.1080p-GRP", 0},
		{"Lost.Girl.S01E01.1080p-GRP", 0},
		{"Dark.Matter.S01E01.1080p-GRP", 0},
		{"Su.Up.2020.1080p-GRP", 0},
		{"Noi.Us.1990.1080p-GRP", 0},
		{"La.Mummia.Il.Ritorno.2017.1080p-GRP", 0},
		{"Up.Su.Giu.2009.1080p-GRP", 0},
		{"Spirited.Away.Away.2001.1080p-GRP", 0},
		{"The.Mummy.The.Mummy.Returns.2001.1080p-GRP", 0},
		{"Money.Heist.Korea.S01E01.1080p-GRP", 0},
		{"Doctor.Who.Doctor.Who.S01E01.1080p-GRP", 0},
	}
	for _, c := range cases {
		m := MatchTitle(releasename.Parse(c.name, releasename.Hints{}), lib)
		var got int64
		if m.Title != nil {
			got = m.Title.ID
		}
		if got != c.want {
			t.Errorf("%s: matched Title %d, want %d (%s)", c.name, got, c.want, m.Detail)
		}
	}
}

// BenchmarkMatchTitleTwoNames runs a miss against a 10,000-title library
// with a few aliases each: the two-name check runs for every title.
func BenchmarkMatchTitleTwoNames(b *testing.B) {
	lib := make([]TitleRef, 10000)
	for i := range lib {
		lib[i] = TitleRef{ID: int64(i + 1), Name: fmt.Sprintf("Title Number %d", i), Year: 2000,
			Aliases: []string{fmt.Sprintf("Il Titolo %d", i), fmt.Sprintf("Der Titel %d", i), fmt.Sprintf("Le Titre d'Or %d", i)}}
	}
	p := releasename.Parse("Some.Unknown.Movie.2001.1080p.BluRay-GRP", releasename.Hints{})
	b.ReportAllocs()
	for b.Loop() {
		MatchTitle(p, lib)
	}
}
