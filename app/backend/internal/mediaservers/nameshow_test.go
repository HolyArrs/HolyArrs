package mediaservers

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestMediaServerTestShowsWhatTheNameTurnedInto(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	w := newWorld(t)
	cases := []struct {
		url, want string
		cause     doctor.Cause
	}{
		// The trailing dot makes the name absolute: a DNS search domain with a
		// wildcard record would otherwise turn the name into one that resolves.
		{"http://holyarrs-nameshow-test.home.arpa.:8096", "holyarrs-nameshow-test.home.arpa. → not found. HolyArrs can't find this name. Use the IP instead.", doctor.DNS},
		{"http://localhost:" + port, "→ refused on port " + port + ". The name works, but nothing answers on that port. Check the port.", doctor.Refused},
	}
	for i, tc := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		local := true
		m, err := Add(ctx, w.s, w.v, Input{Name: "Name " + strconv.Itoa(i), Kind: "jellyfin", URL: tc.url, Token: "x", AllowLocal: &local}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		_, err = (&Linker{Store: w.s, Vault: w.v}).Test(ctx, m)
		cancel()
		if err == nil || !strings.Contains(err.Error(), tc.want) || doctor.Of(err) != tc.cause {
			t.Fatalf("%s: %v (cause %q)", tc.url, err, doctor.Of(err))
		}
	}
}
