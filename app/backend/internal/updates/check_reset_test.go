package updates

import (
	"context"
	"testing"
)

func TestResetUpdateForgetsLastAnswer(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = `{"tag_name": "v9.0.0"}`
	w.e.ReleaseFeed = f.srv.URL
	ctx := context.Background()
	if i := w.e.CheckUpdate(ctx); i.Latest != "v9.0.0" {
		t.Fatalf("check %+v", i)
	}
	w.e.ResetUpdate(ctx)
	if i := w.e.Update(ctx); i.Latest != "" || i.Newer {
		t.Fatalf("after reset %+v", i)
	}
	if v := w.e.setting(ctx, settingUpdateState); v != "" {
		t.Fatalf("update_state still %q", v)
	}
	e2 := &Checker{Store: w.s, Version: "v1.1.0", ReleaseFeed: f.srv.URL, Now: w.e.Now}
	if i := e2.Update(ctx); i.Latest != "" {
		t.Fatalf("restart brought back %+v", i)
	}
}
