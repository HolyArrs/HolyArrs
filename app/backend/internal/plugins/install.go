package plugins

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// Install refusals.
var (
	ErrHashMismatch = errors.New("plugin file hash does not match its manifest")
	ErrNotOnList    = errors.New("plugin is not on the signed plugin list")
	ErrBlocked      = errors.New("plugin is on the signed blocklist")
	ErrFileTooLarge = errors.New("plugin file is too large")
	ErrStaleBlock   = errors.New("blocklist is older than the plugin list requires")
	ErrManifestHash = errors.New("plugin manifest does not match the signed plugin list")
)

// maxFileSize caps a plugin file (a var so tests can lower it).
var maxFileSize int64 = 512 << 20

// InstallInput is everything one install check needs. Counters are those of
// the last lists accepted, so an older list is refused.
type InstallInput struct {
	PublicKey        ed25519.PublicKey
	List             []byte
	LastListCounter  uint64
	Blocklist        []byte
	LastBlockCounter uint64
	Manifest         []byte
	File             io.Reader
	// TempDir holds the checked file until it is installed; "" is the
	// system temp folder.
	TempDir string
}

// Accepted is the result of a passed check; the caller stores the counters
// and installs FilePath, the file whose hash was checked. Store.Install
// hashes it again while copying, so only those bytes are installed. Close
// removes the file.
type Accepted struct {
	Manifest     *Manifest
	FilePath     string
	ListCounter  uint64
	BlockCounter uint64
}

// Close removes the checked file.
func (a *Accepted) Close() error {
	if a == nil || a.FilePath == "" {
		return nil
	}
	return os.Remove(a.FilePath)
}

// CheckInstall accepts a plugin only when both lists verify, the file hash
// matches the manifest, the exact id, version and hash are on the list, and
// nothing on the blocklist covers it. Anything else is refused.
func CheckInstall(in InstallInput) (*Accepted, error) {
	list, err := VerifyList(in.PublicKey, in.List, KindPlugins, in.LastListCounter)
	if err != nil {
		return nil, fmt.Errorf("plugin list: %w", err)
	}
	block, err := VerifyList(in.PublicKey, in.Blocklist, KindBlocklist, in.LastBlockCounter)
	if err != nil {
		return nil, fmt.Errorf("blocklist: %w", err)
	}
	if block.Counter < *list.MinBlocklistCounter {
		return nil, fmt.Errorf("%w: counter %d, need %d", ErrStaleBlock, block.Counter, *list.MinBlocklistCounter)
	}
	m, err := ParseManifest(in.Manifest)
	if err != nil {
		return nil, err
	}
	if in.File == nil {
		return nil, ErrHashMismatch
	}
	msum := sha256.Sum256(in.Manifest)
	manHash := hex.EncodeToString(msum[:])
	var listed *Entry
	for i, e := range list.Entries {
		if e.ID == m.ID && e.Version == m.Version && e.SHA256 == m.SHA256 {
			listed = &list.Entries[i]
			break
		}
	}
	if listed == nil {
		return nil, fmt.Errorf("%w: %s %s", ErrNotOnList, m.ID, m.Version)
	}
	if listed.ManifestSHA256 != manHash {
		return nil, fmt.Errorf("%w: %s %s", ErrManifestHash, m.ID, m.Version)
	}
	for _, e := range block.Entries {
		if e.blocks(m) {
			return nil, fmt.Errorf("%w: %s %s", ErrBlocked, m.ID, m.Version)
		}
	}
	path, err := spoolFile(in.File, in.TempDir, m.SHA256)
	if err != nil {
		return nil, err
	}
	return &Accepted{Manifest: m, FilePath: path, ListCounter: list.Counter, BlockCounter: block.Counter}, nil
}

// spoolFile streams r to a private temp file while hashing it, and keeps the
// file only when the size and hash are right.
func spoolFile(r io.Reader, dir, want string) (string, error) {
	// The first install makes the plugins folder.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("plugins folder: %w", err)
	}
	f, err := os.CreateTemp(dir, "plugin-*.part")
	if err != nil {
		return "", fmt.Errorf("plugin temp file: %w", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, maxFileSize+1))
	if err == nil {
		err = f.Close()
	} else {
		f.Close()
	}
	fail := func(e error) (string, error) { os.Remove(f.Name()); return "", e }
	if err != nil {
		return fail(fmt.Errorf("reading plugin file: %w", err))
	}
	if n > maxFileSize {
		return fail(fmt.Errorf("%w: over %d bytes", ErrFileTooLarge, maxFileSize))
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fail(fmt.Errorf("%w: file %s, manifest %s", ErrHashMismatch, got, want))
	}
	return f.Name(), nil
}
