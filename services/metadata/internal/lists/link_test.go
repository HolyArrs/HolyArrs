package lists

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMBLinkReadsTheLinkedArtist(t *testing.T) {
	const id = "8e66ea2b-b57b-47d9-8df0-df4630aeb8e5"
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("resource")
		switch {
		case strings.HasSuffix(asked, "/two"):
			_, _ = w.Write([]byte(`{"relations":[{"target-type":"artist","artist":{"id":"` + id + `"}},{"target-type":"artist","artist":{"id":"11111111-b57b-47d9-8df0-df4630aeb8e5"}}]}`))
		case strings.HasSuffix(asked, "/none"):
			w.WriteHeader(404)
		default:
			_, _ = w.Write([]byte(`{"relations":[{"target-type":"artist","artist":{"id":"` + id + `"}}]}`))
		}
	}))
	defer srv.Close()
	link := MBLink(srv.Client(), srv.URL, "test")
	ctx := context.Background()
	if got, err := link(ctx, "artist", "abc"); err != nil || got != id || asked != linkPage+"abc" {
		t.Fatalf("got %q %v asked %q", got, err, asked)
	}
	for _, ext := range []string{"none", "two"} {
		if _, err := link(ctx, "artist", ext); err == nil {
			t.Fatalf("%s: want no link", ext)
		}
	}
	if _, err := link(ctx, "album", "abc"); err == nil {
		t.Fatal("albums go by name")
	}
	if _, err := link(ctx, "artist", "a/../b"); err == nil {
		t.Fatal("bad id passed")
	}
}
