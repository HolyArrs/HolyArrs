package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// A plain key has no passphrase: one base64 line of the 8-byte key id and
// the 32-byte ed25519 seed. The release automation keeps it only as a
// GitHub environment secret, like the plugin signing key.

const plainLen = 8 + ed25519.SeedSize

// keygenPlain writes a new plain key to w and its public key file to pubPath.
func keygenPlain(w io.Writer, pubPath string) (updates.PublicKey, error) {
	var none updates.PublicKey
	if _, err := os.Stat(pubPath); err == nil {
		return none, fmt.Errorf("%s already exists; not overwriting", pubPath)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return none, err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return none, err
	}
	k := publicOf(id, priv)
	if dir := filepath.Dir(pubPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return none, err
		}
	}
	pubText := "untrusted comment: HolyArrs update list key " + k.KeyID() + "\n" + publicLine(k) + "\n"
	if err := writeNew(pubPath, []byte(pubText), 0o644); err != nil {
		return none, err
	}
	line := base64.StdEncoding.EncodeToString(append(id[:], priv.Seed()...))
	if _, err := fmt.Fprintln(w, line); err != nil {
		_ = os.Remove(pubPath)
		return none, err
	}
	return k, nil
}

// parsePlain reads a plain key; ok is false when data is not one.
func parsePlain(data []byte) (id [8]byte, priv ed25519.PrivateKey, ok bool) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(b) != plainLen {
		return id, nil, false
	}
	copy(id[:], b[:8])
	return id, ed25519.NewKeyFromSeed(b[8:]), true
}

func runKeygenPlain(pubOut string) error {
	if pubOut == "" {
		return errors.New("-plain needs -pipe and -pub")
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("-pipe writes the private key to stdout; pipe it into a secret store, not a terminal")
	}
	k, err := keygenPlain(os.Stdout, pubOut)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Public key:", publicLine(k))
	fmt.Fprintln(os.Stderr, "Key id:    ", k.KeyID())
	return nil
}

// listFlags make one stable release's update list.
type listFlags struct {
	version, dist, base, notes, out string
	serial                          int64
	safe, oneWay, security          bool
}

// runList writes the update list for a release: version, changes link,
// short note, and one program per platform from dist (the bare programs
// named holyarrs_<version>_linux_<arch>), with size and sha256.
func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var f listFlags
	fs.StringVar(&f.version, "version", "", "release version (1.2.3)")
	fs.StringVar(&f.dist, "dist", "", "folder with the bare programs")
	fs.StringVar(&f.base, "base", "", "download address the files are under (GitHub release download link)")
	fs.StringVar(&f.notes, "notes", "", "short note shown with the notice")
	fs.StringVar(&f.out, "out", "", "list file to write")
	fs.Int64Var(&f.serial, "serial", 0, "list serial (higher than any before; default: now in seconds)")
	fs.BoolVar(&f.safe, "auto-safe", false, "Automatic mode may install this release")
	fs.BoolVar(&f.oneWay, "one-way", false, "going back from this release is not possible")
	fs.BoolVar(&f.security, "security", false, "security release")
	_ = fs.Parse(args)
	if f.version == "" || f.dist == "" || f.base == "" || f.out == "" {
		return errors.New("-version, -dist, -base and -out are required")
	}
	l, err := makeList(f, time.Now().UTC())
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(f.out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("Update list for %s written to %s (%d programs)\n", f.version, f.out, len(l.Channels["stable"].Files))
	return nil
}

// arches maps the download name to the app's platform name.
var arches = map[string]string{"amd64": "linux-amd64", "arm64": "linux-arm64", "armv7": "linux-armv7"}

func makeList(f listFlags, now time.Time) (*updates.List, error) {
	if updates.Plain(f.version, 40) != f.version || !strings.Contains(f.version, ".") {
		return nil, fmt.Errorf("not a version: %q", f.version)
	}
	if f.serial == 0 {
		f.serial = now.Unix()
	}
	c := updates.Channel{
		Version:        f.version,
		ReleasedAt:     now,
		ChangelogURL:   updates.ProjectPages + "releases/tag/" + f.version,
		NotesShort:     updates.Plain(f.notes, updates.MaxNotes),
		OneWay:         f.oneWay,
		AutoUpdateSafe: f.safe && !f.oneWay,
		Security:       f.security,
		Files:          map[string]updates.File{},
		DataCompat:     updates.DataCompat,
	}
	for arch, platform := range arches {
		name := "holyarrs_" + f.version + "_linux_" + arch
		data, err := os.ReadFile(filepath.Join(f.dist, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(f.dist, name+updates.SigSuffix)); err != nil {
			return nil, fmt.Errorf("%s has no signature: sign the programs first", name)
		}
		sum := sha256.Sum256(data)
		file := updates.File{URL: strings.TrimSuffix(f.base, "/") + "/" + name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
		if !updates.DownloadAllowed(file.URL) {
			return nil, fmt.Errorf("the app would not download from %s", file.URL)
		}
		c.Files[platform] = file
	}
	if len(c.Files) == 0 {
		return nil, fmt.Errorf("no programs for %s in %s", f.version, f.dist)
	}
	return &updates.List{Schema: updates.Schema, Serial: f.serial, GeneratedAt: now,
		Channels: map[string]updates.Channel{"stable": c}, Revoked: []string{}}, nil
}
