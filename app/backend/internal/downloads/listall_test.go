package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQbitListAllEveryCategory(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Query().Get("category") == "holyarrs" {
			_, _ = w.Write([]byte(`[{"hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","category":"holyarrs","state":"downloading"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","category":"holyarrs","state":"downloading"},
{"hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","name":"Other","category":"linux","state":"stalledUP","added_on":1700000000,"ratio":2.5},
{"hash":"cccccccccccccccccccccccccccccccccccccccc","category":"","state":"pausedDL"}]`))
	}))
	defer srv.Close()
	q := NewQbittorrent(srv.URL, nil)
	var _ AllLister = q
	all, err := q.ListAll(context.Background())
	if err != nil || len(all) != 3 {
		t.Fatalf("all %v %+v", err, all)
	}
	if all[0].ID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || all[1].Category != "linux" || all[1].State != StateFinished || all[1].AddedAt != 1700000000 || all[1].Ratio != 2.5 || all[2].State != StatePaused {
		t.Fatalf("jobs %+v", all)
	}
	own, err := q.List(context.Background(), "holyarrs")
	if err != nil || len(own) != 1 {
		t.Fatalf("own %v %+v", err, own)
	}
	if queries[0] != "/api/v2/torrents/info?" || queries[1] != "/api/v2/torrents/info?category=holyarrs" {
		t.Fatalf("queries %v", queries)
	}
}

func TestInspectable(t *testing.T) {
	for st, want := range map[string]bool{"downloading": true, "finished": true, "seeding": true, "failed": true, "cleaned": false, "vanished": false, "": false} {
		if Inspectable(st) != want {
			t.Fatalf("%s: %v", st, !want)
		}
	}
}
