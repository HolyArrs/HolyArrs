package plugins

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// Kinds of signed list. The kind is inside the signed payload, so a
// blocklist can never be passed off as the plugin list or the other way.
const (
	KindPlugins   = "plugins"
	KindBlocklist = "blocklist"
	// KindLanguages lists the signed language packs (text only, no program).
	KindLanguages = "languages"
	// KindTest is the list of test versions: signed like the plugin list,
	// with its own counter, and only read for plugins whose owner asked
	// for test versions. Older apps never fetch it.
	KindTest = "plugins_test"
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
	Kind    string `json:"kind"`
	Counter uint64 `json:"counter"`
	// MinBlocklistCounter is required on the plugin and languages lists: a blocklist with a
	// lower counter is stale and refused at install.
	MinBlocklistCounter *uint64 `json:"min_blocklist_counter,omitempty"`
	Entries             []Entry `json:"entries"`
}

// Entry names one plugin version. On a blocklist an entry needs an id or a
// hash: an empty Version blocks every version, a hash alone blocks that file.
type Entry struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	// ManifestSHA256 is the hash of the manifest file's bytes; required on
	// the plugin list, so the manifest is signed through the list.
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	// FileURL is where the plugin file is downloaded from when it is not
	// below the list address (large files live on a file host). Optional;
	// https only. The file is still checked against SHA256.
	FileURL string `json:"file_url,omitempty"`
	// Size is the file's size in bytes, shown before install. Optional.
	Size int64 `json:"size,omitempty"`
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
	if err := requireEOF(dec); err != nil {
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
	if err := requireEOF(pdec); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListFormat, err)
	}
	if l.Kind != kind {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrListKind, l.Kind, kind)
	}
	if l.Counter < lastCounter {
		return nil, fmt.Errorf("%w: counter %d, last accepted %d", ErrListOlder, l.Counter, lastCounter)
	}
	listed := kind == KindPlugins || kind == KindTest
	if (listed || kind == KindLanguages) && l.MinBlocklistCounter == nil {
		return nil, fmt.Errorf("%w: missing min_blocklist_counter", ErrListFormat)
	}
	for i, e := range l.Entries {
		if (listed && (e.ID == "" || e.Version == "" || e.SHA256 == "" || e.ManifestSHA256 == "")) ||
			(kind == KindLanguages && (e.ID == "" || e.Version == "" || e.SHA256 == "")) {
			return nil, fmt.Errorf("%w: entry %d is incomplete", ErrListFormat, i)
		}
		if e.ID == "" && e.SHA256 == "" {
			return nil, fmt.Errorf("%w: entry %d needs an id or a hash", ErrListFormat, i)
		}
		if (e.ID != "" && !idRe.MatchString(e.ID)) || (e.Version != "" && !semverRe.MatchString(e.Version)) ||
			(e.SHA256 != "" && !validSHA256(e.SHA256)) || (e.ManifestSHA256 != "" && !validSHA256(e.ManifestSHA256)) {
			return nil, fmt.Errorf("%w: entry %d has a bad id, version or hash", ErrListFormat, i)
		}
		if e.FileURL != "" {
			if u, err := url.Parse(e.FileURL); err != nil || u.Scheme != "https" || u.Host == "" {
				return nil, fmt.Errorf("%w: entry %d has a bad file_url", ErrListFormat, i)
			}
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
	return e.ID != "" && e.ID == m.ID && (e.Version == "" || e.Version == m.Version)
}
