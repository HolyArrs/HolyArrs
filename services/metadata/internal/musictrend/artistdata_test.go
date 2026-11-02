package musictrend

import (
	"strings"
	"testing"
)

func TestNormaliseSimilar(t *testing.T) {
	got := NormaliseSimilar([]SimilarArtist{{MBID: "a", Score: 50}, {MBID: "b", Score: 200}, {MBID: "c", Score: 0}}, "applemusic")
	if len(got) != 2 || got[0].S != 0.25 || got[1].S != 1 || got[0].Source != "applemusic" {
		t.Fatalf("%+v", got)
	}
	if got := NormaliseSimilar([]SimilarArtist{{MBID: "a", Score: 0}}, "listenbrainz"); len(got) != 0 {
		t.Fatalf("top 0 must give none: %+v", got)
	}
	if got := NormaliseSimilar(nil, "listenbrainz"); got == nil || len(got) != 0 {
		t.Fatal("empty seed must give an empty list")
	}
}

const ad1 = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
const ad2 = "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d"
const rg1 = "6e335887-60ba-38f0-95af-fae7774336bf"
const rg2 = "1e335887-60ba-38f0-95af-fae7774336bf"

func TestParseLBArtistData(t *testing.T) {
	body := `[{"artist":{"artists":[{"artist_mbid":"` + ad1 + `"}]},"release_group":{"name":"In Rainbows","date":"2007-10-10","type":"Album"},"release_group_mbid":"` + rg1 + `","total_user_count":277665,
	"tag":{"artist":[{"artist_mbid":"` + ad1 + `","count":43,"tag":"Alternative Rock","genre_mbid":"x"},{"artist_mbid":"` + ad1 + `","count":14,"tag":"british"},{"artist_mbid":"` + ad1 + `","count":0,"tag":"pop","genre_mbid":"y"}]}},
	{"artist":{"artists":[{"artist_mbid":"` + ad2 + `","name":"Two"},{"artist_mbid":"` + ad1 + `"}]},"release_group":{"name":"Collab","type":"Single"},"release_group_mbid":"` + rg2 + `","total_user_count":10,"tag":{}},
	{"artist":{"artists":[]},"release_group":{"name":"Dup","type":"Album"},"release_group_mbid":"` + rg1 + `","total_user_count":1,"tag":{}}]`
	d, err := ParseLBArtistData(strings.NewReader(body), ad1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Genres) != 1 || d.Genres[0].Name != "alternative rock" || d.Genres[0].Count != 43 {
		t.Fatalf("genres %+v", d.Genres)
	}
	if len(d.Albums) != 2 || d.Albums[0].Listeners != 277665 || d.Albums[1].Artists[0] != ad2 || d.Albums[1].Names[0] != "Two" || d.Albums[0].Type != "album" {
		t.Fatalf("albums %+v", d.Albums)
	}
}
