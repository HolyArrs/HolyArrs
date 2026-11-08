package mediaservers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound/adaptertest"
)

func TestMediaServerAdapterConformance(t *testing.T) {
	adaptertest.Run(t, adaptertest.Contract{Name: "in-memory-fake/Test", Success: func(ctx context.Context) error { _, err := (memoryMediaServer{}).Test(ctx); return err },
		Failure: func(ctx context.Context) error {
			_, err := (memoryMediaServer{err: errors.New("synthetic media-server failure")}).Test(ctx)
			return err
		},
		Cancellation: true,
	})
	adaptertest.Run(t, adaptertest.Contract{Name: "in-memory-fake/Tell", Success: func(ctx context.Context) error { return (memoryMediaServer{}).Tell(ctx, nil) },
		Failure: func(ctx context.Context) error {
			return (memoryMediaServer{err: errors.New("synthetic media-server failure")}).Tell(ctx, nil)
		}, Cancellation: true})
	w := newWorld(t)
	for _, kind := range []string{"plex", "jellyfin", "emby"} {
		kind := kind
		successLinker, successServer := mediaServerAdapter(t, w, kind, http.StatusOK)
		failureLinker, failureServer := mediaServerAdapter(t, w, kind, http.StatusServiceUnavailable)
		adaptertest.Run(t, adaptertest.Contract{Name: kind + "/Test", Cancellation: true,
			Success: func(ctx context.Context) error { _, err := successLinker.Test(ctx, successServer); return err },
			Failure: func(ctx context.Context) error { _, err := failureLinker.Test(ctx, failureServer); return err }})
		adaptertest.Run(t, adaptertest.Contract{Name: kind + "/Tell", Cancellation: true,
			Success: func(ctx context.Context) error {
				return successLinker.Tell(ctx, successServer, []Update{{Path: "/media/Synthetic", Change: Created}})
			},
			Failure: func(ctx context.Context) error {
				return failureLinker.Tell(ctx, failureServer, []Update{{Path: "/media/Synthetic", Change: Created}})
			}})
	}
}

func mediaServerAdapter(t *testing.T, w *world, kind string, status int) (*Linker, domain.MediaServer) {
	t.Helper()
	srv := mediaServerFixture(t, kind, status)
	m := addConformanceMediaServer(t, context.Background(), w, kind+http.StatusText(status), kind, srv.URL)
	return &Linker{Store: w.s, Vault: w.v}, m
}

type memoryMediaServer struct{ err error }

func (m memoryMediaServer) Test(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "Synthetic 1", m.err
}
func (m memoryMediaServer) Tell(ctx context.Context, _ []Update) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.err
}

func addConformanceMediaServer(t *testing.T, ctx context.Context, w *world, name, kind, rawURL string) domain.MediaServer {
	t.Helper()
	m, err := Add(ctx, w.s, w.v, Input{Name: "Synthetic " + name, Kind: kind, URL: rawURL, Token: "x"}, domain.System)
	if err != nil {
		t.Fatalf("add %s: %v", kind, err)
	}
	return m
}

func mediaServerFixture(t *testing.T, kind string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "synthetic failure", status)
			return
		}
		if kind == "plex" && r.URL.Path == "/library/sections" {
			_, _ = w.Write([]byte(`<MediaContainer><Directory key="1" type="movie" title="Synthetic"><Location path="/media"/></Directory></MediaContainer>`))
			return
		}
		if r.URL.Path == "/System/Info" {
			_, _ = w.Write([]byte(`{"ServerName":"Synthetic","Version":"1"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv
}
