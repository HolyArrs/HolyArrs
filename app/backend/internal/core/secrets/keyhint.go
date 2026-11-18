package secrets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// KeyHint says where a missing key file was looked for and what in that
// folder could be it, so the cause shows without a back-and-forth. It never
// holds or prints a key.
type KeyHint struct {
	// Path is the exact path that was looked for.
	Path string
	// Named is whether the person named the path (not the config folder's
	// default key file).
	Named bool
	// Folder is the folder that was searched.
	Folder string
	// DBFound is whether the database file is in the config folder.
	DBFound bool
	// Link is where Path points when it is a link to something that is not
	// there.
	Link string
	// Similar are names (relative to the folder) that look like a key file.
	Similar []string
	// FolderMissing is whether Folder is not there at all.
	FolderMissing bool
	// Why holds the reason for each Similar name, in the same order.
	Why []string
}

// Error is the log fragment.
func (h *KeyHint) Error() string {
	return h.Plain()
}

// Plain is the hint in plain words.
func (h *KeyHint) Plain() string {
	s := "HolyArrs looked for " + shown(h.Path)
	switch {
	case h.Link != "":
		s += ", which is a link to " + h.Link + ", and that is not there."
	case h.Named:
		s += "."
	case h.DBFound:
		s += " and found the database " + database.MainFile + " in the config folder, so the key should be there too."
	default:
		s += "; the database " + database.MainFile + " is not in the config folder."
	}
	if len(h.Similar) == 0 {
		if h.Link != "" {
			return s + " Put the file the link points to back, or replace the link with the key file itself."
		}
		if h.FolderMissing && h.Named {
			return s + " There is no file like it, and the folder " + h.Folder + " is not there either: check that the volume or secret is mounted there."
		}
		if h.Named {
			return s + " There is no file like it in " + h.Folder + "."
		}
		if h.FolderMissing {
			return s + "; the config folder (" + h.Folder + ") is not there: check that the volume is mounted."
		}
		return s + " There is no file like it in the config folder (" + h.Folder + "): check that the folder you mapped there is the one that held the key."
	}
	var l []string
	for i, n := range h.Similar {
		l = append(l, n+" ("+h.Why[i]+")")
	}
	if h.Named {
		return s + " Files that look like it in " + h.Folder + ": " + strings.Join(l, ", ") + ". Point " + EnvKeyFile + " at the right one, or mount it at " + shown(h.Path) + "."
	}
	return s + " Files that look like it: " + strings.Join(l, ", ") + ". Rename or move the right one to " + KeyFile + " in the config folder."
}

// WhereLooked is the hint for a key file that is not at path, for the
// config folder dir; the doctor shows it too.
func WhereLooked(dir, path string) *KeyHint { return keyHint(dir, path, false) }

// WhereNamedLooked is the same for a key file the person named, which is
// searched for next to the named path.
func WhereNamedLooked(dir, path string) *KeyHint { return keyHint(dir, path, true) }

const (
	similarMax   = 5
	similarScan  = 400
	similarWidth = 80
)

// keyHint looks, without reading any file's content, for what could be the
// key in the config folder dir (or next to a named path): other capitals,
// another key file, one level of subfolders.
func keyHint(dir, path string, named bool) *KeyHint {
	h := &KeyHint{Path: path, Named: named, Folder: dir}
	if named {
		h.Folder = filepath.Dir(path)
	}
	_, err := os.Stat(filepath.Join(dir, database.MainFile))
	_, derr := os.Stat(h.Folder)
	h.FolderMissing = errors.Is(derr, fs.ErrNotExist)
	h.DBFound = err == nil || !errors.Is(err, fs.ErrNotExist)
	if t, err := os.Readlink(path); err == nil {
		h.Link = t
	}
	base := ""
	if named {
		base = filepath.Base(path)
	}
	seen := 0
	var look func(folder, prefix string, depth int)
	look = func(folder, prefix string, depth int) {
		ents, err := os.ReadDir(folder)
		if err != nil {
			return
		}
		var subs []string
		for _, e := range ents {
			seen++
			if seen > similarScan || len(h.Similar) >= similarMax {
				return
			}
			name := e.Name()
			if e.IsDir() {
				switch name {
				case "logs", "backups", "artwork":
				default:
					if depth == 0 {
						subs = append(subs, name)
					}
				}
				continue
			}
			ref := KeyFile
			if named {
				ref = base
			}
			if filepath.Join(folder, name) == path || !(keyLike(name) || (named && sharesName(name, base))) {
				continue
			}
			why := "another key file"
			switch {
			case depth > 0:
				why = "in a subfolder"
			case named && strings.TrimFunc(base, unicode.IsSpace) != base && name == strings.TrimFunc(base, unicode.IsSpace):
				why = "your setting has a space at the end"
			case strings.EqualFold(strings.TrimFunc(name, unicode.IsSpace), strings.TrimFunc(ref, unicode.IsSpace)) && name != ref:
				why = "different capitals"
			case strings.TrimFunc(name, unicode.IsSpace) != name:
				why = "a space in the name"
			}
			h.Similar = append(h.Similar, tidy(prefix+name))
			h.Why = append(h.Why, why)
		}
		// Subfolders only after every name at this level was checked.
		for _, n := range subs {
			look(filepath.Join(folder, n), prefix+n+"/", 1)
		}
	}
	look(h.Folder, "", 0)
	return h
}

func keyLike(name string) bool {
	l := strings.ToLower(strings.TrimFunc(name, unicode.IsSpace))
	return strings.HasPrefix(l, KeyFile) || (strings.HasPrefix(l, "holyarrs") && strings.Contains(l, "key")) || strings.HasSuffix(l, ".key") || strings.Contains(l, ".key.") || strings.Contains(l, "key-kit") || strings.Contains(l, "key kit") || strings.Contains(l, "holyarrs-key")
}

// tidy keeps a name short and printable.
func tidy(n string) string {
	edge := strings.TrimFunc(n, unicode.IsSpace) != n
	n = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, n)
	if r := []rune(n); len(r) > similarWidth {
		n = string(r[:similarWidth]) + "..."
	}
	if edge {
		return strconv.Quote(n)
	}
	return n
}

// sharesName is whether name is the named file's name in other capitals, with
// spaces around it, or with more after it.
func sharesName(name, base string) bool {
	n := strings.ToLower(strings.TrimFunc(name, unicode.IsSpace))
	b := strings.ToLower(strings.TrimFunc(base, unicode.IsSpace))
	return b != "" && strings.HasPrefix(n, b)
}

// shown quotes a path that has a space at either end, so it can be seen.
func shown(p string) string {
	if strings.TrimFunc(p, unicode.IsSpace) != p {
		return strconv.Quote(p)
	}
	return p
}
