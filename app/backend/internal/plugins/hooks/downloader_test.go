package hooks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// fakeClient is a stub download-client plugin: it keeps jobs in memory.
func fakeClient(t *testing.T, b *Broker, got *[]AddJob) {
	jobs := map[string]downloads.Job{}
	stub(t, b, "dlplug", Downloader, func(j Job) Answer {
		var out any
		switch j.Op {
		case "health":
			out = map[string]string{"version": "stub 1"}
		case "add":
			var a AddJob
			_ = json.Unmarshal(j.Payload, &a)
			*got = append(*got, a)
			jobs["j1"] = downloads.Job{ID: "j1", Name: a.Name, State: downloads.StateDownloading, Progress: 0.5}
			out = map[string]string{"id": "j1"}
		case "list":
			var l []downloads.Job
			for _, v := range jobs {
				l = append(l, v)
			}
			out = l
		case "get":
			var g struct{ ID string }
			_ = json.Unmarshal(j.Payload, &g)
			v, ok := jobs[g.ID]
			if !ok {
				return Answer{Error: "not found"}
			}
			out = v
		case "remove", "pause", "resume":
			out = map[string]bool{"ok": true}
		}
		b, _ := json.Marshal(out)
		return Answer{Data: b}
	})
}

func TestPluginDownloaderTable(t *testing.T) {
	b := newTestBroker(map[string][]string{"dlplug": {"sources.downloader"}})
	var got []AddJob
	fakeClient(t, b, &got)
	e := &PluginEngine{Broker: b, Plugin: "dlplug", Wait: time.Second}
	ctx := context.Background()
	if h, err := e.Health(ctx); err != nil || h.Version != "stub 1" {
		t.Fatalf("health %v %v", h, err)
	}
	for _, c := range []struct {
		name string
		req  downloads.AddRequest
		ok   bool
	}{
		{"torrent file", downloads.AddRequest{Name: "Dune", File: []byte("d8:announce..."), URL: "https://tracker.example/dl?passkey=SECRET"}, true},
		{"magnet", downloads.AddRequest{Name: "Dune", URL: "magnet:?xt=urn:btih:abc"}, true},
		{"raw indexer link only", downloads.AddRequest{Name: "Dune", URL: "https://indexer.example/api?apikey=SECRET&t=get"}, false},
	} {
		n := len(got)
		_, err := e.Add(ctx, c.req)
		if (err == nil) != c.ok {
			t.Errorf("%s: err %v", c.name, err)
			continue
		}
		if c.ok {
			a := got[n]
			if strings.Contains(a.Magnet+string(a.File), "SECRET") || (a.File == nil && a.Magnet == "") {
				t.Errorf("%s: plugin got %+v", c.name, a)
			}
		}
	}
	if l, err := e.List(ctx, ""); err != nil || len(l) != 1 || l[0].Name != "Dune" {
		t.Fatalf("list %v %v", l, err)
	}
	if j, err := e.Get(ctx, "j1"); err != nil || j.Progress != 0.5 {
		t.Fatalf("get %v %v", j, err)
	}
	if _, err := e.Get(ctx, "nope"); err == nil {
		t.Error("missing job found")
	}
	for _, f := range []func() error{
		func() error { return e.Remove(ctx, "j1", false) },
		func() error { return e.PauseItem(ctx, "j1") },
		func() error { return e.ResumeItem(ctx, "j1") },
	} {
		if err := f(); err != nil {
			t.Error(err)
		}
	}
	b.Drop("dlplug")
	if _, err := e.Health(ctx); err == nil {
		t.Error("a dropped plugin still answers as a download client")
	}
}
