package sites

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// assrtFake stands in for the Assrt API. Answer shapes follow the site's API
// notes and Bazarr's assrt provider; every title and file is made up.
type assrtFake struct {
	mu     sync.Mutex
	token  string
	status int // when set, every call answers with this status
	asked  []string
}

func (f *assrtFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	f.asked = append(f.asked, r.URL.Path+" "+q.Get("q")+q.Get("id"))
	answer := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		// Like the real site, an error status comes with HTTP 400.
		if m, ok := v.(map[string]any); ok && m["status"] != 0 {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(v)
	}
	if strings.HasPrefix(r.URL.Path, "/files/") {
		_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\n" + strings.TrimPrefix(r.URL.Path, "/files/") + "\n"))
		return
	}
	if q.Get("token") != f.token {
		answer(map[string]any{"status": 20001, "errmsg": "token invalid"})
		return
	}
	if f.status != 0 {
		answer(map[string]any{"status": f.status, "errmsg": "no"})
		return
	}
	switch r.URL.Path {
	case "/v1/sub/search":
		subs := []any{}
		switch q.Get("q") {
		case "Quiet Harbour S01E02":
			subs = append(subs,
				map[string]any{"id": 601, "videoname": "静港.Quiet.Harbour.S01E02.1080p.WEB-DL-GRP", "native_name": []string{"静港"},
					"lang": map[string]any{"desc": "简 英", "langlist": map[string]bool{"langchs": true, "langeng": true}}},
				map[string]any{"id": 602, "videoname": "Quiet.Harbour.S01.720p.HDTV", "native_name": "静港",
					"lang": map[string]any{"desc": "繁", "langlist": map[string]bool{"langcht": true}}},
				map[string]any{"id": 603, "videoname": "不知道", "lang": map[string]any{"desc": "日", "langlist": map[string]bool{"langjpn": true}}},
				map[string]any{"id": 604, "videoname": "静港 第一季"})
		case "The Lantern 2010":
			subs = append(subs, map[string]any{"id": 701, "videoname": "The.Lantern.2010.BluRay.x264", "lang": map[string]any{"langlist": map[string]bool{"langchs": true}}})
		case "Ab":
			answer(map[string]any{"status": 101, "errmsg": "keyword too short"})
			return
		}
		answer(map[string]any{"status": 0, "sub": map[string]any{"result": "succeed", "subs": subs}})
	case "/v1/sub/detail":
		base := "http://" + r.Host + "/files/"
		var sub map[string]any
		switch q.Get("id") {
		case "601":
			sub = map[string]any{"id": 601, "filename": "set.rar", "url": base + "set.rar", "filelist": []any{
				map[string]any{"f": "readme.txt", "url": base + "readme", "s": "1KB"},
				map[string]any{"f": "Quiet.Harbour.S01E02.eng.srt", "url": base + "e02-eng", "s": "30KB"},
				map[string]any{"f": "Quiet.Harbour.S01E02.chs.srt", "url": base + "e02-chs", "s": "30KB"},
			}}
		case "602":
			sub = map[string]any{"id": 602, "filelist": []any{
				map[string]any{"f": "Quiet.Harbour.S01E01.cht.ass", "url": base + "e01-cht"},
				map[string]any{"f": "Quiet.Harbour.S01E02.cht.ass", "url": base + "e02-cht"},
			}}
		case "701":
			sub = map[string]any{"id": 701, "filename": "The.Lantern.srt", "url": base + "lantern", "filelist": []any{}}
		case "702":
			sub = map[string]any{"id": 702, "url": "http://elsewhere.example/x.srt", "filelist": []any{}}
		default:
			answer(map[string]any{"status": 0, "sub": map[string]any{"subs": []any{}}})
			return
		}
		answer(map[string]any{"status": 0, "sub": map[string]any{"subs": []any{sub}}})
	default:
		http.NotFound(w, r)
	}
}

func assrtSite(t *testing.T, f *assrtFake, key string) Site {
	t.Helper()
	old := assrtMinGap
	assrtMinGap = time.Millisecond
	t.Cleanup(func() { assrtMinGap = old })
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New("assrt", Config{Name: "Assrt", URL: srv.URL + "/v1", Key: key, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAssrtSearchAndDownload(t *testing.T) {
	f := &assrtFake{token: fakeKey}
	s := assrtSite(t, f, fakeKey)
	ctx := context.Background()
	got, err := s.Search(ctx, Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, Languages: []string{"zh", "en"}})
	if err != nil || len(got) != 3 {
		t.Fatalf("%+v %v", got, err)
	}
	byKey := map[string][]string{}
	for _, c := range got {
		byKey[c.Key] = append(byKey[c.Key], c.Language)
		if c.Link != "S01E02" || c.Title != "" || c.Season != 0 {
			t.Fatalf("candidate %+v", c)
		}
	}
	if len(byKey["601"]) != 2 || len(byKey["602"]) != 1 || byKey["602"][0] != "zh" {
		t.Fatalf("languages %v", byKey)
	}
	// The Chinese title before the release name is dropped so the name parses.
	for _, c := range got {
		if c.Key == "601" && (len(c.ReleaseNames) != 1 || c.ReleaseNames[0] != "Quiet.Harbour.S01E02.1080p.WEB-DL-GRP") {
			t.Fatalf("release %+v", c)
		}
	}
	if f.asked[0] != "/v1/sub/search Quiet Harbour S01E02" {
		t.Fatalf("asked %v", f.asked)
	}
	// Download picks the file for the language and the episode out of a set.
	for _, tc := range []struct{ key, lang, want string }{
		{"601", "zh", "e02-chs"}, {"601", "en", "e02-eng"}, {"602", "zh", "e02-cht"},
	} {
		b, err := s.Download(ctx, Candidate{Key: tc.key, Language: tc.lang, Link: "S01E02"})
		if err != nil || !strings.Contains(string(b), tc.want) {
			t.Fatalf("%+v: %q %v", tc, b, err)
		}
	}
	// A movie set with one file has its link on the set itself.
	got, err = s.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: []string{"zh"}})
	if err != nil || len(got) != 1 || got[0].Link != "" {
		t.Fatalf("movie %+v %v", got, err)
	}
	if b, err := s.Download(ctx, got[0]); err != nil || !strings.Contains(string(b), "lantern") {
		t.Fatalf("movie download %q %v", b, err)
	}
	// A plain http link to another host is refused.
	if _, err := s.Download(ctx, Candidate{Key: "702", Language: "zh"}); kindOf(err) != KindUnreadable {
		t.Fatalf("http elsewhere: %v", err)
	}
	// Gone, and a key that is not a number.
	if _, err := s.Download(ctx, Candidate{Key: "999", Language: "zh"}); kindOf(err) != KindUnreadable {
		t.Fatalf("gone: %v", err)
	}
	n := len(f.asked)
	if _, err := s.Download(ctx, Candidate{Key: "../1"}); kindOf(err) != KindUnreadable || len(f.asked) != n {
		t.Fatalf("bad key: %v", err)
	}
	// Languages the site lacks, and a too-short keyword, make no call or an empty one.
	if got, err := s.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Languages: []string{"pl"}}); err != nil || len(got) != 0 || len(f.asked) != n {
		t.Fatalf("polish: %+v %v", got, err)
	}
	if got, err := s.Search(ctx, Query{Kind: "movie", Title: "Ab", Languages: []string{"zh"}}); err != nil || len(got) != 0 {
		t.Fatalf("short: %+v %v", got, err)
	}
}

func TestAssrtStatuses(t *testing.T) {
	ctx := context.Background()
	q := Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: []string{"zh"}}
	// A wrong token rests the site and never shows in the error.
	s := assrtSite(t, &assrtFake{token: fakeKey}, fakeKey+"x")
	_, err := s.Search(ctx, q)
	if kindOf(err) != KindLoginRefused || strings.Contains(err.Error(), fakeKey+"x") {
		t.Fatalf("token: %v", err)
	}
	for status, want := range map[int]string{30900: KindRateLimited, 30001: KindDown, 555: KindUnreadable} {
		s := assrtSite(t, &assrtFake{token: "t", status: status}, "t")
		if _, err := s.Search(ctx, q); kindOf(err) != want {
			t.Errorf("%d: %v", status, err)
		}
	}
}

func TestAssrtRelease(t *testing.T) {
	for in, want := range map[string]string{
		"瑞克和莫蒂.Rick.and.Morty.S07E10.1080p": "Rick.and.Morty.S07E10.1080p",
		"瑞克和莫蒂 Rick and Morty S07E10":       "Rick and Morty S07E10",
		"Rick.and.Morty.S07E10":             "Rick.and.Morty.S07E10",
		"不知道":                               "",
		"静港 第一季":                            "",
		"":                                  "",
	} {
		if got := assrtRelease(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
