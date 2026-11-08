package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bannedWords are upstream coined words glossary.md tells us to avoid in
// labels, API names and descriptions, docs and plans.
var bannedWords = []string{"Monitored", "Activity", "Wanted", "Cutoff", "Quality Profile", "Root Folder", "Download Client",
	"Interactive Search", "Manual Import", "Grabbed", "Connections", "Discover", "Collections", "Series"}

// wordExceptions are the only places a banned word may stand, each with
// why (logged in decisions/log-autonomous.md, 5.1.18): rule lines that list
// the words to avoid, and the pre-phase research plan, which quotes the
// upstream apps on the research side.
var wordExceptions = []struct{ file, line, why string }{
	{"plans/active/2026-10-12-walking-skeleton.md", "- Words: glossary terms in code", "the rule naming the words to avoid"},
	{"plans/active/2026-10-13-m1-movies.md", "- Words: use glossary terms in code", "the rule naming the words to avoid"},
	{"plans/active/holyarrs-prephase-design.md", "- **Words.** Ordinary words", "the clean-room rule naming coined terms"},
	{"plans/active/holyarrs-prephase-design.md", "| 3 | **Seerr** (full)", "research side: names the upstream app's own feature"},
	{"plans/done/2026-10-10-holyarrs-prephase-run.md", "", "research-side run plan (done) quoting upstream screens"},
}

var bannedRx = func() *regexp.Regexp {
	q := make([]string, len(bannedWords))
	for i, w := range bannedWords {
		q[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`\b(` + strings.Join(q, "|") + `)\b`)
}()

// relName names p for messages and wordExceptions: repo/<path> inside the
// repo, plans/<path> beside it. Both are made absolute first, as
// filepath.Rel cannot climb out of a relative base like ../../..
func relName(repo, p string) string {
	absRepo, err1 := filepath.Abs(repo)
	absP, err2 := filepath.Abs(p)
	if err1 != nil || err2 != nil {
		return filepath.ToSlash(p)
	}
	if r, err := filepath.Rel(absRepo, absP); err == nil && !strings.HasPrefix(r, "..") {
		return "repo/" + filepath.ToSlash(r)
	}
	if r, err := filepath.Rel(filepath.Dir(absRepo), absP); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(absP)
}

func excepted(rel, line string) bool {
	rel = filepath.ToSlash(rel)
	for _, e := range wordExceptions {
		if rel == e.file && strings.Contains(line, e.line) {
			return true
		}
	}
	return false
}

// TestNoBannedWords greps the web app's source, the user docs, the README,
// every name and description in openapi.json and, when the plans folder is
// next to the repo (it is not in the repo, so CI skips that part), plans\.
func TestNoBannedWords(t *testing.T) {
	t.Parallel()
	repo := filepath.Join("..", "..", "..", "..")
	type root struct {
		dir  string
		exts string
	}
	roots := []root{{filepath.Join(repo, "app", "frontend", "src"), ".svelte .ts .js .html .css"}, {filepath.Join(repo, "docs"), ".md"},
		{filepath.Join(repo, "README.md"), ".md"}}
	// The plans folder lives outside the repo, so a repo test only checks it
	// when asked to (HOLYARRS_REQUIRE_PLANS); otherwise results would depend
	// on whatever sits next to the clone.
	plans := filepath.Join(repo, "..", "plans")
	if os.Getenv("HOLYARRS_REQUIRE_PLANS") == "" {
		t.Log("HOLYARRS_REQUIRE_PLANS not set: plans outside the repo not checked")
	} else if st, err := os.Stat(plans); err == nil && st.IsDir() {
		roots = append(roots, root{plans, ".md"})
	} else {
		t.Fatalf("HOLYARRS_REQUIRE_PLANS is set but %s is not a folder", plans)
	}
	checked := 0
	for _, r := range roots {
		err := filepath.WalkDir(r.dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.Contains(r.exts, filepath.Ext(p)) || filepath.Ext(p) == "" || strings.HasSuffix(p, ".test.ts") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			// Files in the repo show as repo/<path> (whatever the checkout's
			// folder is called), plans as plans/<path>.
			rel := relName(repo, p)
			for i, line := range strings.Split(string(b), "\n") {
				if m := bannedRx.FindString(line); m != "" && !excepted(rel, line) {
					t.Errorf("%s:%d uses %q (glossary.md); say it in HolyArrs words or add a logged exception", rel, i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 50 {
		t.Fatalf("checked only %d files; the roots moved", checked)
	}
	// API names and descriptions.
	var doc any
	if err := json.Unmarshal(mustRead(t, "openapi.json"), &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(v any, at string)
	walk = func(v any, at string) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if m := bannedRx.FindString(k); m != "" {
					t.Errorf("openapi.json %s: name %q uses %q", at, k, m)
				}
				if s, ok := val.(string); ok && (k == "summary" || k == "description" || k == "operationId" || k == "title") {
					if m := bannedRx.FindString(s); m != "" {
						t.Errorf("openapi.json %s/%s uses %q", at, k, m)
					}
				}
				walk(val, at+"/"+k)
			}
		case []any:
			for _, y := range x {
				if s, ok := y.(string); ok {
					if m := bannedRx.FindString(s); m != "" {
						t.Errorf("openapi.json %s: value %q uses %q", at, s, m)
					}
				}
				walk(y, at)
			}
		}
	}
	walk(doc, "")
}

// TestBannedWordsAreCaught keeps the check honest.
func TestBannedWordsAreCaught(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"Go to Wanted", "the Root Folder", "Interactive Search", "TV Series"} {
		if bannedRx.FindString(s) == "" {
			t.Errorf("%q passed", s)
		}
	}
	for _, s := range []string{"wanted state", "Missing", "Library folder", "Choose a release", "Unwanted"} {
		if m := bannedRx.FindString(s); m != "" {
			t.Errorf("%q caught %q", s, m)
		}
	}
	if !excepted("plans/active/2026-10-13-m1-movies.md", "- Words: use glossary terms in code, API (Monitored)") || excepted("plans/active/x.md", "- Words: use glossary terms in code") {
		t.Error("exceptions")
	}
}

// mustRead reads a file the test needs.
func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
