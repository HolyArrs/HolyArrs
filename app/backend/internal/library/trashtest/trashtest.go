// Package trashtest serves a small made-up TRaSH Guides tree for tests.
package trashtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

// Files is the made-up sonarr tree: one profile, three formats (a name
// test, a group-and-source test, and one with a language test HolyArrs
// skips) and one default group adding a fourth.
var Files = map[string]string{
	"docs/json/sonarr/quality-profiles/test-1080p.json": `{"trash_id":"p1","name":"Test 1080p","trash_description":"Line one<br>line two",
		"minFormatScore":0,"cutoffFormatScore":10000,"minUpgradeFormatScore":1,
		"formatItems":{"HEVC":"f1","Good group":"f2","Lang only":"f3"}}`,
	"docs/json/sonarr/cf/hevc.json": `{"trash_id":"f1","name":"HEVC","trash_scores":{"default":-500},
		"specifications":[{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":true,"fields":{"value":"[xh][ ._-]?265|\\bHEVC\\b"}}]}`,
	"docs/json/sonarr/cf/good-group.json": `{"trash_id":"f2","name":"Good group","trash_scores":{"default":1700},
		"specifications":[{"name":"Group","implementation":"ReleaseGroupSpecification","negate":false,"required":false,"fields":{"value":"^(GOODA)$"}},
		{"name":"WEB","implementation":"SourceSpecification","negate":false,"required":false,"fields":{"value":3}}]}`,
	"docs/json/sonarr/cf/lang.json": `{"trash_id":"f3","name":"Lang only","trash_scores":{"default":-10000},
		"specifications":[{"name":"Not English","implementation":"LanguageSpecification","negate":true,"required":false,"fields":{"value":1}}]}`,
	"docs/json/sonarr/cf/lq.json": `{"trash_id":"f4","name":"LQ","trash_scores":{"default":-20000},
		"specifications":[{"name":"LQ","implementation":"ReleaseGroupSpecification","negate":false,"required":false,"fields":{"value":"^(BADGRP)$"}}]}`,
	"docs/json/sonarr/cf-groups/unwanted.json": `{"name":"Unwanted","default":"true",
		"custom_formats":[{"name":"LQ","trash_id":"f4","required":false,"default":true}],
		"quality_profiles":{"include":{"Test 1080p":"p1"}}}`,
	"docs/json/radarr/quality-profiles/none.json": `{"trash_id":"m1","name":"Movie test","formatItems":{}}`,
}

// Serve starts a server for files and answers the raw-files and listing
// bases to point a TRaSH Guides reader at.
func Serve(t testing.TB, files map[string]string) (raw, api string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if dir, ok := strings.CutPrefix(p, "/api/"); ok {
			var items []map[string]string
			for f := range files {
				if path.Dir(f) == dir {
					items = append(items, map[string]string{"name": path.Base(f), "type": "file"})
				}
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		if body, ok := files[strings.TrimPrefix(p, "/raw/master/")]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/raw", srv.URL + "/api"
}
