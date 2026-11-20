package migrate

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

const mib = int64(1) << 20

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// mbMin is MB a minute as the per-minute bytes a size limit holds.
func mbMin(mb float64) int64 { return int64(mb * float64(mib)) }

// The size tables of a real pair of apps: Sonarr allows far larger 1080p
// copies than Radarr, so one ready-made range cannot match both.
var (
	sonarrSizes = []QualitySize{{Name: "HDTV-720p", Min: 2, Max: 60}, {Name: "WEBDL-1080p", Min: 4, Max: 130}, {Name: "WEBRip-1080p", Min: 3, Max: 120},
		{Name: "Bluray-1080p", Min: 4, Max: 155}, {Name: "Remux-2160p", Min: 35}, {Name: "Unknown", Max: 199}}
	radarrSizes = []QualitySize{{Name: "WEBDL-1080p", Max: 68.7}, {Name: "Bluray-1080p", Max: 85.4}, {Name: "Remux-2160p", Max: 0}}
)

// TestSizesAreRead: the size table comes from /qualitydefinition or from a
// backup's QualityDefinitions; a backup whose table has no sizes gives none.
func TestSizesAreRead(t *testing.T) {
	u, key := sonarrAPIWith(t, false, map[string]any{"/api/v3/qualitydefinition": []any{
		map[string]any{"id": 1, "quality": map[string]any{"id": 3, "name": "WEBDL-1080p"}, "title": "Web 1080", "minSize": 4, "maxSize": 130, "preferredSize": 95},
		map[string]any{"id": 2, "quality": map[string]any{"id": 7, "name": "Bluray-1080p"}, "title": "Bluray-1080p", "minSize": 4.5, "maxSize": nil}}})
	s := read(t, api(Sonarr, u, key))
	want := []QualitySize{{Name: "WEBDL-1080p", Min: 4, Max: 130}, {Name: "Bluray-1080p", Min: 4.5}}
	if len(s.QualitySizes) != 2 || s.QualitySizes[0] != want[0] || s.QualitySizes[1] != want[1] {
		t.Fatalf("api %+v", s.QualitySizes)
	}
	var stmts []string
	for _, st := range sonarrDB(false) {
		if !strings.Contains(st, "QualityDefinitions") {
			stmts = append(stmts, st)
		}
	}
	p := filepath.Join(t.TempDir(), "sonarr.db")
	makeDB(t, p, append(stmts, `CREATE TABLE QualityDefinitions (Id INTEGER PRIMARY KEY, Quality INTEGER, Title TEXT, MinSize REAL, MaxSize REAL, PreferredSize REAL)`,
		`INSERT INTO QualityDefinitions VALUES (1, 3, 'WEBDL-1080p', 4, 130, 95), (2, 7, 'Bluray-1080p', 4.5, NULL, NULL)`)...)
	s = read(t, file(Sonarr, p))
	if len(s.QualitySizes) != 2 || s.QualitySizes[0] != want[0] || s.QualitySizes[1] != want[1] {
		t.Fatalf("backup %+v", s.QualitySizes)
	}
	if prof := s.QualityProfiles[0]; prof.Cutoff != "WEBDL-1080p" {
		t.Fatalf("the size table no longer names the profile's qualities: %+v", prof)
	}
	p = filepath.Join(t.TempDir(), "sonarr.db")
	makeDB(t, p, sonarrDB(false)...)
	if s = read(t, file(Sonarr, p)); len(s.QualitySizes) != 0 {
		t.Fatalf("a table without sizes %+v", s.QualitySizes)
	}
}

// sizesPair is a Sonarr and a Radarr, each with an HD-1080p profile and a
// title using it, and their own size tables.
func sizesPair() (sonarr, radarr *Snapshot) {
	prof := Profile{ID: 4, Name: "HD-1080p", UpgradeAllowed: true, Cutoff: "Bluray-1080p", Allowed: []string{"WEBDL-1080p", "Bluray-1080p"}}
	sonarr = &Snapshot{Kind: Sonarr, Folders: []Folder{{Path: "/tv"}}, QualityProfiles: []Profile{prof}, QualitySizes: sonarrSizes,
		Titles: []Title{{SourceID: 1, Kind: "show", Name: "Lost", IDs: map[string]string{"tvdb": "73739"}, Path: "/tv/Lost", Monitored: true, ProfileID: 4}}}
	radarr = &Snapshot{Kind: Radarr, Folders: []Folder{{Path: "/movies"}}, QualityProfiles: []Profile{prof}, QualitySizes: radarrSizes,
		Titles: []Title{{SourceID: 1, Kind: "movie", Name: "The Matrix", IDs: map[string]string{"tmdb": "603"}, Path: "/movies/The Matrix (1999)", Monitored: true, ProfileID: 4}}}
	return sonarr, radarr
}

// TestPlanBringsSizes: each app's profile becomes a copy of Standard with
// that app's sizes per step, named apart and used by that app's titles;
// the Import step shows the sizes. Off, both stay on Standard as before.
func TestPlanBringsSizes(t *testing.T) {
	sonarr, radarr := sizesPair()
	p := BuildPlan([]Input{{1, radarr}, {2, sonarr}}, Choices{})
	rit, sit := item(t, p, GroupRules, "pick_rules:1:radarr:4"), item(t, p, GroupRules, "pick_rules:2:sonarr:4")
	r, s := rit.Rules, sit.Rules
	if r.Name != "HD-1080p" || s.Name != "HD-1080p (sonarr)" || r.CopyOf != "Standard" || s.CopyOf != "Standard" || !r.Sizes || !s.Sizes {
		t.Fatalf("rules %+v / %+v", r, s)
	}
	if r.Kinds[0] != "movie" || s.Kinds[0] != "show" || r.ProfileKey != "radarr:backup:4" || len(r.Traits) != 0 {
		t.Fatalf("kinds %v %v key %q", r.Kinds, s.Kinds, r.ProfileKey)
	}
	if rit.Detail != "becomes Pick rules HD-1080p, a copy of Standard with the old app's sizes" ||
		!strings.Contains(sit.Why, "sizes from the old app: 720p TV 0.12 to 3.52 GB an hour, 1080p web 0.18 to 7.62 GB an hour, 1080p Blu-ray 0.23 to 9.08 GB an hour") {
		t.Fatalf("shown as %q / %q", rit.Detail, sit.Why)
	}
	std := mediafiles.GoodEveryday()
	sl, rl := s.Body.SizeLimits, r.Body.SizeLimits
	if sl["1080p_web"] != (mediafiles.SizeLimit{Min: 3 * mib, Max: 130 * mib, PerMinute: true}) || // WEBDL and WEBRip: the widest pair
		sl["1080p_bluray"] != (mediafiles.SizeLimit{Min: 4 * mib, Max: 155 * mib, PerMinute: true}) ||
		sl["2160p_remux"] != (mediafiles.SizeLimit{Min: 35 * mib, Max: library.NoSizeMax, PerMinute: true}) ||
		sl["*"] != std.SizeLimits["*"] {
		t.Fatalf("sonarr sizes %+v", sl)
	}
	if rl["1080p_web"] != (mediafiles.SizeLimit{Max: mbMin(68.7), PerMinute: true}) || rl["1080p_bluray"] != (mediafiles.SizeLimit{Max: mbMin(85.4), PerMinute: true}) ||
		rl["2160p_remux"] != (mediafiles.SizeLimit{PerMinute: true}) || len(rl) != 4 {
		t.Fatalf("radarr sizes %+v", rl)
	}
	// The rest is Standard as it comes.
	body := *s.Body
	body.SizeLimits = std.SizeLimits
	if b1, b2 := mustJSON(t, body), mustJSON(t, std); b1 != b2 {
		t.Fatalf("not a copy of Standard:\n%s\n%s", b1, b2)
	}
	for _, rp := range []*RulesPlan{r, s} {
		if err := mediafiles.CheckPickRules(rp.Name, rp.Kinds, *rp.Body); err != nil {
			t.Fatalf("%s refused: %v", rp.Name, err)
		}
	}
	if m := item(t, p, GroupTitles, "title:movie:tmdb:603").Title; m.Preset != "HD-1080p" {
		t.Fatalf("movie uses %q", m.Preset)
	}
	if sh := item(t, p, GroupTitles, "title:show:tvdb:73739").Title; sh.Preset != "HD-1080p (sonarr)" {
		t.Fatalf("show uses %q", sh.Preset)
	}
	if n, ok := notMoved(p, "sonarr quality_sizes[5]"); !ok || n.Why != "no HolyArrs quality step matches it" {
		t.Fatalf("unknown quality %+v", n)
	}
	if !accounted(p, "radarr quality_sizes[0]") || !accounted(p, "sonarr quality_sizes[1]") {
		t.Fatal("brought sizes not accounted")
	}

	// Off: today's ready-made match, and each row listed as left behind.
	off := BuildPlan([]Input{{1, radarr}, {2, sonarr}}, Choices{NoSizes: true})
	for _, key := range []string{"pick_rules:1:radarr:4", "pick_rules:2:sonarr:4"} {
		if it := item(t, off, GroupRules, key); it.Rules.Body != nil || it.Rules.Preset != "Standard" || it.Detail != "becomes Standard" {
			t.Fatalf("sizes off %+v", it)
		}
	}
	if m := item(t, off, GroupTitles, "title:movie:tmdb:603").Title; m.Preset != "Standard" {
		t.Fatalf("sizes off, movie uses %q", m.Preset)
	}
	if n, ok := notMoved(off, "radarr quality_sizes[1]"); !ok || !strings.Contains(n.Why, "you left size limits behind") {
		t.Fatalf("sizes off %+v", n)
	}
}

// TestPlanSizesSameAsPreset: a table that matches the ready-made sizes on
// every step the profile takes changes nothing, so the match stays.
func TestPlanSizesSameAsPreset(t *testing.T) {
	_, radarr := sizesPair()
	radarr.QualitySizes = nil
	for _, q := range []string{"HDTV-720p", "WEBDL-720p", "Bluray-720p", "HDTV-1080p", "WEBDL-1080p", "Bluray-1080p"} {
		radarr.QualitySizes = append(radarr.QualitySizes, QualitySize{Name: q, Min: 512.0 / 60, Max: 6 * 1024.0 / 60})
	}
	// A step Standard does not take may differ.
	radarr.QualitySizes = append(radarr.QualitySizes, QualitySize{Name: "Remux-2160p", Min: 100})
	p := BuildPlan([]Input{{1, radarr}}, Choices{})
	if it := item(t, p, GroupRules, "pick_rules:1:radarr:4"); it.Rules.Body != nil || it.Rules.Preset != "Standard" {
		t.Fatalf("same sizes %+v", it.Rules)
	}
	if !accounted(p, "radarr quality_sizes[6]") {
		t.Fatal("a row the match already has is not accounted")
	}
}

// TestPlanSizesWithFormats: a profile that brings its formats takes its
// app's sizes on the same Pick rules, not on a second copy.
func TestPlanSizesWithFormats(t *testing.T) {
	radarr := &Snapshot{Kind: Radarr}
	withFormats(radarr, nil, nil)
	radarr.QualitySizes = []QualitySize{{Name: "WEBDL-2160p", Min: 10, Max: 400}, {Name: "Remux-2160p", Min: 50}}
	p := BuildPlan([]Input{{1, radarr}}, Choices{})
	if n := len(p.Group(GroupRules).Items); n != 1 {
		t.Fatalf("%d rules items", n)
	}
	it := item(t, p, GroupRules, "pick_rules:1:radarr:2")
	rp := it.Rules
	if rp.CopyOf != "" || !rp.Sizes || len(rp.Traits) != 2 || it.Detail != "becomes Pick rules Ultra-HD · 2 traits" {
		t.Fatalf("rules %+v", it)
	}
	if rp.Body.SizeLimits["2160p_web"] != (mediafiles.SizeLimit{Min: 10 * mib, Max: 400 * mib, PerMinute: true}) || len(rp.Body.SizeLimits) != 2 {
		t.Fatalf("sizes %+v", rp.Body.SizeLimits)
	}
	if strings.Contains(it.Why, "size limits are not brought") || !strings.Contains(it.Why, "2160p web 0.59 to 23.44 GB an hour") {
		t.Fatalf("why %q", it.Why)
	}
	// Sizes off, the formats path is as before: no size limits.
	off := item(t, BuildPlan([]Input{{1, radarr}}, Choices{NoSizes: true}), GroupRules, "pick_rules:1:radarr:2")
	if off.Rules.Sizes || len(off.Rules.Body.SizeLimits) != 0 || !strings.Contains(off.Why, "size limits are not brought") {
		t.Fatalf("sizes off %+v", off)
	}
}

// TestPlanCopyNeverTakesPresetName: a profile named like a ready-made
// Pick rules gets another name, so other titles keep the ready-made ones.
func TestPlanCopyNeverTakesPresetName(t *testing.T) {
	_, radarr := sizesPair()
	radarr.QualityProfiles[0].Name = "Standard"
	rp := item(t, BuildPlan([]Input{{1, radarr}}, Choices{}), GroupRules, "pick_rules:1:radarr:4").Rules
	if rp.Name != "Standard (radarr)" {
		t.Fatalf("named %q", rp.Name)
	}
}

// TestMoveBringsSizes: the move makes one copy of the ready-made Pick
// rules per profile a title uses, with that app's sizes and kind; the
// title uses it; it judges as the ready-made ones do; a profile nothing
// uses gets no copy; a second move makes no more.
func TestMoveBringsSizes(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.QualitySizes = []QualitySize{{Name: "WEBDL-2160p", Max: 250}, {Name: "Bluray-2160p", Max: 300}, {Name: "Remux-2160p", Max: 400}}
	})
	ctx := context.Background()
	sonarr, _ := sizesPair()
	sonarr.Titles, sonarr.Folders, sonarr.Via = nil, nil, ViaAPI
	sonarr.finish()
	saved, err := Save(ctx, l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	mv := l.move(t, Choices{})
	rkey := "pick_rules:" + strconv.FormatInt(l.snaps[0], 10) + ":radarr:2"
	if got := lineOf(t, mv.Report, rkey); got.Result != ResCreated || !strings.Contains(got.Detail, "a copy of 4K with the old app's sizes") {
		t.Fatalf("move %+v", got)
	}
	rules := func(name string) mediafiles.PickRules {
		t.Helper()
		pr, err := database.Select[mediafiles.PickRules](ctx, l.st.Read(), "pick_rules", "name = ?", name)
		if err != nil || len(pr) != 1 {
			t.Fatalf("pick rules %q: %+v %v", name, pr, err)
		}
		return pr[0]
	}
	movie := rules("Ultra-HD")
	if movie.Body.Synced == nil || movie.Body.Synced.From != mediafiles.SourceImportSizes || movie.Body.Synced.OldApp() || movie.Kinds[0] != "movie" {
		t.Fatalf("copy %+v", movie)
	}
	// The Sonarr here brings no titles, so its profile is not copied.
	if n := l.count(t, "SELECT count(*) FROM pick_rules WHERE name = 'HD-1080p'"); n != 0 {
		t.Fatalf("an unused profile got %d copies", n)
	}
	if movie.Body.SizeLimits["2160p_remux"] != (mediafiles.SizeLimit{Max: 400 * mib, PerMinute: true}) || movie.Body.GoodEnoughAt != mediafiles.FourK().GoodEnoughAt {
		t.Fatalf("movie rules %+v", movie.Body)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE pick_rules_id = ?", movie.ID); n == 0 {
		t.Fatal("the movie does not use its copy")
	}
	// Moving again updates the same copies.
	if again := l.move(t, Choices{}); lineOf(t, again.Report, rkey).Result != ResChanged {
		t.Fatalf("again %+v", lineOf(t, again.Report, rkey))
	}
	if n := l.count(t, "SELECT count(*) FROM pick_rules WHERE json_extract(body, '$.synced.from') = 'import_sizes'"); n != 1 {
		t.Fatalf("%d copies after a second move", n)
	}
}

// TestMoveSizesOff: left behind, no copy is made and the movie uses 4K.
func TestMoveSizesOff(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.QualitySizes = []QualitySize{{Name: "Remux-2160p", Max: 400}}
	})
	l.move(t, Choices{NoSizes: true})
	if n := l.count(t, "SELECT count(*) FROM pick_rules WHERE json_extract(body, '$.synced.from') = 'import_sizes'"); n != 0 {
		t.Fatalf("%d copies", n)
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN pick_rules p ON p.id = v.pick_rules_id WHERE p.name = '4K'"); n == 0 {
		t.Fatal("the movie does not use 4K")
	}
}
