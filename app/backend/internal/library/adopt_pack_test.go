package library

import (
	"context"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestCollectionPackLookup(t *testing.T) {
	for _, tc := range []struct {
		path, title string
		year        int
	}{
		{"Halloween Complete Collection/01 Halloween - Horror 1978 Donald Pleasence Eng Subs 720p [H264-mp4].mp4", "Halloween", 1978},
		{"Halloween Complete Collection/02 Halloween II - Horror 1981 Eng Subs 720p.mp4", "Halloween II", 1981},
		{"Halloween Complete Collection/03 Halloween III Season of the Witch - Horror 1982 Eng Subs 720p.mp4", "Halloween III Season of the Witch", 1982},
		{"Halloween Complete Collection/04 Halloween 4 The Return of Michael Myers - Horror 1988 Eng Subs 720p.mp4", "Halloween 4 The Return of Michael Myers", 1988},
		{"Halloween Complete Collection/05 Halloween 5 The Revenge of Michael Myers - Horror 1989 Eng Subs 720p.mp4", "Halloween 5 The Revenge of Michael Myers", 1989},
		{"Halloween Complete Collection/06 Halloween The Curse of Michael Myers - Horror 1995 Eng Subs 720p.mp4", "Halloween The Curse of Michael Myers", 1995},
		{"Halloween Complete Collection/07 Halloween H20 20 Years Later - Horror 1998 Eng Subs 720p.mp4", "Halloween H20 20 Years Later", 1998},
		{"Halloween Complete Collection/08 Halloween Resurrection - Horror 2002 Eng Subs 720p.mp4", "Halloween Resurrection", 2002},
		{"Halloween Complete Collection/09 Halloween Rob Zombie - Horror 2007 Eng Subs 720p.mp4", "Halloween Rob Zombie", 2007},
		{"Halloween Complete Collection/10 Halloween II Rob Zombie - Horror 2009 Eng Subs 720p [H264-mp4].mp4", "Halloween II Rob Zombie", 2009},
		{"Final Destination Complete 5 Movie Collection/04 The Final Destination 4 - Horror 2009 Eng Rus Subs 720p [H264-mp4].mp4", "The Final Destination 4", 2009},
		{"Final Destination Complete 5 Movie Collection/01 Final Destination 1 - Horror 2000 Eng Rus Subs 720p [H264-mp4].mp4", "Final Destination 1", 2000},
		{"Final Destination Complete 5 Movie Collection/02 Final Destination 2 - Horror 2003 Eng Rus Subs 720p.mp4", "Final Destination 2", 2003},
		{"Final Destination Complete 5 Movie Collection/03 Final Destination 3 - Horror 2006 Eng Rus Subs 720p.mp4", "Final Destination 3", 2006},
		{"Final Destination Complete 5 Movie Collection/05 Final Destination 5 - Horror 2011 Eng Rus Subs 720p.mp4", "Final Destination 5", 2011},
		{"Alien Collection/Alien Directors Cut - Sci-Fi 1979 Eng Subs 720p [H264-mp4].mp4", "Alien", 1979},
		{"Collection/101 Dalmatians - Animation 1961 1080p.mkv", "101 Dalmatians", 1961},
		{"Collection/12 Monkeys - Sci-Fi 1995 1080p.mkv", "12 Monkeys", 1995},
		{"12 Monkeys Collection/12 Monkeys - Sci-Fi 1995 1080p.mkv", "12 Monkeys", 1995},
		{"10 Cloverfield Lane Collection/10 Cloverfield Lane - Horror 2016 1080p.mkv", "10 Cloverfield Lane", 2016},
		{"101.Dalmatians.1961.1080p.BluRay.H264.AAC-RARBG", "101 Dalmatians", 1961},
		{"Collection/2001 A Space Odyssey (1968).mkv", "2001 A Space Odyssey", 1968},
		{"Collection/1917 (2019).mkv", "1917", 2019},
		{"Halloween Collection/01 Halloween (1978).mkv", "01 Halloween", 1978},
	} {
		t.Run(tc.path, func(t *testing.T) {
			lookup := matchingLookup(tc.path)
			name, year := FolderName(lookup)
			if name != tc.title || year != tc.year {
				t.Fatalf("got %q (%d), want %q (%d)", name, year, tc.title, tc.year)
			}
			read := folderRead(lookup)
			if read.Query.Get("query") != name {
				t.Fatalf("prefetch query differs: %+v", read)
			}
		})
	}
}

type packMetadata struct {
	metadata.Provider
	queries []string
}

func (m *packMetadata) Search(_ context.Context, name string, year int) ([]metadata.Movie, error) {
	m.queries = append(m.queries, fmt.Sprintf("%s/%d", name, year))
	if name == "Halloween" && year == 1978 {
		return []metadata.Movie{{Name: "Halloween", Year: 1978, IDs: map[string]string{"tmdb": "948"}}}, nil
	}
	if name == "Halloween II" && year == 1981 {
		return []metadata.Movie{{Name: "Halloween II", Year: 1981, IDs: map[string]string{"tmdb": "11281"}}}, nil
	}
	return nil, fmt.Errorf("unexpected lookup: %s/%d", name, year)
}

func TestCollectionPackPreservesExplicitTMDB(t *testing.T) {
	r := folderRead(matchingLookup("Halloween Collection/01 Halloween - Horror 1978 720p [tmdb-948].mp4"))
	if r.Path != "/movie/948" {
		t.Fatalf("lost explicit TMDB: %+v", r)
	}
}
