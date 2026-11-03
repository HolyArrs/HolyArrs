package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicpkg"
)

const pkgArtist = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
const pkgOther = "20244d07-534f-4eff-b4d4-930878889970"

func pkgStore(t *testing.T) *musicpkg.Store {
	st, err := musicpkg.Open(filepath.Join(t.TempDir(), "p.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for i, id := range []string{pkgArtist, pkgOther} {
		p := musicpkg.Package{SchemaVersion: 1, MBID: id, Hash: "h" + itoa(i), BuiltAt: time.Now(), Artist: json.RawMessage(`{"name":"x"}`)}
		if _, err := st.Put(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestPackageRoutesArePublicAndKept(t *testing.T) {
	st := pkgStore(t)
	s, _, calls := setup(Options{Burst: 100, Packages: st})
	w := req(s, "/v1/music/packages/"+pkgArtist, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mbid":"`+pkgArtist+`"`) {
		t.Fatalf("anonymous package: %d %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "public") || w.Header().Get("Vary") != "" {
		t.Fatalf("package cache-control %q vary %q", cc, w.Header().Get("Vary"))
	}
	if w := req(s, "/v1/music/packages/"+strings.Repeat("0", 8)+"-0000-0000-0000-000000000000", ""); w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("uncovered: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := req(s, "/v1/music/packages/"+pkgArtist+"?queue=1", ""); w.Code != 400 {
		t.Fatalf("query on one package: %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("package routes must never fetch")
	}
}

func TestPackageSyncPages(t *testing.T) {
	st := pkgStore(t)
	s, _, _ := setup(Options{Burst: 100, Packages: st})
	type page struct {
		Since, Next, Latest int64
		More                bool
		Packages            []struct {
			MBID    string `json:"mbid"`
			Version int64  `json:"version"`
		}
	}
	read := func(path string) page {
		w := req(s, path, "")
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var p page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := read("/v1/music/packages?limit=1")
	if len(p.Packages) != 1 || !p.More || p.Next != 1 || p.Latest != 2 || p.Packages[0].MBID != pkgArtist {
		t.Fatalf("page 1 %+v", p)
	}
	p = read("/v1/music/packages?since=1&limit=1")
	if len(p.Packages) != 1 || p.More || p.Next != 2 || p.Packages[0].MBID != pkgOther {
		t.Fatalf("page 2 %+v", p)
	}
	p = read("/v1/music/packages?since=2")
	if len(p.Packages) != 0 || p.More || p.Next != 2 {
		t.Fatalf("caught up %+v", p)
	}
	idx := req(s, "/v1/music/packages?view=index", "")
	if idx.Code != 200 || !strings.Contains(idx.Body.String(), `"hash":"h1"`) || strings.Contains(idx.Body.String(), `"artist"`) {
		t.Fatalf("index: %d %s", idx.Code, idx.Body.String())
	}
	for _, bad := range []string{"?since=-1", "?since=x", "?limit=0", "?limit=201", "?view=all", "?other=1", "?since=1&since=2"} {
		if w := req(s, "/v1/music/packages"+bad, ""); w.Code != 400 {
			t.Fatalf("%s: %d", bad, w.Code)
		}
	}
}

func TestPackageWantNeedsCredentialAndKeepsOnlyID(t *testing.T) {
	st := pkgStore(t)
	var asked []string
	s, _, _ := setup(Options{Burst: 100, Packages: st, WantPackage: func(_ context.Context, mbid string) error {
		asked = append(asked, mbid)
		return nil
	}})
	id := "9fff2f8a-21e6-47de-a2b8-7f449929d43f"
	if w := req(s, "/v1/music/package-want/"+id, ""); w.Code != 401 {
		t.Fatalf("anonymous want: %d", w.Code)
	}
	w := req(s, "/v1/music/package-want/"+strings.ToUpper(id), "synthetic-test-token")
	if w.Code != 202 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"state":"queued"`) {
		t.Fatalf("want: %d %q %s", w.Code, w.Header().Get("Cache-Control"), w.Body.String())
	}
	if len(asked) != 1 || asked[0] != id {
		t.Fatalf("asked %v", asked)
	}
	if w := req(s, "/v1/music/package-want/"+pkgArtist, "synthetic-test-token"); w.Code != 200 || !strings.Contains(w.Body.String(), `"covered"`) {
		t.Fatalf("covered want: %d %s", w.Code, w.Body.String())
	}
	if len(asked) != 1 {
		t.Fatal("a covered artist is not queued again")
	}
	if w := req(s, "/v1/music/package-want/not-an-id", "synthetic-test-token"); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestPackageWantQuotaPerCredential(t *testing.T) {
	st := pkgStore(t)
	oldB := wantBurst
	wantBurst = 2
	defer func() { wantBurst = oldB }()
	n := 0
	s, _, _ := setup(Options{Burst: 100, Packages: st, WantPackage: func(context.Context, string) error { n++; return nil }})
	ids := []string{"9fff2f8a-21e6-47de-a2b8-7f449929d43f", "9fff2f8a-21e6-47de-a2b8-7f449929d430", "9fff2f8a-21e6-47de-a2b8-7f449929d431"}
	codes := []int{}
	for _, id := range ids {
		codes = append(codes, req(s, "/v1/music/package-want/"+id, "synthetic-test-token").Code)
	}
	if codes[0] != 202 || codes[1] != 202 || codes[2] != 429 || n != 2 {
		t.Fatalf("codes %v asked %d", codes, n)
	}
}

// Security L3: many credentials from one address group share one want
// bucket, so extra credentials do not multiply the asks.
func TestPackageWantQuotaPerAddressGroup(t *testing.T) {
	st := pkgStore(t)
	oldB := wantBurst
	wantBurst = 2
	defer func() { wantBurst = oldB }()
	n := 0
	s := New(twoTokens{&memoryStore{entries: map[string]model.Entry{}, authorized: true}}, fetchFunc(func(context.Context, model.Query) (model.Result, error) { return model.Result{}, nil }),
		Options{Burst: 100, Packages: st, WantPackage: func(context.Context, string) error { n++; return nil }})
	ids := []string{"9fff2f8a-21e6-47de-a2b8-7f449929d43f", "9fff2f8a-21e6-47de-a2b8-7f449929d430", "9fff2f8a-21e6-47de-a2b8-7f449929d431"}
	codes := []int{
		req(s, "/v1/music/package-want/"+ids[0], "synthetic-test-token").Code,
		req(s, "/v1/music/package-want/"+ids[1], "synthetic-test-token").Code,
		req(s, "/v1/music/package-want/"+ids[2], "another-synthetic-token").Code,
	}
	if codes[0] != 202 || codes[1] != 202 || codes[2] != 429 || n != 2 {
		t.Fatalf("codes %v asked %d", codes, n)
	}
}

// twoTokens authorizes two synthetic credentials.
type twoTokens struct{ *memoryStore }

func (twoTokens) Authorized(_ context.Context, t string) (bool, error) {
	return t == "synthetic-test-token" || t == "another-synthetic-token", nil
}

// Security 38: a cursor between stored versions is sent to the stored one,
// so random cursors share one edge answer instead of each reading a page.
func TestPackageSyncCursorCanonical(t *testing.T) {
	st := pkgStore(t)
	s, _, _ := setup(Options{Burst: 100, Packages: st})
	w := req(s, "/v1/music/packages?since=999999&limit=200", "")
	if w.Code != 307 || w.Header().Get("Location") != "/v1/music/packages?limit=200&since=2" {
		t.Fatalf("random cursor %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := req(s, "/v1/music/packages?since=2&limit=200", ""); w.Code != 200 {
		t.Fatalf("stored cursor %d", w.Code)
	}
}
