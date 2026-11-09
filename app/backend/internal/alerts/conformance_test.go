package alerts

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound/adaptertest"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func TestAlertAdapterConformance(t *testing.T) {
	fake := memoryAlert{}
	adaptertest.Run(t, adaptertest.Contract{
		Name: "in-memory-fake", Success: func(ctx context.Context) error { return fake.Send(ctx, Message{}) },
		Failure: func(ctx context.Context) error {
			return (memoryAlert{err: errors.New("synthetic alert failure")}).Send(ctx, Message{})
		},
		Cancellation: true,
	})
	s, v := open(t)
	for _, kind := range []string{"discord", "webhook", "email"} {
		kind := kind
		success := alertOperation(t, s, v, kind, false)
		failure := alertOperation(t, s, v, kind, true)
		adaptertest.Run(t, adaptertest.Contract{
			Name: kind, Cancellation: true,
			Success: success,
			Failure: failure,
		})
	}
}

type memoryAlert struct{ err error }

func (m memoryAlert) Send(ctx context.Context, _ Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.err
}

func alertOperation(t *testing.T, s *database.Store, v *secrets.Vault, kind string, fail bool) func(context.Context) error {
	t.Helper()
	name := "success"
	if fail {
		name = "failure"
	}
	in := Input{Name: "Synthetic " + kind + " " + name, Kind: kind, AllowLocal: &yes}
	if kind == "email" {
		port := conformanceSMTP(t, fail)
		in.SMTPHost, in.SMTPPort, in.SMTPSecurity = "127.0.0.1", port, "none"
		in.SMTPFrom, in.StaffTo = "holyarrs@example.test", "staff@example.test"
	} else {
		status := http.StatusNoContent
		if fail {
			status = http.StatusServiceUnavailable
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(status)
		}))
		t.Cleanup(srv.Close)
		in.URL = srv.URL + "/synthetic"
		if kind == "webhook" {
			in.WebhookAuth = "none"
		}
	}
	a, err := Add(context.Background(), s, v, in, domain.System)
	if err != nil {
		t.Fatalf("add %s: %v", kind, err)
	}
	sender := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	message := Message{Moment: MomentWaiting, Subject: "Synthetic subject", Text: "Synthetic body"}
	return func(ctx context.Context) error { return sender.TestOne(ctx, a, message) }
}

func conformanceSMTP(t *testing.T, fail bool) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveConformanceSMTP(conn, fail)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func serveConformanceSMTP(conn net.Conn, fail bool) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	_, _ = io.WriteString(conn, "220 synthetic ready\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			_, _ = io.WriteString(conn, "250 synthetic\r\n")
		case strings.HasPrefix(cmd, "MAIL FROM:") && fail:
			_, _ = io.WriteString(conn, "550 refused\r\n")
		case cmd == "DATA":
			_, _ = io.WriteString(conn, "354 continue\r\n")
			for {
				line, err = r.ReadString('\n')
				if err != nil || line == ".\r\n" {
					break
				}
			}
			_, _ = io.WriteString(conn, "250 queued\r\n")
		case cmd == "QUIT":
			_, _ = io.WriteString(conn, "221 bye\r\n")
			return
		default:
			_, _ = io.WriteString(conn, "250 ok\r\n")
		}
	}
}
