package alerts

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A plugin's message goes only to Alerts with plugin messages ticked, named
// as coming from the plugin.
func TestSendPluginOnlyWhereTicked(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	on, gotOn := fakeHook(t)
	off, gotOff := fakeHook(t)
	if _, err := Add(ctx, s, v, Input{Name: "On", Kind: "webhook", URL: on.URL + "/in", AllowLocal: &yes, Moments: []string{MomentPlugin}}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(ctx, s, v, Input{Name: "Off", Kind: "webhook", URL: off.URL + "/in", AllowLocal: &yes, Moments: []string{MomentImport}}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendPlugin(ctx, "backup-bot", "Backup done", "All 3 folders"); err != nil {
		t.Fatal(err)
	}
	if len(gotOff()) != 0 {
		t.Error("an Alert without plugin messages got one")
	}
	g := gotOn()
	if len(g) != 1 {
		t.Fatalf("posts %d", len(g))
	}
	var p Payload
	_ = json.Unmarshal(g[0].body, &p)
	if p.Moment != MomentPlugin || p.Subject != "From backup-bot: Backup done" || p.Text != "All 3 folders" {
		t.Errorf("payload %s", g[0].body)
	}
}
