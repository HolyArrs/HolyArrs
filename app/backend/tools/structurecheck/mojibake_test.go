package structurecheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// doubleEncoded matches UTF-8 that was read as cp1252 and saved again, such as
// "Â·" for "·", "â€¦" for "…" or "Ã‚Â·". A real Ã or Â in a language is
// followed by a letter, never by one of these.
var doubleEncoded = regexp.MustCompile("[ÂÃ][\u0080-¿€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ]|â[€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ -¿]")

func TestNoDoubleEncodedText(t *testing.T) {
	root := repoRoot(t)
	exts := map[string]bool{".go": true, ".svelte": true, ".ts": true, ".js": true, ".json": true, ".css": true, ".html": true, ".md": true}
	for _, dir := range []string{"app/frontend/src", "app/backend"} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != base && (skipDir(d.Name()) || d.Name() == "testdata") {
					return filepath.SkipDir
				}
				return nil
			}
			if !exts[filepath.Ext(p)] || strings.HasSuffix(p, "mojibake_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(b), "\n") {
				if m := doubleEncoded.FindString(line); m != "" {
					rel, _ := filepath.Rel(root, p)
					t.Errorf("%s:%d has double-encoded text %q; write the real character", filepath.ToSlash(rel), i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
