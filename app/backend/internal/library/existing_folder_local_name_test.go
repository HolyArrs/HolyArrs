package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExistingFolderLocalName: with folders named in another language, the
// folder already there is found by the localized name the naming setup
// gives, also inside a first-letter parent folder.
func TestExistingFolderLocalName(t *testing.T) {
	for _, c := range []struct{ want, dir, exp string }{
		{"Maman, j'ai raté l'avion ! (1990)", "Maman, J'ai Rate L'avion ! (1990)", "Maman, J'ai Rate L'avion ! (1990)"},
		{"M/Maman, j'ai raté l'avion ! (1990)", "M/Maman, J'ai Rate L'avion ! (1990)", "M/Maman, J'ai Rate L'avion ! (1990)"},
	} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(c.dir)), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := ExistingFolder(root, c.want, "Home Alone", 1990, nil); got != c.exp {
			t.Fatalf("got %q, want %q", got, c.exp)
		}
	}
}
