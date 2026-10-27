package metadata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestMusicPicturesBatch: one call for a row's artists and albums, with the
// credential; odd records (bad version, size or colour) are dropped and
// ids that were not asked are never kept.
func TestMusicPicturesBatch(t *testing.T) {
	const ar, al = "a74b1b7f-71a5-4011-9441-d0b5e4122711", "21159e3f-172e-43f6-aa7d-8e06a81fea49"
	calls := 0
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/music/pictures" || r.Header.Get("Authorization") == "" {
			t.Errorf("asked %s (credential %q)", r.URL.Path, r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("artists") != ar || r.URL.Query().Get("albums") != al {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"artists":{"` + ar + `":{"photo":{"v":"p1","w":1200,"h":1200,"color":"#A1B2C3"},"wide":{"v":"bad v","w":1920,"h":1080},"logo":{"v":"l1","w":800,"h":310,"color":"red"}},
			"11111111-1111-4111-8111-111111111111":{"photo":{"v":"x","w":1,"h":1}}},
			"albums":{"` + strings.ToUpper(al) + `":{"cover":{"v":"c1","w":1200,"h":1200,"color":"#102030"}}}}`))
	})
	got, err := s.MusicPictures(context.Background(), []string{strings.ToUpper(ar), ar, "not-an-id"}, []string{al})
	if err != nil || calls != 1 {
		t.Fatalf("%v, %d calls", err, calls)
	}
	a := got.Artists[ar]
	if a.Photo == nil || *a.Photo != (PictureRecord{V: "p1", W: 1200, H: 1200, Color: "#a1b2c3"}) {
		t.Fatalf("photo %+v", a.Photo)
	}
	if a.Wide != nil {
		t.Fatalf("a record with a bad version kept: %+v", a.Wide)
	}
	if a.Logo == nil || a.Logo.Color != "" || a.Logo.W != 800 {
		t.Fatalf("logo %+v", a.Logo)
	}
	if _, ok := got.Artists["11111111-1111-4111-8111-111111111111"]; ok || len(got.Artists) != 1 {
		t.Fatalf("an id not asked was kept: %+v", got.Artists)
	}
	if c := got.Albums[al].Cover; c == nil || c.V != "c1" || c.Color != "#102030" {
		t.Fatalf("cover %+v", c)
	}
	// Nothing to ask: no call at all.
	if _, err := s.MusicPictures(context.Background(), nil, []string{"song-12"}); err != nil || calls != 1 {
		t.Fatalf("empty batch asked: %v, %d calls", err, calls)
	}
}

// TestMusicPicturesCap: a batch never asks for more than MusicPicturesMax ids.
func TestMusicPicturesCap(t *testing.T) {
	var n int
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		n = len(strings.Split(q.Get("artists"), ","))
		if q.Get("albums") != "" {
			n += len(strings.Split(q.Get("albums"), ","))
		}
		_, _ = w.Write([]byte(`{"artists":{},"albums":{}}`))
	})
	var ids []string
	for i := 0; i < 150; i++ {
		ids = append(ids, fmt.Sprintf("00000000-0000-4000-8000-%012d", i))
	}
	if _, err := s.MusicPictures(context.Background(), ids[:80], ids[80:]); err != nil || n != MusicPicturesMax {
		t.Fatalf("%v: asked %d ids", err, n)
	}
}
