package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Hard cases for releases named with two of a Title's names:
// accents, other scripts, punctuation between the names, articles and
// names that must not match.
func TestMatchTitleTwoNamesScripts(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Name: "Charmed", Year: 1998, Kind: "show", Aliases: []string{"Streghe", "Зачарованные", "圣女魔咒"}},
		{ID: 2, Name: "The Mummy Returns", Year: 2001, Kind: "movie", Aliases: []string{"La Mummia - Il Ritorno", "Мумия возвращается"}},
		{ID: 3, Name: "Amélie", Year: 2001, Kind: "movie", Aliases: []string{"Le Fabuleux Destin d'Amélie Poulain", "Il favoloso mondo di Amélie"}},
		{ID: 4, Name: "Money Heist", Year: 2017, Kind: "show", Aliases: []string{"La casa de papel", "La Casa di Carta"}},
		{ID: 5, Name: "Crouching Tiger, Hidden Dragon", Year: 2000, Kind: "movie", Aliases: []string{"臥虎藏龍", "La tigre e il dragone"}},
		{ID: 6, Name: "Spirited Away", Year: 2001, Kind: "movie", Aliases: []string{"Sen to Chihiro no Kamikakushi", "La città incantata", "千と千尋の神隠し"}},
		{ID: 7, Name: "Law & Order", Year: 1990, Kind: "show", Aliases: []string{"Law and Order: I due volti della giustizia"}},
		{ID: 8, Name: "Agents of S.H.I.E.L.D.", Year: 2013, Kind: "show", Aliases: []string{"Agenti dello S.H.I.E.L.D."}},
		{ID: 9, Name: "Dark", Year: 2017, Kind: "show"},
		{ID: 10, Name: "Șapte ani", Year: 2010, Kind: "movie", Aliases: []string{"Seven Years"}},
		{ID: 11, Name: "Cuộc chiến", Year: 2015, Kind: "movie", Aliases: []string{"The War Within"}},
		{ID: 12, Name: "Þrestir", Year: 2015, Kind: "movie", Aliases: []string{"Sparrows"}},
		{ID: 13, Name: "Gomorrah", Year: 2014, Kind: "show", Aliases: []string{"Gomorra - La serie"}},
		{ID: 14, Name: "Up", Year: 2009, Kind: "movie", Aliases: []string{"Su"}},
		{ID: 15, Name: "It", Year: 2017, Kind: "movie", Aliases: []string{"Es"}},
		{ID: 16, Name: "Pokémon: The First Movie", Year: 1998, Kind: "movie", Aliases: []string{"Pokémon: Il film - Mewtwo contro Mew"}},
		{ID: 17, Name: "Đất rừng phương Nam", Year: 2023, Kind: "movie", Aliases: []string{"Song of the South"}},
		{ID: 18, Name: "Ödipussi", Year: 1988, Kind: "movie", Aliases: []string{"Oedipussi Movie"}},
		{ID: 19, Name: "Ziemia obiecana", Year: 1975, Kind: "movie", Aliases: []string{"The Promised Land"}},
		{ID: 20, Name: "İstanbul Kırmızısı", Year: 2017, Kind: "movie", Aliases: []string{"Red Istanbul"}},
		{ID: 21, Name: "Ǆungla", Year: 2020, Kind: "movie", Aliases: []string{"Jungle Tale"}},
		{ID: 22, Name: "Ça", Year: 2019, Kind: "movie", Aliases: []string{"That Thing Two"}},
	}
	cases := []struct {
		name string
		want int64
	}{
		{"Streghe.Charmed.S01E01.ITA.ENG.1080p-GRP", 1},
		{"STREGHE.CHARMED.S01E01.1080p-GRP", 1},
		{"streghe charmed s01e01 1080p", 1},
		{"Streghe - Charmed - S01E01 - 1080p", 1},
		{"Streghe_Charmed_S01E01_1080p", 1},
		{"Streghe AKA Charmed S01E01 1080p", 1},
		{"Streghe aka Charmed S01E01 1080p", 1},
		{"Streghe.a.k.a.Charmed.S01E01.1080p", 1},
		{"Streghe / Charmed S01E01 1080p", 1},
		{"Streghe | Charmed S01E01 1080p", 1},
		{"Streghe+Charmed.S01E01.1080p", 1},
		{"Зачарованные.Charmed.S01E01.1080p", 1},
		{"Зачарованные / Charmed / S01E01 1080p", 1},
		{"圣女魔咒.Charmed.S01E01.1080p", 1},
		{"Charmed.Зачарованные.S01E01.1080p", 1},
		{"Streghe.Charmed.1998.S01E01.1080p", 1},
		{"Мумия возвращается / The Mummy Returns (2001) 1080p", 2},
		{"The.Mummy.Returns.La.Mummia.Il.Ritorno.2001.1080p-GRP", 2},
		{"Mummy.Returns.La.Mummia.Il.Ritorno.2001.1080p-GRP", 2},
		{"La.Mummia.Il.Ritorno.AKA.The.Mummy.Returns.2001.1080p", 2},
		{"Il.Favoloso.Mondo.Di.Amelie.Amelie.2001.1080p", 3},
		{"Il Favoloso Mondo di Amélie - Amélie 2001 1080p", 3},
		{"Le.Fabuleux.Destin.D.Amelie.Poulain.Amelie.2001.1080p", 3},
		{"Le.Fabuleux.Destin.Damelie.Poulain.Amelie.2001.1080p", 3},
		{"La.Casa.Di.Carta.Money.Heist.S01E01.1080p", 4},
		{"La.Casa.De.Papel.La.Casa.Di.Carta.S01E01.1080p", 4},
		{"Money.Heist.La.Casa.De.Papel.S01E01.1080p", 4},
		{"La.Tigre.E.Il.Dragone.Crouching.Tiger.Hidden.Dragon.2000.1080p", 5},
		{"臥虎藏龍.Crouching.Tiger.Hidden.Dragon.2000.1080p", 5},
		{"La.Citta.Incantata.Spirited.Away.2001.1080p", 6},
		{"La Città Incantata - Spirited Away 2001 1080p", 6},
		{"千と千尋の神隠し.Spirited.Away.2001.1080p", 6},
		{"Sen.to.Chihiro.no.Kamikakushi.Spirited.Away.2001.1080p", 6},
		{"Law.and.Order.I.Due.Volti.Della.Giustizia.Law.&.Order.S01E01.1080p", 7},
		{"Agenti.Dello.S.H.I.E.L.D.Agents.of.S.H.I.E.L.D.S01E01.1080p", 8},
		{"Agenti.Dello.SHIELD.Agents.Of.SHIELD.S01E01.1080p", 8},
		{"Sapte.Ani.Seven.Years.2010.1080p", 10},
		{"Şapte.Ani.Seven.Years.2010.1080p", 10},
		{"Cuoc.Chien.The.War.Within.2015.1080p", 11},
		{"Threstir.Sparrows.2015.1080p", 12},
		{"Gomorra.La.Serie.Gomorrah.S01E01.1080p", 13},
		{"Dat.Rung.Phuong.Nam.Song.Of.The.South.2023.1080p", 17},
		{"Ziemia.Obiecana.The.Promised.Land.1975.1080p", 19},
		{"Istanbul.Kirmizisi.Red.Istanbul.2017.1080p", 20},
		{"ISTANBUL.KIRMIZISI.RED.ISTANBUL.2017.1080p", 20},
		// must not match
		{"Streghe.Other.S01E01.1080p", 0},
		{"Dark.Matter.S01E01.1080p", 0},
		{"Up.Su.2009.1080p", 14},
		{"Up.In.The.Air.2009.1080p", 0},
		{"It.Follows.2014.1080p", 0},
		{"Es.It.2017.1080p", 15},
		{"Streghe.Charmed.Extra.S01E01.1080p", 0},
		{"Mummy.2001.1080p", 0},
	}
	for _, c := range cases {
		m := MatchTitle(releasename.Parse(c.name, releasename.Hints{}), lib)
		var got int64
		if m.Title != nil {
			got = m.Title.ID
		}
		if got != c.want {
			t.Errorf("%s: matched %d, want %d (title %q: %s)", c.name, got, c.want, releasename.Parse(c.name, releasename.Hints{}).Title.Value, m.Detail)
		}
	}
}

func BenchmarkMatchTitleTwoNamesScripts(b *testing.B) {
	lib := make([]TitleRef, 5000)
	for i := range lib {
		lib[i] = TitleRef{ID: int64(i + 1), Name: "Some Show Number " + string(rune('a'+i%26)), Year: 2000, Kind: "show", Aliases: []string{"Uno Spettacolo", "Ein Ding", "Une Chose"}}
	}
	p := releasename.Parse("Streghe.Charmed.S01E01.1080p-GRP", releasename.Hints{})
	b.ReportAllocs()
	for b.Loop() {
		MatchTitle(p, lib)
	}
}
