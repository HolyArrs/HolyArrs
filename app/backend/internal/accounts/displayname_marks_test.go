package accounts

import "testing"

func TestCleanDisplayNameRefusesHiddenMarks(t *testing.T) {
	for _, s := range []string{"Ana⁣7", "x⁣Luis⁤", "Ana​"} {
		if _, ok := cleanDisplayName(s); ok {
			t.Errorf("cleanDisplayName(%q) accepted a hidden character", s)
		}
	}
	for _, s := range []string{"", "Ana", "José María", "علی‌رضا"} {
		if _, ok := cleanDisplayName(s); !ok {
			t.Errorf("cleanDisplayName(%q) refused a plain name", s)
		}
	}
}
