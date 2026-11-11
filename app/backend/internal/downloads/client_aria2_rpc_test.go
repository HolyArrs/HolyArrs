package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// a2Canned answers every call with the same status and body and keeps the last request body.
func a2Canned(t *testing.T, status int, body string, got *[]byte) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if got != nil {
			*got = b
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func a2FaultBody(msg string) string {
	b, _ := json.Marshal(map[string]any{"id": "holyarrs", "jsonrpc": "2.0", "error": map[string]any{"code": 1, "message": msg}})
	return string(b)
}

func TestAria2RequestShape(t *testing.T) {
	var got []byte
	base := a2Canned(t, 200, `{"id":"holyarrs","jsonrpc":"2.0","result":"OK"}`, &got)
	for _, c := range []struct {
		secret string
		want   []any
	}{
		{"s3cret", []any{"token:s3cret", "0123456789abcdef"}},
		{"", []any{"0123456789abcdef"}},
	} {
		var out string
		if err := NewAria2(base, c.secret, nil).call(context.Background(), "aria2.tellStatus", &out, "0123456789abcdef"); err != nil || out != "OK" {
			t.Fatalf("call: %q %v", out, err)
		}
		var req struct {
			JSONRPC, ID, Method string
			Params              []any
		}
		if err := json.Unmarshal(got, &req); err != nil {
			t.Fatal(err)
		}
		if req.JSONRPC != "2.0" || req.ID != "holyarrs" || req.Method != "aria2.tellStatus" || len(req.Params) != len(c.want) {
			t.Fatalf("request %s", got)
		}
		for i := range c.want {
			if req.Params[i] != c.want[i] {
				t.Errorf("param %d: %v, want %v", i, req.Params[i], c.want[i])
			}
		}
	}
}

func TestAria2Numbers(t *testing.T) {
	for in, want := range map[string]int64{"12": 12, "0": 0, "": 0, "x": 0, " 7 ": 7, "-3": -3} {
		if got := a2Num(in); got != want {
			t.Errorf("a2Num(%q) = %d, want %d", in, got, want)
		}
	}
	var s a2Status
	if err := json.Unmarshal([]byte(`{"gid":"a","totalLength":"100","completedLength":"oops","seeder":"true"}`), &s); err != nil {
		t.Fatal(err)
	}
	if a2Num(s.TotalLength) != 100 || a2Num(s.CompletedLength) != 0 || s.Seeder != "true" {
		t.Errorf("decoded %+v", s)
	}
}

func TestAria2Faults(t *testing.T) {
	for _, c := range []struct {
		msg      string
		is       error
		contains string
	}{
		{"Unauthorized", ErrAuth, ""},
		{"GID 0123456789abcdef is not found", ErrNotFound, ""},
		{"Invalid GID zz", ErrNotFound, ""},
		{"No such method: aria2.nosuch", nil, "/jsonrpc"},
		{"We encountered a problem at http://t.example/a?passkey=secret   now", nil, "We encountered a problem at http://t.example"},
		{"Active Download not found for GID#0123456789abcdef", nil, "Active Download not found"},
	} {
		err := NewAria2(a2Canned(t, 400, a2FaultBody(c.msg), nil), "", nil).call(context.Background(), "aria2.tellStatus", nil, "x")
		if err == nil {
			t.Errorf("%q: no error", c.msg)
			continue
		}
		if c.is != nil && !errors.Is(err, c.is) {
			t.Errorf("%q: %v, want %v", c.msg, err, c.is)
		}
		if c.is == nil && (errors.Is(err, ErrNotFound) || errors.Is(err, ErrAuth)) {
			t.Errorf("%q: %v should be a plain fault", c.msg, err)
		}
		if !strings.Contains(err.Error(), c.contains) || strings.Contains(err.Error(), "secret") {
			t.Errorf("%q: %v", c.msg, err)
		}
	}
}

func TestAria2NotJSONRPCNamed(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
	}{
		{200, `<?xml version="1.0"?><methodResponse><params/></methodResponse>`},
		{200, "<html>aria2 web ui</html>"},
		{404, "not found"},
		{405, ""},
		{400, "bad request"},
	} {
		err := NewAria2(a2Canned(t, c.status, c.body, nil), "", nil).call(context.Background(), "aria2.getVersion", nil)
		if err == nil || !strings.Contains(err.Error(), "not aria2's JSON-RPC") || !strings.Contains(err.Error(), "/jsonrpc") {
			t.Errorf("%d %q: %v", c.status, c.body, err)
		}
	}
}

func TestAria2AnswerCap(t *testing.T) {
	old := aria2AnswerMax
	aria2AnswerMax = 64
	t.Cleanup(func() { aria2AnswerMax = old })
	big := `{"id":"holyarrs","jsonrpc":"2.0","result":"` + strings.Repeat("x", 200) + `"}`
	err := NewAria2(a2Canned(t, 200, big, nil), "", nil).call(context.Background(), "aria2.getVersion", nil)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("big answer: %v", err)
	}
}

// a2Closer reads the request and drops the connection without an answer, as
// aria2 does with a body over its --rpc-max-request-size.
func a2Closer(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func TestAria2SizeRefusal(t *testing.T) {
	a := NewAria2(a2Closer(t), "", nil)
	err := a.call(context.Background(), "aria2.addTorrent", nil, strings.Repeat("A", 3<<20))
	var big aria2TooLarge
	if !errors.As(err, &big) || !errors.Is(err, ErrTooLarge) || !strings.Contains(err.Error(), "rpc-max-request-size") {
		t.Errorf("big body: %v", err)
	}
	err = a.call(context.Background(), "aria2.getVersion", nil)
	if err == nil || errors.As(err, &big) || errors.Is(err, ErrTooLarge) {
		t.Errorf("small body cut: %v", err)
	}
}

func TestAria2MulticallPerEntry(t *testing.T) {
	var got []byte
	base := a2Canned(t, 200, `{"id":"holyarrs","jsonrpc":"2.0","result":[["1.36.0"],{"code":1,"message":"GID 0123456789abcdef is not found"},{"code":1,"message":"Unauthorized"},[{"x":"y"}]]}`, &got)
	a := NewAria2(base, "s3cret", nil)
	answers, errs, err := a.multicall(context.Background(), []a2Call{
		{"aria2.getVersion", nil}, {"aria2.tellStatus", []any{"0123456789abcdef"}}, {"aria2.getGlobalStat", nil}, {"aria2.getGlobalOption", nil},
	})
	if err != nil || len(answers) != 4 || len(errs) != 4 {
		t.Fatalf("multicall: %v %v %v", answers, errs, err)
	}
	if string(answers[0]) != `"1.36.0"` || errs[0] != nil || !errors.Is(errs[1], ErrNotFound) || !errors.Is(errs[2], ErrAuth) || errs[3] != nil {
		t.Errorf("answers %q errors %v", answers, errs)
	}
	var req struct {
		Method string
		Params [][]struct {
			MethodName string
			Params     []any
		}
	}
	if err := json.Unmarshal(got, &req); err != nil || req.Method != "system.multicall" || len(req.Params) != 1 || len(req.Params[0]) != 4 {
		t.Fatalf("request %s %v", got, err)
	}
	for _, c := range req.Params[0] {
		if len(c.Params) == 0 || c.Params[0] != "token:s3cret" {
			t.Errorf("%s without its token: %v", c.MethodName, c.Params)
		}
	}
	if p := req.Params[0][1].Params; len(p) != 2 || p[1] != "0123456789abcdef" {
		t.Errorf("tellStatus params %v", p)
	}
	short := a2Canned(t, 200, `{"id":"holyarrs","jsonrpc":"2.0","result":[["1.36.0"]]}`, nil)
	if _, _, err := NewAria2(short, "", nil).multicall(context.Background(), []a2Call{{"aria2.getVersion", nil}, {"aria2.getGlobalStat", nil}}); err == nil {
		t.Error("a short multicall answer was taken")
	}
}

func TestAria2PathRules(t *testing.T) {
	for in, want := range map[string]string{
		"http://box:6800":          "http://box:6800/jsonrpc",
		"http://box:6800/":         "http://box:6800/jsonrpc",
		"http://box:6800/rpc":      "http://box:6800/jsonrpc",
		"http://box:6800/x/rpc":    "http://box:6800/x/jsonrpc",
		"http://box:6800/x/rpc/":   "http://box:6800/x/jsonrpc",
		"http://box:6800/jsonrpc2": "http://box:6800/jsonrpc2",
		"http://box:6800/jsonrpc":  "http://box:6800/jsonrpc",
	} {
		if got := NewAria2(in, "", nil).endpoint; got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
	for in, want := range map[string]string{"": "/jsonrpc", "/": "/jsonrpc", "/rpc": "/jsonrpc", "rpc": "/jsonrpc", "/a/rpc": "/a/jsonrpc", "/jsonrpc": "/jsonrpc", "/custom": "/custom"} {
		if got := Aria2Path(in); got != want {
			t.Errorf("Aria2Path(%q) = %q, want %q", in, got, want)
		}
	}
}
