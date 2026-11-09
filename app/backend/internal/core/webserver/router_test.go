package webserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter_RecordsPermissionAndGates(t *testing.T) {
	r := NewRouter()
	r.Get("/a/{id}", PermManageLibrary, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(req.PathValue("id")))
	}, Part("downloads"), Flag("x"))
	r.Post("/b", NeedOwner, func(http.ResponseWriter, *http.Request) {})

	got, ok := r.Route("GET /a/{id}")
	if !ok || got.Perm != PermManageLibrary || got.Part != "downloads" || got.Flag != "x" {
		t.Fatalf("route = %+v, %v", got, ok)
	}
	if _, ok := r.Route("GET /nope"); ok {
		t.Fatal("unknown pattern found")
	}
	if n := len(r.Routes()); n != 2 {
		t.Fatalf("routes = %d, want 2", n)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/a/7", nil))
	if rec.Body.String() != "7" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestCaller_Has(t *testing.T) {
	member := Caller{Role: RoleMember, Perms: []Perm{PermAsk}}
	admin := Caller{Role: RoleAdmin}
	owner := Caller{Role: RoleOwner}
	cases := []struct {
		c    Caller
		p    Perm
		want bool
	}{
		{owner, NeedOwner, true}, {admin, NeedOwner, false}, {admin, NeedUsers, true},
		{member, NeedUsers, false}, {member, NeedSignedIn, true}, {member, NeedSelf, true},
		{member, PermAsk, true}, {member, PermSettings, false}, {admin, PermApprove, false},
	}
	for _, c := range cases {
		if got := c.c.Has(c.p); got != c.want {
			t.Errorf("%s Has(%s) = %v, want %v", c.c.Role, c.p, got, c.want)
		}
	}
	if len(DefaultPerms(RoleOwner)) != len(AllPerms) || len(DefaultPerms(RoleMember)) != 1 {
		t.Fatal("default perms wrong")
	}
}

func TestLimitBodies_RefusesBigAndSkipsExempt(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	h := LimitBodies(10, func(p string) bool { return p == "/api/big" }, ok)
	big := strings.Repeat("x", 50)
	for path, want := range map[string]int{"/api/x": 413, "/api/big": 204, "/page": 204} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(big)))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestWriteJSON_Headers(t *testing.T) {
	rec := httptest.NewRecorder()
	FailNoStore(rec, 400, "bad")
	if rec.Code != 400 || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"error":"bad"`) {
		t.Fatalf("got %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	WriteJSON(rec, 200, map[string]int{"a": 1})
	if rec.Header().Get("Cache-Control") != "" {
		t.Fatal("plain WriteJSON must not set Cache-Control")
	}
}
