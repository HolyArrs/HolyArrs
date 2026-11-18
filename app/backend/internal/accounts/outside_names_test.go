package accounts

import "testing"

// A name from a media server, an outside sign-in or an import never keeps
// the mark a deleted person's name carries, or any other hidden character.
func TestNewPersonNameRefusesHidden(t *testing.T) {
	for in, want := range map[string]string{
		"Ana⁣7":        "plex-user",
		"Ana⁤":         "plex-user",
		"Ana​B":        "plex-user",
		"  Ana   Lee ": "Ana Lee",
		"":             "plex-user",
		"Zoë":          "Zoë",
	} {
		if got := NewPersonName(in, "plex-user", 56); got != want {
			t.Errorf("NewPersonName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := outsideBase("jellyfin", "Ana⁣7"); got != "jellyfin-user" {
		t.Errorf("outsideBase kept a hidden mark: %q", got)
	}
}
