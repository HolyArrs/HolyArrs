package structurecheck

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// commentExts maps a file extension to the way its comments start.
var commentExts = map[string]string{
	".go": "slash", ".ts": "slash", ".js": "slash", ".mjs": "slash", ".svelte": "slash",
	".py": "hash", ".ps1": "hash", ".sh": "hash", ".yml": "hash",
}

// commentSkip are tracked paths whose comments are not checked: frozen
// migrations, the metadata server, archives and test data.
var commentSkip = []string{
	"app/backend/internal/core/database/migrations/**",
	"services/**",
	"design-notes/**",
	"CHANGELOG.md",
	"deploy/release/tests/test_changelog_notes.py", // sample changelog text
	"**/testdata/**",
}

// aiNameAllowed are the paths that may name AI apps: the AI feature itself
// and its user docs.
var aiNameAllowed = []string{
	"app/frontend/src/lib/ai/**",
	"docs/reference/ai.md",
	"app/backend/internal/ai/**",
	"app/backend/internal/accounts/ai_*.go",
	"app/frontend/src/routes/settings/ai/**",
	"app/frontend/src/routes/settings/(home)/settings-search.ts",
}

var (
	commentRules = []struct {
		why string
		re  *regexp.Regexp
	}{
		{"roadmap number", regexp.MustCompile(`#[0-9]{2,3}\b`)},
		{"date", regexp.MustCompile(`\b20[0-9]{2}-[0-9]{2}(-[0-9]{2})?\b`)},
		{"dated note", regexp.MustCompile(`\(\w+, 1[01]-[0-3][0-9]\)`)},
		{"person's name", regexp.MustCompile(`\bLuis\b`)},
		{"em dash", regexp.MustCompile(`—`)},
	}
	aiNames = regexp.MustCompile(`(?i)\bclaude\b|\bcodex\b|\bOMC\b|co-authored-by|generated with`)
)

// globMatch matches rel against a pattern where ** spans folders.
func globMatch(pattern, rel string) bool {
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(base, "**/") {
			return strings.Contains("/"+rel, "/"+strings.TrimPrefix(base, "**/")+"/")
		}
		return strings.HasPrefix(rel, base+"/")
	}
	ok, _ := path.Match(pattern, rel)
	return ok
}

func anyGlob(patterns []string, rel string) bool {
	for _, p := range patterns {
		if globMatch(p, rel) {
			return true
		}
	}
	return false
}

func isTestPath(rel string) bool {
	return strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, ".test.ts") || strings.HasSuffix(rel, ".spec.ts")
}

// lineComments returns the comment text on each line (index = line - 1).
// It follows quotes well enough to skip markers inside strings; a block
// comment carries over to the next lines.
func lineComments(src, kind string) []string {
	lines := strings.Split(src, "\n")
	out := make([]string, len(lines))
	inBlock := ""
	for i, ln := range lines {
		var b strings.Builder
		quote := byte(0)
		for j := 0; j < len(ln); j++ {
			c := ln[j]
			if inBlock != "" {
				if strings.HasPrefix(ln[j:], inBlock) {
					j += len(inBlock) - 1
					inBlock = ""
					continue
				}
				b.WriteByte(c)
				continue
			}
			if quote != 0 {
				if c == '\\' {
					j++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			switch {
			case c == '"' || c == '\'' || c == '`':
				if kind == "hash" && c == '`' {
					continue
				}
				quote = c
			case kind == "slash" && strings.HasPrefix(ln[j:], "//"):
				b.WriteString(ln[j+2:])
				j = len(ln)
			case kind == "slash" && strings.HasPrefix(ln[j:], "/*"):
				inBlock, j = "*/", j+1
			case kind == "slash" && strings.HasPrefix(ln[j:], "<!--"):
				inBlock, j = "-->", j+3
			case kind == "hash" && strings.HasPrefix(ln[j:], "<#"):
				inBlock, j = "#>", j+1
			case kind == "hash" && c == '#' && (j == 0 || ln[j-1] == ' ' || ln[j-1] == '\t'):
				b.WriteString(ln[j+1:])
				j = len(ln)
			}
		}
		out[i] = b.String()
	}
	return out
}

// TestCommentHygiene fails on comments that carry roadmap numbers, dates,
// names or em dashes, and on AI tool names outside the AI feature.
func TestCommentHygiene(t *testing.T) {
	root := repoRoot(t)
	raw, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	hits := 0
	for _, rel := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		kind, ok := commentExts[filepath.Ext(rel)]
		if !ok || anyGlob(commentSkip, rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue // deleted in the work tree
		}
		src := strings.ReplaceAll(string(data), "\r\n", "\n")
		aiOK := anyGlob(aiNameAllowed, rel)
		codeLines := strings.Split(src, "\n")
		for i, c := range lineComments(src, kind) {
			for _, r := range commentRules {
				// The AI protocol names its versions by date, so the AI
				// feature may write those dates.
				if aiOK && r.why == "date" {
					continue
				}
				if c != "" && r.re.MatchString(c) {
					t.Errorf("%s:%d: %s in a comment", rel, i+1, r.why)
					hits++
				}
			}
			if aiOK {
				continue
			}
			// Tests may use an AI app name as data, never in a comment.
			text := codeLines[i]
			if isTestPath(rel) {
				text = c
			}
			if aiNames.MatchString(text) {
				t.Errorf("%s:%d: AI tool name outside the AI feature", rel, i+1)
				hits++
			}
		}
	}
	if hits > 0 {
		t.Logf("%d hits", hits)
	}
}
