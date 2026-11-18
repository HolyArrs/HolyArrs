package mediaservers

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestPlexLabelsLive labels The Matrix (tmdb 603) on a real local Plex at
// HOLYARRS_PLEXLABEL_URL and checks the labels already on it stay.
func TestPlexLabelsLive(t *testing.T) {
	base := os.Getenv("HOLYARRS_PLEXLABEL_URL")
	if base == "" {
		t.Skip("set HOLYARRS_PLEXLABEL_URL to a local test Plex")
	}
	w := newWorld(t)
	ctx := context.Background()
	movie, _ := watchTitles(t, w)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: base, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "holyarrs", Askers: true})
	l := &Linker{Store: w.s, Vault: w.v, Asked: func(context.Context, string) ([]AskedTitle, error) {
		return []AskedTitle{{TitleID: movie, Askers: []string{"ana"}}}, nil
	}}
	n, err := l.LabelPlex(ctx, true)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	ms, _ := enabledServers(ctx, w.s.Read())
	got, err := l.plexLabels(ctx, ms[0], "1")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("labels on Plex now: %v", got)
	for _, want := range []string{"favs", "holyarrs", "ana"} {
		if !slices.ContainsFunc(got, func(s string) bool { return strings.EqualFold(s, want) }) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}
