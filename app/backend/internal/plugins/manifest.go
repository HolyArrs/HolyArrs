// Package plugins is the Plugin Engine: plugin manifests, the signed plugin
// list and blocklist, and the checks a file passes before it is installed.
package plugins

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrManifest marks every manifest refusal.
var ErrManifest = errors.New("plugin manifest refused")

// Manifest describes one plugin program and the access it asks for.
type Manifest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Author        string `json:"author"`
	Access        Access `json:"access"`
	MinAppVersion string `json:"min_app_version"`
	SHA256        string `json:"sha256"`
}

// Access is what a plugin declares it needs; the Engine allows nothing else.
type Access struct {
	Hosts   []string `json:"hosts"`
	Folders []string `json:"folders"`
	Actions []string `json:"actions"`
}

// ParseManifest reads a manifest and refuses unknown, missing or empty fields.
func ParseManifest(data []byte) (*Manifest, error) {
	if err := requireKeys(data, "", "id", "name", "version", "author", "access", "min_app_version", "sha256"); err != nil {
		return nil, err
	}
	var raw struct {
		Access json.RawMessage `json:"access"`
	}
	_ = json.Unmarshal(data, &raw)
	if err := requireKeys(raw.Access, "access.", "hosts", "folders", "actions"); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: extra data after the manifest", ErrManifest)
	}
	for name, v := range map[string]string{
		"id": m.ID, "name": m.Name, "version": m.Version, "author": m.Author, "min_app_version": m.MinAppVersion,
	} {
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%w: %q is empty", ErrManifest, name)
		}
	}
	if !validSHA256(m.SHA256) {
		return nil, fmt.Errorf("%w: \"sha256\" must be 64 lowercase hex characters", ErrManifest)
	}
	return &m, nil
}

// requireKeys checks data is a JSON object holding every key, each not null.
func requireKeys(data []byte, prefix string, keys ...string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return fmt.Errorf("%w: %q must be a JSON object", ErrManifest, strings.TrimSuffix(prefix, "."))
	}
	for _, k := range keys {
		v, ok := obj[k]
		if !ok || string(v) == "null" {
			return fmt.Errorf("%w: missing field %q", ErrManifest, prefix+k)
		}
	}
	return nil
}

func validSHA256(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
