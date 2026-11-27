package hooks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSearchSubtitlesTable: the plugin gets the ask, and offers of the
// wrong shape are left out or trimmed.
func TestSearchSubtitlesTable(t *testing.T) {
	many := "[" + strings.TrimSuffix(strings.Repeat(`{"key":"k","language":"en"},`, 30), ",") + "]"
	for _, c := range []struct {
		name   string
		answer string
		ok     bool
		check  func([]SubtitleOffer) bool
	}{
		{"offers", `[{"key":"a2","language":"en","format":"SRT"},{"key":"a3","language":"de","forced":true}]`, true,
			func(o []SubtitleOffer) bool {
				return len(o) == 2 && o[0].Key == "a2" && o[0].Format == "srt" && o[1].Forced
			}},
		{"none", `[]`, true, func(o []SubtitleOffer) bool { return len(o) == 0 }},
		{"null", `null`, true, func(o []SubtitleOffer) bool { return o != nil && len(o) == 0 }},
		{"no key or language", `[{"language":"en"},{"key":"x"},{"key":"ok","language":"en"}]`, true,
			func(o []SubtitleOffer) bool { return len(o) == 1 && o[0].Key == "ok" }},
		{"control characters", `[{"key":"a\nb","language":"en"},{"key":"c","language":"e\u0000n"}]`, true,
			func(o []SubtitleOffer) bool { return len(o) == 0 }},
		{"key too long", `[{"key":"` + strings.Repeat("k", 201) + `","language":"en"}]`, true,
			func(o []SubtitleOffer) bool { return len(o) == 0 }},
		{"odd format", `[{"key":"k","language":"en","format":"` + strings.Repeat("x", 9) + `"}]`, true,
			func(o []SubtitleOffer) bool { return len(o) == 1 && o[0].Format == "" }},
		{"too many", many, true, func(o []SubtitleOffer) bool { return len(o) == subtitleOffers }},
		{"not a list", `{"key":"k"}`, false, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := newTestBroker(map[string][]string{"ws": {"sources.subtitles"}})
			var got SubtitleAsk
			stub(t, b, "ws", Subtitles, func(j Job) Answer {
				if j.Op != "search" {
					t.Errorf("op %q", j.Op)
				}
				_ = json.Unmarshal(j.Payload, &got)
				return Answer{Data: json.RawMessage(c.answer)}
			})
			ask := SubtitleAsk{Kind: "movie", Title: "Arrival", Year: 2016, Languages: []string{"en"},
				SubtitleFile: SubtitleFile{FileID: 7, Folder: "movies", Path: "Arrival (2016)/Arrival.mkv"}}
			o, err := SearchSubtitles(context.Background(), b, "ws", ask)
			if (err == nil) != c.ok {
				t.Fatalf("err %v", err)
			}
			if got.FileID != 7 || got.Folder != "movies" || got.Path != "Arrival (2016)/Arrival.mkv" || got.Title != "Arrival" {
				t.Errorf("plugin got %+v", got)
			}
			if c.ok && !c.check(o) {
				t.Errorf("offers %+v", o)
			}
		})
	}
}

// TestMakeSubtitleTable: the made file comes back, and an empty or too
// large one is refused.
func TestMakeSubtitleTable(t *testing.T) {
	srt := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")
	for _, c := range []struct {
		name   string
		answer Answer
		ok     bool
	}{
		{"made", Answer{Data: mustJSON(t, SubtitleMade{Format: "SRT", Body: srt})}, true},
		{"empty", Answer{Data: mustJSON(t, SubtitleMade{Format: "srt"})}, false},
		{"too large", Answer{Data: mustJSON(t, SubtitleMade{Body: make([]byte, subtitleMax+1)})}, false},
		{"not json", Answer{Data: json.RawMessage(`"nope"`)}, false},
		{"plugin error", Answer{Error: "the Whisper server did not answer"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := newTestBroker(map[string][]string{"ws": {"sources.subtitles"}})
			var got SubtitleMake
			stub(t, b, "ws", Subtitles, func(j Job) Answer {
				if j.Op != "make" {
					t.Errorf("op %q", j.Op)
				}
				_ = json.Unmarshal(j.Payload, &got)
				return c.answer
			})
			m, err := MakeSubtitle(context.Background(), b, "ws", SubtitleMake{Key: "a2", Language: "en", SubtitleFile: SubtitleFile{FileID: 7}})
			if (err == nil) != c.ok {
				t.Fatalf("err %v", err)
			}
			if got.Key != "a2" || got.FileID != 7 {
				t.Errorf("plugin got %+v", got)
			}
			if c.ok && (m.Format != "srt" || string(m.Body) != string(srt)) {
				t.Errorf("made %+v", m)
			}
		})
	}
}

// TestSubtitles_ScopeAndWait: only a plugin with sources.subtitles is asked,
// and a make may take an hour, far past the other hooks' waits.
func TestSubtitles_ScopeAndWait(t *testing.T) {
	b := newTestBroker(map[string][]string{"ws": {"sources.releases"}})
	if _, err := SearchSubtitles(context.Background(), b, "ws", SubtitleAsk{}); err != ErrNoProvider {
		t.Fatalf("search: %v", err)
	}
	if _, err := MakeSubtitle(context.Background(), b, "ws", SubtitleMake{}); err != ErrNoProvider {
		t.Fatalf("make: %v", err)
	}
	if Scopes[Subtitles] != "sources.subtitles" {
		t.Fatalf("scope %q", Scopes[Subtitles])
	}
	if SubtitleMakeWait < time.Hour || subtitleSearchWait > time.Minute {
		t.Fatalf("waits %v %v", SubtitleMakeWait, subtitleSearchWait)
	}
}

// TestMakeSubtitle_DropEndsTheWait: a plugin stopped mid-make ends the wait
// at once with ErrDropped, so the job can try again once it is back.
func TestMakeSubtitle_DropEndsTheWait(t *testing.T) {
	b := newTestBroker(map[string][]string{"ws": {"sources.subtitles"}})
	started, hold := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(hold) })
	stub(t, b, "ws", Subtitles, func(j Job) Answer {
		close(started)
		<-hold // answers only after the test
		return Answer{}
	})
	go func() { <-started; b.Drop("ws") }()
	if _, err := MakeSubtitle(context.Background(), b, "ws", SubtitleMake{Key: "k", Language: "en"}); err != ErrDropped {
		t.Fatalf("got %v", err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
