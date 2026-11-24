package mediafiles

import (
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var words = []string{"Dark", "Night", "Mummia", "Ritorno", "Streghe", "Über", "Grüße", "Café", "Rocky", "Sombra", "Ştefan", "Città", "Ángel", "Lune", "Fuego", "Kehrt", "Zurück", "Señor", "Ωmega", "Blue"}

func benchLib(n int) []TitleRef {
	lib := make([]TitleRef, n)
	for i := range lib {
		w := func(k int) string { return words[(i*7+k*13)%len(words)] }
		lib[i] = TitleRef{ID: int64(i), Name: fmt.Sprintf("%s %s %d", w(0), w(1), i), Year: 1950 + i%70,
			Aliases: []string{fmt.Sprintf("%s %s %d", w(2), w(3), i), fmt.Sprintf("%s's %s II %d", w(4), w(5), i), fmt.Sprintf("Le %s %d", w(6), i)}}
		if i%3 == 0 {
			lib[i].Kind = "show"
		}
	}
	return lib
}

func benchReleases(n int) []releasename.Result {
	out := make([]releasename.Result, n)
	for i := range out {
		var s string
		switch i % 4 {
		case 0:
			s = fmt.Sprintf("Unknown Film %d 2001 1080p WEB-DL x264-GRP", i)
		case 1:
			s = fmt.Sprintf("ITA Mummia Ritorno %d Night Dark %d 2001 1080p BluRay", i, i)
		case 2:
			s = fmt.Sprintf("Streghe Charmed %d S01E02 720p HDTV", i)
		default:
			s = fmt.Sprintf("Zurück Café Ángel %d aka Ωmega Blue 1999 2160p", i)
		}
		out[i] = releasename.Parse(s, releasename.Hints{})
	}
	return out
}

func BenchmarkMatch10kx1000(b *testing.B) {
	lib := benchLib(10000)
	rels := benchReleases(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, p := range rels {
			MatchTitle(p, lib)
		}
	}
}
