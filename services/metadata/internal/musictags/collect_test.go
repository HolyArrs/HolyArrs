package musictags

import (
	"strings"
	"testing"
	"time"
)

const groupTagsFixture = `{"rgA":{"release_group":{"caa_id":5,"date":"1999","name":"X","type":"Album"},
 "tag":{"artist":[{"artist_mbid":"a","count":7,"tag":"jazz"}],"release_group":[{"count":2,"tag":"bebop"},{"count":1,"tag":"live"}]}},
 "rgB":{"release_group":{"name":"Y"},"tag":{}}}`

func TestParseGroupTags(t *testing.T) {
	got, err := ParseGroupTags(strings.NewReader(groupTagsFixture))
	if err != nil {
		t.Fatal(err)
	}
	a := got["rga"]
	if len(a.Tags) != 2 || a.Tags[0].Name != "bebop" || a.Tags[0].Votes != 2 || len(a.ArtistTags) != 1 || !a.Art || a.Date != "1999" {
		t.Fatalf("rgA = %+v", a)
	}
	if b := got["rgb"]; b.Art || len(b.Tags) != 0 {
		t.Fatalf("rgB = %+v", b)
	}
}

const mbGroupsFixture = `{"release-group-count":5,"release-group-offset":0,"release-groups":[
 {"id":"G1","title":"Debut","primary-type":"Album","secondary-types":[],"first-release-date":"1993-02-22"},
 {"id":"G2","title":"Live At X","primary-type":"Album","secondary-types":["Live"],"first-release-date":"1990"},
 {"id":"G3","title":"Best Of","primary-type":"Album","secondary-types":["Compilation"],"first-release-date":"2008"},
 {"id":"G4","title":"Short","primary-type":"EP","secondary-types":[],"first-release-date":"1992-05"},
 {"id":"G5","title":"Score","primary-type":"Album","secondary-types":["Soundtrack"],"first-release-date":"2010"}]}`

func TestParseMBGroupsKeepsStudioWorkAndMarksDebut(t *testing.T) {
	gs, total, err := ParseMBGroups(strings.NewReader(mbGroupsFixture))
	if err != nil || total != 5 {
		t.Fatalf("total = %d, err %v", total, err)
	}
	by := map[string]Album{}
	for _, g := range gs {
		by[g.ID] = g
	}
	if _, ok := by["g2"]; ok {
		t.Fatal("live album kept")
	}
	if _, ok := by["g3"]; ok {
		t.Fatal("compilation kept")
	}
	if !by["g1"].Debut || by["g4"].Debut || by["g5"].Debut {
		t.Fatal("debut must be the earliest studio Album (not the EP, not the soundtrack)")
	}
	if by["g5"].Type != "Soundtrack" || by["g4"].Type != "EP" || by["g1"].Date != "1993-02-22" {
		t.Fatalf("types/dates = %+v", by)
	}
}

func TestParsePopularity(t *testing.T) {
	got, err := ParsePopularity(strings.NewReader(`[{"release_group_mbid":"AA","total_listen_count":10,"total_user_count":3},{"release_group_mbid":"BB","total_listen_count":null,"total_user_count":null}]`))
	if err != nil || got["aa"] != [2]int64{10, 3} || got["bb"] != [2]int64{0, 0} {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestUntaggedListsAlbumsWithoutTags(t *testing.T) {
	src := NewSource()
	src.Album(Album{ID: "A", Tags: tags("rock", 1)})
	src.Chart("us", "B", 1)
	got := src.Untagged()
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("Untagged = %v, want [b]", got)
	}
}

func TestArtistChartRanks(t *testing.T) {
	src := testSource()
	src.ArtistChart("us", "ar1", 4)
	b := Build(src, testRecipes, time.Now())
	for _, it := range b.Items {
		if it.Kind == "r" && it.ID == "ar1" {
			if it.C["us"] != 4 || it.P != 970 {
				t.Fatalf("artist chart = %v, P %d", it.C, it.P)
			}
			return
		}
	}
	t.Fatal("artist missing")
}
