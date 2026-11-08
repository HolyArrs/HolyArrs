package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// signedIn is one person's browser: cookies and the CSRF token.
type signedIn struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
	id   int64
}

func (s *signedIn) call(method, path, body string, want int) []byte {
	s.t.Helper()
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accounts.CSRFHeader, s.csrf)
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		s.t.Fatalf("%s %s: %d, want %d: %s", method, path, resp.StatusCode, want, b)
	}
	return b
}

// get answers one GET for p, whatever its status.
func (s *signedIn) get(path string) (int, string) {
	s.t.Helper()
	req, _ := http.NewRequest("GET", s.base+path, nil)
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

type askRig struct {
	srv   *httptest.Server
	st    *database.Store
	asks  *requests.Service
	owner *signedIn
	ana   *signedIn
	bo    *signedIn
	kai   *signedIn // a Kid limited to PG (newKidRig only)
	api   *API
}

// newAskRig runs the music routes with sign-in: the owner luis and the
// members ana and bo (members may ask, nothing more).
func newAskRig(t *testing.T) *askRig { t.Helper(); return newSignInRig(t, false) }

// newKidRig is newAskRig plus the Kid kai, limited to PG.
func newKidRig(t *testing.T) *askRig { t.Helper(); return newSignInRig(t, true) }

func newSignInRig(t *testing.T, kid bool) *askRig {
	t.Helper()
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, insertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(st, accounts.Options{Vault: v})
	md := &metadata.Static{Movies: []metadata.Movie{metadata.Lab().Movies[0],
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002"}},
		{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003"}}}}
	q := jobs.New(st, jobs.Options{Workers: 1})
	a := New(Server{Store: st, Queue: q, Metadata: md, Version: "test", Auth: au, Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	r := &askRig{srv: srv, st: st, asks: a.s.Asks, api: a}

	browser := func() *signedIn {
		jar, _ := cookiejar.New(nil)
		return &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	r.owner = browser()
	r.owner.csrf, _ = read(r.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201))
	join := func(name string, invite ...string) *signedIn {
		body := `{}`
		if len(invite) > 0 {
			body = invite[0]
		}
		_, link := read(r.owner.call("POST", "/api/v1/users/invites", body, 201))
		b := browser()
		b.csrf, _ = read(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201))
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	r.ana, r.bo = join("ana"), join("bo")
	if kid {
		r.kai = join("kai", `{"max_rating":"PG"}`)
	}
	return r
}

func insertFolder(t *testing.T) func(tx *sql.Tx) error {
	dir := t.TempDir()
	return func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: dir, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	}
}

func askOf(t *testing.T, b []byte) requests.View {
	t.Helper()
	var x struct {
		Ask requests.View `json:"ask"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x.Ask
}

// newRig runs the music routes without sign-in over a seeded store; the
// count is kept for the old api shape.
func newRig(t *testing.T) (*API, *httptest.Server, *int) {
	t.Helper()
	_, s := discoverRig(t, metadata.Lab())
	a := New(Server{Store: s, Metadata: metadata.Lab(), Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return a, srv, new(int)
}
