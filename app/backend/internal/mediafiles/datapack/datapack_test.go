package datapack

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

//go:embed testdata/synthetic-pack.json
var testdata embed.FS

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := testdata.ReadFile("testdata/synthetic-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeRoundTripAndReleaseFacts(t *testing.T) {
	p, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "home-cinema" || len(p.PickRules) != 1 || len(p.Naming) != 1 || len(p.ReleaseTests) != 1 {
		t.Fatalf("decoded pack: %+v", p)
	}
	b, err := Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := Encode(p2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b2) {
		t.Fatalf("export is not deterministic\n%s\n%s", b, b2)
	}
}

func TestDecodeRejectsStrictAndBoundedInputs(t *testing.T) {
	valid := string(fixture(t))
	tests := map[string][]byte{
		"unknown field":    []byte(strings.Replace(valid, `"version": 1,`, `"version": 1, "script": "run",`, 1)),
		"duplicate key":    []byte(strings.Replace(valid, `"id": "home-cinema",`, `"id": "home-cinema", "id": "other",`, 1)),
		"trailing value":   append(append([]byte{}, fixture(t)...), []byte(` {}`)...),
		"too large":        bytes.Repeat([]byte(" "), MaxBytes+1),
		"unsafe id":        []byte(strings.Replace(valid, `"id": "home-cinema"`, `"id": "../home"`, 1)),
		"unknown quality":  []byte(strings.Replace(valid, `"1080p_web"`, `"ultra_web"`, 1)),
		"unknown trait":    []byte(strings.Replace(valid, `"hevc"`, `"magic_codec"`, 1)),
		"release mismatch": []byte(strings.Replace(valid, `"quality": "1080p_web"`, `"quality": "2160p_web"`, 1)),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(raw); err == nil {
				t.Fatal("accepted invalid pack")
			}
		})
	}
}

func TestDecodeRejectsNumericAndCollectionCaps(t *testing.T) {
	base, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []Pack{base, base, base}
	tests[0].PickRules = slices.Clone(base.PickRules)
	tests[0].PickRules[0].Rules.Traits = slices.Clone(base.PickRules[0].Rules.Traits)
	tests[0].PickRules[0].Rules.Traits[0].Points = 10001
	tests[1].PickRules = slices.Clone(base.PickRules)
	limits := maps.Clone(base.PickRules[0].Rules.SizeLimits)
	limits["1080p_web"] = mediafiles.SizeLimit{Min: 10, Max: -1}
	tests[1].PickRules[0].Rules.SizeLimits = limits
	tests[2].PickRules = make([]PickRule, MaxPickRules+1)
	for i := range tests[2].PickRules {
		tests[2].PickRules[i] = base.PickRules[0]
		tests[2].PickRules[i].ID = fmt.Sprintf("rule-%02d", i)
	}
	for i, pack := range tests {
		if err := pack.Validate(); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestValidationRejectsCanonicalEncodingOverLimit(t *testing.T) {
	p, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	p.ReleaseTests = make([]ReleaseTest, MaxReleaseTests)
	for i := range p.ReleaseTests {
		p.ReleaseTests[i] = p.ReleaseTests[0]
		p.ReleaseTests[i] = ReleaseTest{
			ID:          fmt.Sprintf("release-%03d", i),
			Name:        strings.Repeat("<", maxText),
			ReleaseName: "Example.Movie.2025.1080p.WEB-DL.H265-TEST",
			Expected:    ExpectedFacts{Kind: "movie", Quality: "1080p_web", Resolution: 1080, Source: "web_dl", Codec: "h265", Flags: []string{}},
		}
	}
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		t.Fatal(err)
	}
	if raw.Len() > MaxBytes {
		t.Fatalf("test input is already oversized: %d", raw.Len())
	}
	if _, err := Decode(raw.Bytes()); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("decode error = %v, want ErrTooLarge after canonical encoding", err)
	}
	if err := p.Validate(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("validate error = %v, want ErrTooLarge", err)
	}
}

func TestPickRuleTraitContextsAndDuplicateNames(t *testing.T) {
	p, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	video := p.PickRules[0]
	music := video
	music.Kinds = []string{"music"}
	music.Rules = mediafiles.LosslessFirst()
	for name, rule := range map[string]PickRule{"video": video, "music": music} {
		t.Run(name, func(t *testing.T) {
			for _, field := range []string{"traits", "must_have"} {
				bad := rule
				bad.Rules.Traits = slices.Clone(rule.Rules.Traits)
				bad.Rules.MustHave = slices.Clone(rule.Rules.MustHave)
				if field == "traits" {
					bad.Rules.Traits = []mediafiles.TraitPoints{{Trait: "sample", Points: 1}}
				} else {
					bad.Rules.MustHave = []string{"sample"}
				}
				if err := bad.validate(); err == nil {
					t.Fatalf("accepted safety-only trait in %s", field)
				}
			}
			safe := rule
			safe.Rules.Never = []string{"sample", "executable", "password_protected", "disc_image"}
			if err := safe.validate(); err != nil {
				t.Fatalf("safety-only Never values rejected: %v", err)
			}
		})
	}

	duplicate := p
	second := p.PickRules[0]
	second.ID = "another-rule"
	duplicate.PickRules = append(slices.Clone(p.PickRules), second)
	if err := duplicate.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate pick rule name") {
		t.Fatalf("duplicate-name error = %v", err)
	}

	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(duplicate); err != nil {
		t.Fatal(err)
	}
	before := st.WriteCount()
	if _, err := Import(context.Background(), st, raw.Bytes(), domain.System); err == nil {
		t.Fatal("import accepted duplicate Pick-rule names")
	}
	if st.WriteCount() != before {
		t.Fatalf("rejected duplicate names committed %d writes", st.WriteCount()-before)
	}
}

func TestMaximumAcceptedPerMinuteLimitStaysSafeInJudge(t *testing.T) {
	p, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	p.PickRules[0].Rules.SizeLimits = map[string]mediafiles.SizeLimit{
		"1080p_web": {Min: 1 << 50, Max: 1 << 50, PerMinute: true},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("maximum data-pack size limit rejected: %v", err)
	}
	parsed := releasename.Parse("Example.Movie.2025.1080p.WEB-DL.H265-TEST", releasename.Hints{})
	title := mediafiles.TitleRef{ID: 1, Name: "Example Movie", Year: 2025}
	v := mediafiles.Judge(mediafiles.ReleaseFacts{Name: "Example.Movie.2025.1080p.WEB-DL.H265-TEST", Parse: parsed, Size: 1 << 30}, mediafiles.SlotSet{
		Match: mediafiles.Match{Title: &title, Conf: releasename.Sure, Detail: "test match"}, SlotIDs: []int64{1}, RuntimeMinutes: 9000, Out: true,
	}, p.PickRules[0].Rules)
	for _, reason := range v.Reasons {
		if reason.Step == "hard" && reason.Rule == "size" && reason.Effect == "fail" {
			return
		}
	}
	t.Fatalf("overflowed accepted limit did not fail closed: %+v", v.Reasons)
}

func TestImportIsOneInactiveWriteAndDuplicateConflicts(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	before := st.WriteCount()
	p, err := Import(context.Background(), st, fixture(t), domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "home-cinema" || st.WriteCount() != before+1 {
		t.Fatalf("import result %+v writes=%d before=%d", p, st.WriteCount(), before)
	}
	var value string
	if err := st.Read().QueryRow(`SELECT value FROM settings WHERE grp = 'this_app' AND key = 'data_pack:home-cinema'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(value, `"format":"holyarrs-data-pack"`) {
		t.Fatalf("stored value %q", value)
	}
	if _, err := Import(context.Background(), st, fixture(t), domain.System); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate error %v", err)
	}
	var active int
	if err := st.Read().QueryRow(`SELECT COUNT(*) FROM settings WHERE grp IN ('what_to_pick', 'where_files_go')`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active == 0 {
		t.Fatal("baseline active settings unexpectedly absent")
	}
}

func TestRejectedImportHasNoSideEffects(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bad := bytes.Replace(fixture(t), []byte(`"quality": "1080p_web"`), []byte(`"quality": "2160p_web"`), 1)
	before := st.WriteCount()
	if _, err := Import(context.Background(), st, bad, domain.System); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid import error = %v, want ErrInvalidInput", err)
	}
	if st.WriteCount() != before {
		t.Fatalf("rejected import committed a write: %d -> %d", before, st.WriteCount())
	}
	var n int
	_ = st.Read().QueryRow(`SELECT COUNT(*) FROM settings WHERE key LIKE 'data_pack:%'`).Scan(&n)
	if n != 0 {
		t.Fatalf("rejected import stored %d packs", n)
	}
}

func TestCallerInputErrorsAreClassified(t *testing.T) {
	st, p := importedPack(t)
	for name, err := range map[string]error{
		"export id": func() error { _, err := Export(context.Background(), st, "BAD"); return err }(),
		"preview id": func() error {
			_, err := Preview(context.Background(), st, "BAD", Selection{PickRuleIDs: []string{p.PickRules[0].ID}})
			return err
		}(),
		"selection": func() error {
			_, err := Preview(context.Background(), st, p.ID, Selection{PickRuleIDs: []string{"missing"}})
			return err
		}(),
	} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s error = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestListAndExportRevalidateStoredPack(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := Import(context.Background(), st, fixture(t), domain.System); err != nil {
		t.Fatal(err)
	}
	list, err := List(context.Background(), st)
	if err != nil || len(list) != 1 || list[0].ID != "home-cinema" || list[0].Status != "inactive" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	b, err := Export(context.Background(), st, "home-cinema")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(b); err != nil {
		t.Fatalf("export invalid: %v", err)
	}
	if _, err := Export(context.Background(), st, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing export: %v", err)
	}
}

func importedPack(t *testing.T) (*database.Store, Pack) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := Import(context.Background(), st, fixture(t), domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return st, p
}

func addSecondRule(t *testing.T, st *database.Store, p Pack) Pack {
	t.Helper()
	second := p.PickRules[0]
	second.ID, second.Name = "balanced-web-copy", "Balanced web copy"
	p.ID, p.Name = "two-rules", "Two synthetic rules"
	p.PickRules = append(p.PickRules, second)
	raw, err := Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(context.Background(), st, raw, domain.System); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPreviewApplyUsesCanonicalSelectionAndOneAtomicWrite(t *testing.T) {
	st, p := importedPack(t)
	p = addSecondRule(t, st, p)
	ctx := context.Background()
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('where_files_go', 'adopt_scan_limit', '777', 't', '{}', 't', '{}') ON CONFLICT (grp,key) DO UPDATE SET value='777'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	selection := Selection{PickRuleIDs: []string{p.PickRules[1].ID, p.PickRules[0].ID}, NamingID: p.Naming[0].ID}
	preview, err := Preview(ctx, st, p.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := Preview(ctx, st, p.ID, Selection{PickRuleIDs: []string{p.PickRules[0].ID, p.PickRules[1].ID}, NamingID: p.Naming[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if ordered.ApprovalHash != preview.ApprovalHash {
		t.Fatalf("selection order changed approval hash: %q != %q", ordered.ApprovalHash, preview.ApprovalHash)
	}
	if _, err := Preview(ctx, st, p.ID, Selection{PickRuleIDs: []string{p.PickRules[0].ID, p.PickRules[0].ID}}); err == nil {
		t.Fatal("duplicate selected item ids were accepted")
	}
	if preview.ApprovalHash == "" || preview.PickRules[0].ID != "balanced-web" || preview.Naming == nil || len(preview.Naming.Changes) == 0 {
		t.Fatalf("preview %+v", preview)
	}
	beforeWrites := st.WriteCount()
	beforeRows := tableCounts(t, st, "titles", "versions", "slots", "files")
	res, err := Apply(ctx, st, p.ID, selection, preview.ApprovalHash, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if st.WriteCount() != beforeWrites+1 || len(res.PickRuleIDs) != 2 || res.NamingID != "readable" {
		t.Fatalf("result=%+v writes=%d", res, st.WriteCount()-beforeWrites)
	}
	if after := tableCounts(t, st, "titles", "versions", "slots", "files"); !slices.Equal(beforeRows, after) {
		t.Fatalf("media rows changed: %v -> %v", beforeRows, after)
	}
	var scan string
	_ = st.Read().QueryRow(`SELECT value FROM settings WHERE grp='where_files_go' AND key='adopt_scan_limit'`).Scan(&scan)
	if scan != "777" {
		t.Fatalf("unrelated scan setting changed to %q", scan)
	}
	var n int
	_ = st.Read().QueryRow(`SELECT COUNT(*) FROM pick_rules WHERE name IN ('Balanced web','Balanced web copy')`).Scan(&n)
	if n != 2 {
		t.Fatalf("created %d selected rules", n)
	}
}

func TestApplyRejectsStaleSelectedNamingWithNoWrite(t *testing.T) {
	st, p := importedPack(t)
	ctx := context.Background()
	selection := Selection{NamingID: p.Naming[0].ID}
	preview, err := Preview(ctx, st, p.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp,key,value,created_at,created_by,updated_at,updated_by) VALUES ('where_files_go','file_pattern','changed','t','{}','t','{}') ON CONFLICT (grp,key) DO UPDATE SET value='changed'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := st.WriteCount()
	if _, err := Apply(ctx, st, p.ID, selection, preview.ApprovalHash, domain.System); !errors.Is(err, ErrStaleApproval) {
		t.Fatalf("apply error %v", err)
	}
	if st.WriteCount() != before {
		t.Fatalf("stale apply committed %d writes", st.WriteCount()-before)
	}
}

func TestApplyRuleNameConflictRollsBackAllSelections(t *testing.T) {
	st, p := importedPack(t)
	p = addSecondRule(t, st, p)
	ctx := context.Background()
	selection := Selection{PickRuleIDs: []string{"balanced-web", "balanced-web-copy"}}
	preview, err := Preview(ctx, st, p.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "pick_rules", &mediafiles.PickRules{Name: "Balanced web copy", Kinds: []string{"movie"}, Revision: 1, Body: mediafiles.GoodEveryday()}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := st.WriteCount()
	if _, err := Apply(ctx, st, p.ID, selection, preview.ApprovalHash, domain.System); !errors.Is(err, ErrConflict) {
		t.Fatalf("apply error %v", err)
	}
	if st.WriteCount() != before {
		t.Fatalf("conflict apply committed %d writes", st.WriteCount()-before)
	}
	var free int
	_ = st.Read().QueryRow(`SELECT COUNT(*) FROM pick_rules WHERE name='Balanced web'`).Scan(&free)
	if free != 0 {
		t.Fatal("free rule was inserted despite mixed conflict")
	}
}

func TestApplyIgnoresUnselectedBehaviorRace(t *testing.T) {
	st, p := importedPack(t)
	ctx := context.Background()
	selection := Selection{NamingID: p.Naming[0].ID}
	preview, err := Preview(ctx, st, p.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		for key, value := range map[string]string{"adopt_scan_limit": "888", "rename_on_import": "true"} {
			if _, err := tx.Exec(`INSERT INTO settings (grp,key,value,created_at,created_by,updated_at,updated_by) VALUES ('where_files_go',?,?,'t','{}','t','{}') ON CONFLICT (grp,key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, st, p.ID, selection, preview.ApprovalHash, domain.System); err != nil {
		t.Fatal(err)
	}
	var got string
	_ = st.Read().QueryRow(`SELECT value FROM settings WHERE grp='where_files_go' AND key='adopt_scan_limit'`).Scan(&got)
	if got != "888" {
		t.Fatalf("behavior setting changed to %q", got)
	}
	_ = st.Read().QueryRow(`SELECT value FROM settings WHERE grp='where_files_go' AND key='rename_on_import'`).Scan(&got)
	if got != "true" {
		t.Fatalf("rename behavior setting changed to %q", got)
	}
}

func tableCounts(t *testing.T, st *database.Store, tables ...string) []int {
	t.Helper()
	out := make([]int, len(tables))
	for i, table := range tables {
		if err := st.Read().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// Your own traits belong to one install: a data pack naming one is refused.
func TestPickRuleWithOwnTraitIsRefused(t *testing.T) {
	p, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*mediafiles.PickRulesBody){
		func(b *mediafiles.PickRulesBody) {
			b.Traits = append(slices.Clone(b.Traits), mediafiles.TraitPoints{Trait: "own:3", Points: 10})
		},
		func(b *mediafiles.PickRulesBody) { b.MustHave = []string{"own:3"} },
		func(b *mediafiles.PickRulesBody) { b.Never = append(slices.Clone(b.Never), "own:3") },
	} {
		bad := p.PickRules[0]
		edit(&bad.Rules)
		if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "own:3") {
			t.Fatalf("own trait in a data pack: %v", err)
		}
	}
}
