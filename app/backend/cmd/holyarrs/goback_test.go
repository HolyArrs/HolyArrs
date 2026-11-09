package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// mainWith is this build's main migrations plus extra ones (name: SQL).
func mainWith(t *testing.T, extra map[string]string) fstest.MapFS {
	t.Helper()
	out := mainPlus(t, "")
	delete(out, "9999_future.sql")
	for n, q := range extra {
		out[n] = &fstest.MapFile{Data: []byte(q)}
	}
	return out
}

// captureLog sends the log to a buffer for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

var codeRx = regexp.MustCompile(`one-time code: ([A-Z0-9]{5}-[A-Z0-9]{5})`)

func postCode(t *testing.T, base, code string) (int, string) {
	t.Helper()
	resp, err := http.PostForm(base+"/", url.Values{"code": {code}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// After going back to an older build, its refusal page offers to put back
// the data from before the update. A wrong code changes nothing; the code
// from the log puts the backup in place, keeps the newer data aside and
// asks the app to start again.
func TestGoBackPagePutsBackPreUpdateData(t *testing.T) {
	dir, err := oldThenNew(t, "CREATE TABLE future(x);")
	if err != nil {
		t.Fatal(err)
	}
	before := 0
	_, err = database.Open(dir, database.Options{})
	var nd *database.NewerDataError
	if !errors.As(err, &nd) {
		t.Fatalf("got %v", err)
	}
	before = nd.Know
	logs := captureLog(t)
	done := make(chan struct{})
	srv := httptest.NewServer(refusedHandler(dir, err, done))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/library")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 503 || !strings.Contains(string(b), "Put back the data from before the update") || !strings.Contains(string(b), "<form") ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("page %d %q", resp.StatusCode, b)
	}
	if resp, err := http.Get(srv.URL + "/healthz"); err != nil || resp.StatusCode != 503 {
		t.Fatalf("healthz %v %v", resp, err)
	}
	m := codeRx.FindStringSubmatch(logs.String())
	if m == nil {
		t.Fatalf("no code in the log: %q", logs.String())
	}
	if strings.Contains(string(b), m[1]) {
		t.Fatal("the page shows the code")
	}

	if code, page := postCode(t, srv.URL, "AAAAA-AAAAA"); code != 403 || !strings.Contains(page, "does not match") {
		t.Fatalf("wrong code: %d %q", code, page)
	}
	if _, err := database.Open(dir, database.Options{}); !errors.As(err, &nd) {
		t.Fatalf("a wrong code changed the data: %v", err)
	}
	// Typed loosely (lower case, no dash) it still matches.
	if code, page := postCode(t, srv.URL, strings.ToLower(strings.ReplaceAll(m[1], "-", " "))); code != 200 || !strings.Contains(page, "before-restore-") {
		t.Fatalf("right code: %d %q", code, page)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the app was not asked to start again")
	}
	if v, err := openVersion(t, dir); err != nil || v != before {
		t.Fatalf("after: version %d %v, want %d", v, err, before)
	}
	kept, _ := filepath.Glob(filepath.Join(dir, "backups", "before-restore-*", database.MainFile))
	if len(kept) != 1 {
		t.Fatalf("newer data not kept aside: %v", kept)
	}
	// A second press does nothing more.
	if code, page := postCode(t, srv.URL, m[1]); code != 200 || !strings.Contains(page, "already") {
		t.Fatalf("again: %d %q", code, page)
	}
}

// Wrong codes are counted; after maxCodeTries the form refuses even the
// right code until the app starts again.
func TestGoBackPageLimitsTries(t *testing.T) {
	dir, err := oldThenNew(t, "CREATE TABLE future(x);")
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Open(dir, database.Options{})
	logs := captureLog(t)
	srv := httptest.NewServer(refusedHandler(dir, err, make(chan struct{})))
	defer srv.Close()
	m := codeRx.FindStringSubmatch(logs.String())
	if m == nil {
		t.Fatal("no code")
	}
	for i := 0; i < maxCodeTries; i++ {
		if code, _ := postCode(t, srv.URL, "nope"); code != 403 {
			t.Fatalf("try %d: %d", i, code)
		}
	}
	if code, _ := postCode(t, srv.URL, m[1]); code != 429 {
		t.Fatalf("after %d wrong tries: %d", maxCodeTries, code)
	}
	if _, err := database.Open(dir, database.Options{}); err == nil {
		t.Fatal("the data was changed")
	}
}

// Posts that carry no code (a HolyArrs tab still open, Seerr, a script)
// are not tries, so they cannot use up the tries before a person types one.
func TestGoBackPageIgnoresPostsWithoutACode(t *testing.T) {
	dir, err := oldThenNew(t, "CREATE TABLE future(x);")
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Open(dir, database.Options{})
	logs := captureLog(t)
	srv := httptest.NewServer(refusedHandler(dir, err, make(chan struct{})))
	defer srv.Close()
	m := codeRx.FindStringSubmatch(logs.String())
	if m == nil {
		t.Fatal("no code")
	}
	for i := 0; i < maxCodeTries+2; i++ {
		resp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json", strings.NewReader(`{"name":"a","password":"b"}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("post without a code %d: %d", i, resp.StatusCode)
		}
	}
	if strings.Contains(logs.String(), "wrong code") {
		t.Fatalf("a post without a code was counted: %q", logs.String())
	}
	if code, page := postCode(t, srv.URL, m[1]); code != 200 || !strings.Contains(page, "before-restore-") {
		t.Fatalf("right code after posts without one: %d %q", code, page)
	}
}

// No pre-update backup, or another reason: the plain page, no form.
func TestGoBackPageOnlyWithABackup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{&database.NewerDataError{Have: 2, Know: 1}, database.ErrNoSpace} {
		srv := httptest.NewServer(refusedHandler(dir, err, make(chan struct{})))
		resp, gerr := http.Get(srv.URL + "/")
		if gerr != nil {
			t.Fatal(gerr)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), "<form") || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
			t.Fatalf("%v: %q", err, b)
		}
		if code, _ := postCode(t, srv.URL, "x"); code == 200 {
			t.Fatal("post accepted without a form")
		}
		srv.Close()
	}
}

// Going back two updates (1.2 to 1.0): the newest pre-update backup holds
// 1.1's data, which 1.0 cannot read, so the one taken before the update
// from 1.0 is put back.
func TestGoBackTwoUpdatesTakesTheFittingBackup(t *testing.T) {
	dir := t.TempDir()
	// start opens dir as build v with extra migrations and runs what a good
	// start runs (adopting the update's copy, AfterStart).
	start := func(v string, extra map[string]string) int {
		t.Helper()
		s, err := database.Open(dir, database.Options{MainMigrations: mainWith(t, extra)})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		bm := &backups.Manager{Store: s, Dir: dir, Version: v}
		if _, err := bm.AdoptMigrationCopies(); err != nil {
			t.Fatal(err)
		}
		if err := bm.AfterStart(t.Context()); err != nil {
			t.Fatal(err)
		}
		n, _ := s.Version()
		return n
	}
	want := start("1.0.0", nil)
	start("1.1.0", map[string]string{"9998_one.sql": "CREATE TABLE one(x);"})
	start("1.2.0", map[string]string{"9998_one.sql": "CREATE TABLE one(x);", "9999_two.sql": "CREATE TABLE two(x);"})
	bm := &backups.Manager{Dir: dir}
	all, _ := bm.List()
	var got []string
	for _, b := range all {
		if b.Kind == backups.KindPreUpdate {
			got = append(got, b.FromVersion+">"+b.ToVersion)
		}
	}
	if strings.Join(got, " ") != "1.1.0>1.2.0 1.0.0>1.1.0" {
		t.Fatalf("pre-update backups %q", got)
	}
	if code, out := restoreRun(t, "-config", dir, "--pre-update"); code != 0 {
		t.Fatalf("restore %d %q", code, out)
	}
	if v, err := openVersion(t, dir); err != nil || v != want {
		t.Fatalf("after: %d %v, want %d", v, err, want)
	}
}

// An update with two steps where the second fails: the failed step is
// undone, the copy from before the whole update is kept, and the older
// build (which now sees the first step) offers to put that copy back.
func TestFailedSecondStepThenGoBack(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := s.Version()
	_ = s.Close()
	_, err = database.Open(dir, database.Options{MainMigrations: mainWith(t, map[string]string{
		"9998_one.sql": "CREATE TABLE one(x);", "9999_two.sql": "CREATE TABLE two(x); INSERT INTO nope VALUES (1);"})})
	if !errors.Is(err, database.ErrMigration) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(publicReason(err), "change the image tag back") {
		t.Fatalf("reason %q", publicReason(err))
	}
	_, err = database.Open(dir, database.Options{})
	var nd *database.NewerDataError
	if !errors.As(err, &nd) || nd.Have != 9998 {
		t.Fatalf("older build: %v", err)
	}
	logs := captureLog(t)
	done := make(chan struct{})
	srv := httptest.NewServer(refusedHandler(dir, err, done))
	defer srv.Close()
	m := codeRx.FindStringSubmatch(logs.String())
	if m == nil {
		t.Fatalf("no offer: %q", logs.String())
	}
	if code, page := postCode(t, srv.URL, m[1]); code != 200 {
		t.Fatalf("%d %q", code, page)
	}
	if v, err := openVersion(t, dir); err != nil || v != want {
		t.Fatalf("after: %d %v, want %d", v, err, want)
	}
}
