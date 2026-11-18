package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// missingKeyFolder is a config folder whose database holds the key's check
// value and whose key file is gone.
func missingKeyFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s := openStore(t, dir)
	key, err := LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), s, key); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, KeyFile)); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A missing key says the exact path it looked for, that the database is in
// that folder, and which files look like the key but are not named for it.
func TestMissingKeyNamesWhereItLookedAndLookAlikes(t *testing.T) {
	dir := missingKeyFolder(t)
	const inside = "KEYFILECONTENT"
	for _, f := range []string{"Holyarrs.key", "holyarrs.key.txt", filepath.Join("old", "holyarrs.key"), "notes.txt"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(inside), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	caps := "Holyarrs.key"
	if _, err := os.Stat(filepath.Join(dir, KeyFile)); err == nil {
		// A case-insensitive disk cannot hold both names.
		if err := os.Remove(filepath.Join(dir, KeyFile)); err != nil {
			t.Fatal(err)
		}
		caps = ""
	}
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.Is(err, ErrKeyMissing) || !errors.As(err, &h) {
		t.Fatalf("want a missing key with a hint: %v", err)
	}
	plain := h.Plain()
	for _, want := range []string{filepath.Join(dir, KeyFile), "holyarrs.db", caps, "holyarrs.key.txt", "old/holyarrs.key"} {
		if !strings.Contains(plain, want) {
			t.Errorf("hint lacks %q: %s", want, plain)
		}
	}
	if strings.Contains(plain, "notes.txt") {
		t.Errorf("a plain file was listed: %s", plain)
	}
	if strings.Contains(plain+err.Error(), inside) {
		t.Fatal("the key file's content was printed")
	}
	if !strings.Contains(err.Error(), "holyarrs.key.txt") {
		t.Errorf("the log line lacks the look-alike: %v", err)
	}
}

// With nothing like it in the folder the hint says so and tells the person
// to check which folder is mapped.
func TestMissingKeyWithNoLookAlike(t *testing.T) {
	dir := missingKeyFolder(t)
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.As(err, &h) {
		t.Fatalf("no hint: %v", err)
	}
	if len(h.Similar) != 0 || !h.DBFound {
		t.Fatalf("similar %v, database found %v", h.Similar, h.DBFound)
	}
	if !strings.Contains(h.Plain(), "no file like it") {
		t.Errorf("hint: %s", h.Plain())
	}
}

// A named key file that is not there gets the same hint.
func TestNamedKeyMissingNamesWhereItLooked(t *testing.T) {
	dir := missingKeyFolder(t)
	if err := os.WriteFile(filepath.Join(dir, "Holyarrs.KEY.bak.key"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(dir, "elsewhere.key")
	_, err := LoadKey(dir, named)
	var h *KeyHint
	if !errors.Is(err, ErrKeyUnreadable) || !errors.As(err, &h) {
		t.Fatalf("want an unreadable named key with a hint: %v", err)
	}
	if !strings.Contains(h.Plain(), named) || !strings.Contains(h.Plain(), "Holyarrs.KEY.bak.key") {
		t.Errorf("hint: %s", h.Plain())
	}
}

// Odd and huge folders: names are cleaned of control characters, the list
// is short and the scan stops, and the answer still comes.
func TestMissingKeyHintStaysSmallAndClean(t *testing.T) {
	dir := missingKeyFolder(t)
	if err := os.WriteFile(filepath.Join(dir, "bad\nname.key"), []byte("x"), 0o600); err != nil {
		t.Skip("this disk cannot hold that name")
	}
	for i := 0; i < 600; i++ {
		if err := os.WriteFile(filepath.Join(dir, "k"+strings.Repeat("x", 100)+string(rune('a'+i%26))+".key"+string(rune('0'+i%10))+".bak.key"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.As(err, &h) {
		t.Fatalf("no hint: %v", err)
	}
	if len(h.Similar) > similarMax {
		t.Errorf("%d names listed", len(h.Similar))
	}
	for _, n := range h.Similar {
		if strings.ContainsAny(n, "\n\r") || len([]rune(n)) > similarWidth+3 {
			t.Errorf("name not tidy: %q", n)
		}
	}
}

// A named key file is searched for next to the named path, and the advice
// never tells the person to rename a file to the config folder's key name.
func TestNamedKeyHintTalksAboutTheNamedPath(t *testing.T) {
	dir := missingKeyFolder(t)
	if err := os.WriteFile(filepath.Join(dir, KeyFile), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(t.TempDir(), "hk")
	_, err := LoadKey(dir, named)
	var h *KeyHint
	if !errors.As(err, &h) {
		t.Fatalf("no hint: %v", err)
	}
	p := h.Plain()
	if strings.Contains(p, "should be there too") || strings.Contains(p, "config folder") || strings.Contains(p, "Rename") || strings.Contains(p, KeyFile) {
		t.Errorf("advice is about the wrong folder: %s", p)
	}
	if !strings.Contains(p, named) || !strings.Contains(p, "no file like it in") {
		t.Errorf("hint: %s", p)
	}
	other := filepath.Dir(named)
	if err := os.WriteFile(filepath.Join(other, "hk.key"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadKey(dir, named)
	if !errors.As(err, &h) || !strings.Contains(h.Plain(), "hk.key") || !strings.Contains(h.Plain(), EnvKeyFile) {
		t.Errorf("hint: %v", err)
	}
}

// A link to a file that is not there is named as such, not "no file like it".
func TestMissingKeyDanglingLink(t *testing.T) {
	dir := missingKeyFolder(t)
	if err := os.Symlink(filepath.Join(dir, "gone", "k"), filepath.Join(dir, KeyFile)); err != nil {
		t.Skip("this disk cannot hold links")
	}
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.As(err, &h) {
		t.Fatalf("no hint: %v", err)
	}
	if !strings.Contains(h.Plain(), "is a link to") || strings.Contains(h.Plain(), "no file like it") {
		t.Errorf("hint: %s", h.Plain())
	}
}

// A name with a space or a non-breaking space at the end still counts, and
// is shown in quotes so the space can be seen.
func TestMissingKeyHiddenSpaceInName(t *testing.T) {
	dir := missingKeyFolder(t)
	names := []string{"holyarrs.key\u00a0"}
	if runtime.GOOS != "windows" {
		// Windows drops a trailing space, so there it would be the key itself.
		names = append(names, "holyarrs.key ")
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Skip("this disk cannot hold that name")
		}
	}
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.As(err, &h) {
		t.Fatalf("no hint: %v", err)
	}
	p := h.Plain()
	if len(h.Similar) == 0 || !strings.Contains(p, `"holyarrs.key`) {
		t.Errorf("hint: %s", p)
	}
}

// A busy early subfolder does not use up the scan before later top-level
// names are checked.
func TestMissingKeyTopLevelFirst(t *testing.T) {
	dir := missingKeyFolder(t)
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < similarScan+50; i++ {
		if err := os.WriteFile(filepath.Join(dir, "a", "f"+strings.Repeat("0", i%7)+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676))), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "holyarrs.key.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.As(err, &h) || !strings.Contains(h.Plain(), "holyarrs.key.txt") {
		t.Errorf("look-alike missed: %v", err)
	}
	if !strings.Contains(h.Plain(), dir) && len(h.Similar) == 0 {
		t.Errorf("hint: %s", h.Plain())
	}
}

// With no look-alike the advice names the real config folder.
func TestMissingKeyNoLookAlikeNamesFolder(t *testing.T) {
	dir := missingKeyFolder(t)
	_, err := LoadKey(dir, "")
	var h *KeyHint
	if !errors.As(err, &h) || !strings.Contains(h.Plain(), "("+dir+")") {
		t.Errorf("hint: %v", err)
	}
}

// A space at the end of the named path is shown and the real file is found.
func TestNamedKeyTrailingSpaceInSetting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hk"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "hk") + " "
	plain := WhereNamedLooked(dir, p).Plain()
	for _, want := range []string{strconv.Quote(p), "hk"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("want %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "no file like it") {
		t.Fatalf("the file is there: %q", plain)
	}
}

// Look-alikes of a named file are judged against that file's own name.
func TestNamedKeyLookAlikesUseTheNamedName(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"HK", "hk2.bak", "tls.key"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plain := WhereNamedLooked(dir, filepath.Join(dir, "hk")).Plain()
	if !strings.Contains(plain, "HK (different capitals)") || !strings.Contains(plain, "hk2.bak") {
		t.Fatalf("%q", plain)
	}
	// A file that only differs from holyarrs.key is not "different capitals".
	if err := os.WriteFile(filepath.Join(dir, "Holyarrs.key"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := WhereNamedLooked(dir, filepath.Join(dir, "hk")).Plain(); strings.Contains(p, "Holyarrs.key (different capitals)") {
		t.Fatalf("%q", p)
	}
}

// A folder that is not there is said so.
func TestKeyHintFolderMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope", "hk")
	plain := WhereNamedLooked("", p).Plain()
	if !strings.Contains(plain, "is not there either") || !strings.Contains(plain, "mount") {
		t.Fatalf("%q", plain)
	}
}

// The recheck while the page is held does not build a hint.
func TestHeldRecheckBuildsNoHint(t *testing.T) {
	_, err := LoadKey("", filepath.Join(t.TempDir(), "gone.key"))
	var h *KeyHint
	if !errors.Is(err, ErrKeyUnreadable) || errors.As(err, &h) {
		t.Fatalf("want a plain error: %v", err)
	}
}

// Browser duplicates of the key file are recognised.
func TestKeyHintDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"holyarrs.key (1)", "holyarrs.key~", "holyarrs (2).key"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plain := WhereLooked(dir, filepath.Join(dir, KeyFile)).Plain()
	for _, want := range []string{"holyarrs.key (1)", "holyarrs.key~", "holyarrs (2).key"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("want %q in %q", want, plain)
		}
	}
}
