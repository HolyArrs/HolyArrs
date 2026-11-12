package arrapi

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// A hook may not point back at HolyArrs: not at the port it listens on
// (on loopback or any of its own addresses), and not at the address the
// caller reached it on. Every call carries the self-call marker.
func TestHooks_RefuseSelf(t *testing.T) {
	rc := newReceiver(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(rc.srv.URL, "http://"))
	t.Cleanup(func() { SetListenAddr("") })

	sec := hookSecret{URL: rc.srv.URL + "/hook"}
	if err := callHook(context.Background(), http.MethodPost, sec, testEvent()); err != nil {
		t.Fatalf("a neighbour on another port was refused: %v", err)
	}
	if got := rc.got(); len(got) != 1 || got[0].header.Get(outbound.SelfCallHeader) == "" {
		t.Fatalf("the call did not carry the self-call marker: %+v", got)
	}

	hookHTTP.CloseIdleConnections() // the port is set at start, before any call
	SetListenAddr(":" + port)
	for _, u := range []string{rc.srv.URL, "http://localhost:" + port} {
		err := callHook(context.Background(), http.MethodPost, hookSecret{URL: u}, testEvent())
		if err == nil || !strings.Contains(err.Error(), "HolyArrs itself") {
			t.Fatalf("%s: a call to HolyArrs's own port went out: %v", u, err)
		}
	}
	if len(rc.got()) != 1 {
		t.Fatal("a refused call reached the server")
	}
	SetListenAddr("")

	// The address the caller reached HolyArrs on is refused when saved.
	h := newHookRig(t)
	st, body := h.do("sonarr", "POST", "notification", pulsarrHook(0, "self", "http://example.com/sonarr/api/v3/system/status", "s"))
	failedOn(t, st, body, "Url")
}

func TestHooks_MarkerHeaderNotSettable(t *testing.T) {
	if validHeader(hookHeader{Key: outbound.SelfCallHeader, Value: "x"}) {
		t.Fatal("a record may set the self-call marker")
	}
}
