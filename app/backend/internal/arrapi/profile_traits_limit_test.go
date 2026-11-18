package arrapi

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// pushFormats pushes n formats, each scored, and the profile that scores
// them all, the way Profilarr does. It returns the profile id and notes.
func pushFormats(t *testing.T, h *hookRig, n int) (int64, []any) {
	t.Helper()
	fmts, names := map[int64]int{}, map[int64]string{}
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("Format %03d", i)
		st, out := h.do("radarr", "POST", "customformat", formatBody(0, name, false, spec(name, "ReleaseTitleSpecification", false, false, "value", fmt.Sprintf(`\bTAG%03d\b`, i))))
		id := int64(decodeObj(t, st, out, 201)["id"].(float64))
		fmts[id], names[id] = 10*i, name
	}
	st, out := h.do("radarr", "POST", "qualityprofile", radarrProfile(0, "Remux", fmts, names))
	m := decodeObj(t, st, out, 201)
	notes, _ := m["notes"].([]any)
	return int64(m["id"].(float64)), notes
}

// A Dictionarry-sized profile (139 formats) keeps every format.
func TestProfiles_139FormatsAllKept(t *testing.T) {
	h := newHookRig(t)
	id, _ := pushFormats(t, h, 139)
	if got := len(h.rules(id).Body.Traits); got != 139 {
		t.Fatalf("kept %d of 139 formats", got)
	}
}

// Above the limit the weakest are left out, and the push still says which.
func TestProfiles_300FormatsReportsLeftOut(t *testing.T) {
	h := newHookRig(t)
	id, _ := pushFormats(t, h, 300)
	if got := len(h.rules(id).Body.Traits); got != 256 {
		t.Fatalf("kept %d of 300 formats, want 256", got)
	}
	// The report of what was left out: read it where the push shows it.
	rows, err := listProfileRows(t.Context(), h.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	var left int
	for _, r := range rows {
		left += strings.Count(strings.Join(r.Notes, "\n"), "is left out: Pick rules hold at most 256 traits")
	}
	if left != 44 {
		t.Fatalf("%d left-out notes, want 44", left)
	}
}

// Pick rules an earlier version cut to 64 traits get the formats they
// left out when the app starts, and keep the points a person changed.
func TestProfiles_CutProfilesRestoredAtStart(t *testing.T) {
	h := newHookRig(t)
	id, _ := pushFormats(t, h, 139)
	ctx := t.Context()

	// What 1.1.4 stored: the 64 strongest, and a note for each of the other 75.
	cut := h.rules(id)
	sort.SliceStable(cut.Body.Traits, func(i, j int) bool { return cut.Body.Traits[i].Points > cut.Body.Traits[j].Points })
	cut.Body.Traits = cut.Body.Traits[:64]
	cut.Body.Traits[0].Points = 1
	if _, err := library.UpdatePickRules(ctx, h.st, id, library.PickRulesInput{Name: cut.Name, Kinds: cut.Kinds, Body: cut.Body}, database.Actor{Kind: "user", ID: 1}); err != nil {
		t.Fatal(err)
	}
	row, err := profileRow(ctx, h.st.Read(), id)
	if err != nil {
		t.Fatal(err)
	}
	row.Notes = []string{"Format 001 is left out: Pick rules hold at most 64 traits; the strongest scores were kept"}
	if err := saveProfileRow(ctx, h.st, row, database.Actor{Kind: "user", ID: 1}); err != nil {
		t.Fatal(err)
	}

	hs := &Handlers{Deps: Deps{Store: h.st, Secrets: h.vault}}
	hs.RestoreCut(ctx)
	got := h.rules(id).Body.Traits
	if len(got) != 139 {
		t.Fatalf("after start: %d traits, want 139", len(got))
	}
	if got[0].Points != 1 {
		t.Fatalf("a changed score was overwritten: %+v", got[0])
	}
	row, _ = profileRow(ctx, h.st.Read(), id)
	if strings.Contains(strings.Join(row.Notes, "\n"), "at most 64") {
		t.Fatalf("notes still say cut: %v", row.Notes)
	}
	// A second start changes nothing.
	hs.RestoreCut(ctx)
	if n := len(h.rules(id).Body.Traits); n != 139 {
		t.Fatalf("second start: %d traits", n)
	}
}
