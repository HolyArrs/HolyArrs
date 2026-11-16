package plugins

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
		"access":          map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{"/config/plugins/tailscale"}, "actions": []string{}},
		"min_app_version": "1.2.0",
		"sha256":          fileHash(programFile),
	}
	if mod != nil {
		mod(m)
	}
	b, _ := json.Marshal(m)
	return b
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
	if m, err := ParseManifest(manifestJSON(nil)); err != nil || m.ID != "tailscale" || len(m.Access.Hosts) != 1 {
		t.Fatalf("valid manifest: %v %+v", err, m)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"not json", []byte("{"), "JSON object"},
		{"array", []byte("[]"), "JSON object"},
		{"unknown field", manifestJSON(func(m map[string]any) { m["extra"] = 1 }), "unknown field"},
		{"unknown access field", manifestJSON(func(m map[string]any) {
			m["access"] = map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}, "docker": true}
		}), "unknown field"},
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
		{"trailing data", append(manifestJSON(nil), []byte(" {}")...), "JSON object"},
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
	good := List{Kind: KindPlugins, Counter: 5, Entries: []Entry{{ID: "tailscale", Version: "1.0.0", SHA256: fileHash(programFile)}}}

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
		{"entry without hash", pub, sign(t, priv, List{Kind: KindPlugins, Entries: []Entry{{ID: "a", Version: "1"}}}), KindPlugins, 0, ErrListFormat},
		{"entry without id", pub, sign(t, priv, List{Kind: KindBlocklist, Entries: []Entry{{}}}), KindBlocklist, 0, ErrListFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := VerifyList(tc.pub, tc.data, tc.kind, tc.last); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCheckInstall(t *testing.T) {
	pub, priv := testKey(t)
	_, otherPriv := testKey(t)
	h := fileHash(programFile)
	list := sign(t, priv, List{Kind: KindPlugins, Counter: 3, Entries: []Entry{{ID: "tailscale", Version: "1.0.0", SHA256: h}}})
	emptyBlock := sign(t, priv, List{Kind: KindBlocklist, Counter: 2})

	base := func() InstallInput {
		return InstallInput{PublicKey: pub, List: list, LastListCounter: 3, Blocklist: emptyBlock, LastBlockCounter: 2,
			Manifest: manifestJSON(nil), File: bytes.NewReader(programFile)}
	}

	acc, err := CheckInstall(base())
	if err != nil || acc.Manifest.ID != "tailscale" || acc.ListCounter != 3 || acc.BlockCounter != 2 {
		t.Fatalf("valid install refused: %v", err)
	}

	otherFile := []byte("another program")
	tests := []struct {
		name string
		mod  func(*InstallInput)
		want error
	}{
		{"wrong hash", func(in *InstallInput) { in.File = bytes.NewReader(otherFile) }, ErrHashMismatch},
		{"no file", func(in *InstallInput) { in.File = nil }, ErrHashMismatch},
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
			in.Blocklist = sign(t, priv, List{Kind: KindBlocklist, Counter: 2, Entries: []Entry{{ID: "x", Version: "9", SHA256: h}}})
		}, ErrBlocked},
		{"older list", func(in *InstallInput) { in.LastListCounter = 4 }, ErrListOlder},
		{"older blocklist", func(in *InstallInput) { in.LastBlockCounter = 3 }, ErrListOlder},
		{"list signed by other key", func(in *InstallInput) {
			in.List = sign(t, otherPriv, List{Kind: KindPlugins, Counter: 3, Entries: []Entry{{ID: "tailscale", Version: "1.0.0", SHA256: h}}})
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
