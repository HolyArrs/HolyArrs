package plugins

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Install refusals.
var (
	ErrHashMismatch = errors.New("plugin file hash does not match its manifest")
	ErrNotOnList    = errors.New("plugin is not on the signed plugin list")
	ErrBlocked      = errors.New("plugin is on the signed blocklist")
)

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
}

// Accepted is the result of a passed check; the caller stores the counters.
type Accepted struct {
	Manifest     *Manifest
	ListCounter  uint64
	BlockCounter uint64
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
	m, err := ParseManifest(in.Manifest)
	if err != nil {
		return nil, err
	}
	if in.File == nil {
		return nil, ErrHashMismatch
	}
	h := sha256.New()
	if _, err := io.Copy(h, in.File); err != nil {
		return nil, fmt.Errorf("reading plugin file: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != m.SHA256 {
		return nil, fmt.Errorf("%w: file %s, manifest %s", ErrHashMismatch, got, m.SHA256)
	}

	onList := false
	for _, e := range list.Entries {
		if e.ID == m.ID && e.Version == m.Version && e.SHA256 == m.SHA256 {
			onList = true
			break
		}
	}
	if !onList {
		return nil, fmt.Errorf("%w: %s %s", ErrNotOnList, m.ID, m.Version)
	}
	for _, e := range block.Entries {
		if e.blocks(m) {
			return nil, fmt.Errorf("%w: %s %s", ErrBlocked, m.ID, m.Version)
		}
	}
	return &Accepted{Manifest: m, ListCounter: list.Counter, BlockCounter: block.Counter}, nil
}
