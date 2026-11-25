package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// Kometa's bulk calls, sent the way its arr library sends them: the key in
// X-Api-Key, lists of ids in the body, tags with an apply mode.

func (f *oldAppFix) kometa(key, method, path, body string) (int, string) {
	f.t.Helper()
	return f.pulsarr(key, method, path, body)
}

func tagNums(v any) []int64 {
	out := []int64{}
	l, _ := v.([]any)
	for _, x := range l {
		out = append(out, num(x))
	}
	slices.Sort(out)
	return out
}

func TestOldAppKometaTagByID(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	for _, side := range []struct{ key, base string }{{f.sonarrKey, "/sonarr/api/v3/"}, {f.radarrKey, "/radarr/api/v3/"}} {
		st, b := f.kometa(side.key, "POST", side.base+"tag", `{"label":"kometa"}`)
		if st != 201 {
			t.Fatalf("tag: %d %s", st, b)
		}
		id := num(aiAny(t, b)["id"])
		for _, c := range []struct {
			path string
			want int
		}{{fmt.Sprint(id), 200}, {"999", 404}, {"x", 404}} {
			st, b := f.kometa(side.key, "GET", side.base+"tag/"+c.path, "")
			if st != c.want {
				t.Fatalf("GET tag/%s: %d, want %d: %s", c.path, st, c.want, b)
			}
			if st == 200 && (aiAny(t, b)["label"] != "kometa" || num(aiAny(t, b)["id"]) != id) {
				t.Fatalf("GET tag/%s: %s", c.path, b)
			}
		}
	}
}

func TestOldAppKometaMovieBulk(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	mk := func(label string) int64 {
		_, b := f.kometa(f.radarrKey, "POST", r+"tag", fmt.Sprintf(`{"label":%q}`, label))
		return num(aiAny(t, b)["id"])
	}
	a, bt := mk("a"), mk("b")
	// Bulk add: the known movie is added, an unknown id and a repeat are
	// skipped, as the old apps skip them.
	item := func(tmdb int) string {
		return fmt.Sprintf(`{"title":"x","tmdbId":%d,"qualityProfileId":%d,"rootFolderPath":%q,"monitored":true,"minimumAvailability":"announced","tags":[%d],"addOptions":{"searchForMovie":false}}`,
			tmdb, f.profile(), f.movieDir, a)
	}
	st, b := f.kometa(f.radarrKey, "POST", r+"movie/import", "["+item(693134)+","+item(1)+"]")
	if st != 200 {
		t.Fatalf("import: %d %s", st, b)
	}
	added := arrList(t, "import", kometaJSON(t, b), 1)
	id := num(added[0].(map[string]any)["id"])
	if got := tagNums(added[0].(map[string]any)["tags"]); !slices.Equal(got, []int64{a}) {
		t.Fatalf("import tags %v", got)
	}
	if st, b := f.kometa(f.radarrKey, "POST", r+"movie/import", "["+item(693134)+"]"); st != 200 || strings.TrimSpace(b) != "[]" {
		t.Fatalf("repeat import: %d %s", st, b)
	}
	// Editor: each apply mode, then monitored and the stage.
	for _, c := range []struct {
		body string
		tags []int64
	}{
		{fmt.Sprintf(`{"movieIds":[%d],"tags":[%d],"applyTags":"add"}`, id, bt), []int64{a, bt}},
		{fmt.Sprintf(`{"movieIds":[%d],"tags":[%d],"applyTags":"remove"}`, id, a), []int64{bt}},
		{fmt.Sprintf(`{"movieIds":[%d],"tags":[%d],"applyTags":"replace"}`, id, a), []int64{a}},
		{fmt.Sprintf(`{"movieIds":[%d],"monitored":false,"minimumAvailability":"released","moveFiles":false}`, id), []int64{a}},
	} {
		st, b := f.kometa(f.radarrKey, "PUT", r+"movie/editor", c.body)
		if st != 202 {
			t.Fatalf("editor %s: %d %s", c.body, st, b)
		}
		got := arrList(t, "editor", kometaJSON(t, b), 1)[0].(map[string]any)
		if tg := tagNums(got["tags"]); !slices.Equal(tg, c.tags) {
			t.Fatalf("editor %s: tags %v, want %v", c.body, tg, c.tags)
		}
	}
	m := f.arrOK(f.radarrKey, "GET", r+"movie/"+arrItoa(id), "", 200).(map[string]any)
	if m["monitored"] != false || m["minimumAvailability"] != "released" {
		t.Fatalf("after editor: %v %v", m["monitored"], m["minimumAvailability"])
	}
	// Refusals: an unknown id changes nothing, an empty list and an
	// over-long list are refused, a bad apply mode is refused.
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{"PUT", "movie/editor", fmt.Sprintf(`{"movieIds":[%d,99999],"monitored":true}`, id), 404},
		{"PUT", "movie/editor", `{"movieIds":[]}`, 400},
		{"PUT", "movie/editor", fmt.Sprintf(`{"movieIds":[%d],"tags":[1],"applyTags":"swap"}`, id), 400},
		{"PUT", "movie/editor", `{"movieIds":[` + strings.Repeat("1,", 1000) + `1]}`, 400},
		{"DELETE", "movie/editor", `{"movieIds":[99999]}`, 404},
		{"POST", "movie/import", "[" + strings.Repeat(item(1)+",", 500) + item(1) + "]", 400},
		{"POST", "movie/import", `{}`, 400},
	} {
		if st, b := f.kometa(f.radarrKey, c.method, r+c.path, c.body); st != c.want {
			t.Fatalf("%s %s: %d, want %d: %s", c.method, c.path, st, c.want, b)
		}
	}
	if m := f.arrOK(f.radarrKey, "GET", r+"movie/"+arrItoa(id), "", 200).(map[string]any); m["monitored"] != false {
		t.Fatal("a refused editor call changed the movie")
	}
	// Delete: with the list exclusion the movie goes on the Never-add list.
	if st, b := f.kometa(f.radarrKey, "DELETE", r+"movie/editor", fmt.Sprintf(`{"movieIds":[%d],"deleteFiles":true,"addImportExclusion":true}`, id)); st != 200 {
		t.Fatalf("delete: %d %s", st, b)
	}
	arrList(t, "after delete", f.arrOK(f.radarrKey, "GET", r+"movie", "", 200), 0)
	if st, b := f.kometa(f.radarrKey, "POST", r+"movie/import", "["+item(693134)+"]"); st != 200 || strings.TrimSpace(b) != "[]" {
		t.Fatalf("import of a Never-add movie: %d %s", st, b)
	}
}

func TestOldAppKometaSeriesBulk(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	s := "/sonarr/api/v3/"
	_, b := f.kometa(f.sonarrKey, "POST", s+"tag", `{"label":"kometa"}`)
	tag := num(aiAny(t, b)["id"])
	body := fmt.Sprintf(`[{"title":"Lab Show","tvdbId":200,"qualityProfileId":%d,"rootFolderPath":%q,"monitored":true,"seasonFolder":true,"seriesType":"standard",
		"addOptions":{"monitor":"firstSeason","searchForMissingEpisodes":false},"tags":[]},
		{"title":"Gone","tvdbId":0,"qualityProfileId":%d,"rootFolderPath":%q}]`, f.profile(), f.showDir, f.profile(), f.showDir)
	st, b := f.kometa(f.sonarrKey, "POST", s+"series/import", body)
	if st != 200 {
		t.Fatalf("import: %d %s", st, b)
	}
	id := num(arrList(t, "import", kometaJSON(t, b), 1)[0].(map[string]any)["id"])
	if got := f.keptEpisodes(id); !slices.Equal(got, []string{"S1E1", "S1E2"}) {
		t.Fatalf("import kept %v", got)
	}
	st, b = f.kometa(f.sonarrKey, "PUT", s+"series/editor", fmt.Sprintf(`{"seriesIds":[%d],"monitored":false,"tags":[%d],"applyTags":"add","moveFiles":false}`, id, tag))
	if st != 202 {
		t.Fatalf("editor: %d %s", st, b)
	}
	got := arrList(t, "editor", kometaJSON(t, b), 1)[0].(map[string]any)
	if got["monitored"] != false || !slices.Equal(tagNums(got["tags"]), []int64{tag}) {
		t.Fatalf("editor answer: %v %v", got["monitored"], got["tags"])
	}
	// Season pass: per-season switches, then a choice for every season.
	for _, c := range []struct {
		body string
		want []string
	}{
		{fmt.Sprintf(`{"series":[{"id":%d,"monitored":true,"seasons":[{"seasonNumber":1,"monitored":false},{"seasonNumber":2,"monitored":true}]}]}`, id), []string{"S2E1", "S2E2"}},
		{fmt.Sprintf(`{"series":[{"id":%d,"monitored":true}],"monitoringOptions":{"monitor":"none"}}`, id), []string{}},
		{fmt.Sprintf(`{"series":[{"id":%d,"monitored":true}],"monitoringOptions":{"monitor":"all"}}`, id), []string{"S1E1", "S1E2", "S2E1", "S2E2"}},
	} {
		if st, b := f.kometa(f.sonarrKey, "POST", s+"seasonPass", c.body); st != 202 {
			t.Fatalf("season pass %s: %d %s", c.body, st, b)
		}
		if got := f.keptEpisodes(id); !slices.Equal(got, c.want) {
			t.Fatalf("season pass %s: kept %v, want %v", c.body, got, c.want)
		}
	}
	if st, b := f.kometa(f.sonarrKey, "POST", s+"seasonpass", `{"series":[{"id":99999,"monitored":true}]}`); st != 404 {
		t.Fatalf("season pass unknown show: %d %s", st, b)
	}
	// The Radarr key does not reach the Sonarr side.
	if st, _ := f.kometa(f.radarrKey, "PUT", s+"series/editor", fmt.Sprintf(`{"seriesIds":[%d],"monitored":true}`, id)); st != 401 && st != 403 {
		t.Fatalf("other side's key: %d", st)
	}
	if st, b := f.kometa(f.sonarrKey, "DELETE", s+"series/editor", fmt.Sprintf(`{"seriesIds":[%d],"deleteFiles":false,"addImportListExclusion":false}`, id)); st != 200 {
		t.Fatalf("delete: %d %s", st, b)
	}
	arrList(t, "after delete", f.arrOK(f.sonarrKey, "GET", s+"series", "", 200), 0)
}

func kometaJSON(t *testing.T, b string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(b), &v); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return v
}
