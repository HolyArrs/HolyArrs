package requests

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func ids(ls []QualityLine) string {
	out := []string{}
	for _, l := range ls {
		out = append(out, l.ID)
	}
	return strings.Join(out, ",")
}

// saveQualities stores the lines (names and hidden) through the Request settings.
func saveQualities(t *testing.T, r *rig, edit func(q *QualityLists)) {
	t.Helper()
	ctx := context.Background()
	set, err := ReadSettings(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	edit(&set.Qualities)
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
}

// An owner who changed nothing has Standard then 4K for movies and shows,
// filled from the Request settings; 4K is offered only once it is set up,
// per kind.
func TestQualityLinesComeFromTheRequestSettings(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	set, _ := ReadSettings(ctx, r.st.Read())
	fourK := rulesID(t, r, mediafiles.FourKName)
	for _, kind := range []string{"movie", "show"} {
		ls, err := qualityLines(ctx, r.st.Read(), set, kind)
		if err != nil {
			t.Fatal(err)
		}
		if ids(ls) != "standard,4k" || ls[0].Extra || !ls[0].Ready || ls[0].PickRulesID != 0 || !ls[1].Extra || !ls[1].Ready || ls[1].PickRulesID != fourK {
			t.Fatalf("%s lines %+v", kind, ls)
		}
		if got := ids(offered(ls, r.ana)); got != "standard,4k" {
			t.Fatalf("%s offered %s", kind, got)
		}
	}
	// The 4K settings of each kind feed its own 4K line.
	other := rulesID(t, r, mediafiles.GoodEverydayName)
	set.ShowFourKPickRulesID = other
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	set, _ = ReadSettings(ctx, r.st.Read())
	m, _ := qualityLines(ctx, r.st.Read(), set, "movie")
	s, _ := qualityLines(ctx, r.st.Read(), set, "show")
	if m[1].PickRulesID != fourK || s[1].PickRulesID != other {
		t.Fatalf("4K rules movie %d show %d", m[1].PickRulesID, s[1].PickRulesID)
	}
	// No 4K Pick rules anywhere: the movie 4K line is not ready and is not
	// offered, so the row has one choice; the show keeps its own 4K rules.
	pinExec(t, r, "DELETE FROM pick_rules WHERE name = ?", mediafiles.FourKName)
	m, _ = qualityLines(ctx, r.st.Read(), set, "movie")
	s, _ = qualityLines(ctx, r.st.Read(), set, "show")
	if m[1].Ready || ids(offered(m, r.ana)) != "standard" || !s[1].Ready || ids(offered(s, r.ana)) != "standard,4k" {
		t.Fatalf("not set up: movie %+v show %+v", m, s)
	}
}

// The stored list keeps names, hidden lines and order; anything unreadable
// gives the built-in lines, and a missing built-in line comes back.
func TestQualitiesStoredShape(t *testing.T) {
	if got := parseQualities("not json"); ids(got.Movie) != "standard,4k" || ids(got.Show) != "standard,4k" {
		t.Fatalf("bad json %+v", got)
	}
	got := parseQualities(`{"movie":[{"id":"4k","name":" UHD ","hidden":true},{"id":"nope"},{"id":"4k"}],"show":[]}`)
	if ids(got.Movie) != "4k,standard" || got.Movie[0].Name != "UHD" || !got.Movie[0].Hidden || ids(got.Show) != "standard,4k" {
		t.Fatalf("parsed %+v", got)
	}
	back := parseQualities(storeQualities(got))
	if ids(back.Movie) != "4k,standard" || back.Movie[0].Name != "UHD" || !back.Movie[0].Hidden {
		t.Fatalf("round trip %+v", back)
	}
	// Only what the owner changed is stored, never the settings' values.
	if s := storeQualities(defaultQualities()); strings.Contains(s, "pick_rules") || strings.Contains(s, "name") {
		t.Fatalf("stored %s", s)
	}
}

// Saving checks the names and keeps one line shown per kind.
func TestQualitiesSaveChecks(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	set, _ := ReadSettings(ctx, r.st.Read())
	bad := set
	bad.Qualities = defaultQualities()
	bad.Qualities.Movie[0].Name = strings.Repeat("x", 31)
	if err := SaveSettings(ctx, r.st, bad, domain.System); !errors.Is(err, ErrQualityName) {
		t.Fatalf("long name: %v", err)
	}
	bad.Qualities = defaultQualities()
	bad.Qualities.Show[0].Hidden, bad.Qualities.Show[1].Hidden = true, true
	if err := SaveSettings(ctx, r.st, bad, domain.System); !errors.Is(err, ErrQualityAllHidden) {
		t.Fatalf("all hidden: %v", err)
	}
	saveQualities(t, r, func(q *QualityLists) { q.Movie[0].Name = "720p x265"; q.Movie[1].Hidden = true })
	back, _ := ReadSettings(ctx, r.st.Read())
	if back.Qualities.Movie[0].Name != "720p x265" || !back.Qualities.Movie[1].Hidden || back.Qualities.Show[1].Hidden {
		t.Fatalf("saved %+v", back.Qualities)
	}
}

// An ask remembers its line, with the name and Pick rules it stood for;
// open asks group by line: a second 4K ask joins the first, a Standard one
// does not.
func TestAskRemembersItsLineAndGroupsByIt(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	uhd, err := r.svc.AskQuality(ctx, r.ana, "693134", QualityFourK)
	if err != nil {
		t.Fatal(err)
	}
	a := uhd.Ask
	if a.Quality != QualityFourK || a.QualityName != "4K" || !a.QualityExtra || !a.FourK || a.QualityRulesID != rulesID(t, r, mediafiles.FourKName) {
		t.Fatalf("4K ask %+v", a)
	}
	joined, err := r.svc.AskQuality(ctx, r.bo, "693134", QualityFourK)
	if err != nil || !joined.Joined || joined.Ask.ID != a.ID {
		t.Fatalf("second 4K ask %+v %v", joined, err)
	}
	plain, err := r.svc.AskQuality(ctx, r.bo, "693134", "")
	if err != nil || plain.Joined || plain.Ask.ID == a.ID || plain.Ask.Quality != QualityStandard || plain.Ask.FourK || plain.Ask.QualityExtra {
		t.Fatalf("plain ask %+v %v", plain, err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", a.ID)
	if len(vs) != 1 || vs[0].Quality != QualityFourK || !vs[0].FourK || vs[0].QualityName != "" {
		t.Fatalf("view %+v", vs)
	}
	// The older calls still ask on the built-in lines.
	if old, err := r.svc.Ask4K(ctx, r.luis, "900002"); err != nil || old.Ask.Quality != QualityFourK {
		t.Fatalf("Ask4K %+v %v", old, err)
	}
}

// A hidden line cannot be asked for, and a Kid gets the first line only.
func TestHiddenLineAndKidCannotAskFor4K(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	kid := r.bo
	kid.MaxRating = "PG"
	if l, err := pickQuality(ctx, r.st.Read(), kid, "movie", ""); err != nil || l.ID != QualityStandard {
		t.Fatalf("kid default %+v %v", l, err)
	}
	if _, err := pickQuality(ctx, r.st.Read(), kid, "movie", QualityFourK); !errors.Is(err, ErrQualityNotOffered) {
		t.Fatalf("kid 4K: %v", err)
	}
	saveQualities(t, r, func(q *QualityLists) { q.Movie[1].Hidden = true })
	if _, err := r.svc.AskQuality(ctx, r.ana, "693134", QualityFourK); !errors.Is(err, ErrQualityNotOffered) {
		t.Fatalf("hidden 4K: %v", err)
	}
	if _, err := r.svc.AskQuality(ctx, r.ana, "693134", "made-up"); !errors.Is(err, ErrQualityNotOffered) {
		t.Fatalf("unknown line: %v", err)
	}
	// Shows keep their own list.
	if l, err := pickQuality(ctx, r.st.Read(), r.ana, "show", QualityFourK); err != nil || l.ID != QualityFourK {
		t.Fatalf("show 4K %+v %v", l, err)
	}
	// With Standard hidden, an ask with no line takes the first shown one.
	saveQualities(t, r, func(q *QualityLists) { q.Movie[0].Hidden, q.Movie[1].Hidden = true, false })
	if l, err := pickQuality(ctx, r.st.Read(), r.ana, "movie", ""); err != nil || l.ID != QualityFourK {
		t.Fatalf("first shown %+v %v", l, err)
	}
}

// Approving as asked: Standard adds the main copy only, 4K only the 4K copy,
// for a movie that is new and for one already in the Library.
func TestApproveAsAskedAddsOnlyThatCopy(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fourK := rulesID(t, r, mediafiles.FourKName)
	std, _ := r.svc.AskQuality(ctx, r.ana, "900002", QualityStandard)
	got, err := r.svc.Approve(ctx, std.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || cs[0].fourK || cs[0].rules == fourK || !cs[0].main {
		t.Fatalf("standard on a new movie %+v", cs)
	}
	uhd, _ := r.svc.AskQuality(ctx, r.ana, "900003", QualityFourK)
	got, err = r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || !cs[0].fourK || cs[0].rules != fourK {
		t.Fatalf("4K on a new movie %+v", cs)
	}
	// In the Library with its main copy: 4K adds the 4K copy beside it.
	more, err := r.svc.AskQuality(ctx, r.bo, "900002", QualityFourK)
	if err != nil {
		t.Fatal(err)
	}
	got, err = r.svc.Approve(ctx, more.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cs := copies(t, r, *got.TitleID)
	k, ok := fourKOf(cs)
	if len(cs) != 2 || !ok || k.main || k.rules != fourK {
		t.Fatalf("4K beside the main copy %+v", cs)
	}
}

// The owner changes the quality before approving: a 4K ask approved as
// Standard adds the main copy (and is then a Standard ask), a Standard ask
// approved as 4K adds the 4K copy, and other Pick rules land on the main copy.
func TestOwnerChangesTheQualityWhenApproving(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fourK, good := rulesID(t, r, mediafiles.FourKName), rulesID(t, r, mediafiles.GoodEverydayName)
	uhd, _ := r.svc.AskQuality(ctx, r.ana, "900002", QualityFourK)
	got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{Quality: QualityStandard})
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality != QualityStandard || got.FourK || got.QualityExtra {
		t.Fatalf("ask after the change %+v", got)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || cs[0].fourK || cs[0].rules == fourK {
		t.Fatalf("4K approved as Standard %+v", cs)
	}
	std, _ := r.svc.AskQuality(ctx, r.ana, "900003", QualityStandard)
	got, err = r.svc.Approve(ctx, std.Ask.ID, r.luis, ApproveOptions{Quality: QualityFourK})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); !got.FourK || len(cs) != 1 || !cs[0].fourK || cs[0].rules != fourK {
		t.Fatalf("Standard approved as 4K %+v %+v", got, cs)
	}
	other, _ := r.svc.AskQuality(ctx, r.ana, "900005", QualityStandard)
	got, err = r.svc.Approve(ctx, other.Ask.ID, r.luis, ApproveOptions{PickRulesID: good})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || cs[0].rules != good || cs[0].fourK {
		t.Fatalf("other rules %+v", cs)
	}
	// A line that does not exist is refused.
	bad, _ := r.svc.AskQuality(ctx, r.bo, "693134", "")
	if _, err := r.svc.Approve(ctx, bad.Ask.ID, r.luis, ApproveOptions{Quality: "nope"}); !errors.Is(err, ErrQualityNotOffered) {
		t.Fatalf("unknown line at approval: %v", err)
	}
}

// Changing a 4K ask to Standard while someone else waits on Standard for the
// same movie is refused: decide theirs instead.
func TestOwnerChangeOntoAnotherOpenAskIsRefused(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_, _ = r.svc.AskQuality(ctx, r.ana, "693134", QualityStandard)
	uhd, _ := r.svc.AskQuality(ctx, r.bo, "693134", QualityFourK)
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{Quality: QualityStandard}); !errors.Is(err, ErrQualityTaken) {
		t.Fatalf("change onto an open ask: %v", err)
	}
	if a, _ := get(ctx, r.st.Read(), uhd.Ask.ID); a.State != domain.AskAsked || a.Quality != QualityFourK {
		t.Fatalf("ask after the refusal %+v", a)
	}
}

// When the copy is already there, the approver's pick cannot be used, and
// the approval says so instead of passing over it.
func TestApprovalSaysWhenTheCopyIsAlreadyThere(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	good := rulesID(t, r, mediafiles.GoodEverydayName)
	first, _ := r.svc.Ask(ctx, r.ana, "693134")
	if _, err := r.svc.Approve(ctx, first.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	// The Standard copy is in the Library (an approved Standard ask still open
	// would refuse the change instead: ErrQualityTaken).
	pinExec(t, r, "UPDATE asks SET state = 'available' WHERE id = ?", first.Ask.ID)
	// A 4K ask for the same movie, approved as Standard: the main copy is there.
	uhd, err := r.svc.AskQuality(ctx, r.bo, "693134", QualityFourK)
	if err != nil {
		t.Fatal(err)
	}
	_, said, err := r.svc.ApproveSaying(ctx, uhd.Ask.ID, r.luis, ApproveOptions{Quality: QualityStandard})
	if err != nil || said != CopyKept {
		t.Fatalf("change onto a copy that is there: %q %v", said, err)
	}
	// A show in the Library: more seasons approved with other Pick rules
	// keep the main copy's rules, and say so.
	s := showRig(t)
	first2, _ := s.svc.AskShow(ctx, s.ana, "tmdb:100", []int{1})
	if _, err := s.svc.Approve(ctx, first2.Ask.ID, s.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	more, err := s.svc.AskShow(ctx, s.bo, "tmdb:100", []int{2})
	if err != nil {
		t.Fatal(err)
	}
	got, said, err := s.svc.ApproveSaying(ctx, more.Ask.ID, s.luis, ApproveOptions{PickRulesID: rulesID(t, s, mediafiles.FourKName)})
	if err != nil || said != CopyKept || got.State != domain.AskApproved {
		t.Fatalf("other rules on a show's copy: %q %v %+v", said, err, got)
	}
	if keeps := seasonKeeps(t, s.st, *got.TitleID); !keeps[2] {
		t.Fatalf("season 2 not kept %v", keeps)
	}
	_ = good
	// Approved as asked, nothing is said.
	second, _ := r.svc.AskQuality(ctx, r.ana, "900002", QualityStandard)
	if _, said, err := r.svc.ApproveSaying(ctx, second.Ask.ID, r.luis, ApproveOptions{}); err != nil || said != "" {
		t.Fatalf("as asked: %q %v", said, err)
	}
}

// A renamed line still approves as asked, and the alerts name it as it is
// now; Pick rules changed after the ask do not change what was asked.
func TestRenamedLineAndChangedRulesStillApproveAsAsked(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fourK, good := rulesID(t, r, mediafiles.FourKName), rulesID(t, r, mediafiles.GoodEverydayName)
	uhd, _ := r.svc.AskQuality(ctx, r.ana, "693134", QualityFourK)
	saveQualities(t, r, func(q *QualityLists) { q.Movie[1].Name = "UHD" })
	set, _ := ReadSettings(ctx, r.st.Read())
	set.FourKPickRulesID = good
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", uhd.Ask.ID)
	if vs[0].QualityName != "UHD" {
		t.Fatalf("view name %q", vs[0].QualityName)
	}
	got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || !cs[0].fourK || cs[0].rules != fourK {
		t.Fatalf("renamed 4K approved %+v", cs)
	}
	r.svc.Wait()
	if m := r.fake.last(); !strings.Contains(m.Subject, "Dune: Part Two (2024) in UHD") {
		t.Fatalf("approved alert %q", m.Subject)
	}
}

// The ready alert names the line when it is not the first one, so two
// alerts for one title tell apart.
func TestReadyAlertSaysWhichCopy(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	std, _ := r.svc.AskQuality(ctx, r.ana, "693134", QualityStandard)
	uhd, _ := r.svc.AskQuality(ctx, r.ana, "693134", QualityFourK)
	got, err := r.svc.Approve(ctx, std.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, c := range copies(t, r, *got.TitleID) {
		fillCopy(t, r, c.id, "in_library")
	}
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	var subjects []string
	r.fake.mu.Lock()
	for _, m := range r.fake.sent {
		if strings.HasPrefix(m.Subject, "Ready to watch: ") {
			subjects = append(subjects, m.Subject)
		}
	}
	r.fake.mu.Unlock()
	want := []string{"Ready to watch: Dune: Part Two (2024)", "Ready to watch: Dune: Part Two (2024) in 4K"}
	if strings.Join(subjects, "|") != strings.Join(want, "|") {
		t.Fatalf("ready alerts %q", subjects)
	}
}
