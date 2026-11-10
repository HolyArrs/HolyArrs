package migrate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Kind names an old app.
type Kind string

// The apps a reader exists for.
const (
	Radarr      Kind = "radarr"
	Sonarr      Kind = "sonarr"
	Lidarr      Kind = "lidarr"
	Prowlarr    Kind = "prowlarr"
	Bazarr      Kind = "bazarr"
	Seerr       Kind = "seerr"
	Overseerr   Kind = "overseerr"
	QBittorrent Kind = "qbittorrent"
	SABnzbd     Kind = "sabnzbd"
)

// Kinds lists every kind a reader exists for.
var Kinds = []Kind{Radarr, Sonarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, QBittorrent, SABnzbd}

// kindNames are the apps' own names, for text shown to the owner.
var kindNames = map[Kind]string{Radarr: "Radarr", Sonarr: "Sonarr", Lidarr: "Lidarr", Prowlarr: "Prowlarr", Bazarr: "Bazarr",
	Seerr: "Seerr", Overseerr: "Overseerr", QBittorrent: "qBittorrent", SABnzbd: "SABnzbd"}

// kindName names k as the app does, or k itself.
func kindName(k Kind) string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return string(k)
}

// ValidKind reports whether k has a reader.
func ValidKind(k Kind) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Ways a Snapshot was read.
const (
	ViaAPI    = "api"
	ViaBackup = "backup"
)

// Source says where to read from: an API address with its key (or, for
// qBittorrent, a user name and password), or a backup path. Key and
// Password live only in memory: they are never stored, logged or put in a
// Snapshot or an error.
type Source struct {
	Kind Kind
	// URL is the app's address, with any URL base (http://10.0.0.5:7878).
	URL      string
	Key      string
	Username string
	Password string
	// Path is a backup zip, a folder or a single database or settings file.
	Path string
	// AllowLocal lets this read reach the local network (the old apps
	// usually live there); without it only public addresses are dialled.
	AllowLocal bool
	// Limits are the size and time limits; zero means DefaultLimits.
	Limits Limits
}

// Via says how the Source is read.
func (s Source) Via() string {
	if s.Path != "" {
		return ViaBackup
	}
	return ViaAPI
}

// Info is what a probe finds: the app and its version.
type Info struct {
	Kind    Kind   `json:"kind"`
	Via     string `json:"via"`
	Version string `json:"version"`
	// Schema is the database's migration number (backups and arr apps).
	Schema int `json:"schema,omitempty"`
	// Notes are things the reader will skip or could not tell.
	Notes []string `json:"notes,omitempty"`
}

// Reader reads one app's setup.
type Reader interface {
	Kind() Kind
	// Probe checks the Source is this app and says which version.
	Probe(ctx context.Context, src Source) (Info, error)
	// Read reads the whole setup.
	Read(ctx context.Context, src Source) (*Snapshot, error)
}

// Errors a reader answers with. Their text is safe to show.
var (
	ErrWrongApp     = errors.New("this is not the app you picked")
	ErrUnreachable  = errors.New("the app does not answer")
	ErrBadKey       = errors.New("the app refused the key or password")
	ErrTooBig       = errors.New("the answer or file is bigger than the import tool reads")
	ErrNotBackup    = errors.New("no backup of this app was found there")
	ErrUnsafeBackup = errors.New("the backup has an entry that leaves it, a link, or unpacks too big")
	ErrNotSQLite    = errors.New("the database file is not a SQLite database")
	ErrBadAnswer    = errors.New("the app's answer could not be read")
	ErrBadSource    = errors.New("give an address with a key, or a backup path")
	ErrVersion      = errors.New("this version of the app is not one the import tool reads")
	ErrBackupBusy   = errors.New("another backup is being read; try again when it is done")
)

// For returns the reader for a kind.
func For(k Kind) (Reader, error) {
	switch k {
	case Radarr, Sonarr, Lidarr, Prowlarr:
		return arrReader{kind: k}, nil
	case Bazarr:
		return bazarrReader{}, nil
	case Seerr, Overseerr:
		return seerrReader{kind: k}, nil
	case QBittorrent:
		return qbitReader{}, nil
	case SABnzbd:
		return sabReader{}, nil
	}
	return nil, fmt.Errorf("unknown app %q", k)
}

// check validates a Source before any read.
func check(src Source) error {
	if !ValidKind(src.Kind) {
		return fmt.Errorf("unknown app %q", src.Kind)
	}
	if (src.Path == "") == (src.URL == "") {
		return ErrBadSource
	}
	return nil
}

// Probe probes src with the reader for its kind.
func Probe(ctx context.Context, src Source) (Info, error) {
	if err := check(src); err != nil {
		return Info{}, err
	}
	r, err := For(src.Kind)
	if err != nil {
		return Info{}, err
	}
	return r.Probe(ctx, src)
}

// Read reads src with the reader for its kind. The Snapshot is checked
// for the typed key and password before it is handed back.
func Read(ctx context.Context, src Source) (*Snapshot, error) {
	if err := check(src); err != nil {
		return nil, err
	}
	r, err := For(src.Kind)
	if err != nil {
		return nil, err
	}
	s, err := r.Read(ctx, src)
	if err != nil {
		return nil, err
	}
	s.Kind, s.Via = src.Kind, src.Via()
	s.finish()
	if err := s.mustNotHold(src.Key, src.Password); err != nil {
		return nil, err
	}
	return s, nil
}

// noteList keeps notes unique and sorted.
func noteList(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range in {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
