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
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
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

// detailKeys are the optional manifest keys.
var detailKeys = []string{"description", "icon", "icon_url", "long_description", "how_it_works", "whats_new", "developers", "updated", "licence", "repo", "category", "platforms", "support_email", "support_url", "size", "settings", "resources"}

// Limits for the optional page fields.
const (
	maxLong  = 4000
	maxShort = 600
	maxList  = 12
)

var (
	loginRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	dateRe  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	emailRe = regexp.MustCompile(`^[^\s@<>"']{1,64}@[A-Za-z0-9.-]{1,190}\.[A-Za-z]{2,24}$`)
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
	Icon string `json:"icon,omitempty"`
	// IconURL is the plugin's own logo on the plugin site, copied and
	// resized there by the plugins robot: images/<id>/icon-256.webp
	// (a 128 px copy sits next to it). Optional; it wins over Icon.
	IconURL string `json:"icon_url,omitempty"`
	// Details holds the optional store-page fields; all may be missing.
	Details
	Access Access `json:"access"`
	// Resources asks for caps above the defaults; the owner approves them.
	Resources     *Resources `json:"resources,omitempty"`
	MinAppVersion string     `json:"min_app_version"`
	SHA256        string     `json:"sha256"`
}

// Details are optional plain-text fields for the plugin's page in the app.
// Older manifests have none of them; newer apps may add more.
type Details struct {
	// LongDescription is a few plain paragraphs on what the plugin does.
	LongDescription string `json:"long_description,omitempty"`
	// HowItWorks is a short list of plain steps.
	HowItWorks []string `json:"how_it_works,omitempty"`
	// WhatsNew is what changed in this version.
	WhatsNew string `json:"whats_new,omitempty"`
	// Developers are GitHub logins.
	Developers []string `json:"developers,omitempty"`
	// Updated is the release date, YYYY-MM-DD.
	Updated  string `json:"updated,omitempty"`
	Licence  string `json:"licence,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Category string `json:"category,omitempty"`
	// Platforms are the CPU types this version is built for.
	Platforms []string `json:"platforms,omitempty"`
	// SupportEmail and SupportURL are how people reach the author for help; the
	// signed list asks for at least one, older manifests have neither.
	SupportEmail string `json:"support_email,omitempty"`
	SupportURL   string `json:"support_url,omitempty"`
	// Size is the plugin program's size in bytes for this CPU type.
	Size int64 `json:"size,omitempty"`
	// Settings are fields the owner fills in on the plugin's settings sheet.
	Settings []Setting `json:"settings,omitempty"`
}

// Access is what a plugin declares it needs; the Engine allows nothing else.
type Access struct {
	Hosts   []string    `json:"hosts"`
	Folders []FolderReq `json:"folders"`
	Actions []string    `json:"actions"`
	// Events are the moments the plugin wants told as they happen (see
	// EventNames); optional, and older manifests have none.
	Events []string `json:"events,omitempty"`
}

// EventNames are the events a plugin may ask for, in the order screens show
// them: the staff Alerts moments and more Library, download, search and
// system events. Requests and problem reports are offered with their title
// and ids only, so a plugin never learns who asked, who decided or what
// anyone wrote.
var EventNames = []string{
	"library.add", "library.delete", "library.import", "library.upgraded", "library.rename", "library.artwork", "library.subtitle",
	"library.needs_decision", "library.edited", "library.refreshed", "library.moved", "library.episodes_changed", "library.adopted",
	"library.file_missing", "library.file_back",
	"download.grab", "download.done", "download.failed", "download.rejected", "download.vanished",
	"search.done", "search.failed", "search.release_held",
	"request.made", "request.approved", "request.declined", "request.available", "report.made", "report.fixed",
	"system.health", "system.media_server_updated", "system.list_synced", "system.folder_unreachable", "system.disk_low",
}

// Folder modes.
const (
	FolderRead      = "read"
	FolderReadWrite = "read_write"
)

// maxFolders is how many folders a manifest may ask for.
const maxFolders = 100

// FolderReq is one folder a plugin asks for. The owner picks the real folder
// at install; nothing is granted before that. In a manifest it is either a
// plain name (read only) or {"name", "mode", "why", "required"}.
type FolderReq struct {
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Why      string `json:"why,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// UnmarshalJSON takes the plain-name form or the object form.
func (f *FolderReq) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err == nil {
		*f = FolderReq{Name: name, Mode: FolderRead}
		return nil
	}
	if _, err := strictKeys(b, "access.folders.", []string{"mode", "why", "required"}, "name"); err != nil {
		return err
	}
	type plain FolderReq
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	if p.Mode == "" {
		p.Mode = FolderRead
	}
	*f = FolderReq(p)
	return nil
}

// ParseManifest reads a manifest, ignores unknown fields (so newer optional
// fields don't break older apps) and refuses case variants of known fields,
// duplicate, missing or empty fields and unsafe ids, versions, folders and hosts.
func ParseManifest(data []byte) (*Manifest, error) {
	top, err := strictKeys(data, "", detailKeys, "id", "name", "version", "author", "access", "min_app_version", "sha256")
	if err != nil {
		return nil, err
	}
	if _, err := strictKeys(top["access"], "access.", []string{"events"}, "hosts", "folders", "actions"); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
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
	if mm := iconURLRe.FindStringSubmatch(m.IconURL); m.IconURL != "" && (mm == nil || mm[1] != m.ID) {
		return nil, fmt.Errorf("%w: \"icon_url\" must be images/%s/icon-256.webp", ErrManifest, m.ID)
	}
	if err := m.Details.check(); err != nil {
		return nil, err
	}
	if err := m.Access.check(); err != nil {
		return nil, err
	}
	if m.Resources != nil && newerVersion(resourcesSince, m.MinAppVersion) {
		return nil, fmt.Errorf("%w: \"resources\" needs min_app_version %s or newer", ErrManifest, resourcesSince)
	}
	if len(m.Access.Actions) > 0 && newerVersion(resourcesSince, m.MinAppVersion) {
		return nil, fmt.Errorf("%w: \"access.actions\" needs min_app_version %s or newer", ErrManifest, resourcesSince)
	}
	return &m, nil
}

// check refuses unsafe folder names and modes and host patterns that are
// not plain names (see validHostPattern).
func (a Access) check() error {
	if len(a.Folders) > maxFolders {
		return fmt.Errorf("%w: more than %d folders", ErrManifest, maxFolders)
	}
	seen := map[string]bool{}
	for _, f := range a.Folders {
		// A folder is a label the owner maps to a real folder: no "/x", "a/b" or "C:".
		if !safeItem(f.Name) || path.IsAbs(f.Name) || strings.ContainsAny(f.Name, ":/") || len(f.Name) > 40 || seen[strings.ToLower(f.Name)] {
			return fmt.Errorf("%w: unsafe folder %q", ErrManifest, f.Name)
		}
		seen[strings.ToLower(f.Name)] = true
		if f.Mode != FolderRead && f.Mode != FolderReadWrite {
			return fmt.Errorf("%w: folder %q mode must be read or read_write", ErrManifest, f.Name)
		}
		if len(f.Why) > maxShort || strings.ContainsAny(f.Why, "<>") {
			return fmt.Errorf("%w: folder %q why is too long or has markup", ErrManifest, f.Name)
		}
	}
	seenScope := map[string]bool{}
	for _, sc := range a.Actions {
		if !accounts.ValidPluginScope(sc) || seenScope[sc] {
			return fmt.Errorf("%w: unknown or repeated permission %q", ErrManifest, sc)
		}
		seenScope[sc] = true
	}
	seenEvent := map[string]bool{}
	for _, ev := range a.Events {
		if seenEvent[ev] || !slices.Contains(EventNames, ev) {
			return fmt.Errorf("%w: unknown or repeated event %q", ErrManifest, ev)
		}
		seenEvent[ev] = true
	}
	if len(a.Hosts) > maxHosts {
		return fmt.Errorf("%w: more than %d hosts", ErrManifest, maxHosts)
	}
	for _, h := range a.Hosts {
		if !safeItem(h) || !validHostPattern(h) {
			return fmt.Errorf("%w: unsafe host %q", ErrManifest, h)
		}
	}
	return nil
}

// check refuses markup, overlong text, bad logins, dates and repo links.
func (d Details) check() error {
	bad := func(what string) error { return fmt.Errorf("%w: %q is too long or not plain text", ErrManifest, what) }
	plain := func(s string, max int) bool { return len(s) <= max && !strings.ContainsAny(s, "<>") }
	if !plain(d.LongDescription, maxLong) {
		return bad("long_description")
	}
	if !plain(d.WhatsNew, maxShort*2) {
		return bad("whats_new")
	}
	for _, v := range []struct {
		name string
		list []string
	}{{"how_it_works", d.HowItWorks}, {"developers", d.Developers}, {"platforms", d.Platforms}} {
		if len(v.list) > maxList {
			return bad(v.name)
		}
		for _, s := range v.list {
			if strings.TrimSpace(s) == "" || !plain(s, maxShort) {
				return bad(v.name)
			}
		}
	}
	for _, l := range d.Developers {
		if !loginRe.MatchString(l) {
			return fmt.Errorf("%w: developer %q is not a GitHub login", ErrManifest, l)
		}
	}
	if d.Updated != "" && !dateRe.MatchString(d.Updated) {
		return fmt.Errorf("%w: \"updated\" must be YYYY-MM-DD", ErrManifest)
	}
	if d.Repo != "" && (!strings.HasPrefix(d.Repo, "https://") || !plain(d.Repo, 200) || strings.ContainsAny(d.Repo, " \"'")) {
		return fmt.Errorf("%w: \"repo\" must be an https link", ErrManifest)
	}
	if d.SupportEmail != "" && !emailRe.MatchString(d.SupportEmail) {
		return fmt.Errorf("%w: \"support_email\" is not an email address", ErrManifest)
	}
	if d.SupportURL != "" && (!strings.HasPrefix(d.SupportURL, "https://") || !plain(d.SupportURL, 200) || strings.ContainsAny(d.SupportURL, " \"'")) {
		return fmt.Errorf("%w: \"support_url\" must be an https link", ErrManifest)
	}
	if d.Size < 0 {
		return fmt.Errorf("%w: \"size\" is negative", ErrManifest)
	}
	if !plain(d.Licence, 64) || !plain(d.Category, 40) {
		return bad("licence or category")
	}
	return checkSettings(d.Settings)
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
			// Unknown keys are ignored so manifests can gain optional fields
			// later, but a case variant of a known key is refused: Go's JSON
			// decoder would match it to the known field.
			for a := range allowed {
				if strings.EqualFold(a, k) {
					return nil, fmt.Errorf("%w: unknown field %q", ErrManifest, prefix+k)
				}
			}
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, notObject
			}
			continue
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

// newerVersion compares plain semver cores; a pre-release sorts below its
// release.
func newerVersion(a, b string) bool {
	ca, pa, _ := strings.Cut(a, "-")
	cb, pb, _ := strings.Cut(b, "-")
	na, nb := strings.Split(ca, "."), strings.Split(cb, ".")
	for i := 0; i < 3 && i < len(na) && i < len(nb); i++ {
		if len(na[i]) != len(nb[i]) {
			return len(na[i]) > len(nb[i])
		}
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}
	if (pa == "") != (pb == "") {
		return pa == ""
	}
	return pa > pb
}
