package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/arrapi"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Pulsarr's calls against /sonarr and /radarr, replayed as the Pulsarr
// manual lists them: the key in a header, the status check, the hook
// records with their test call, the paged Never-add lists, the adds with
// their season choices and text ids, and the import events sent back.

// pulsarr calls one side as Pulsarr does, with the key in X-Api-Key.
func (f *oldAppFix) pulsarr(key, method, path, body string) (int, string) {
	f.t.Helper()
	st, _, b := f.call(http.DefaultClient, "", method, path, body, "X-Api-Key", key, "Accept", "application/json")
	return st, b
}

// keptEpisodes lists a show's episodes kept up to date, as "S1E2".
func (f *oldAppFix) keptEpisodes(showID int64) []string {
	f.t.Helper()
	rows, err := f.s.Read().Query(`SELECT json_extract(numbers, '$.season'), json_extract(numbers, '$.episode') FROM items
		WHERE title_id = ? AND kind = 'episode' AND keep_up_to_date = 1 ORDER BY 1, 2`, showID)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s, e int
		if err := rows.Scan(&s, &e); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("S%dE%d", s, e))
	}
	return out
}

func pulsarrSeries(profile any, tags, folder, monitor string, search bool) string {
	b, _ := json.Marshal(map[string]any{"title": "Lab Show", "tvdbId": 200, "qualityProfileId": profile, "rootFolderPath": folder,
		"addOptions":        map[string]any{"monitor": monitor, "searchForMissingEpisodes": search, "searchForCutoffUnmetEpisodes": search},
		"languageProfileId": nil, "monitored": true, "monitorNewItems": "all", "tags": json.RawMessage(tags), "seriesType": "standard", "seasonFolder": true})
	return string(b)
}

// Each of the old apps' season choices keeps the episodes its closest
// HolyArrs fit keeps. "none" and "skip" never search. Ids may come as text.
func TestOldAppPulsarrSeasonChoices(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	s := "/sonarr/api/v3/"
	profile := f.profile()
	st, b := f.pulsarr(f.sonarrKey, "POST", s+"tag", `{"label":"pulsarr-luis"}`)
	if st != 201 {
		t.Fatalf("tag: %d %s", st, b)
	}
	tag := num(aiAny(t, b)["id"])
	all := []string{"S1E1", "S1E2", "S2E1", "S2E2"}
	for _, c := range []struct {
		monitor string
		want    []string
		search  bool
	}{
		{"pilot", []string{"S1E1"}, true},
		{"none", []string{}, false},
		{"skip", []string{}, false},
		{"firstSeason", []string{"S1E1", "S1E2"}, true},
		{"lastSeason", []string{"S2E1", "S2E2"}, true},
		{"latestSeason", []string{"S2E1", "S2E2"}, true},
		{"future", []string{"S2E1", "S2E2"}, true},
		{"recent", []string{"S2E1", "S2E2"}, true},
		{"monitorSpecials", append([]string{"S0E1"}, all...), true},
		{"unmonitorSpecials", all, true},
		{"all", all, true},
		{"missing", all, true},
		{"existing", []string{}, true},
		{"unknown", all, true},
		{"somethingNew", all, true},
	} {
		body := pulsarrSeries(fmt.Sprint(profile), fmt.Sprintf(`["%d"]`, tag), f.showDir, c.monitor, true)
		st, b := f.pulsarr(f.sonarrKey, "POST", s+"series", body)
		if st != 201 {
			t.Fatalf("%s: add %d %s", c.monitor, st, b)
		}
		added := aiAny(t, b)
		id := num(added["id"])
		if got := f.keptEpisodes(id); !slices.Equal(got, c.want) {
			t.Errorf("%s: kept %v, want %v", c.monitor, got, c.want)
		}
		if tags, _ := added["tags"].([]any); len(tags) != 1 || num(tags[0]) != tag || num(added["qualityProfileId"]) != profile {
			t.Errorf("%s: tags or profile not applied: %v %v", c.monitor, added["tags"], added["qualityProfileId"])
		}
		searched := false
		for _, j := range f.jobs(library.KindScanTitle) {
			if j.Subject != nil && j.Subject.ID == id {
				searched = strings.Contains(fmt.Sprint(j.Params["then"]), "search_show")
			}
		}
		if searched != c.search {
			t.Errorf("%s: search after the scan %t, want %t", c.monitor, searched, c.search)
		}
		// The duplicate refusal carries the old apps' phrase.
		if st, b := f.pulsarr(f.sonarrKey, "POST", s+"series", body); st != 400 || !strings.Contains(b, "already been added") {
			t.Fatalf("%s: duplicate: %d %s", c.monitor, st, b)
		}
		if st, b := f.pulsarr(f.sonarrKey, "DELETE", s+"series/"+arrItoa(id)+"?deleteFiles=false&addImportListExclusion=false", ""); st != 200 {
			t.Fatalf("%s: remove: %d %s", c.monitor, st, b)
		}
	}
	// A season list with no choice still picks seasons, as Seerr sends it.
	body := strings.Replace(pulsarrSeries(profile, "[]", f.showDir, "", false), `"seasonFolder":true`,
		`"seasonFolder":true,"seasons":[{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":false}]`, 1)
	st, b = f.pulsarr(f.sonarrKey, "POST", s+"series", body)
	if st != 201 {
		t.Fatalf("season list: %d %s", st, b)
	}
	if got := f.keptEpisodes(num(aiAny(t, b)["id"])); !slices.Equal(got, []string{"S1E1", "S1E2"}) {
		t.Fatalf("season list kept %v", got)
	}
	// A Pick rules id that is text but not a number is a failed check on
	// that field, not a broken body.
	st, b = f.pulsarr(f.sonarrKey, "POST", s+"series", pulsarrSeries("HD-1080p", "[]", f.showDir, "all", false))
	if st != 400 || !strings.Contains(b, `"propertyName":"QualityProfileId"`) {
		t.Fatalf("text profile: %d %s", st, b)
	}
}

// A movie add takes tag ids and the Pick rules id as text, keeps the tags,
// and a repeat carries the old apps' phrase.
func TestOldAppPulsarrMovieAdd(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	st, b := f.pulsarr(f.radarrKey, "POST", r+"tag", `{"label":"pulsarr-luis"}`)
	if st != 201 {
		t.Fatalf("tag: %d %s", st, b)
	}
	tag := num(aiAny(t, b)["id"])
	body := fmt.Sprintf(`{"title":"Dune: Part Two","tmdbId":693134,"qualityProfileId":"%d","rootFolderPath":%q,"addOptions":{"searchForMovie":false,"monitor":"none"},
		"tags":["%d"],"minimumAvailability":"released","monitored":false}`, f.profile(), f.movieDir, tag)
	st, b = f.pulsarr(f.radarrKey, "POST", r+"movie", body)
	if st != 201 {
		t.Fatalf("add: %d %s", st, b)
	}
	m := aiAny(t, b)
	if tags, _ := m["tags"].([]any); len(tags) != 1 || num(tags[0]) != tag || m["monitored"] != false {
		t.Fatalf("added: %v", m)
	}
	list := arrList(t, "movies", f.arrOK(f.radarrKey, "GET", r+"movie", "", 200), 1)
	if tags := list[0].(map[string]any)["tags"].([]any); len(tags) != 1 {
		t.Fatalf("list tags: %v", tags)
	}
	if st, b := f.pulsarr(f.radarrKey, "POST", r+"movie", body); st != 400 || !strings.Contains(b, "already been added") || !strings.Contains(b, "MovieExistsValidator") {
		t.Fatalf("duplicate: %d %s", st, b)
	}
}

func aiAny(t *testing.T, b string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(b), &m); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return m
}

// pulsarrReceiver stands in for Pulsarr's incoming webhook route: it
// checks the secret header and records each body.
type pulsarrReceiver struct {
	srv    *httptest.Server
	secret string
	mu     sync.Mutex
	bodies []map[string]any
}

func newPulsarrReceiver(t *testing.T, secret string) *pulsarrReceiver {
	p := &pulsarrReceiver{secret: secret}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Pulsarr-Secret") != p.secret || r.URL.Query().Get("instanceId") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.mu.Lock()
		p.bodies = append(p.bodies, m)
		p.mu.Unlock()
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *pulsarrReceiver) events(kind string) []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, b := range p.bodies {
		if b["eventType"] == kind {
			out = append(out, b)
		}
	}
	return out
}

func pulsarrHookBody(id int64, address, secret string) string {
	b, _ := json.Marshal(map[string]any{"id": id, "name": "Pulsarr (pulsarr.lan)", "implementation": "Webhook", "implementationName": "Webhook",
		"configContract": "WebhookSettings", "infoLink": "https://wiki.example/webhook", "tags": []any{}, "onGrab": false, "onDownload": true,
		"onUpgrade": false, "onImportComplete": false, "onRename": false, "onSeriesAdd": false, "onHealthIssue": false, "supportsOnDownload": true,
		"fields": []any{map[string]any{"name": "url", "value": address}, map[string]any{"name": "method", "value": 1},
			map[string]any{"name": "headers", "value": []any{map[string]any{"key": "X-Pulsarr-Secret", "value": secret}}}}})
	return string(b)
}

// recordImport puts a file on an item's main Slot and records the import
// Event for it, replacing an older copy or not.
func (f *oldAppFix) recordImport(titleID, itemID int64, rel string, replaces bool) {
	f.t.Helper()
	var sl mediafiles.Slot
	if err := f.s.Read().QueryRow(`SELECT s.id, s.version_id FROM slots s JOIN versions v ON v.id = s.version_id WHERE s.item_id = ? AND v.is_main = 1`, itemID).Scan(&sl.ID, &sl.VersionID); err != nil {
		f.t.Fatal(err)
	}
	f.putFileAs(sl, mediafiles.File{RelativePath: rel, Size: 1 << 20, Quality: mediafiles.Quality{Resolution: 1080, Source: "web_dl"}})
	step := mediafiles.PlanStep{TargetSlotIDs: []int64{sl.ID}, TargetPath: rel, Method: "hardlink", Why: []string{}}
	if replaces {
		old := int64(1)
		step.ReplacesFileID = &old
	}
	imp := &mediafiles.Import{Kind: "episodes", Mode: "auto", State: "done", Plan: []mediafiles.PlanStep{step}, Result: []mediafiles.PlaceResult{{Outcome: "placed"}}}
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "imports", imp, domain.System)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.s.AddEvent(context.Background(), &domain.Event{Kind: "import_done", TitleID: &titleID, Data: map[string]any{"import_id": imp.ID}}); err != nil {
		f.t.Fatal(err)
	}
}

// The whole Pulsarr path: connect, keep a hook after its test call, read
// the Never-add lists, add a show and a movie, and hear of each import
// once, never of an upgrade, and nothing once the arr API is off.
func TestOldAppPulsarrFlow(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	hooks := &arrapi.Handlers{Deps: arrapi.Deps{Store: f.s, Secrets: f.vault}, Auth: accounts.New(f.s, accounts.Options{})}
	ctx, stop := context.WithCancel(context.Background())
	hooks.Watch(ctx)
	t.Cleanup(func() { stop(); hooks.Wait() })
	pul := newPulsarrReceiver(t, "pulsarr-secret")
	for _, side := range []struct{ name, key, app string }{{"sonarr", f.sonarrKey, "Sonarr"}, {"radarr", f.radarrKey, "Radarr"}} {
		p := "/" + side.name + "/api/v3/"
		st, b := f.pulsarr(side.key, "GET", p+"system/status", "")
		m := aiAny(t, b)
		if st != 200 || m["appName"] != side.app {
			t.Fatalf("%s status: %d %s", side.name, st, b)
		}
		for _, k := range []string{"appName", "version", "branch", "authentication"} {
			if s, _ := m[k].(string); s == "" {
				t.Fatalf("%s status %s: %v", side.name, k, m[k])
			}
		}
		for _, path := range []string{"qualityprofile", "rootfolder", "tag"} {
			if st, b := f.pulsarr(side.key, "GET", p+path, ""); st != 200 {
				t.Fatalf("%s %s: %d %s", side.name, path, st, b)
			}
		}
		if st, b := f.pulsarr(side.key, "GET", p+"notification", ""); st != 200 || strings.TrimSpace(b) != "[]" {
			t.Fatalf("%s hooks: %d %s", side.name, st, b)
		}
		address := pul.srv.URL + "/v1/notifications/webhook?instanceId=holyarrs" + side.name
		// A wrong secret: Pulsarr answers 401, the save is refused on the
		// address field and nothing is kept.
		st, b = f.pulsarr(side.key, "POST", p+"notification", pulsarrHookBody(0, address, "wrong"))
		if st != 400 || !strings.Contains(b, `"propertyName":"Url"`) {
			t.Fatalf("%s wrong secret: %d %s", side.name, st, b)
		}
		st, b = f.pulsarr(side.key, "POST", p+"notification", pulsarrHookBody(0, address, "pulsarr-secret"))
		if st != 201 || strings.Contains(b, "pulsarr-secret") {
			t.Fatalf("%s add hook: %d %s", side.name, st, b)
		}
		id := num(aiAny(t, b)["id"])
		if st, b := f.pulsarr(side.key, "POST", p+"notification/test", pulsarrHookBody(id, address, "pulsarr-secret")); st != 200 {
			t.Fatalf("%s test hook: %d %s", side.name, st, b)
		}
		if st, b := f.pulsarr(side.key, "PUT", p+"notification/"+arrItoa(id), pulsarrHookBody(id, address, "pulsarr-secret")); st != 202 {
			t.Fatalf("%s change hook: %d %s", side.name, st, b)
		}
	}
	if n := len(pul.events("Test")); n != 6 {
		t.Fatalf("%d test events, want 6", n)
	}
	// The Never-add lists, empty and then in two pages.
	page := func(path string) map[string]any {
		st, b := f.pulsarr(f.sonarrKey, "GET", path, "")
		if st != 200 {
			t.Fatalf("%s: %d %s", path, st, b)
		}
		return aiAny(t, b)
	}
	const showPage = "/sonarr/api/v3/importlistexclusion/paged?page=%d&pageSize=1&sortDirection=ascending&sortKey=title"
	if p := page(fmt.Sprintf(showPage, 1)); num(p["totalRecords"]) != 0 || len(p["records"].([]any)) != 0 {
		t.Fatalf("empty list: %v", p)
	}
	f.exec(`INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, source_ids, name) VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'show', '{"tvdb":"901"}', 'B Show'),
		('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'show', '{"tvdb":"900"}', 'A Show')`)
	p1, p2 := page(fmt.Sprintf(showPage, 1)), page(fmt.Sprintf(showPage, 2))
	if num(p1["totalRecords"]) != 2 || num(p1["records"].([]any)[0].(map[string]any)["tvdbId"]) != 900 || num(p2["records"].([]any)[0].(map[string]any)["tvdbId"]) != 901 {
		t.Fatalf("pages: %v %v", p1, p2)
	}
	if st, b := f.pulsarr(f.radarrKey, "GET", "/radarr/api/v3/exclusions/paged?page=1&pageSize=1000&sortDirection=ascending&sortKey=movieTitle", ""); st != 200 || !strings.Contains(b, `"totalRecords":0`) {
		t.Fatalf("movie list: %d %s", st, b)
	}

	// A pilot-rolling show and a movie, then their imports.
	st, b := f.pulsarr(f.sonarrKey, "POST", "/sonarr/api/v3/series", pulsarrSeries(f.profile(), "[]", f.showDir, "pilot", true))
	if st != 201 {
		t.Fatalf("add show: %d %s", st, b)
	}
	showID := num(aiAny(t, b)["id"])
	movieID := f.addLabMovie()
	f.recordImport(showID, f.itemOf(showID, 1, 1), "Lab Show (2025)/Season 01/Lab Show - S01E01.mkv", false)
	var movieItem int64
	_ = f.s.Read().QueryRow("SELECT id FROM items WHERE title_id = ?", movieID).Scan(&movieItem)
	f.recordImport(movieID, movieItem, "Dune Part Two (2024)/Dune Part Two (2024).mkv", false)
	hooks.Wait()
	downloads := pul.events("Download")
	if len(downloads) != 2 {
		t.Fatalf("%d import events, want 2: %v", len(downloads), downloads)
	}
	var show, movie map[string]any
	for _, d := range downloads {
		if d["series"] != nil {
			show = d
		} else {
			movie = d
		}
	}
	if show == nil || num(show["series"].(map[string]any)["tvdbId"]) != 200 || len(show["episodes"].([]any)) != 1 || show["episodeFile"].(map[string]any)["quality"] != "WEBDL-1080p" {
		t.Fatalf("show event: %v", show)
	}
	if movie == nil || num(movie["movie"].(map[string]any)["tmdbId"]) != 693134 || movie["instanceName"] == "" {
		t.Fatalf("movie event: %v", movie)
	}
	// An upgrade is not news to the people who asked.
	f.recordImport(showID, f.itemOf(showID, 1, 2), "Lab Show (2025)/Season 01/Lab Show - S01E02.mkv", true)
	hooks.Wait()
	// With the arr API off, the hooks stay quiet.
	if st, b := f.session("PUT", "/api/v1/settings/old-app-api", `{"enabled":false}`); st != 200 {
		t.Fatalf("off: %d %s", st, b)
	}
	f.recordImport(showID, f.itemOf(showID, 2, 1), "Lab Show (2025)/Season 02/Lab Show - S02E01.mkv", false)
	hooks.Wait()
	if n := len(pul.events("Download")); n != 2 {
		t.Fatalf("%d import events after an upgrade and with the API off, want 2", n)
	}
}
