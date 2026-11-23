package requests

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A request a plugin makes for a person keeps the plugin on the record and
// in its view; one the person makes has none.
func TestAskKeepsViaPlugin(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct {
		name, tmdb, plugin string
	}{
		{"by the person", "900002", ""},
		{"through a bot", "900003", "discord-bot"},
	} {
		ctx := context.Background()
		if c.plugin != "" {
			ctx = database.WithViaPlugin(ctx, c.plugin)
		}
		res, err := r.svc.AskQuality(ctx, r.ana, c.tmdb, QualityStandard)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if res.Ask.ViaPlugin != nil {
			got = *res.Ask.ViaPlugin
		}
		if got != c.plugin {
			t.Errorf("%s: record via %q", c.name, got)
		}
		vs, err := r.svc.Views(context.Background(), "a.id = ?", res.Ask.ID)
		if err != nil || len(vs) != 1 || vs[0].ViaPlugin != c.plugin {
			t.Errorf("%s: view %+v %v", c.name, vs, err)
		}
	}
}
