package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The payloads below are shaped as the Profilarr manual describes them:
// only the keys it sends, fields as name and value pairs, the rename switch
// left out when off, every quality of the side listed best last with
// groups under made-up ids from 1001, and a score for every format on the
// side, zeros included.

func spec(name, impl string, negate, required bool, fields ...any) map[string]any {
	fs := []any{}
	for i := 0; i+1 < len(fields); i += 2 {
		fs = append(fs, map[string]any{"name": fields[i], "value": fields[i+1]})
	}
	return map[string]any{"name": name, "implementation": impl, "negate": negate, "required": required, "fields": fs}
}

func formatBody(id int64, name string, rename bool, specs ...map[string]any) string {
	m := map[string]any{"name": name, "specifications": specs}
	if specs == nil {
		m["specifications"] = []any{}
	}
	if rename {
		m["includeCustomFormatWhenRenaming"] = true
	}
	if id > 0 {
		m["id"] = id
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// radarrProfile is a rule set as Profilarr pushes it to Radarr: WEB 1080p
// as a group (id 1001) that is also the cutoff, Bluray-1080p alone, and
// every other quality off. scores maps format id to score.
func radarrProfile(id int64, name string, scores map[int64]int, names map[int64]string) string {
	items := []any{}
	for _, q := range sideQualities["radarr"] {
		switch q.ID {
		case 3, 15, 7:
			continue
		}
		items = append(items, map[string]any{"quality": map[string]any{"id": q.ID, "name": q.Name}, "items": []any{}, "allowed": false})
	}
	items = append(items,
		map[string]any{"id": 1001, "name": "WEB 1080p", "allowed": true, "items": []any{
			map[string]any{"quality": map[string]any{"id": 3, "name": "WEBDL-1080p"}, "items": []any{}, "allowed": true},
			map[string]any{"quality": map[string]any{"id": 15, "name": "WEBRip-1080p"}, "items": []any{}, "allowed": true}}},
		map[string]any{"quality": map[string]any{"id": 7, "name": "Bluray-1080p"}, "items": []any{}, "allowed": true})
	fi := []any{}
	for fid, s := range scores {
		fi = append(fi, map[string]any{"format": fid, "name": names[fid], "score": s})
	}
	m := map[string]any{"name": name, "items": items, "language": map[string]any{"id": 2, "name": "French"},
		"upgradeAllowed": true, "cutoff": 1001, "minFormatScore": -50, "cutoffFormatScore": 2000, "minUpgradeFormatScore": 1,
		"formatItems": fi}
	if id > 0 {
		m["id"] = id
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func decodeObj(t *testing.T, code int, body string, want int) map[string]any {
	t.Helper()
	if code != want {
		t.Fatalf("want %d, got %d %s", want, code, body)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return m
}

func decodeList(t *testing.T, code int, body string) []map[string]any {
	t.Helper()
	if code != 200 {
		t.Fatalf("list: %d %s", code, body)
	}
	var l []map[string]any
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return l
}

func (h *hookRig) list(side, path string) []map[string]any {
	h.t.Helper()
	st, body := h.do(side, "GET", path, "")
	return decodeList(h.t, st, body)
}

func byName(l []map[string]any, name string) map[string]any {
	for _, m := range l {
		if m["name"] == name {
			return m
		}
	}
	return nil
}

func (h *hookRig) trait(id int64) mediafiles.OwnTrait {
	h.t.Helper()
	var tr mediafiles.OwnTrait
	if err := database.Get(context.Background(), h.st.Read(), "own_traits", id, &tr); err != nil {
		h.t.Fatalf("trait %d: %v", id, err)
	}
	return tr
}

func (h *hookRig) rules(id int64) mediafiles.PickRules {
	h.t.Helper()
	var p mediafiles.PickRules
	if err := database.Get(context.Background(), h.st.Read(), "pick_rules", id, &p); err != nil {
		h.t.Fatalf("pick rules %d: %v", id, err)
	}
	return p
}

func (h *hookRig) formatTrait(side string, id int64) int64 {
	h.t.Helper()
	f, err := formatRow(context.Background(), h.st.Read(), side, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return f.TraitID
}

// A format comes in, lists back as sent and becomes a trait with the same
// tests; a second push of the same name is refused, as there.
func TestFormats_PushListAndTrait(t *testing.T) {
	h := newHookRig(t)
	if l := h.list("radarr", "customformat"); len(l) != 0 {
		t.Fatalf("first list: %v", l)
	}
	body := formatBody(0, "x265 (HD)", false,
		spec("x265", "ReleaseTitleSpecification", false, false, "value", `[xh][ ._-]?265|\bHEVC(\b|\d)`),
		spec("Not 2160p", "ResolutionSpecification", true, true, "value", 2160))
	st, out := h.do("radarr", "POST", "customformat", body)
	m := decodeObj(t, st, out, 201)
	id := int64(m["id"].(float64))
	l := h.list("radarr", "customformat")
	got := byName(l, "x265 (HD)")
	if got == nil || got["includeCustomFormatWhenRenaming"] != false || int64(got["id"].(float64)) != id {
		t.Fatalf("listed: %v", l)
	}
	sp := got["specifications"].([]any)
	if len(sp) != 2 || sp[1].(map[string]any)["fields"].([]any)[0].(map[string]any)["value"] != float64(2160) {
		t.Fatalf("tests not as sent: %v", sp)
	}
	tr := h.trait(h.formatTrait("radarr", id))
	if tr.Source != mediafiles.SourceArrAPI || tr.SourceID != fmt.Sprintf("radarr:%d", id) || len(tr.Specs) != 2 ||
		tr.Specs[1].Kind != "resolution" || !tr.Specs[1].Negate || !tr.Specs[1].Required {
		t.Fatalf("trait: %+v", tr)
	}
	if st, out := h.do("radarr", "POST", "customformat", body); st != 400 {
		t.Fatalf("same name again: %d %s", st, out)
	}
	// The other side keeps its own list.
	if l := h.list("sonarr", "customformat"); len(l) != 0 {
		t.Fatalf("sonarr sees radarr's: %v", l)
	}
	// An update keeps the id and the trait, with the new tests.
	st, out = h.do("radarr", "PUT", fmt.Sprintf("customformat/%d", id), formatBody(id, "x265 (HD)", true,
		spec("x265", "ReleaseTitleSpecification", false, false, "value", `x265`)))
	if st != 202 {
		t.Fatalf("put: %d %s", st, out)
	}
	tr2 := h.trait(h.formatTrait("radarr", id))
	if tr2.ID != tr.ID || len(tr2.Specs) != 1 || tr2.Specs[0].Value != "x265" {
		t.Fatalf("after put: %+v", tr2)
	}
	if got := byName(h.list("radarr", "customformat"), "x265 (HD)"); got["includeCustomFormatWhenRenaming"] != true {
		t.Fatalf("rename switch: %v", got)
	}
}

// Language tests read the side's ids; a test HolyArrs cannot run keeps the
// format but makes no trait, with the reason kept; no tests fit everything.
func TestFormats_KindsAndSkips(t *testing.T) {
	h := newHookRig(t)
	st, out := h.do("radarr", "POST", "customformat", formatBody(0, "Not French", false,
		spec("French", "LanguageSpecification", false, true, "value", 2, "exceptLanguage", true)))
	fr := int64(decodeObj(t, st, out, 201)["id"].(float64))
	if tr := h.trait(h.formatTrait("radarr", fr)); len(tr.Specs) != 1 || tr.Specs[0].Kind != "language" || tr.Specs[0].Value != "!fr" {
		t.Fatalf("language: %+v", tr)
	}
	// Sonarr numbers Arabic 26; Radarr calls 26 Hindi.
	st, out = h.do("sonarr", "POST", "customformat", formatBody(0, "Arabic", false, spec("ar", "LanguageSpecification", false, false, "value", 26)))
	ar := int64(decodeObj(t, st, out, 201)["id"].(float64))
	if tr := h.trait(h.formatTrait("sonarr", ar)); tr.Specs[0].Value != "ar" {
		t.Fatalf("sonarr language: %+v", tr)
	}
	// Year is a Radarr test; on Sonarr it is kept and skipped.
	st, out = h.do("sonarr", "POST", "customformat", formatBody(0, "Old", false, spec("2000s", "YearSpecification", false, false, "min", 2000, "max", 2009)))
	old := int64(decodeObj(t, st, out, 201)["id"].(float64))
	f, err := formatRow(context.Background(), h.st.Read(), "sonarr", old)
	if err != nil || f.TraitID != 0 || !strings.Contains(f.Skipped, "year") {
		t.Fatalf("skipped: %+v %v", f, err)
	}
	if l := h.list("sonarr", "customformat"); byName(l, "Old") == nil {
		t.Fatal("a skipped format must still list back")
	}
	// The settings answer lists it with its reason.
	pushed, err := (&Handlers{Deps: Deps{Store: h.st}}).pushed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range pushed["formats"].([]PushedFormat) {
		found = found || f.Side == "sonarr" && f.Name == "Old" && strings.Contains(f.Skipped, "year") && f.TraitID == 0 && f.UpdatedAt != ""
	}
	if !found {
		t.Fatalf("settings answer: %+v", pushed)
	}
	st, out = h.do("radarr", "POST", "customformat", formatBody(0, "Everything", false))
	all := int64(decodeObj(t, st, out, 201)["id"].(float64))
	if tr := h.trait(h.formatTrait("radarr", all)); len(tr.Specs) != 0 || !tr.OldApp() {
		t.Fatalf("empty: %+v", tr)
	}
	// A format turned unreadable drops its trait.
	st, _ = h.do("radarr", "PUT", fmt.Sprintf("customformat/%d", fr), formatBody(fr, "Not French", false,
		spec("x", "IndexerFlagSpecification", false, false, "value", 1)))
	if st != 202 || h.formatTrait("radarr", fr) != 0 {
		t.Fatalf("now skipped: %d", st)
	}
}

// pushAll sends three formats and a profile scoring them to Radarr.
func pushAll(t *testing.T, h *hookRig) (fmts map[string]int64, profile int64) {
	t.Helper()
	fmts = map[string]int64{}
	for _, f := range []struct{ name, pat string }{{"Good group", "GRP"}, {"Unused", "zzz"}, {"Banned", "BAD"}} {
		st, out := h.do("radarr", "POST", "customformat", formatBody(0, f.name, false, spec(f.name, "ReleaseGroupSpecification", false, false, "value", f.pat)))
		fmts[f.name] = int64(decodeObj(t, st, out, 201)["id"].(float64))
	}
	names := map[int64]string{}
	for n, id := range fmts {
		names[id] = n
	}
	st, out := h.do("radarr", "POST", "qualityprofile", radarrProfile(0, "HD Bluray + WEB",
		map[int64]int{fmts["Good group"]: 1500, fmts["Unused"]: 0, fmts["Banned"]: -999999}, names))
	return fmts, int64(decodeObj(t, st, out, 201)["id"].(float64))
}

// A profile becomes Pick rules for movies: its qualities and cutoff as
// steps, its scores as trait points (as sent), its score settings and its
// language; the list answers it as sent, with raw scores.
func TestProfiles_PushBecomesPickRules(t *testing.T) {
	h := newHookRig(t)
	fmts, id := pushAll(t, h)
	p := h.rules(id)
	b := p.Body
	if !slices.Equal(p.Kinds, []string{"movie"}) || b.Synced == nil || b.Synced.From != mediafiles.SourceArrAPI || !b.Synced.OldApp() {
		t.Fatalf("rules: %+v", p)
	}
	var allowed []string
	for _, s := range b.Steps {
		if s.Allowed {
			allowed = append(allowed, s.Quality)
		}
	}
	if !slices.Equal(allowed, []string{"1080p_web", "1080p_bluray"}) || b.GoodEnoughAt != "1080p_web" {
		t.Fatalf("steps %v, good enough %s", allowed, b.GoodEnoughAt)
	}
	if b.MinPoints != -50 || b.GoodEnoughPoints == nil || *b.GoodEnoughPoints != 2000 || b.UpgradeMinPoints != 1 || b.Order != "quality_then_points" {
		t.Fatalf("points: %+v", b)
	}
	if !slices.Equal(b.Language.Wanted, []string{"fr"}) {
		t.Fatalf("language: %+v", b.Language)
	}
	points := map[string]int{}
	for _, tp := range b.Traits {
		points[tp.Trait] = tp.Points
	}
	good, banned := mediafiles.OwnTraitKey(h.formatTrait("radarr", fmts["Good group"])), mediafiles.OwnTraitKey(h.formatTrait("radarr", fmts["Banned"]))
	if len(points) != 2 || points[good] != 1500 || points[banned] != -999999 {
		t.Fatalf("traits: %v", b.Traits)
	}
	l := h.list("radarr", "qualityprofile")
	got := byName(l, "HD Bluray + WEB")
	if st, one := h.do("radarr", "GET", fmt.Sprintf("qualityprofile/%d", id), ""); st != 200 || !strings.Contains(one, `"HD Bluray + WEB"`) {
		t.Fatalf("get one: %d %s", st, one)
	}
	if st, _ := h.do("radarr", "GET", "qualityprofile/999999", ""); st != 404 {
		t.Fatalf("get missing: %d", st)
	}
	if got == nil || int64(got["id"].(float64)) != id || got["cutoff"] != float64(1001) || got["minFormatScore"] != float64(-50) ||
		got["language"].(map[string]any)["name"] != "French" {
		t.Fatalf("listed: %v", got)
	}
	items := got["items"].([]any)
	if len(items) != len(sideQualities["radarr"])-1 || items[len(items)-2].(map[string]any)["id"] != float64(1001) {
		t.Fatalf("items not as sent: %v", items)
	}
	scores := map[string]float64{}
	for _, fi := range got["formatItems"].([]any) {
		m := fi.(map[string]any)
		scores[m["name"].(string)] = m["score"].(float64)
	}
	if len(scores) != 3 || scores["Banned"] != -999999 || scores["Unused"] != 0 || scores["Good group"] != 1500 {
		t.Fatalf("raw scores: %v", scores)
	}
	// Ready-made rules list too, with the wire fields, never as sent rules.
	std := byName(l, mediafiles.GoodEverydayName)
	if std == nil || std["items"] == nil || std["formatItems"] == nil || std["language"] == nil {
		t.Fatalf("preset: %v", std)
	}
	// The Sonarr side neither lists Radarr's profile nor reports version 5.
	if byName(h.list("sonarr", "qualityprofile"), "HD Bluray + WEB") != nil {
		t.Fatal("sonarr lists radarr's profile")
	}
	if !strings.HasPrefix(SonarrVersion, "4.") {
		t.Fatalf("sonarr version %s", SonarrVersion)
	}
}

// A later sync updates the same Pick rules; one name on both sides makes
// two Pick rules, each listed under the name sent.
func TestProfiles_UpdateAndBothSides(t *testing.T) {
	h := newHookRig(t)
	fmts, id := pushAll(t, h)
	names := map[int64]string{}
	for n, fid := range fmts {
		names[fid] = n
	}
	st, out := h.do("radarr", "PUT", fmt.Sprintf("qualityprofile/%d", id), radarrProfile(id, "HD Bluray + WEB", map[int64]int{fmts["Good group"]: 10}, names))
	if st != 202 {
		t.Fatalf("put: %d %s", st, out)
	}
	if b := h.rules(id).Body; len(b.Traits) != 1 || b.Traits[0].Points != 10 {
		t.Fatalf("after put: %v", b.Traits)
	}
	son := strings.Replace(radarrProfile(0, "HD Bluray + WEB", nil, nil), `"language":{"id":2,"name":"French"},`, "", 1)
	st, out = h.do("sonarr", "POST", "qualityprofile", son)
	sid := int64(decodeObj(t, st, out, 201)["id"].(float64))
	if sid == id || h.rules(sid).Name == h.rules(id).Name || !slices.Equal(h.rules(sid).Kinds, []string{"show"}) {
		t.Fatalf("sonarr rules: %+v", h.rules(sid))
	}
	if byName(h.list("sonarr", "qualityprofile"), "HD Bluray + WEB") == nil {
		t.Fatal("sonarr lists its own under the sent name")
	}
	// Rules made in HolyArrs are listed, so a profile of their name is
	// refused, and they are never changed here.
	var preset int64
	if err := h.st.Read().QueryRow("SELECT id FROM pick_rules WHERE preset = 1 ORDER BY id LIMIT 1").Scan(&preset); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.do("radarr", "POST", "qualityprofile", radarrProfile(0, h.rules(preset).Name, nil, nil)); st != 400 {
		t.Fatalf("preset name: %d", st)
	}
	if st, _ := h.do("radarr", "PUT", fmt.Sprintf("qualityprofile/%d", preset), radarrProfile(preset, "x", nil, nil)); st != 409 {
		t.Fatalf("put preset: %d", st)
	}
	if st, _ := h.do("sonarr", "PUT", fmt.Sprintf("qualityprofile/%d", id), radarrProfile(id, "x", nil, nil)); st != 404 {
		t.Fatalf("other side's: %d", st)
	}
}

// Delete takes only what was sent here: rules in use answer 500 (read as
// "in use"), ready-made and hand-made rules 409, hand-made traits 404. A
// removed format leaves the Pick rules it scored.
func TestProfiles_DeleteGuards(t *testing.T) {
	h := newHookRig(t)
	fmts, id := pushAll(t, h)
	var preset int64
	if err := h.st.Read().QueryRow("SELECT id FROM pick_rules WHERE preset = 1 ORDER BY id LIMIT 1").Scan(&preset); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("qualityprofile/%d", preset), ""); st != 409 {
		t.Fatalf("preset delete: %d", st)
	}
	folder := h.exec(`INSERT INTO library_folders (path, kind, label, default_pick_rules_id, health, same_disk_as, created_at, created_by, updated_at, updated_by)
		VALUES ('/m', 'movie', 'Movies', ?, 'ok', '[]', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`, id)
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("qualityprofile/%d", id), ""); st != 500 {
		t.Fatalf("in use: %d", st)
	}
	h.exec("DELETE FROM library_folders WHERE id = ?", folder)
	good := h.formatTrait("radarr", fmts["Good group"])
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("customformat/%d", fmts["Good group"]), ""); st != 200 {
		t.Fatalf("format delete: %d", st)
	}
	for _, tp := range h.rules(id).Body.Traits {
		if tp.Trait == mediafiles.OwnTraitKey(good) {
			t.Fatal("removed format still scores")
		}
	}
	if byName(h.list("radarr", "qualityprofile"), "HD Bluray + WEB")["formatItems"].([]any) == nil {
		t.Fatal("no format scores")
	}
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("qualityprofile/%d", id), ""); st != 200 {
		t.Fatalf("delete: %d", st)
	}
	if _, err := profileRow(context.Background(), h.st.Read(), id); err != database.ErrNotFound {
		t.Fatalf("row stays: %v", err)
	}
	// A trait made by hand is never listed or removed as a format.
	hand := h.exec(`INSERT INTO own_traits (name, match, terms, created_at, created_by, updated_at, updated_by)
		VALUES ('Mine', 'name', '["x"]', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`)
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("customformat/%d", hand), ""); st != 404 {
		t.Fatalf("hand-made trait: %d", st)
	}
	if byName(h.list("radarr", "customformat"), "Mine") != nil {
		t.Fatal("hand-made trait listed")
	}
	h.trait(hand)
}

// Writes need a key whose maker may change settings; reads do not.
func TestRules_NeedSettingsPermission(t *testing.T) {
	h := newHookRig(t)
	h.caller = webserver.Caller{UserID: 2, Name: "staff", Via: "api_key", Role: "member", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	for _, c := range [][2]string{{"POST", "customformat"}, {"PUT", "customformat/1"}, {"DELETE", "customformat/1"},
		{"POST", "qualityprofile"}, {"PUT", "qualityprofile/1"}, {"DELETE", "qualityprofile/1"}} {
		if st, _ := h.do("radarr", c[0], c[1], formatBody(0, "a", false)); st != 403 {
			t.Errorf("%s %s: %d", c[0], c[1], st)
		}
	}
	for _, p := range []string{"customformat", "qualityprofile"} {
		if st, _ := h.do("radarr", "GET", p, ""); st != 200 {
			t.Errorf("list %s: %d", p, st)
		}
	}
}
