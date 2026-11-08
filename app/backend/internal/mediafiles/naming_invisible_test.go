package mediafiles

import "testing"

// u builds a string from code points, so the invisible characters stay
// visible in this file.
func u(rs ...rune) string { return string(rs) }

// Bidi controls and zero-width characters never reach a file or folder
// name: they hide or reorder what a name shows.
func TestCleanDropsBidiAndZeroWidth(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Night" + u(0x202E) + "Train", "NightTrain"},
		{"A" + u(0x202A, 0x202B, 0x202C, 0x202D) + "b", "Ab"},
		{"x" + u(0x2066, 0x2067, 0x2068, 0x2069) + "y", "xy"},
		{"Zero" + u(0x200B) + "Width" + u(0x200C, 0x200D, 0x200E, 0x200F), "ZeroWidth"},
		{"Word" + u(0x2060) + "Joiner" + u(0xFEFF), "WordJoiner"},
		{"Beyonc" + u(0xE9) + " " + u(0x2013) + " Halo", "Beyonc" + u(0xE9) + " " + u(0x2013) + " Halo"},
	} {
		if got := CleanName(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Movies and shows use the same rules.
	s := NamingDefaults()
	if got := Folder(s, NameFacts{Title: "Dune" + u(0x200B, 0x202E), Year: 2021}); got != "Dune (2021)" {
		t.Errorf("movie folder %q", got)
	}
	if got := ShowFolder(s, NameFacts{Title: u(0xFEFF) + "Severance", Year: 2022}); got != "Severance (2022)" {
		t.Errorf("show folder %q", got)
	}
}
