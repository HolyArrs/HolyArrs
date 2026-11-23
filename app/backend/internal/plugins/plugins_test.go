package plugins

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

var programFile = []byte("plugin program bytes")

func fileHash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func manifestJSON(mod func(map[string]any)) []byte {
	m := map[string]any{
		"id": "tailscale", "name": "Tailscale", "version": "1.0.0", "author": "HolyArrs",
		"access":          map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{"tailscale"}, "actions": []string{}},
		"min_app_version": "1.2.0",
		"sha256":          fileHash(programFile),
	}
	if mod != nil {
		mod(m)
	}
	b, _ := json.Marshal(m)
	return b
}

func u64(n uint64) *uint64 { return &n }

func withAccess(m map[string]any, folders, hosts []string) {
	if folders == nil {
		folders = []string{}
	}
	if hosts == nil {
		hosts = []string{}
	}
	m["access"] = map[string]any{"hosts": hosts, "folders": folders, "actions": []string{}}
}

func sign(t *testing.T, priv ed25519.PrivateKey, l List) []byte {
	t.Helper()
	b, err := SignList(priv, l)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseManifest(t *testing.T) {
	for name, set := range map[string]any{
		"bad type":         []map[string]any{{"key": "a", "label": "A", "type": "file"}},
		"bad key":          []map[string]any{{"key": "A b", "label": "A", "type": "text"}},
		"select no option": []map[string]any{{"key": "a", "label": "A", "type": "select"}},
		"secret default":   []map[string]any{{"key": "a", "label": "A", "type": "secret", "default": "x"}},
		"markup label":     []map[string]any{{"key": "a", "label": "<b>", "type": "text"}},
		"duplicate":        []map[string]any{{"key": "a", "label": "A", "type": "text"}, {"key": "a", "label": "B", "type": "text"}},
	} {
		if _, err := ParseManifest(manifestJSON(func(m map[string]any) { m["settings"] = set })); !errors.Is(err, ErrManifest) {
			t.Fatalf("settings %s accepted: %v", name, err)
		}
	}
	if m, err := ParseManifest(manifestJSON(nil)); err != nil || m.ID != "tailscale" || len(m.Access.Hosts) != 1 {
		t.Fatalf("valid manifest: %v %+v", err, m)
	}
	withShow := manifestJSON(func(m map[string]any) {
		m["description"] = "Reach HolyArrs anywhere."
		m["icon"] = "data:image/svg+xml;base64,PHN2Zy8+"
	})
	if m, err := ParseManifest(withShow); err != nil || m.Description == "" || m.Icon == "" {
		t.Fatalf("manifest with description and icon: %v %+v", err, m)
	}
	if m, err := ParseManifest(manifestJSON(func(m map[string]any) { m["icon_url"] = "images/tailscale/icon-256.webp" })); err != nil || m.IconURL != "images/tailscale/icon-256.webp" {
		t.Fatalf("manifest with icon_url: %v %+v", err, m)
	}
	future := manifestJSON(func(m map[string]any) {
		m["homepage"] = "https://example.com"
		m["extra"] = map[string]any{"nested": []any{1, "two", nil}}
		m["access"] = map[string]any{"hosts": []string{"a.example"}, "folders": []string{}, "actions": []string{}, "docker": true}
	})
	if m, err := ParseManifest(future); err != nil || m.ID != "tailscale" || len(m.Access.Hosts) != 1 {
		t.Fatalf("manifest with unknown fields: %v %+v", err, m)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"not json", []byte("{"), "JSON object"},
		{"description markup", manifestJSON(func(m map[string]any) { m["description"] = "<b>x</b>" }), `"description"`},
		{"description too long", manifestJSON(func(m map[string]any) { m["description"] = strings.Repeat("a", 241) }), `"description"`},
		{"icon not data url", manifestJSON(func(m map[string]any) { m["icon"] = "https://x/icon.png" }), `"icon"`},
		{"icon html", manifestJSON(func(m map[string]any) { m["icon"] = "data:text/html;base64,PGI+" }), `"icon"`},
		{"icon_url other host", manifestJSON(func(m map[string]any) { m["icon_url"] = "https://github.com/x.png" }), `"icon_url"`},
		{"icon_url other plugin", manifestJSON(func(m map[string]any) { m["icon_url"] = "images/other/icon-256.webp" }), `"icon_url"`},
		{"icon_url climbs out", manifestJSON(func(m map[string]any) { m["icon_url"] = "images/tailscale/../x/icon-256.webp" }), `"icon_url"`},
		{"array", []byte("[]"), "JSON object"},
		{"unknown field bad id", manifestJSON(func(m map[string]any) { m["extra"] = 1; m["id"] = "Bad" }), `"id" must`},
		{"case variant optional", manifestJSON(func(m map[string]any) { m["Icon"] = "x" }), `unknown field "Icon"`},
		{"missing id", manifestJSON(func(m map[string]any) { delete(m, "id") }), `"id"`},
		{"missing sha256", manifestJSON(func(m map[string]any) { delete(m, "sha256") }), `"sha256"`},
		{"missing access", manifestJSON(func(m map[string]any) { delete(m, "access") }), `"access"`},
		{"null author", manifestJSON(func(m map[string]any) { m["author"] = nil }), `"author"`},
		{"missing access.hosts", manifestJSON(func(m map[string]any) {
			m["access"] = map[string]any{"folders": []string{}, "actions": []string{}}
		}), `"access.hosts"`},
		{"empty name", manifestJSON(func(m map[string]any) { m["name"] = " " }), `"name" is empty`},
		{"empty min version", manifestJSON(func(m map[string]any) { m["min_app_version"] = "" }), `"min_app_version" is empty`},
		{"short sha", manifestJSON(func(m map[string]any) { m["sha256"] = "abc" }), "64 lowercase"},
		{"upper sha", manifestJSON(func(m map[string]any) { m["sha256"] = strings.ToUpper(fileHash(programFile)) }), "64 lowercase"},
		{"wrong type", manifestJSON(func(m map[string]any) { m["version"] = 1 }), "cannot unmarshal"},
		{"trailing data", append(manifestJSON(nil), []byte(" {}")...), "extra data"},
		{"trailing garbage", append(manifestJSON(nil), []byte(" x")...), "extra data"},
		{"duplicate id", []byte(strings.Replace(string(manifestJSON(nil)), `"id":"tailscale"`, `"id":"tailscale","id":"other"`, 1)), "duplicate field"},
		{"case variant ID", manifestJSON(func(m map[string]any) { m["ID"] = "x" }), `unknown field "ID"`},
		{"case variant access key", manifestJSON(func(m map[string]any) {
			m["access"] = map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}, "Hosts": []string{}}
		}), `unknown field "access.Hosts"`},
		{"upper id", manifestJSON(func(m map[string]any) { m["id"] = "Tailscale" }), `"id" must`},
		{"id with slash", manifestJSON(func(m map[string]any) { m["id"] = "../x" }), `"id" must`},
		{"id leading dash", manifestJSON(func(m map[string]any) { m["id"] = "-x" }), `"id" must`},
		{"id too long", manifestJSON(func(m map[string]any) { m["id"] = strings.Repeat("a", 65) }), `"id" must`},
		{"version not semver", manifestJSON(func(m map[string]any) { m["version"] = "1.0" }), "semver"},
		{"version with path", manifestJSON(func(m map[string]any) { m["version"] = "1.0.0/../x" }), "semver"},
		{"empty folder", manifestJSON(func(m map[string]any) { withAccess(m, []string{""}, nil) }), "unsafe folder"},
		{"dotdot folder", manifestJSON(func(m map[string]any) { withAccess(m, []string{"a/../b"}, nil) }), "unsafe folder"},
		{"backslash folder", manifestJSON(func(m map[string]any) { withAccess(m, []string{`a\b`}, nil) }), "unsafe folder"},
		{"absolute folder", manifestJSON(func(m map[string]any) { withAccess(m, []string{"/etc"}, nil) }), "unsafe folder"},
		{"drive folder", manifestJSON(func(m map[string]any) { withAccess(m, []string{"C:/x"}, nil) }), "unsafe folder"},
		{"empty host", manifestJSON(func(m map[string]any) { withAccess(m, nil, []string{""}) }), "unsafe host"},
		{"dotdot host", manifestJSON(func(m map[string]any) { withAccess(m, nil, []string{"a..b"}) }), "unsafe host"},
		{"backslash host", manifestJSON(func(m map[string]any) { withAccess(m, nil, []string{`a\b`}) }), "unsafe host"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest(tc.data)
			if !errors.Is(err, ErrManifest) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want ErrManifest containing %q", err, tc.want)
			}
		})
	}
}

func TestVerifyList(t *testing.T) {
	pub, priv := testKey(t)
	otherPub, otherPriv := testKey(t)
	h := fileHash(programFile)
	if _, err := VerifyList(pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{SHA256: h}}}), KindBlocklist, 0); err != nil {
		t.Fatalf("hash-only blocklist entry must pass: %v", err)
	}
	good := List{Kind: KindPlugins, Counter: 5, MinBlocklistCounter: u64(0), Entries: []Entry{{ID: "tailscale", Version: "1.0.0", SHA256: fileHash(programFile), ManifestSHA256: fileHash(manifestJSON(nil))}}}

	if l, err := VerifyList(pub, sign(t, priv, good), KindPlugins, 5); err != nil || l.Counter != 5 {
		t.Fatalf("same counter must pass: %v", err)
	}

	tampered := func() []byte {
		var sl SignedList
		_ = json.Unmarshal(sign(t, priv, good), &sl)
		sl.Payload = bytes.Replace(sl.Payload, []byte(`"counter":5`), []byte(`"counter":9`), 1)
		b, _ := json.Marshal(sl)
		return b
	}()
	badSig := func() []byte {
		var sl SignedList
		_ = json.Unmarshal(sign(t, priv, good), &sl)
		sl.Signature[0] ^= 1
		b, _ := json.Marshal(sl)
		return b
	}()

	tests := []struct {
		name string
		pub  ed25519.PublicKey
		data []byte
		kind string
		last uint64
		want error
	}{
		{"older list", pub, sign(t, priv, good), KindPlugins, 6, ErrListOlder},
		{"tampered payload", pub, tampered, KindPlugins, 0, ErrListSignature},
		{"flipped signature", pub, badSig, KindPlugins, 0, ErrListSignature},
		{"other key signed", pub, sign(t, otherPriv, good), KindPlugins, 0, ErrListSignature},
		{"wrong key passed", otherPub, sign(t, priv, good), KindPlugins, 0, ErrListSignature},
		{"no key", nil, sign(t, priv, good), KindPlugins, 0, ErrListSignature},
		{"blocklist as plugin list", pub, sign(t, priv, List{Kind: KindBlocklist, Counter: 5}), KindPlugins, 0, ErrListKind},
		{"plugin list as blocklist", pub, sign(t, priv, good), KindBlocklist, 0, ErrListKind},
		{"not json", pub, []byte("nope"), KindPlugins, 0, ErrListFormat},
		{"unknown outer field", pub, []byte(`{"payload":"","signature":"","x":1}`), KindPlugins, 0, ErrListFormat},
		{"missing signature", pub, []byte(`{"payload":"e30="}`), KindPlugins, 0, ErrListFormat},
		{"entry without hash", pub, sign(t, priv, List{Kind: KindPlugins, MinBlocklistCounter: u64(0), Entries: []Entry{{ID: "a", Version: "1.0.0"}}}), KindPlugins, 0, ErrListFormat},
		{"plugin entry hash only", pub, sign(t, priv, List{Kind: KindPlugins, MinBlocklistCounter: u64(0), Entries: []Entry{{SHA256: h}}}), KindPlugins, 0, ErrListFormat},
		{"blocklist entry empty", pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{}}}), KindBlocklist, 0, ErrListFormat},
		{"blocklist entry version only", pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{Version: "1.0.0"}}}), KindBlocklist, 0, ErrListFormat},
		{"bad entry id", pub, sign(t, priv, List{Kind: KindPlugins, MinBlocklistCounter: u64(0), Entries: []Entry{{ID: "A/b", Version: "1.0.0", SHA256: h}}}), KindPlugins, 0, ErrListFormat},
		{"bad entry version", pub, sign(t, priv, List{Kind: KindPlugins, MinBlocklistCounter: u64(0), Entries: []Entry{{ID: "a", Version: "1", SHA256: h}}}), KindPlugins, 0, ErrListFormat},
		{"bad blocklist version", pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{ID: "a", Version: "v1"}}}), KindBlocklist, 0, ErrListFormat},
		{"upper blocklist hash", pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{SHA256: strings.ToUpper(h)}}}), KindBlocklist, 0, ErrListFormat},
		{"short blocklist hash", pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{ID: "a", SHA256: "abc"}}}), KindBlocklist, 0, ErrListFormat},
		{"missing min_blocklist_counter", pub, sign(t, priv, List{Kind: KindPlugins, Counter: 5}), KindPlugins, 0, ErrListFormat},
		{"trailing outer data", pub, append(sign(t, priv, good), []byte(" {}")...), KindPlugins, 0, ErrListFormat},
		{"trailing payload data", pub, signRaw(priv, []byte(`{"kind":"blocklist","counter":1,"entries":[]} {}`)), KindBlocklist, 0, ErrListFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := VerifyList(tc.pub, tc.data, tc.kind, tc.last); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func signRaw(priv ed25519.PrivateKey, payload []byte) []byte {
	b, _ := json.Marshal(SignedList{Payload: payload, Signature: ed25519.Sign(priv, payload)})
	return b
}

func TestCheckInstall(t *testing.T) {
	pub, priv := testKey(t)
	_, otherPriv := testKey(t)
	h := fileHash(programFile)
	list := sign(t, priv, List{Kind: KindPlugins, Counter: 3, MinBlocklistCounter: u64(2), Entries: []Entry{{ID: "tailscale", Version: "1.0.0", SHA256: h, ManifestSHA256: fileHash(manifestJSON(nil))}}})
	emptyBlock := sign(t, priv, List{Kind: KindBlocklist, Counter: 2})

	base := func() InstallInput {
		return InstallInput{PublicKey: pub, List: list, LastListCounter: 3, Blocklist: emptyBlock, LastBlockCounter: 2,
			Manifest: manifestJSON(nil), File: bytes.NewReader(programFile), TempDir: t.TempDir()}
	}

	acc, err := CheckInstall(base())
	if err != nil || acc.Manifest.ID != "tailscale" || acc.ListCounter != 3 || acc.BlockCounter != 2 {
		t.Fatalf("valid install refused: %v", err)
	}
	if got, _ := os.ReadFile(acc.FilePath); !bytes.Equal(got, programFile) {
		t.Fatalf("checked file = %q", got)
	}
	acc.Close()

	old := maxFileSize
	maxFileSize = int64(len(programFile)) - 1
	if _, err := CheckInstall(base()); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("oversized file: got %v, want ErrFileTooLarge", err)
	}
	maxFileSize = old

	otherFile := []byte("another program")
	tests := []struct {
		name string
		mod  func(*InstallInput)
		want error
	}{
		{"wrong hash", func(in *InstallInput) { in.File = bytes.NewReader(otherFile) }, ErrHashMismatch},
		{"no file", func(in *InstallInput) { in.File = nil }, ErrHashMismatch},
		{"tampered manifest, program hash still matching", func(in *InstallInput) {
			in.Manifest = manifestJSON(func(m map[string]any) {
				m["access"] = map[string]any{"hosts": []string{"evil.example"}, "folders": []string{"tailscale", "media"}, "actions": []string{}}
			})
		}, ErrManifestHash},
		{"not on list", func(in *InstallInput) {
			in.Manifest = manifestJSON(func(m map[string]any) { m["id"] = "unsigned"; m["sha256"] = fileHash(otherFile) })
			in.File = bytes.NewReader(otherFile)
		}, ErrNotOnList},
		{"other version not on list", func(in *InstallInput) {
			in.Manifest = manifestJSON(func(m map[string]any) { m["version"] = "1.0.1" })
		}, ErrNotOnList},
		{"same id, file swapped", func(in *InstallInput) {
			in.Manifest = manifestJSON(func(m map[string]any) { m["sha256"] = fileHash(otherFile) })
			in.File = bytes.NewReader(otherFile)
		}, ErrNotOnList},
		{"blocklisted by version", func(in *InstallInput) {
			in.Blocklist = sign(t, priv, List{Kind: KindBlocklist, Counter: 2, Entries: []Entry{{ID: "tailscale", Version: "1.0.0"}}})
		}, ErrBlocked},
		{"blocklisted all versions", func(in *InstallInput) {
			in.Blocklist = sign(t, priv, List{Kind: KindBlocklist, Counter: 2, Entries: []Entry{{ID: "tailscale"}}})
		}, ErrBlocked},
		{"blocklisted by hash", func(in *InstallInput) {
			in.Blocklist = sign(t, priv, List{Kind: KindBlocklist, Counter: 2, Entries: []Entry{{ID: "x", Version: "9.0.0", SHA256: h}}})
		}, ErrBlocked},
		{"blocklisted by hash only", func(in *InstallInput) {
			in.Blocklist = sign(t, priv, List{Kind: KindBlocklist, Counter: 2, Entries: []Entry{{SHA256: h}}})
		}, ErrBlocked},
		{"stale blocklist", func(in *InstallInput) {
			in.Blocklist = sign(t, priv, List{Kind: KindBlocklist, Counter: 1})
			in.LastBlockCounter = 0
		}, ErrStaleBlock},
		{"older list", func(in *InstallInput) { in.LastListCounter = 4 }, ErrListOlder},
		{"older blocklist", func(in *InstallInput) { in.LastBlockCounter = 3 }, ErrListOlder},
		{"list signed by other key", func(in *InstallInput) {
			in.List = sign(t, otherPriv, List{Kind: KindPlugins, Counter: 3, MinBlocklistCounter: u64(2), Entries: []Entry{{ID: "tailscale", Version: "1.0.0", SHA256: h, ManifestSHA256: fileHash(manifestJSON(nil))}}})
		}, ErrListSignature},
		{"tampered list", func(in *InstallInput) {
			var sl SignedList
			_ = json.Unmarshal(list, &sl)
			sl.Payload = bytes.Replace(sl.Payload, []byte("tailscale"), []byte("tailscalf"), 1)
			in.List, _ = json.Marshal(sl)
		}, ErrListSignature},
		{"tampered blocklist", func(in *InstallInput) {
			var sl SignedList
			_ = json.Unmarshal(emptyBlock, &sl)
			sl.Payload = bytes.Replace(sl.Payload, []byte(`"counter":2`), []byte(`"counter":7`), 1)
			in.Blocklist, _ = json.Marshal(sl)
		}, ErrListSignature},
		{"blocklist missing", func(in *InstallInput) { in.Blocklist = nil }, ErrListFormat},
		{"bad manifest", func(in *InstallInput) { in.Manifest = []byte(`{}`) }, ErrManifest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mod(&in)
			if _, err := CheckInstall(in); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
