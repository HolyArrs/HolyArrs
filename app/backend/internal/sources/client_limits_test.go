package sources

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestChange_KeyAgainWhenProxyOfPlainHTTPSourceChanges checks that a plain http Source needs its key typed again when the proxy changes, and an https Source does not.
func TestChange_KeyAgainWhenProxyOfPlainHTTPSourceChanges(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	by := domain.System
	src, err := Add(ctx, s, v, Input{Name: "Plain", Format: "torznab", URL: "http://plain.example/api", APIKey: "k-" + "plain",
		Proxy: ptr("http://me:pp@proxy-a.example:3128")}, by)
	if err != nil {
		t.Fatal(err)
	}
	// The key would cross a new proxy in the clear.
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "Plain", Proxy: ptr("http://me:pp@proxy-b.example:3128")}, by); !errors.Is(err, ErrKeyAgainProxy) {
		t.Fatalf("new proxy for an http Source: %v", err)
	}
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "Plain", APIKey: "k-" + "plain", Proxy: ptr("http://me:pp@proxy-b.example:3128")}, by); err != nil {
		t.Fatalf("with the key again: %v", err)
	}
	// Another proxy name on the same host: the password must be typed again.
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "Plain", Proxy: ptr("http://other@proxy-b.example:3128")}, by); !errors.Is(err, ErrProxyAgain) {
		t.Fatalf("new proxy name: %v", err)
	}
	// An https Source keeps its key across proxies, because the proxy only sees a tunnel.
	sec, err := Add(ctx, s, v, Input{Name: "Secure", Format: "torznab", URL: "https://secure.example/api", APIKey: "k-" + "secure",
		Proxy: ptr("http://me:pp@proxy-a.example:3128")}, by)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Change(ctx, s, v, sec.ID, Input{Name: "Secure", Proxy: ptr("http://me:pp@proxy-b.example:3128")}, by); err != nil {
		t.Fatalf("https Source, new proxy: %v", err)
	}
}

// TestRetryAfter_IsCapped checks that a very long Retry-After wait is cut to 24 hours.
func TestRetryAfter_IsCapped(t *testing.T) {
	for _, h := range []string{"99999999999999", "86401"} {
		srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", h)
			w.WriteHeader(http.StatusTooManyRequests)
		})
		_, err := New(Config{URL: srv.URL + "/api", AllowLocal: true}).Caps(context.Background())
		var se *Error
		if !errors.As(err, &se) || se.Kind != KindRateLimited || se.RetryAfter != 24*time.Hour {
			t.Fatalf("%s: %v %v", h, err, se)
		}
	}
}

// TestSOCKS5_ChecksReplyVersions checks that a SOCKS5 reply with a wrong version is refused at each step.
func TestSOCKS5_ChecksReplyVersions(t *testing.T) {
	ok := []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for name, addr := range map[string]string{
		"auth version":  socksReplying(t, []byte{5, 2}, []byte{9, 0}, ok),
		"reply version": socksReplying(t, []byte{5, 0}, nil, []byte{4, 0, 0, 1, 0, 0, 0, 0, 0, 0}),
	} {
		user := ""
		if name == "auth version" {
			user = "u"
		}
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		err = socks5Connect(context.Background(), conn, "127.0.0.1:80", user, "p")
		conn.Close()
		if err == nil {
			t.Fatalf("%s: a bad answer was taken", name)
		}
	}
	conn, err := net.Dial("tcp", socksReplying(t, []byte{5, 0}, nil, ok))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := socks5Connect(context.Background(), conn, ":80", "", ""); err == nil {
		t.Fatal("an empty host was sent")
	}
}

// TestScrub_HidesEveryEncoding checks that a key is hidden in every form it may appear in.
func TestScrub_HidesEveryEncoding(t *testing.T) {
	pass := "p@ss/w:rd é"
	pu, _ := url.Parse("http://proxy.example:3128")
	pu.User = url.UserPassword("me", pass)
	c := New(Config{URL: "http://x/api", APIKey: "a/b c+d", Proxy: pu})
	for _, form := range []string{pass, url.QueryEscape(pass), url.PathEscape(pass), pu.User.String(), "a/b c+d", url.PathEscape("a/b c+d")} {
		if got := c.scrub("before " + form + " after"); strings.Contains(got, form) {
			t.Fatalf("%q left %q", form, got)
		}
	}
}

// TestSummaryAndRecord_AreBounded checks that the call summary and the call record stay within their limits.
func TestSummaryAndRecord_AreBounded(t *testing.T) {
	ctx := context.Background()
	f := seed(t, "http://x/api")
	old := maxCallsPerSource
	maxCallsPerSource = 5
	t.Cleanup(func() { maxCallsPerSource = old })
	now := time.Now().UTC()
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO source_calls(source_id, at, op, ok, ms, error_kind) VALUES (?, ?, 'search', 1, 5, '')",
			f.srcID, database.FormatTime(now.Add(-100*24*time.Hour)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// A success older than the record keeps is not the last success.
	sum, err := Summarise(ctx, f.s.Read(), now)
	if err != nil {
		t.Fatal(err)
	}
	if s := sum[f.srcID]; s.LastSuccess != nil {
		t.Fatalf("old success counted: %+v", s)
	}
	for i := 0; i < 8; i++ {
		Record(ctx, f.s, f.srcID, "search", time.Millisecond, nil)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM source_calls WHERE source_id = ?", f.srcID).Scan(&n)
	if n != 5 {
		t.Fatalf("kept %d calls, want 5", n)
	}
}

// socksReplying starts a fake proxy that answers the greeting with greet, the sign-in with auth and the connect request with reply.
// It returns the address to dial.
func socksReplying(t *testing.T, greet, auth, reply []byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.ReadFull(c, make([]byte, 3))
				_, _ = c.Write(greet)
				if auth != nil {
					b := make([]byte, 2)
					_, _ = io.ReadFull(c, b)
					_, _ = io.ReadFull(c, make([]byte, b[1]))
					_, _ = io.ReadFull(c, b[:1])
					_, _ = io.ReadFull(c, make([]byte, b[0]))
					_, _ = c.Write(auth)
				}
				_, _ = io.ReadFull(c, make([]byte, 10))
				_, _ = c.Write(reply)
				_, _ = io.Copy(io.Discard, c)
			}(c)
		}
	}()
	return ln.Addr().String()
}
