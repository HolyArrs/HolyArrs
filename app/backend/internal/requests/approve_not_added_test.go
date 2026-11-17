package requests

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// lookups answers movie lookups through movie, everything else from the rig's list.
type lookups struct {
	metadata.Provider
	movie func(ctx context.Context, id string) (metadata.Movie, error)
}

func (l lookups) Movie(ctx context.Context, id string) (metadata.Movie, error) {
	return l.movie(ctx, id)
}

// liveLines starts a live hub for people who approve and collects what it sends.
func liveLines(t *testing.T) (*webserver.LiveHub, func() string) {
	t.Helper()
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	srv := httptest.NewServer(hub.Handler(func(*http.Request) webserver.LiveViewer {
		return webserver.LiveViewer{UserID: 1, Approve: true}
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 64)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if l := sc.Text(); strings.HasPrefix(l, "data: ") {
				lines <- l
			}
		}
	}()
	for hub.Clients() != 1 {
		time.Sleep(5 * time.Millisecond)
	}
	return hub, func() string {
		var all []string
		deadline := time.After(400 * time.Millisecond)
		for {
			select {
			case l := <-lines:
				all = append(all, l)
			case <-deadline:
				return strings.Join(all, "\n")
			}
		}
	}
}

// An approval whose title cannot be added in time goes back to Waiting and
// says so: one History line with the reason, a live change for the
// Requests page, and an error the approver reads.
func TestApproveNotAddedSaysSo(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	r.svc.Metadata = lookups{Provider: r.svc.Metadata, movie: func(context.Context, string) (metadata.Movie, error) {
		return metadata.Movie{}, fmt.Errorf("TMDB: %w (%w)", metadata.ErrServiceSlow, context.DeadlineExceeded)
	}}
	hub, live := liveLines(t)
	r.svc.Live = hub
	_, err = r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if !errors.Is(err, ErrNotAddedSlow) {
		t.Fatalf("approve error = %v", err)
	}
	got, _ := get(ctx, r.st.Read(), res.Ask.ID)
	if got.State != domain.AskAsked || got.DecidedBy != nil || got.TitleID != nil {
		t.Fatalf("ask after a failed add: %+v", got)
	}
	n := 0
	for _, e := range r.events(t) {
		if e.Kind == domain.EventAskNotAdded {
			n++
			if e.Data["reason"] != "the metadata server did not answer in time" || e.Data["approved_by"] != "luis" || !strings.Contains(e.Summary, "Dune: Part Two") {
				t.Errorf("event: %+v", e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d not-added events, want 1", n)
	}
	if hasEvent(r.events(t), domain.EventAskApproved) != nil {
		t.Error("an approved event was written for an ask that is waiting again")
	}
	if l := live(); !strings.Contains(l, `"state":"asked"`) {
		t.Fatalf("the Requests page was not told: %s", l)
	}
}

// The add goes on when the approver's request ends halfway (a closed tab,
// another page): the ask stays approved and the title is in the Library.
func TestApproveAddOutlivesTheRequest(t *testing.T) {
	r := newRig(t)
	res, err := r.svc.Ask(context.Background(), r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	reqCtx, leave := context.WithCancel(context.Background())
	static := r.svc.Metadata
	r.svc.Metadata = lookups{Provider: static, movie: func(ctx context.Context, id string) (metadata.Movie, error) {
		leave() // the browser goes away while the lookup runs
		time.Sleep(20 * time.Millisecond)
		if err := ctx.Err(); err != nil {
			return metadata.Movie{}, err
		}
		return static.Movie(ctx, id)
	}}
	if _, err := r.svc.Approve(reqCtx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	got, _ := get(context.Background(), r.st.Read(), res.Ask.ID)
	if got.State != domain.AskApproved || got.TitleID == nil {
		t.Fatalf("ask after the request ended: %+v", got)
	}
	if tid, err := library.TitleByTMDB(context.Background(), r.st.Read(), "693134"); err != nil || tid != *got.TitleID {
		t.Fatalf("title %d %v, ask points at %d", tid, err, *got.TitleID)
	}
}

// A metadata server that cannot be reached is said in plain words in
// History; the raw error with its address stays in the log.
func TestApproveNotAddedUnreachableSaysItPlainly(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	r.svc.Metadata = lookups{Provider: r.svc.Metadata, movie: func(context.Context, string) (metadata.Movie, error) {
		return metadata.Movie{}, fmt.Errorf("TMDB: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("address refused by the network policy")})
	}}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err == nil {
		t.Fatal("approved without its title")
	}
	e := hasEvent(r.events(t), domain.EventAskNotAdded)
	if e == nil || e.Data["reason"] != "the metadata server could not be reached" || e.Data["down"] != true || e.Data["slow"] != false {
		t.Fatalf("event: %+v", e)
	}
}
