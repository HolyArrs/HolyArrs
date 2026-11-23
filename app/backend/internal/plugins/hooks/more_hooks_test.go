package hooks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestArtworkHookTable(t *testing.T) {
	g := newRig(t, map[string][]string{"poster-ai": {"library.artwork.set"}, "reader": {"library.read"}})
	var got []string
	g.h.SetArtwork = func(_ context.Context, id int64, kind string, pic []byte) error {
		if string(pic) != "PNGBYTES" {
			return errors.New("send a JPEG or PNG picture")
		}
		got = append(got, kind)
		return nil
	}
	img := base64.StdEncoding.EncodeToString([]byte("PNGBYTES"))
	for _, c := range []struct {
		who, body string
		code      int
	}{
		{"poster-ai", `{"title_id":4,"kind":"poster","image":"` + img + `"}`, http.StatusNoContent},
		{"poster-ai", `{"title_id":4,"kind":"poster","image":"` + base64.StdEncoding.EncodeToString([]byte("http://10.0.0.1/x.png")) + `"}`, http.StatusBadRequest},
		{"poster-ai", `{"title_id":0,"kind":"poster","image":"` + img + `"}`, http.StatusBadRequest},
		{"poster-ai", `{"title_id":4,"kind":"poster","url":"https://evil/x.png"}`, http.StatusBadRequest},
		{"reader", `{"title_id":4,"kind":"poster","image":"` + img + `"}`, http.StatusForbidden},
	} {
		if w := g.do(plug(c.who), "POST", "/api/v1/plugin-hooks/artwork", c.body); w.Code != c.code {
			t.Errorf("%s %s: %d %s", c.who, c.body[:30], w.Code, w.Body)
		}
	}
	if len(got) != 1 {
		t.Errorf("set %v", got)
	}
}

func TestNotifyHookTable(t *testing.T) {
	g := newRig(t, map[string][]string{"bell": {"alerts.webhooks.send"}, "reader": {"library.read"}})
	var sent []string
	g.h.Notify = func(_ context.Context, plugin, subject, text string) error {
		sent = append(sent, plugin+": "+subject)
		return nil
	}
	for _, c := range []struct {
		who, body string
		code      int
	}{
		{"bell", `{"subject":"Backup done","text":"All 3 folders"}`, http.StatusNoContent},
		{"bell", `{"subject":"","text":"x"}`, http.StatusBadRequest},
		{"bell", `{"subject":"<b>hi</b>","text":"x"}`, http.StatusBadRequest},
		{"bell", `{"subject":"x","text":"` + strings.Repeat("a", 1001) + `"}`, http.StatusBadRequest},
		{"reader", `{"subject":"x","text":"y"}`, http.StatusForbidden},
	} {
		if w := g.do(plug(c.who), "POST", "/api/v1/plugin-hooks/notify", c.body); w.Code != c.code {
			t.Errorf("%s %s: %d %s", c.who, c.body, w.Code, w.Body)
		}
	}
	for range notifyPerHour - 1 {
		g.do(plug("bell"), "POST", "/api/v1/plugin-hooks/notify", `{"subject":"x","text":"y"}`)
	}
	if w := g.do(plug("bell"), "POST", "/api/v1/plugin-hooks/notify", `{"subject":"x","text":"y"}`); w.Code != http.StatusTooManyRequests {
		t.Errorf("over the hourly cap: %d", w.Code)
	}
	if len(sent) != notifyPerHour || sent[0] != "bell: Backup done" {
		t.Errorf("sent %d: %v", len(sent), sent[:1])
	}
}

func TestMetadataRatingsTable(t *testing.T) {
	b := newTestBroker(map[string][]string{"lb": {"sources.metadata"}})
	r := NewRegistry(b)
	stub(t, b, "lb", Metadata, func(j Job) Answer {
		raw, _ := json.Marshal(map[string]any{"scores": []metadata.ServerScore{
			{Source: "Letterboxd", Value: 4.1, Max: 5, Votes: 900},
			{Source: "IMDb", Value: 9.9, Max: 10},         // the server already rates IMDb
			{Source: "<script>", Value: 1, Max: 5},        // markup
			{Source: "Odd", Value: 7, Max: 5},             // over its max
			{Source: "Huge", Value: 1, Max: 1000},         // max out of range
			{Source: "Neg", Value: -1, Max: 5, Votes: -3}, // negative
		}})
		return Answer{Data: raw}
	})
	server := func(context.Context, string, string) (metadata.ServerRatings, error) {
		return metadata.ServerRatings{IMDbID: "tt1", Scores: []metadata.ServerScore{{Source: "IMDb", Value: 8.0, Max: 10}}}, nil
	}
	got, err := r.WithPluginRatings(server)(context.Background(), "movie", "438631")
	if err != nil || len(got.Scores) != 2 || got.Scores[0].Value != 8.0 || got.Scores[1].Source != "Letterboxd" {
		t.Fatalf("ratings %+v %v", got, err)
	}
	off := func(context.Context, string, string) (metadata.ServerRatings, error) {
		return metadata.ServerRatings{}, metadata.ErrSourceOff
	}
	if got, err := r.WithPluginRatings(off)(context.Background(), "movie", "1"); err != nil || len(got.Scores) != 2 {
		t.Errorf("server off, plugin on: %+v %v", got, err)
	}
	r.Drop("lb")
	start := time.Now()
	if _, err := r.WithPluginRatings(off)(context.Background(), "movie", "1"); err == nil || time.Since(start) > time.Second {
		t.Errorf("no plugin, server off: %v", err)
	}
}
