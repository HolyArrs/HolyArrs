package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQbitKeepsSessionCookie(t *testing.T) {
	var second string
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 2 {
			if c, err := r.Cookie("SID"); err == nil {
				second = c.Value
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		w.Write([]byte("v5.1.2"))
	}))
	defer srv.Close()
	q := NewQbittorrentKey(srv.URL, "k", nil)
	for i := 0; i < 2; i++ {
		if _, err := q.Health(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if second != "abc" {
		t.Fatalf("cookie not sent back: %q", second)
	}
}

func TestQbitHealthNotesKeyBefore52(t *testing.T) {
	for v, want := range map[string]bool{"v5.1.2": true, "v4.6.0": true, "v5.2.0": false, "v6.0": false, "odd": false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(v)) }))
		h, err := NewQbittorrentKey(srv.URL, "k", nil).Health(context.Background())
		srv.Close()
		if err != nil || (h.Note != "") != want || (want && !strings.Contains(h.Note, "5.2")) {
			t.Fatalf("%s: %v %q", v, err, h.Note)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("v5.1.2")) }))
	defer srv.Close()
	if h, _ := NewQbittorrent(srv.URL, nil).Health(context.Background()); h.Note != "" {
		t.Fatal("note without a key")
	}
}

func TestStatusFromMain(t *testing.T) {
	d, err := parseMainData([]byte(`{"rid":1,"full_update":true,"torrents":{"AA":{"state":"stoppedDL","name":"a"}},"server_state":{"free_space_on_disk":99,"dl_info_speed":5,"connection_status":"connected"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var s MainState
	if _, err := s.Apply(d); err != nil {
		t.Fatal(err)
	}
	st := StatusFromMain(&s)
	if st.FreeSpace != 99 || st.Speed != 5 || st.Connection != "connected" || !st.Paused {
		t.Fatalf("%+v", st)
	}
	d2, _ := parseMainData([]byte(`{"rid":2,"torrents":{"AA":{"state":"downloading"}},"server_state":{"dl_info_speed":7}}`))
	s.Apply(d2)
	st = StatusFromMain(&s)
	if st.FreeSpace != 99 || st.Speed != 7 || st.Paused {
		t.Fatalf("delta %+v", st)
	}
}
