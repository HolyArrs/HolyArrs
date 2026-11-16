package plugins

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
)

// Kinds of signed list. The kind is inside the signed payload, so a
// blocklist can never be passed off as the plugin list or the other way.
const (
	KindPlugins   = "plugins"
	KindBlocklist = "blocklist"
)

// List refusals.
var (
	ErrListFormat    = errors.New("signed list is malformed")
	ErrListSignature = errors.New("signed list signature does not match")
	ErrListOlder     = errors.New("signed list is older than the last accepted one")
	ErrListKind      = errors.New("signed list is the wrong kind")
)

// SignedList is the file we host: a JSON payload and its ed25519 signature.
// Signatures carry no expiry.
type SignedList struct {
	Payload   []byte `json:"payload"`   // base64 in JSON
	Signature []byte `json:"signature"` // base64 in JSON
}

// List is the verified payload.
type List struct {
	Kind    string  `json:"kind"`
	Counter uint64  `json:"counter"`
	Entries []Entry `json:"entries"`
}

// Entry names one plugin version. On a blocklist an empty Version blocks
// every version and SHA256 may be empty.
type Entry struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// VerifyList checks the signature with pub, the kind, and that the counter is
// not below lastCounter (the counter of the last list accepted; 0 at first).
func VerifyList(pub ed25519.PublicKey, data []byte, kind string, lastCounter uint64) (*List, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: public key has the wrong size", ErrListSignature)
	}
	var sl SignedList
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sl); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListFormat, err)
	}
	if len(sl.Payload) == 0 || len(sl.Signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: payload or signature missing", ErrListFormat)
	}
	if !ed25519.Verify(pub, sl.Payload, sl.Signature) {
		return nil, ErrListSignature
	}

	var l List
	pdec := json.NewDecoder(bytes.NewReader(sl.Payload))
	pdec.DisallowUnknownFields()
	if err := pdec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListFormat, err)
	}
	if l.Kind != kind {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrListKind, l.Kind, kind)
	}
	if l.Counter < lastCounter {
		return nil, fmt.Errorf("%w: counter %d, last accepted %d", ErrListOlder, l.Counter, lastCounter)
	}
	for i, e := range l.Entries {
		if e.ID == "" || (kind == KindPlugins && (e.Version == "" || !validSHA256(e.SHA256))) {
			return nil, fmt.Errorf("%w: entry %d is incomplete", ErrListFormat, i)
		}
	}
	return &l, nil
}

// SignList builds a signed list file. The Engine never signs; this serves
// the signing tool and tests.
func SignList(priv ed25519.PrivateKey, l List) ([]byte, error) {
	payload, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	return json.Marshal(SignedList{Payload: payload, Signature: ed25519.Sign(priv, payload)})
}

// blocks reports whether a blocklist entry covers this plugin version or file.
func (e Entry) blocks(m *Manifest) bool {
	if e.SHA256 != "" && e.SHA256 == m.SHA256 {
		return true
	}
	return e.ID == m.ID && (e.Version == "" || e.Version == m.Version)
}
