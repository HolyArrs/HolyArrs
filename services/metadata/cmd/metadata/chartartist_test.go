package main

import (
	"context"
	"testing"
)

// The chart's own artist id resolves only exactly: a MusicBrainz link,
// else an exact stored match, else a barcode; two answers at one step, a
// bad id or nothing leave it unresolved.
func TestChartArtistResolver(t *testing.T) {
	const (
		a = "a1000000-0000-4000-8000-000000000001"
		b = "b1000000-0000-4000-8000-000000000002"
	)
	links := map[string][]string{"applemusic:111": {a}, "applemusic:222": {a, b}}
	holders := map[string][]string{"spotify:0OjRa6r1EIPF5RZPy1SGz9": {b}}
	barcodes := map[string]string{"885288695823": a, "885288695824": b}
	res := &chartArtists{
		linked:  func(_ context.Context, source, id string) []string { return links[source+":"+id] },
		holders: func(source, id string) []string { return holders[source+":"+id] },
		barcode: func(_ context.Context, code string) string { return barcodes[code] },
		names: func(_ context.Context, mbid string) []string {
			return map[string][]string{a: {"Баста", "Basta"}, b: {"Omar"}}[mbid]
		},
	}
	ctx := context.Background()
	cases := []struct {
		source, id string
		codes      []string
		want       string
		name       string
	}{
		{"applemusic", "111", nil, a, ""},                                        // MusicBrainz link
		{"applemusic", "222", nil, "", ""},                                       // two linked: no guess
		{"spotify", "0OjRa6r1EIPF5RZPy1SGz9", nil, b, ""},                        // stored exact match
		{"applemusic", "", []string{"885288695823"}, a, "Basta"},                 // barcode
		{"applemusic", "", []string{"885288695823"}, "", "Various Artists"},      // barcode artist named otherwise
		{"applemusic", "", []string{"885288695823", "885288695824"}, a, "Basta"}, // other barcode's artist named otherwise: skipped
		{"applemusic", "../1", []string{"12"}, "", ""},                           // bad id, bad barcode
		{"spotify", "short", nil, "", ""},
	}
	for _, c := range cases {
		if got := res.resolve(ctx, c.source, c.id, c.name, c.codes); got != c.want {
			t.Errorf("%s %q %v: %q, want %q", c.source, c.id, c.codes, got, c.want)
		}
	}
}

// sharedName: two or more slim artists carrying the name; asked once per
// name.
func TestSharedNameCached(t *testing.T) {
	calls, ver := 0, "1"
	f := sharedName(func(_ context.Context, name string) int {
		calls++
		if name == "Basta" {
			return 8
		}
		return 1
	}, func() string { return ver })
	ctx := context.Background()
	if !f(ctx, "Basta") || f(ctx, "Unique") || !f(ctx, "Basta") || calls != 2 {
		t.Fatalf("calls %d", calls)
	}
	// A new copy asks again; with no copy nothing is kept.
	ver = "2"
	f(ctx, "Basta")
	ver = ""
	f(ctx, "Basta")
	f(ctx, "Basta")
	if calls != 5 {
		t.Fatalf("calls %d after a swap and with no copy", calls)
	}
}

// Two MusicBrainz entries linking one id (a duplicate, "BTS" and
// "Bangtan Sonyeondan"): the one our exact match holds, else the only one
// with release groups; otherwise still none.
func TestChartArtistLinkedTwice(t *testing.T) {
	const (
		a = "a1000000-0000-4000-8000-000000000001"
		b = "b1000000-0000-4000-8000-000000000002"
		c = "c1000000-0000-4000-8000-000000000003"
	)
	groups := map[string]int{a: 40, b: 0, c: 5}
	res := &chartArtists{
		linked: func(_ context.Context, _, id string) []string {
			switch id {
			case "1":
				return []string{a, b}
			case "2":
				return []string{a, c}
			}
			return []string{b, c}
		},
		holders: func(_, id string) []string {
			if id == "3" {
				return []string{c}
			}
			return nil
		},
		groups:  func(_ context.Context, id string) int { return groups[id] },
		barcode: func(context.Context, string) string { return "" },
	}
	ctx := context.Background()
	for id, want := range map[string]string{"1": a, "2": "", "3": c} {
		if got := res.resolve(ctx, "applemusic", id, "", nil); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
}
