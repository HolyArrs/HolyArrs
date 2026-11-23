package mediaservers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// kodiCall is one JSON-RPC call the fake Kodi received.
type kodiCall struct {
	Method string
	Params map[string]any
}

// fakeKodi plays Kodi's JSON-RPC web server with user "kodi" and password
// pw. answer, when set, replaces the answer to a method.
func fakeKodi(t *testing.T, pw string, answer map[string]string) (*httptest.Server, func() []kodiCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []kodiCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "kodi" || p != pw {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/jsonrpc" || r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      any            `json:"id"`
			Method  string         `json:"method"`
			Params  map[string]any `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.JSONRPC != "2.0" || req.ID == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		calls = append(calls, kodiCall{req.Method, req.Params})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if a, ok := answer[req.Method]; ok {
			_, _ = w.Write([]byte(a))
			return
		}
		switch req.Method {
		case "Application.GetProperties":
			_, _ = w.Write([]byte(`{"id":1,"jsonrpc":"2.0","result":{"name":"Kodi","version":{"major":21,"minor":1,"revision":"","tag":"stable"}}}`))
		case "VideoLibrary.Scan", "VideoLibrary.Clean":
			_, _ = w.Write([]byte(`{"id":1,"jsonrpc":"2.0","result":"OK"}`))
		default:
			_, _ = w.Write([]byte(`{"id":1,"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found."}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []kodiCall { mu.Lock(); defer mu.Unlock(); return append([]kodiCall(nil), calls...) }
}

func TestKodiTestAndTell(t *testing.T) {
	cases := []struct {
		name    string
		updates []Update
		want    []string // method directory
	}{
		{"import scans the folder", []Update{{Path: "/tv/Show", Change: Created, Kind: "show"}}, []string{"VideoLibrary.Scan /tv/Show/"}},
		{"rename scans then cleans", []Update{{Path: "/movies/Film (2020)", Change: Modified, Kind: "movie"}},
			[]string{"VideoLibrary.Scan /movies/Film (2020)/", "VideoLibrary.Clean /movies/Film (2020)/"}},
		{"delete cleans the folder", []Update{{Path: "/movies/Gone", Change: Deleted}}, []string{"VideoLibrary.Clean /movies/Gone/"}},
		{"smb path keeps its slash", []Update{{Path: "smb://nas/tv/Show/", Change: Created, Kind: "show"}}, []string{"VideoLibrary.Scan smb://nas/tv/Show/"}},
		{"windows path ends with a backslash", []Update{{Path: `D:\tv\Show`, Change: Created, Kind: "show"}}, []string{`VideoLibrary.Scan D:\tv\Show\`}},
		{"music is left out", []Update{{Path: "/music/Artist", Change: Created, Kind: "artist"}}, nil},
		{"several folders in order", []Update{{Path: "/tv/A", Change: Created}, {Path: "/tv/B", Change: Deleted}},
			[]string{"VideoLibrary.Scan /tv/A/", "VideoLibrary.Clean /tv/B/"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			srv, calls := fakeKodi(t, "s3cret pw", nil)
			m, err := Add(context.Background(), w.s, w.v, Input{Name: "Kodi", Kind: "kodi", URL: srv.URL, Username: " kodi ", Token: "s3cret pw"}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			got, err := l.Test(context.Background(), m)
			if err != nil || got != "Kodi 21.1" {
				t.Fatalf("test = %q, %v", got, err)
			}
			if err := l.Tell(context.Background(), m, tc.updates); err != nil {
				t.Fatal(err)
			}
			var seen []string
			for _, c := range calls()[1:] {
				if c.Params["showdialogs"] != false {
					t.Fatalf("%s showdialogs = %v, want false", c.Method, c.Params["showdialogs"])
				}
				seen = append(seen, c.Method+" "+c.Params["directory"].(string))
			}
			if strings.Join(seen, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("calls = %q, want %q", seen, tc.want)
			}
		})
	}
}

func TestKodiRefusals(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		answer  map[string]string
		wantErr string
	}{
		{"wrong password", "other", nil, ""},
		{"json-rpc error", "s3cret pw", map[string]string{"VideoLibrary.Scan": `{"id":1,"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params."}}`}, "Kodi refused the call: Invalid params."},
		{"not kodi", "s3cret pw", map[string]string{"Application.GetProperties": `{"hello":"world"}`}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			srv, _ := fakeKodi(t, tc.pw, tc.answer)
			m, err := Add(context.Background(), w.s, w.v, Input{Name: "Kodi", Kind: "kodi", URL: srv.URL, Username: "kodi", Token: "s3cret pw"}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			log.SetOutput(&logs)
			defer log.SetOutput(os.Stderr)
			l := &Linker{Store: w.s, Vault: w.v}
			_, testErr := l.Test(context.Background(), m)
			tellErr := l.Tell(context.Background(), m, []Update{{Path: "/tv/Show", Change: Created}})
			if tc.wantErr != "" {
				if testErr != nil || tellErr == nil || tellErr.Error() != tc.wantErr {
					t.Fatalf("test %v, tell %v; want tell %q", testErr, tellErr, tc.wantErr)
				}
			} else if testErr == nil {
				t.Fatal("test passed, want a refusal")
			}
			for _, e := range []error{testErr, tellErr} {
				if e != nil && strings.Contains(e.Error(), "s3cret") {
					t.Fatalf("error holds the password: %v", e)
				}
			}
			if strings.Contains(logs.String(), "s3cret") {
				t.Fatal("log holds the password")
			}
		})
	}
}

func TestKodiNeedsUserAndPassword(t *testing.T) {
	w := newWorld(t)
	for _, in := range []Input{
		{Name: "Kodi", Kind: "kodi", URL: "http://kodi.lan:8080", Token: "pw"},
		{Name: "Kodi", Kind: "kodi", URL: "http://kodi.lan:8080", Username: "kodi"},
	} {
		if _, err := Add(context.Background(), w.s, w.v, in, domain.System); !errors.Is(err, ErrNoKodiUser) && !errors.Is(err, ErrNoKey) {
			t.Fatalf("add %+v = %v, want a refusal", in, err)
		}
	}
}

// A Kodi connection never reads people, watch state or labels.
func TestKodiIsRefreshOnly(t *testing.T) {
	if !refreshOnly(domain.MediaServer{Kind: "kodi"}) || !refreshOnly(domain.MediaServer{Kind: "subsonic"}) || refreshOnly(domain.MediaServer{Kind: "plex"}) {
		t.Fatal("refreshOnly is wrong")
	}
}
