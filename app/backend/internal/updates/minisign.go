package updates

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// PinnedKeys are the trusted minisign public keys (main and next), in
// minisign's public key format (the base64 line of a .pub file).
// The list stays empty until release keys exist; while it is empty the
// release feed check is used instead.
var PinnedKeys = []string{}

// ErrSignature is any problem with the signature: the file is ignored.
var ErrSignature = errors.New("the update list signature is not valid")

// PublicKey is one minisign public key.
type PublicKey struct {
	ID  [8]byte
	Key ed25519.PublicKey
}

// KeyID is minisign's key id in hex, as minisign prints it.
func (k PublicKey) KeyID() string { return keyHex(k.ID) }

func keyHex(id [8]byte) string {
	r := make([]byte, 8)
	for i := range id {
		r[i] = id[7-i]
	}
	return strings.ToUpper(hex.EncodeToString(r))
}

// ParsePublicKey reads a minisign public key: the base64 line alone or a
// whole .pub file (its comment line is ignored).
func ParsePublicKey(s string) (PublicKey, error) {
	var k PublicKey
	line := ""
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "untrusted comment:") {
			line = l
			break
		}
	}
	b, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(b) != 2+8+ed25519.PublicKeySize || string(b[:2]) != "Ed" {
		return k, errors.New("not a minisign public key")
	}
	copy(k.ID[:], b[2:10])
	k.Key = ed25519.PublicKey(bytes.Clone(b[10:]))
	return k, nil
}

// Pinned parses PinnedKeys; a key that does not parse is skipped.
func Pinned() []PublicKey {
	var out []PublicKey
	for _, s := range PinnedKeys {
		if k, err := ParsePublicKey(s); err == nil {
			out = append(out, k)
		}
	}
	return out
}

// Verify checks a minisign signature of data against the pinned keys and
// returns the trusted comment. "ED" signs the BLAKE2b-512 of the file,
// legacy "Ed" signs the file itself. The global signature over the
// signature and the trusted comment must verify too. The untrusted comment
// is never used.
func Verify(data, sig []byte, keys []PublicKey) (string, error) {
	lines := strings.Split(strings.ReplaceAll(string(sig), "\r", ""), "\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "untrusted comment:") || !strings.HasPrefix(lines[2], "trusted comment: ") {
		return "", ErrSignature
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(raw) != 2+8+ed25519.SignatureSize {
		return "", ErrSignature
	}
	trusted := strings.TrimPrefix(lines[2], "trusted comment: ")
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != ed25519.SignatureSize {
		return "", ErrSignature
	}
	var id [8]byte
	copy(id[:], raw[2:10])
	var key *PublicKey
	for i := range keys {
		if keys[i].ID == id {
			key = &keys[i]
			break
		}
	}
	if key == nil {
		return "", fmt.Errorf("%w: unknown key id %s", ErrSignature, keyHex(id))
	}
	msg := data
	switch string(raw[:2]) {
	case "ED":
		h := blake2b.Sum512(data)
		msg = h[:]
	case "Ed":
	default:
		return "", ErrSignature
	}
	s := raw[10:]
	if !ed25519.Verify(key.Key, msg, s) {
		return "", ErrSignature
	}
	if !ed25519.Verify(key.Key, append(bytes.Clone(s), trusted...), global) {
		return "", ErrSignature
	}
	return trusted, nil
}
