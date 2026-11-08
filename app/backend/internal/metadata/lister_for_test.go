package metadata

import (
	"context"
	"testing"
)

func TestListerForUnwrapsSwitch(t *testing.T) {
	ctx := context.Background()
	tm := &TMDB{}
	pick := func(use bool) *Switch {
		return &Switch{Use: func(context.Context) bool { return use }, Then: tm, Else: Lab()}
	}
	cases := []struct {
		name string
		f    func(p Provider) (any, bool)
	}{
		{"Discoverer", func(p Provider) (any, bool) { return DiscovererFor(ctx, p) }},
		{"SeasonLister", func(p Provider) (any, bool) { return SeasonListerFor(ctx, p) }},
		{"ReviewLister", func(p Provider) (any, bool) { return ReviewListerFor(ctx, p) }},
		{"MadeByLister", func(p Provider) (any, bool) { return MadeByListerFor(ctx, p) }},
		{"ServiceLister", func(p Provider) (any, bool) { return ServiceListerFor(ctx, p) }},
		{"WatchLister", func(p Provider) (any, bool) { return WatchListerFor(ctx, p) }},
	}
	for _, c := range cases {
		if got, ok := c.f(pick(true)); !ok || got != any(tm) {
			t.Errorf("%s: TMDB pick gave %v %v", c.name, got, ok)
		}
		if _, ok := c.f(pick(false)); ok {
			t.Errorf("%s: lab pick answered ok", c.name)
		}
		if got, ok := c.f(tm); !ok || got != any(tm) {
			t.Errorf("%s: bare TMDB gave %v %v", c.name, got, ok)
		}
	}
}
