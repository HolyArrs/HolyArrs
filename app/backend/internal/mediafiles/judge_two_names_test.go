package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// a release named "<local title> <original title>" (Italian style)
// matches when both names are ones the Title already has.
func TestMatchTitleTwoNames(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Name: "Charmed", Year: 1998, Kind: "show", Aliases: []string{"Streghe"}},
		{ID: 2, Name: "The Mummy Returns", Year: 2001, Kind: "movie", Aliases: []string{"La Mummia - Il Ritorno"}},
		{ID: 3, Name: "Harry Potter and the Chamber of Secrets", Year: 2002, Kind: "movie", Aliases: []string{"Harry Potter e la camera dei segreti"}},
	}
	cases := []struct {
		name string
		want int64
	}{
		{"Streghe.Charmed.S01E01.1080p.WEB-DL.ITA.ENG-GRP", 1},
		{"Charmed.Streghe.S01E01.1080p.WEB-DL.ITA.ENG-GRP", 1},
		{"La.Mummia.Il.Ritorno.The.Mummy.Returns.2001.1080p.BluRay.ITA.ENG-GRP", 2},
		{"La Mummia Il Ritorno - Mummy Returns 2001 1080p BluRay ITA ENG", 2},
		{"Harry.Potter.E.La.Camera.Dei.Segreti.Harry.Potter.and.the.Chamber.of.Secrets.2002.1080p.BluRay-GRP", 3},
		{"Streghe.Other.S01E01.1080p.WEB-DL-GRP", 0},
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
