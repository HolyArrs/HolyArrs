package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMusicArtistExtras(t *testing.T) {
	const drake = "9fff2f8a-21e6-47de-a2b8-7f449929d43f"
	const none = "22222222-2222-4222-8222-222222222222"
	const down = "33333333-3333-4333-8333-333333333333"
	const mb = "44444444-4444-4444-8444-444444444444"
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("no credential")
		}
		switch r.URL.Path {
		case "/v1/music/top-songs/" + drake:
			_, _ = w.Write([]byte(`{"schema_version":1,"mbid":"` + drake + `","source":"deezer","songs":[{"title":"One Dance","length_ms":173000,"explicit":false,"album":"Views","cover":"https://x/c.jpg"}]}`))
		case "/v1/music/top-songs/" + none, "/v1/music/discography/" + none:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"not_found"}`))
		case "/v1/music/top-songs/" + down:
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":"provider_unavailable"}`))
		case "/v1/music/charts/rising":
			_, _ = w.Write([]byte(`{"schema_version":1,"as_of":"2026-10-27","items":[{"mbid":"9FFF2F8A-21E6-47DE-A2B8-7F449929D43F","name":"Drake","listeners":80000000,"gain":120000,"rank":7},{"mbid":"bad","name":"x"}]}`))
		case "/v1/music/discography/" + drake:
			_, _ = w.Write([]byte(`{"schema_version":1,"mbid":"` + drake + `","source":"applemusic","albums":[{"title":"Views","release_date":"2016-04-29","type":"album","track_count":20,"explicit":true}]}`))
		case "/v1/music/discography/" + mb:
			_, _ = w.Write([]byte(`{"schema_version":1,"mbid":"` + mb + `","source":"musicbrainz","albums":[]}`))
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	top, err := s.MusicTopSongs(ctx, drake)
	if err != nil || len(top.Songs) != 1 || top.Songs[0].Title != "One Dance" || top.Songs[0].LengthMS != 173000 || top.Songs[0].Cover == "" {
		t.Fatalf("top songs %+v %v", top, err)
	}
	if _, err := s.MusicTopSongs(ctx, none); !errors.Is(err, ErrNotFound) {
		t.Fatalf("none known %v", err)
	}
	if _, err := s.MusicTopSongs(ctx, down); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("try later %v", err)
	}
	if _, err := s.MusicTopSongs(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id %v", err)
	}
	ris, err := s.MusicRisingToday(ctx)
	if err != nil || len(ris.Items) != 1 || ris.Items[0].MBID != drake || ris.Items[0].Gain != 120000 || ris.AsOf != "2026-10-27" {
		t.Fatalf("rising %+v %v", ris, err)
	}
	d, err := s.MusicDiscography(ctx, drake)
	if err != nil || d.Source != "applemusic" || len(d.Albums) != 1 || d.Albums[0].TrackCount != 20 || !d.Albums[0].Explicit {
		t.Fatalf("discography %+v %v", d, err)
	}
	if d, err := s.MusicDiscography(ctx, mb); err != nil || d.Source != "musicbrainz" || len(d.Albums) != 0 {
		t.Fatalf("musicbrainz stands %+v %v", d, err)
	}
	if _, err := s.MusicDiscography(ctx, none); !errors.Is(err, ErrNotFound) {
		t.Fatalf("none %v", err)
	}
}

func TestArtistPopularityOrNil(t *testing.T) {
	var nothing *ArtistPopularity
	if nothing.orNil() != nil || (&ArtistPopularity{AsOf: "2026-10-27"}).orNil() != nil {
		t.Fatal("an answer with no figure shows nothing")
	}
	p := (&ArtistPopularity{TotalPlays: 9000, WorldRank: 3, ListenersChangeToday: 5}).orNil()
	if p == nil || p.TotalPlays != 9000 || p.WorldRank != 0 || p.ListenersChangeToday != 0 {
		t.Fatalf("plays only %+v", p)
	}
	p = (&ArtistPopularity{MonthlyListeners: 80e6, WorldRank: 7, ListenersChangeToday: -1200}).orNil()
	if p == nil || p.WorldRank != 7 || p.ListenersChangeToday != -1200 {
		t.Fatalf("top artist %+v", p)
	}
}

func TestServiceArtistPopularity(t *testing.T) {
	item := serviceItem{Source: "musicbrainz", Kind: "artist", SourceID: "9fff2f8a-21e6-47de-a2b8-7f449929d43f",
		SourceData: []byte(`{"id":"9fff2f8a-21e6-47de-a2b8-7f449929d43f","name":"Drake","popularity":{"monthly_listeners":80000000,"world_rank":7,"listeners_change_today":120000,"as_of":"2026-10-27"}}`)}
	a, err := parseServiceArtist(item, false)
	if err != nil || a.Popularity == nil || a.Popularity.WorldRank != 7 {
		t.Fatalf("%+v %v", a, err)
	}
	item.SourceData = []byte(`{"id":"9fff2f8a-21e6-47de-a2b8-7f449929d43f","name":"Drake"}`)
	item.Popularity = &ArtistPopularity{TotalPlays: 42}
	if a, err := parseServiceArtist(item, false); err != nil || a.Popularity == nil || a.Popularity.TotalPlays != 42 {
		t.Fatalf("beside the data %+v %v", a, err)
	}
	item.Popularity = nil
	if a, err := parseServiceArtist(item, false); err != nil || a.Popularity != nil {
		t.Fatalf("no object %+v %v", a, err)
	}
}

func TestServicePopularityEitherPlace(t *testing.T) {
	src := []byte(`{"id":"9fff2f8a-21e6-47de-a2b8-7f449929d43f","name":"Drake","popularity":{"monthly_listeners":94742435,"world_rank":7}}`)
	if p := servicePopularity(serviceItem{Source: "musicbrainz", Kind: "artist", SourceID: "9fff2f8a-21e6-47de-a2b8-7f449929d43f", SourceData: src}); p == nil || p.WorldRank != 7 {
		t.Fatalf("inside %+v", p)
	}
	beside := serviceItem{SourceData: []byte(`{"id":"x"}`), Popularity: &ArtistPopularity{TotalPlays: 5}}
	if p := servicePopularity(beside); p == nil || p.TotalPlays != 5 {
		t.Fatalf("beside %+v", p)
	}
	if servicePopularity(serviceItem{}) != nil {
		t.Fatal("none")
	}
}
