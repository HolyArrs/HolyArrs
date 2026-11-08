package alerts

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func TestSMTPCallerContextInterruptsAfterDial(t *testing.T) {
	for _, tc := range []struct {
		name string
		want error
	}{
		{name: "short deadline", want: context.DeadlineExceeded},
		{name: "cancel after accept", want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) { testSMTPInterruption(t, tc.want) })
	}
}

func testSMTPInterruption(t *testing.T, want error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		close(accepted)
		defer conn.Close()
		_, _ = conn.Read(make([]byte, 1))
	}()

	addr := ln.Addr().(*net.TCPAddr)
	a := domain.Alert{ID: int64(900000 + addr.Port), Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: addr.Port, SMTPSecurity: "none",
		SMTPFrom: "holyarrs@example.test", StaffTo: "staff@example.test", AllowLocal: true}
	op, err := outbound.Begin(context.Background(), alertTarget(a))
	if err != nil {
		t.Fatal(err)
	}
	defer op.Finish(outbound.Canceled)
	var ctx context.Context
	var cancel context.CancelFunc
	if errors.Is(want, context.DeadlineExceeded) {
		ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- (&Sender{}).attempt(ctx, op, a, Message{Subject: "test", Text: "test"})
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("SMTP connection was not established")
	}
	start := time.Now()
	if errors.Is(want, context.Canceled) {
		cancel()
	}
	select {
	case err = <-result:
		if !errors.Is(err, want) {
			t.Fatalf("SMTP error = %v, want %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP context did not interrupt the connected session")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("SMTP interruption took %s", time.Since(start))
	}
}
