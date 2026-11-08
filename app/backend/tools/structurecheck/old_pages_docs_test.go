package structurecheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// reOldPageMention finds an old page address that starts a path in docs
// or e2e specs, plain (/status) or as an escaped URL pattern (/\/status/).
var reOldPageMention = regexp.MustCompile(`(?::[0-9]+/|(?:^|[^A-Za-z0-9_.\-/\\])/|(?:^|[^A-Za-z0-9_.\-\\])\\/)(?:wishlist|status|new|reports|ai-connect|ui|settings(?:\\?/)wishlist)(?:[^A-Za-z0-9_-]|$)`)

// TestNoOldPagesInTestsOrDocs: e2e specs and docs never send anyone to an
// old page address; those pages are gone and have no redirect.
func TestNoOldPagesInTestsOrDocs(t *testing.T) {
	root := repoRoot(t)
	for _, base := range []string{"app/tests", "docs"} {
		dir := filepath.Join(root, base)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				if err == nil && skipDir(d.Name()) && p != dir {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(p)
			if ext != ".ts" && ext != ".md" && ext != ".cjs" && ext != ".js" && ext != ".mjs" && ext != ".json" {
				return nil
			}
			b, _ := os.ReadFile(p)
			for i, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, "/api/") || strings.Contains(line, `\/api\/`) || strings.Contains(line, "/v1/") {
					continue
				}
				if m := reOldPageMention.FindString(line); m != "" {
					rel, _ := filepath.Rel(root, p)
					t.Errorf("%s:%d: old page address in %q", filepath.ToSlash(rel), i+1, m)
				}
			}
			return nil
		})
	}
}
