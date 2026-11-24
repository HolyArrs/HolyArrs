package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckOnSwitchOverHTTP: "Tell me about new versions" off through the
// mode route means no update request at all, no notice and no scheduled
// check; on again asks the feed.
func TestCheckOnSwitchOverHTTP(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = "v9.0.0"
	w.e.ReleaseFeed = f.srv.URL
	mux := http.NewServeMux()
	(&Handlers{Checker: w.e}).Routes(mux)
	put := func(body string) InstallView {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/app/update/mode", strings.NewReader(body)))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
		var v InstallView
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := put(`{"mode":"off"}`); !v.CheckOn {
		t.Fatal("check must be on by default")
	}
	if v := put(`{"check_on":false}`); v.CheckOn || v.Mode != ModeOff {
		t.Fatalf("off: %+v", v)
	}
	ctx := context.Background()
	if i := w.e.CheckUpdate(ctx); i.On || i.Newer || i.Latest != "" {
		t.Fatalf("notice while off: %+v", i)
	}
	if _, ok := w.run(t)["update"]; ok {
		t.Fatal("scheduled check ran while off")
	}
	if n := f.calls.Load(); n != 0 {
		t.Fatalf("%d update requests while off", n)
	}
	if v := put(`{"check_on":true}`); !v.CheckOn {
		t.Fatal("on again")
	}
	if i := w.e.CheckUpdate(ctx); !i.On || !i.Newer || f.calls.Load() != 1 {
		t.Fatalf("on again: %d %+v", f.calls.Load(), i)
	}
}
