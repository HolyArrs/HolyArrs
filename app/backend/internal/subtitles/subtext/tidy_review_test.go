package subtext

import (
	"strings"
	"testing"
)

func TestTidyReviewCases(t *testing.T) {
	one := func(text string, td Tidy) string {
		out, _ := CleanCues([]Cue{{StartMS: 0, EndMS: 1500, Text: text}}, td)
		if len(out) == 0 {
			return ""
		}
		return out[0].Text
	}
	for _, c := range []struct {
		name, in, want string
		td             Tidy
	}{
		{"hash number stays", "We're #1 in the league!", "We're #1 in the league!", Tidy{HI: true}},
		{"hash music mark goes", "# La la la #", "La la la", Tidy{HI: true}},
		{"opening note goes", "(sighs) Okay, fine.", "Okay, fine.", Tidy{HI: true}},
		{"dashed opening note goes", "- (laughing) Stop it.", "- Stop it.", Tidy{HI: true}},
		{"aside stays", "He said yes (and he agreed).", "He said yes (and he agreed).", Tidy{HI: true}},
		{"French keeps the space before ? ! : ; only", "Oui , je sais : c'est vrai . Quoi ?", "Oui, je sais : c'est vrai. Quoi ?", Tidy{Spacing: true, Language: "fr"}},
	} {
		if got := one(c.in, c.td); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	big := strings.Repeat("JAN I MARIA IDĄ DO DOMU. ", 10)
	if got := one(big, Tidy{Caps: true, Language: "pl"}); !strings.HasPrefix(got, "Jan i maria") {
		t.Errorf("polish i raised: %q", got[:20])
	}
	if got := one(strings.Repeat("I THINK I CAN. ", 10), Tidy{Caps: true, Language: "en"}); !strings.HasPrefix(got, "I think I can") {
		t.Errorf("english I: %q", got[:20])
	}
}
