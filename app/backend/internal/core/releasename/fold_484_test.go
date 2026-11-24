package releasename

import (
	"slices"
	"testing"
)

func TestCleanNameEveryAccent(t *testing.T) {
	for in, want := range map[string]string{"Ștefan cel Mare": "stefan cel mare", "Ştefan": "stefan", "Nữ hoàng": "nu hoang",
		"Ơn": "on", "Άλφα": "αλφα", "Тьма": "тьма", "Йод": "йод", "Amélie": "amelie"} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := FoldAccents("Ștefan"); got != "Stefan" {
		t.Errorf("FoldAccents = %q", got)
	}
}

func TestNameFormsUmlautsAndNumerals(t *testing.T) {
	for lib, want := range map[string]string{"Die Mumie kehrt zurück": "die mumie kehrt zurueck", "Rambo III": "rambo 3",
		"Rocky 2": "rocky ii", "Mission: Impossible II": "mission impossible 2"} {
		if !slices.Contains(NameForms(lib), want) {
			t.Errorf("NameForms(%q) = %q, missing %q", lib, NameForms(lib), want)
		}
	}
	if SameName("Rocky II", "rocky iii") || SameName("Rocky II", "rocky 3") || len(NameForms("X")) != 1 {
		t.Error("a different sequel number matched")
	}
}

func TestLocalSeasonWords(t *testing.T) {
	for in, title := range map[string]string{"Streghe.Charmed.Stagione.1.1080p.ITA": "Streghe Charmed",
		"Зачарованные / Charmed / Сезон: 1 / Серии: 1-22 (1998) WEB-DL 1080p": "Зачарованные / Charmed"} {
		p := Parse(in, Hints{})
		if !p.IsShow() || CleanName(p.Title.Value) != CleanName(title) || !slices.Equal(p.Show.Seasons, []int{1}) {
			t.Errorf("%s: title %q show %v", in, p.Title.Value, p.Show)
		}
	}
}
