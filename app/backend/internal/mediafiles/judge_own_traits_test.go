package judge

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Own traits (#97): a name test finds any word or pattern in the Release
// name, a group test needs the release group; Pick rules use them like the
// built-in traits.
func withOwnTraits(t *testing.T, ts ...domain.OwnTrait) {
	t.Helper()
	SetOwnTraits(ts)
	t.Cleanup(func() { SetOwnTraits(nil) })
}

func TestOwnTraitsMatchNameWordsPatternsAndGroups(t *testing.T) {
	withOwnTraits(t,
		domain.OwnTrait{Meta: domain.Meta{ID: 1}, Name: "Dutch subs", Match: "name", Terms: []string{"NLSubs", "/\\bdutch\\b/"}},
		domain.OwnTrait{Meta: domain.Meta{ID: 2}, Name: "Bad groups", Match: "group", Terms: []string{"yify", "/^evo$/"}},
	)
	for _, c := range []struct {
		name  string
		trait string
		want  bool
	}{
		{"Dune.Part.Two.2024.1080p.WEB.H264.NLSubs-HOLYARRS", "own:1", true},
		{"Dune.Part.Two.2024.1080p.WEB.DUTCH.H264-HOLYARRS", "own:1", true},
		{"Dune.Part.Two.2024.1080p.WEB.DUTCHIE.H264-HOLYARRS", "own:1", false}, // the pattern wants a whole word
		{"Dune.Part.Two.2024.1080p.WEB.H264-YIFY", "own:2", true},
		{"Dune.Part.Two.2024.1080p.WEB.H264-EVO", "own:2", true},
		{"Dune.Part.Two.2024.1080p.WEB.H264-EVOLVE", "own:2", false},
		{"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "own:3", false}, // deleted
	} {
		ok, why := traitMatches(c.trait, releasename.Parse(c.name, releasename.Hints{}))
		if ok != c.want {
			t.Errorf("%s %s: %v (%s), want %v", c.trait, c.name, ok, why, c.want)
		}
	}
}

func TestOwnTraitsWorkInPickRules(t *testing.T) {
	withOwnTraits(t,
		domain.OwnTrait{Meta: domain.Meta{ID: 7}, Name: "Bad group", Match: "group", Terms: []string{"GRAVELY"}},
		domain.OwnTrait{Meta: domain.Meta{ID: 8}, Name: "Favourite", Match: "name", Terms: []string{"HOLYARRS"}},
	)
	rules := domain.GoodEveryday()
	rules.Never = append(rules.Never, domain.OwnTraitKey(7))
	rules.Traits = append(rules.Traits, domain.TraitPoints{Trait: domain.OwnTraitKey(8), Points: 300})
	if err := domain.CheckPickRules("x", []string{"movie"}, rules); err != nil {
		t.Fatalf("Pick rules with own traits refused: %v", err)
	}

	bad := release(row{Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-GRAVELY"})
	if v := Judge(bad, setFor(t, bad, nil), rules); !slices.Contains(codes(v, "fail"), "hard.never:own:7") {
		t.Fatalf("never own trait: %s", dump(v))
	}
	good := release(row{Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-HOLYARRS"})
	v := Judge(good, setFor(t, good, nil), rules)
	if !slices.Contains(codes(v, "add"), "points.trait:own:8") {
		t.Fatalf("points own trait: %s", dump(v))
	}
}

func TestCheckOwnTrait(t *testing.T) {
	for _, c := range []struct {
		t  domain.OwnTrait
		ok bool
	}{
		{domain.OwnTrait{Name: "A", Match: "name", Terms: []string{" x ", ""}}, true},
		{domain.OwnTrait{Name: "", Match: "name", Terms: []string{"x"}}, false},
		{domain.OwnTrait{Name: "A", Match: "title", Terms: []string{"x"}}, false},
		{domain.OwnTrait{Name: "A", Match: "name", Terms: []string{" "}}, false},
		{domain.OwnTrait{Name: "A", Match: "name", Terms: []string{"/(unclosed/"}}, false},
	} {
		tr := c.t
		if err := domain.CheckOwnTrait(&tr); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.t, err)
		}
	}
}

func TestOwnTraitPatternErrorSaysWhatIsWrong(t *testing.T) {
	tr := domain.OwnTrait{Name: "A", Match: "name", Terms: []string{"/(open/"}}
	err := domain.CheckOwnTrait(&tr)
	if err == nil || err.Error() != "/(open/ is not a pattern HolyArrs can read (missing closing ))." {
		t.Fatalf("pattern error: %v", err)
	}
}

func TestOwnTraitKeysHaveOneForm(t *testing.T) {
	for key, ok := range map[string]bool{"own:7": true, "own:07": false, "own:+7": false, "own:0": false, "own:-1": false, "own:": false, "own:x": false, "hevc": false} {
		if _, got := domain.OwnTraitID(key); got != ok {
			t.Errorf("%s: %v, want %v", key, got, ok)
		}
	}
	rules := domain.GoodEveryday()
	rules.MustHave = []string{"own:07"}
	if domain.CheckPickRules("x", []string{"movie"}, rules) == nil {
		t.Fatal("own:07 accepted in Pick rules")
	}
}
