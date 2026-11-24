package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// {french_version} names the French dub; next to {release} it adds
// only a tag the release name leaves out. A pattern without it is unchanged.
func TestFrenchVersionWord544(t *testing.T) {
	s := NamingDefaults()
	s.FilePattern = "{title} ({year}) {quality}{ french_version}"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	name := "Movie.2024.MULTi.VFQ.1080p.WEB-GRP"
	f := FactsFrom("Movie", 2024, nil, releasename.Parse(name, releasename.Hints{}), name, ".mkv")
	if got := FileName(s, f); got != "Movie (2024) 1080p WEB VFQ.mkv" {
		t.Fatalf("from name = %q", got)
	}
	name = "Movie.2024.MULTi.1080p.WEB-GRP"
	f = FactsFrom("Movie", 2024, nil, releasename.Parse(name, releasename.Hints{}), name, ".mkv")
	f.AddFrenchVersion("VFF")
	if got := FileName(s, f); got != "Movie (2024) 1080p WEB VFF.mkv" {
		t.Fatalf("from tracks = %q", got)
	}
	f.FrenchVersion = ""
	if got := FileName(s, f); got != "Movie (2024) 1080p WEB.mkv" {
		t.Fatalf("none = %q", got)
	}

	s.FilePattern = "{release}{.french_version}"
	f = FactsFrom("Movie", 2024, nil, releasename.Parse(name, releasename.Hints{}), name, ".mkv")
	f.AddFrenchVersion("VFQ")
	if got := FileName(s, f); got != "Movie.2024.MULTi.1080p.WEB-GRP.VFQ.mkv" {
		t.Fatalf("missing tag = %q", got)
	}
	tagged := "Movie.2024.MULTi.VFQ.1080p.WEB-GRP"
	f = FactsFrom("Movie", 2024, nil, releasename.Parse(tagged, releasename.Hints{}), tagged, ".mkv")
	f.AddFrenchVersion("VFF") // the name wins over the tracks
	if got := FileName(s, f); got != tagged+".mkv" {
		t.Fatalf("already tagged = %q", got)
	}
	if f.FrenchVersion != "VFQ" {
		t.Fatalf("version = %q", f.FrenchVersion)
	}
	s.FilePattern = "{Movie Title} {Mediainfo FrenchVersion}"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}
