package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

func envSetupFile(t *testing.T, v string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "value")
	if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func envSetupKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "hak_" + base64.RawURLEncoding.EncodeToString(b)
}

func clearEnvSetupVars(t *testing.T) {
	t.Helper()
	for _, k := range []string{accounts.EnvOwnerName, accounts.EnvOwnerPassword, accounts.EnvAPIKey, accounts.EnvAPIKeyScopes} {
		t.Setenv(k, "")
	}
}

// From the first request the routes answer, the owner from the environment
// exists and its key works: the wizard never had a turn. The start is watched
// from its first listening moment, so setup moved after the routes shows up
// as an answer that still needs setup.
func TestEnvSetupIsDoneBeforeTheRoutes(t *testing.T) {
	clearEnvSetupVars(t)
	k := envSetupKey()
	t.Setenv(accounts.EnvOwnerName, "river")
	t.Setenv(accounts.EnvOwnerPassword, envSetupFile(t, "a long enough phrase\n"))
	t.Setenv(accounts.EnvAPIKey, envSetupFile(t, k+"\n"))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	get := func(path string, hdr map[string]string) (int, string) {
		req, _ := http.NewRequest("GET", "http://"+addr+path, nil)
		for h, v := range hdr {
			req.Header.Set(h, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, config{ConfigDir: t.TempDir(), Addr: addr}, runOpts{ready: func(*jobs.Registry, string) { close(ready) }})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	open := ""
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		for {
			select {
			case <-ready:
				return
			default:
			}
			if _, body := get("/api/v1/auth/state", nil); strings.Contains(body, `"needs_setup":true`) {
				open = body
				return
			}
		}
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("serve ended: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("the app did not start")
	}
	<-watched
	if open != "" {
		t.Fatalf("the wizard was open during the start: %s", open)
	}
	if code, body := get("/api/v1/auth/state", nil); code != 200 || !strings.Contains(body, `"needs_setup":false`) {
		t.Fatalf("state: %d %s", code, body)
	}
	if code, body := get("/api/v1/auth/keys", map[string]string{accounts.APIKeyHeader: k}); code != 200 || !strings.Contains(body, "HOLYARRS_API_KEY_FILE") {
		t.Fatalf("keys with the file's key: %d %s", code, body)
	}
}

// A setup a person has to fix holds the plain failure page instead of
// exiting into a restart loop: it says why, names no file path and no value,
// and serves none of the real routes.
func TestEnvSetupRefusalHoldsThePage(t *testing.T) {
	for _, c := range []struct {
		name, want string
		set        func(t *testing.T) string
	}{
		{"key with no owner", "finish setup first", func(t *testing.T) string {
			t.Setenv(accounts.EnvAPIKey, envSetupFile(t, envSetupKey()))
			return ""
		}},
		{"missing password file", "cannot read the file named there", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "owner-password")
			t.Setenv(accounts.EnvOwnerName, "river")
			t.Setenv(accounts.EnvOwnerPassword, p)
			return p
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			clearEnvSetupVars(t)
			path := c.set(t)
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := l.Addr().String()
			_ = l.Close()
			ready := make(chan struct{}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- serve(ctx, config{ConfigDir: t.TempDir(), Addr: addr}, runOpts{ready: func(*jobs.Registry, string) { ready <- struct{}{} }})
			}()
			get := func(method, path string) (int, string) {
				req, _ := http.NewRequest(method, "http://"+addr+path, strings.NewReader(`{"name":"late","password":"a long enough phrase"}`))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return 0, err.Error()
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(resp.Body)
				return resp.StatusCode, string(b)
			}
			deadline := time.Now().Add(60 * time.Second)
			for {
				if _, body := get("GET", "/healthz"); strings.Contains(body, `"state":"failed"`) {
					break
				}
				select {
				case err := <-done:
					t.Fatalf("serve exited instead of holding the page: %v", err)
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("the failure page never showed")
				}
				time.Sleep(20 * time.Millisecond)
			}
			code, page := get("GET", "/")
			if code != http.StatusServiceUnavailable || !strings.Contains(page, "could not be set up") || !strings.Contains(page, c.want) ||
				(path != "" && strings.Contains(page, path)) {
				t.Fatalf("page: %d %s", code, page)
			}
			for _, r := range []struct{ method, path string }{{"GET", "/api/v1/auth/state"}, {"POST", "/api/v1/auth/setup"}} {
				if code, body := get(r.method, r.path); code != http.StatusServiceUnavailable || strings.Contains(body, "needs_setup") || body != page {
					t.Fatalf("%s %s was served: %d %s", r.method, r.path, code, body)
				}
			}
			cancel()
			if err := <-done; err == nil {
				t.Fatal("serve returned no error after holding the page")
			}
			select {
			case <-ready:
				t.Fatal("the routes were served")
			default:
			}
		})
	}
}
