// Package plugins is the Plugin Engine: plugin manifests, the signed plugin
// list and blocklist, and the checks a file passes before it is installed.
package plugins

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

var (
	idRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	semverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
)

// Display limits for the optional manifest fields.
const (
	maxDescription = 240
	maxIcon        = 48 << 10
)

// ErrManifest marks every manifest refusal.
var ErrManifest = errors.New("plugin manifest refused")

// Manifest describes one plugin program and the access it asks for.
type Manifest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Author  string `json:"author"`
	// Description is one or two plain lines on what the plugin does; optional.
	Description string `json:"description,omitempty"`
	// Icon is a small base64 PNG or SVG data URL shown in an img tag; optional.
	Icon          string `json:"icon,omitempty"`
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

// ParseManifest reads a manifest and refuses unknown, duplicate, missing or
// empty fields and unsafe ids, versions, folders and hosts.
func ParseManifest(data []byte) (*Manifest, error) {
	top, err := strictKeys(data, "", []string{"description", "icon"}, "id", "name", "version", "author", "access", "min_app_version", "sha256")
	if err != nil {
		return nil, err
	}
	if _, err := strictKeys(top["access"], "access.", nil, "hosts", "folders", "actions"); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	if err := requireEOF(dec); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	for name, v := range map[string]string{
		"id": m.ID, "name": m.Name, "version": m.Version, "author": m.Author, "min_app_version": m.MinAppVersion,
	} {
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%w: %q is empty", ErrManifest, name)
		}
	}
	if !idRe.MatchString(m.ID) {
		return nil, fmt.Errorf("%w: \"id\" must be lowercase letters, digits and dashes", ErrManifest)
	}
	if !semverRe.MatchString(m.Version) {
		return nil, fmt.Errorf("%w: \"version\" must be plain semver", ErrManifest)
	}
	if !validSHA256(m.SHA256) {
		return nil, fmt.Errorf("%w: \"sha256\" must be 64 lowercase hex characters", ErrManifest)
	}
	if len(m.Description) > maxDescription || strings.ContainsAny(m.Description, "<>") {
		return nil, fmt.Errorf("%w: \"description\" is too long or has markup", ErrManifest)
	}
	if m.Icon != "" && (len(m.Icon) > maxIcon || !(strings.HasPrefix(m.Icon, "data:image/png;base64,") || strings.HasPrefix(m.Icon, "data:image/svg+xml;base64,"))) {
		return nil, fmt.Errorf("%w: \"icon\" must be a base64 PNG or SVG data URL under 48 KB", ErrManifest)
	}
	for _, f := range m.Access.Folders {
		// Folders are relative to the plugin's own area: no "/x" or "C:".
		if !safeItem(f) || path.IsAbs(f) || strings.Contains(f, ":") {
			return nil, fmt.Errorf("%w: unsafe folder %q", ErrManifest, f)
		}
	}
	for _, h := range m.Access.Hosts {
		if !safeItem(h) {
			return nil, fmt.Errorf("%w: unsafe host %q", ErrManifest, h)
		}
	}
	return &m, nil
}

// safeItem refuses empty strings, ".." and backslashes.
func safeItem(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.Contains(s, "..") && !strings.Contains(s, `\`)
}

// strictKeys checks data is one JSON object whose keys are exactly keys (plus
// any of optional), each once and keys not null, and returns its values.
func strictKeys(data []byte, prefix string, optional []string, keys ...string) (map[string]json.RawMessage, error) {
	name := strings.TrimSuffix(prefix, ".")
	notObject := fmt.Errorf("%w: %q must be a JSON object", ErrManifest, name)
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, notObject
	}
	allowed := map[string]bool{}
	for _, k := range append(append([]string{}, keys...), optional...) {
		allowed[k] = true
	}
	obj := map[string]json.RawMessage{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, notObject
		}
		k, _ := t.(string)
		if !allowed[k] {
			return nil, fmt.Errorf("%w: unknown field %q", ErrManifest, prefix+k)
		}
		if _, dup := obj[k]; dup {
			return nil, fmt.Errorf("%w: duplicate field %q", ErrManifest, prefix+k)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, notObject
		}
		obj[k] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, notObject
	}
	if err := requireEOF(dec); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	for _, k := range keys {
		if v, ok := obj[k]; !ok || string(v) == "null" {
			return nil, fmt.Errorf("%w: missing field %q", ErrManifest, prefix+k)
		}
	}
	return obj, nil
}

// requireEOF refuses anything after the decoded value.
func requireEOF(dec *json.Decoder) error {
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("extra data after the JSON value")
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
