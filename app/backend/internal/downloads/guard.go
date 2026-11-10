package downloads

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

var errDownloaderUnavailable = errors.New("the Downloader did not answer")

var _ Syncer = (*guardedEngine)(nil)

type adapterHTTPError struct{ status int }

// guardedEngine wraps an adapter and records each outcome after the adapter's own
// handshake and reconciliation. It never retries a write and never counts a
// session challenge as a failure.
type guardedEngine struct {
	Engine
	target outbound.Target
	begin  func(context.Context, outbound.Target) (*outbound.Operation, error)
}

func (e adapterHTTPError) Error() string {
	return fmt.Sprintf("the Downloader answered HTTP %d", e.status)
}

func (g *guardedEngine) Health(ctx context.Context) (h Health, err error) {
	err = g.run(ctx, func() error { var e error; h, e = g.Engine.Health(ctx); return e })
	return
}

func (g *guardedEngine) Add(ctx context.Context, r AddRequest) (id string, err error) {
	if !addAllowed(g.Engine) {
		return "", ErrReadOnly
	}
	err = g.run(ctx, func() error { var e error; id, e = g.Engine.Add(ctx, r); return e })
	return
}

func (g *guardedEngine) List(ctx context.Context, category string) (jobs []Job, err error) {
	err = g.run(ctx, func() error { var e error; jobs, e = g.Engine.List(ctx, category); return e })
	return
}

func (g *guardedEngine) Get(ctx context.Context, id string) (job Job, err error) {
	err = g.run(ctx, func() error { var e error; job, e = g.Engine.Get(ctx, id); return e })
	return
}

func (g *guardedEngine) Remove(ctx context.Context, id string, files bool) error {
	if isReadOnly(g.Engine) {
		return ErrReadOnly
	}
	return g.run(ctx, func() error { return g.Engine.Remove(ctx, id, files) })
}

// SyncMain, History, Queue and ListAll are methods of the guard itself, so the
// background reader cannot reach an optional part around outbound.Begin, as
// unwrapping through ControlEngine would allow.
func (g *guardedEngine) SyncMain(ctx context.Context, rid int64) (d MainData, err error) {
	s, ok := g.Engine.(MainSyncer)
	if !ok {
		return d, ErrNotSupported
	}
	err = g.run(ctx, func() error { var e error; d, e = s.SyncMain(ctx, rid); return e })
	return
}

func (g *guardedEngine) History(ctx context.Context, since int64) (p HistoryPage, err error) {
	s, ok := g.Engine.(HistoryReader)
	if !ok {
		return p, ErrNotSupported
	}
	err = g.run(ctx, func() error { var e error; p, e = s.History(ctx, since); return e })
	return
}

func (g *guardedEngine) Queue(ctx context.Context) (p QueuePage, err error) {
	s, ok := g.Engine.(QueueReader)
	if !ok {
		return p, ErrNotSupported
	}
	err = g.run(ctx, func() error { var e error; p, e = s.Queue(ctx); return e })
	return
}

func (g *guardedEngine) ListAll(ctx context.Context) (jobs []Job, err error) {
	s, ok := g.Engine.(AllLister)
	if !ok {
		return nil, ErrNotSupported
	}
	err = g.run(ctx, func() error { var e error; jobs, e = s.ListAll(ctx); return e })
	return
}

func protectEngine(e Engine, d domain.Downloader, instance string) Engine {
	secret := ""
	if d.SecretRef != nil {
		secret = strconv.FormatInt(*d.SecretRef, 10)
	}
	return &guardedEngine{Engine: e, target: outbound.Target{Kind: "downloader", Identity: []string{instance, strconv.FormatInt(d.ID, 10), d.Adapter, d.URL, d.Username, secret, d.Category, strings.Join(otherCategories(d), "/"), strconv.FormatBool(d.AllowLocal)}}, begin: outbound.Begin}
}

func (g *guardedEngine) run(ctx context.Context, call func() error) error {
	op, err := g.begin(ctx, g.target)
	if err != nil {
		return err
	}
	err = call()
	outcome := outbound.Success
	if ctx.Err() != nil {
		outcome = outbound.Canceled
	} else if err != nil {
		var network net.Error
		var status adapterHTTPError
		switch {
		case errors.Is(err, ErrReadOnly):
			// Refused before reaching the client, so it says nothing about the client.
			outcome = outbound.Canceled
		case errors.Is(err, ErrSyncParse):
			// A garbled change feed is a passing failure, not a refusal.
			outcome = outbound.TransientFailure
		case errors.Is(err, outbound.ErrAddressRefused):
			outcome = outbound.TerminalFailure
		case errors.As(err, &status):
			outcome = outbound.HTTPOutcome(status.status, nil, ctx)
		case errors.Is(err, errDownloaderUnavailable) || errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded):
			outcome = outbound.TransientFailure
		default:
			outcome = outbound.TerminalFailure
		}
	}
	op.Finish(outcome)
	return err
}
