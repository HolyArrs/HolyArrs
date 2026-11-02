package musictrend

import (
	"context"
	"path/filepath"
	"testing"
)

const (
	realLP   = "f59c5520-5f46-4d2c-b2c4-822eabf53419"
	parodyLP = "dd07eac1-d363-4560-9e89-d20b73c65d5a"
	parodyRG = "5654269e-ae75-4e91-8c4c-293d20fd81b1"
	realRG   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

// homonymSearch answers a parody's "Meteora" ahead of the band's, and the
// band first for the name alone (as MusicBrainz does).
type homonymSearch struct{ calls map[string]int }

func (h *homonymSearch) Search(_ context.Context, kind, name, artist string) ([]Candidate, error) {
	if h.calls == nil {
		h.calls = map[string]int{}
	}
	h.calls[kind+"|"+name]++
	switch kind {
	case "artist":
		return []Candidate{{MBID: realLP, Name: "Linkin Park"}, {MBID: parodyLP, Name: "Linkin Park"}}, nil
	default:
		return []Candidate{
			{MBID: parodyRG, Name: "Meteora", Artists: []string{"Linkin Park"}, ArtistMBID: parodyLP},
			{MBID: realRG, Name: "Meteora", Artists: []string{"Linkin Park"}, ArtistMBID: realLP},
		}, nil
	}
}

// Two same-titled albums by two same-named artists: the album is the one
// by the artist the artist match takes, never simply the first result.
func TestAlbumMatchFollowsTheArtistMatch(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "m.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := &Matcher{DB: db, Search: &homonymSearch{}}
	c, ok := m.Match(context.Background(), "album", "Meteora (20th Anniversary Edition)", "LINKIN PARK")
	if !ok || c.MBID != realRG || c.ArtistMBID != realLP {
		t.Fatalf("matched %+v %v", c, ok)
	}
}

// A stored album match whose artist is not the one the artist match takes
// is re-pointed when the slim copy has the album under that artist; the
// rest stay as they are.
func TestRepairHomonymAlbumMatches(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "m.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ins := func(kind, name, artist, mbid, artistMBID string) {
		if _, err := db.Exec(`INSERT INTO matches VALUES(?,?,?,?,?,1)`, kind, name, artist, mbid, artistMBID); err != nil {
			t.Fatal(err)
		}
	}
	ins("artist", "LINKIN PARK", "", realLP, "")
	ins("album", "Meteora (Deluxe Edition)", "LINKIN PARK", parodyRG, parodyLP)
	ins("album", "Unknown Album", "LINKIN PARK", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", parodyLP) // not under the band in the copy
	m := &Matcher{DB: db, Search: &homonymSearch{}}
	slim := func(_ context.Context, artist, title string) string {
		if artist == realLP && baseTitle(title) == "Meteora" {
			return realRG
		}
		return ""
	}
	n, err := m.RepairHomonyms(context.Background(), slim)
	if err != nil || n != 1 {
		t.Fatalf("repaired %d %v", n, err)
	}
	c, _, _ := m.Known(context.Background(), "album", "Meteora (Deluxe Edition)", "LINKIN PARK")
	if c.MBID != realRG || c.ArtistMBID != realLP {
		t.Fatalf("not repaired: %+v", c)
	}
	c, _, _ = m.Known(context.Background(), "album", "Unknown Album", "LINKIN PARK")
	if c.ArtistMBID != parodyLP {
		t.Fatalf("an album the copy does not hold under the band was moved: %+v", c)
	}
}

// The other way round: the artist match took an empty same-name entry,
// while the charted album points to the artist that holds it. The artist
// holding the most of the name's charted albums wins, and the artist
// match follows it.
func TestRepairHomonymArtistMatches(t *testing.T) {
	const (
		stub  = "af5ff13d-58d3-4cd3-a4a2-626e7e4e4c7e"
		bedry = "bbfda332-f0b3-4499-bb8a-4c2571999666"
		rg    = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	db, err := openDB(filepath.Join(t.TempDir(), "m.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, r := range [][]string{{"artist", "Bedry", "", stub, ""}, {"album", "Beewaba", "Bedry", rg, bedry}} {
		if _, err := db.Exec(`INSERT INTO matches VALUES(?,?,?,?,?,1)`, r[0], r[1], r[2], r[3], r[4]); err != nil {
			t.Fatal(err)
		}
	}
	m := &Matcher{DB: db, Search: &homonymSearch{}}
	slim := func(_ context.Context, artist, title string) string {
		if artist == bedry && title == "Beewaba" {
			return rg
		}
		return ""
	}
	n, err := m.RepairHomonyms(context.Background(), slim)
	if err != nil || n != 1 {
		t.Fatalf("repaired %d %v", n, err)
	}
	if c, _, _ := m.Known(context.Background(), "artist", "Bedry", ""); c.MBID != bedry {
		t.Fatalf("artist match not moved: %+v", c)
	}
	if c, _, _ := m.Known(context.Background(), "album", "Beewaba", "Bedry"); c.MBID != rg || c.ArtistMBID != bedry {
		t.Fatalf("album match changed: %+v", c)
	}
}
