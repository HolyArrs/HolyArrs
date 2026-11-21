package youtube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Name lengths in bytes: video titles, and person and folder names.
const (
	TitleBytes = 120
	NameBytes  = 60
)

// SafeName makes s safe as one file or folder name on every system: path
// and reserved characters and control characters become "_", runs of space
// become one space, leading and trailing spaces and dots go, and the result
// is cut to maxBytes on a character boundary. Nothing left gives "untitled".
func SafeName(s string, maxBytes int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case strings.ContainsRune(`/\:*?"<>|`, r) || unicode.IsControl(r):
			r = '_'
		case unicode.IsSpace(r):
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	out := trimName(b.String())
	if len(out) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut--
		}
		out = trimName(out[:cut])
	}
	if out == "" || out == "." || out == ".." {
		return "untitled"
	}
	return out
}

func trimName(s string) string {
	return strings.Trim(s, " .")
}

// VideoBase is a video's file name without its extension:
// "<YYYY-MM-DD> <title> [<video id>]", with "0000-00-00" when the upload
// date is not known.
func VideoBase(uploaded *time.Time, title, videoID string) string {
	day := "0000-00-00"
	if uploaded != nil {
		day = uploaded.UTC().Format("2006-01-02")
	}
	return fmt.Sprintf("%s %s [%s]", day, SafeName(title, TitleBytes), videoID)
}

// UserDir returns a person's folder under root, "<id>-<name>". A folder
// that already starts with "<id>-" is kept even after a rename; otherwise
// one is made.
func UserDir(root string, userID int64, name string) (string, error) {
	dir, err := userDirName(root, userID, name)
	if err != nil {
		return "", err
	}
	dir = filepath.Join(root, dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// userDirName is the name UserDir uses for a person's folder, without
// making anything.
func userDirName(root string, userID int64, name string) (string, error) {
	prefix := fmt.Sprintf("%d-", userID)
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			return e.Name(), nil
		}
	}
	return prefix + SafeName(name, NameBytes), nil
}
