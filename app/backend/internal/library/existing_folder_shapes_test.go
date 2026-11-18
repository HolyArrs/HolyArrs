package library

import (
	"os"
	"path/filepath"
	"testing"
)

// A folder named the way release names write a title (no apostrophe,
// letters joined) is the same title's folder; a different title's never is.
func TestExistingFolderNameShapes(t *testing.T) {
	cases := []struct {
		dirs []string
		name string
		year int
		want string
	}{
		{[]string{"Schitts Creek (2015)"}, "Schitt's Creek", 2015, "Schitts Creek (2015)"},
		{[]string{"Schitts Creek"}, "Schitt’s Creek", 2015, "Schitts Creek"},
		{[]string{"SWAT (2017)"}, "S.W.A.T.", 2017, "SWAT (2017)"},
		{[]string{"MASH"}, "M*A*S*H", 1972, "MASH"},
		// never another title's folder
		{[]string{"Schitts Creek Reunion (2015)"}, "Schitt's Creek", 2015, "new"},
		{[]string{"Creek (2015)"}, "Schitt's Creek", 2015, "new"},
		{[]string{"Hells Angels (2015)"}, "Hell's Kitchen", 2015, "new"},
		{[]string{"SWAT Kats (1993)"}, "S.W.A.T.", 2017, "new"},
		{[]string{"SWAT (1975)"}, "S.W.A.T.", 2017, "new"},
		{[]string{"Mashle (2023)"}, "M*A*S*H", 1972, "new"},
		{[]string{"The Shield (2002)"}, "Agents of S.H.I.E.L.D.", 2013, "new"},
		{[]string{"IT (2017)"}, "I.T.", 2016, "new"},
		// the folder spelled like the name wins over a near one
		{[]string{"Schitts Creek (2015)", "Schitt's Creek (2015)"}, "Schitt's Creek", 2015, "Schitt's Creek (2015)"},
		{[]string{"Schitts Creek (2015)", "Schitt's Creek"}, "Schitt's Creek", 2015, "Schitt's Creek"},
		{[]string{"Schitts Creek (2015)", "Schitts Creek"}, "Schitt's Creek", 2015, "Schitts Creek (2015)"},
		{[]string{"Schitts Creek", "Schitt s Creek"}, "Schitt's Creek", 0, "Schitt s Creek"},
	}
	for i, c := range cases {
		root := t.TempDir()
		for _, d := range c.dirs {
			if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if got := ExistingFolder(root, "new", c.name, c.year, nil); got != c.want {
			t.Errorf("case %d: %q (%d) in %v = %q, want %q", i, c.name, c.year, c.dirs, got, c.want)
		}
	}
}

func TestNameScoreNameShapes(t *testing.T) {
	if s := NameScore("Schitts Creek", 2015, "Schitt's Creek", 2015); s != 1 {
		t.Errorf("Schitts Creek folder scores %v for Schitt's Creek, want 1", s)
	}
	if s := NameScore("SWAT", 2017, "S.W.A.T.", 2017); s != 1 {
		t.Errorf("SWAT folder scores %v for S.W.A.T., want 1", s)
	}
	if s := NameScore("Hells Angels", 2015, "Hell's Kitchen", 2015); s >= 0.5 {
		t.Errorf("Hells Angels folder scores %v for Hell's Kitchen", s)
	}
}
