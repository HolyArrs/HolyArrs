package downloader

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/outside"
	"github.com/CoderLuii/holyarrs/internal/source"
)

var errDownloaderUnavailable = errors.New("the Downloader did not answer")

type adapterHTTPError struct{ status int }

func (e adapterHTTPError) Error() string {
	return fmt.Sprintf("the Downloader answered HTTP %d", e.status)
}

// guardedEngine records the result after the adapter's existing handshake and
// reconciliation. It never retries a mutation or counts a session challenge.
type guardedEngine struct {
	Engine
	target outside.Target
	begin  func(context.Context, outside.Target) (*outside.Operation, error)
}

func protectEngine(e Engine, d domain.Downloader, instance string) Engine {
	secret := ""
	if d.SecretRef != nil {
		secret = strconv.FormatInt(*d.SecretRef, 10)
	}
	return &guardedEngine{Engine: e, target: outside.Target{Kind: "downloader", Identity: []string{instance, strconv.FormatInt(d.ID, 10), d.Adapter, d.URL, d.Username, secret, d.Category, strconv.FormatBool(d.AllowLocal)}}, begin: outside.Begin}
}

func (g *guardedEngine) run(ctx context.Context, call func() error) error {
	op, err := g.begin(ctx, g.target)
	if err != nil {
		return err
	}
	err = call()
	outcome := outside.Success
	if ctx.Err() != nil {
		outcome = outside.Canceled
	} else if err != nil {
		var network net.Error
		var status adapterHTTPError
		switch {
		case errors.Is(err, ErrReadOnly):
			// Refused here, before the client: proves nothing about it.
			outcome = outside.Canceled
		case errors.Is(err, ErrSyncParse):
			// A garbled change feed is a passing failure, never a refusal.
			outcome = outside.TransientFailure
		case errors.Is(err, source.ErrAddressRefused):
			outcome = outside.TerminalFailure
		case errors.As(err, &status):
			outcome = outside.HTTPOutcome(status.status, nil, ctx)
		case errors.Is(err, errDownloaderUnavailable) || errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded):
			outcome = outside.TransientFailure
		default:
			outcome = outside.TerminalFailure
		}
	}
	op.Finish(outcome)
	return err
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

// SyncMain, History, Queue and ListAll are the guarded Syncer. They are methods of
// the guard itself so the background reader never reaches an optional part
// around outside.Begin (as ControlEngine's unwrapping would allow).
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

var _ Syncer = (*guardedEngine)(nil)
