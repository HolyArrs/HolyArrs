package updates_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates/signtest"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func listJSON(serial, gen string, extra string) []byte {
	return []byte(`{"schema":1,"serial":` + serial + `,"generated_at":"` + gen + `","channels":{"stable":{"version":"v1.2.0",` +
		`"changelog_url":"https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0","notes_short":"Fixes"` + extra + `}},"revoked":[]}`)
}

func TestPinnedKeysEmpty(t *testing.T) {
	if len(updates.PinnedKeys) != 0 || len(updates.Pinned()) != 0 {
		t.Fatal("PinnedKeys must stay empty until the key ceremony")
	}
}

func TestParsePublicKey(t *testing.T) {
	k := signtest.New(t)
	p, err := updates.ParsePublicKey("untrusted comment: minisign public key\n" + k.PublicLine() + "\n")
	if err != nil || p.ID != k.ID || !p.Key.Equal(k.Pub.Key) || len(p.KeyID()) != 16 {
		t.Fatalf("parse: %v", err)
	}
	if _, err := updates.ParsePublicKey("bm9wZQ=="); err == nil {
		t.Fatal("junk key accepted")
	}
}

func TestGoodSignature(t *testing.T) {
	k := signtest.New(t)
	data := listJSON("5", "2026-10-23T11:00:00Z", "")
	for _, sig := range [][]byte{k.Sign(data, "timestamp:1"), k.SignLegacy(data, "timestamp:1")} {
		l, err := updates.Parse(data, sig, []updates.PublicKey{k.Pub}, now, 5)
		if err != nil || l.Serial != 5 || l.Channels["stable"].Version != "v1.2.0" {
			t.Fatalf("good list: %v", err)
		}
	}
}

func TestBadSignatures(t *testing.T) {
	k, other := signtest.New(t), signtest.New(t)
	data := listJSON("5", "2026-10-23T11:00:00Z", "")
	keys := []updates.PublicKey{k.Pub}
	tampered := append([]byte{}, data...)
	tampered[len(tampered)-3] = ' '
	cases := map[string]struct{ data, sig []byte }{
		"bad signature":   {tampered, k.Sign(data, "t")},
		"unknown key id":  {data, other.Sign(data, "t")},
		"not minisig":     {data, []byte("hello")},
		"trusted comment": {data, []byte(strings.Replace(string(k.Sign(data, "key main")), "key main", "key next", 1))},
	}
	for name, c := range cases {
		if _, err := updates.Parse(c.data, c.sig, keys, now, 0); !errors.Is(err, updates.ErrSignature) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := updates.Parse(data, k.Sign(data, "t"), nil, now, 0); !errors.Is(err, updates.ErrSignature) {
		t.Error("no pinned keys must reject")
	}
	// Two keys pinned (main and next): either one is accepted.
	if _, err := updates.Parse(data, other.Sign(data, "t"), []updates.PublicKey{k.Pub, other.Pub}, now, 0); err != nil {
		t.Errorf("next key: %v", err)
	}
}

func TestFutureAndSerial(t *testing.T) {
	k := signtest.New(t)
	keys := []updates.PublicKey{k.Pub}
	fut := listJSON("5", "2026-10-23T12:11:00Z", "")
	if _, err := updates.Parse(fut, k.Sign(fut, "t"), keys, now, 0); err == nil {
		t.Error("future generated_at accepted")
	}
	ok := listJSON("5", "2026-10-23T12:09:00Z", "")
	if _, err := updates.Parse(ok, k.Sign(ok, "t"), keys, now, 0); err != nil {
		t.Errorf("9 min ahead: %v", err)
	}
	if _, err := updates.Parse(ok, k.Sign(ok, "t"), keys, now, 6); !errors.Is(err, updates.ErrOlder) {
		t.Errorf("lower serial: %v", err)
	}
}

func TestPlainTextAndLinks(t *testing.T) {
	k := signtest.New(t)
	data := []byte(`{"schema":1,"serial":1,"generated_at":"2026-10-23T11:00:00Z","channels":{"stable":{"version":"v1.2.0",` +
		`"changelog_url":"https://evil.example/x","notes_short":"<b>Big</b> <script>alert(1)</script>fix <a href=\"x\">here</a>` + strings.Repeat("x", 400) + `"},` +
		`"beta":{"version":"v1.3.0-rc1","changelog_url":"https://github.com/HolyArrs/HolyArrs/releases/tag/v1.3.0-rc1"}}}`)
	l, err := updates.Parse(data, k.Sign(data, "t"), []updates.PublicKey{k.Pub}, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := l.Channels["stable"]
	if strings.ContainsAny(s.NotesShort, "<>") || !strings.HasPrefix(s.NotesShort, "Big alert(1)fix here") || len([]rune(s.NotesShort)) != 280 {
		t.Errorf("notes: %q", s.NotesShort)
	}
	if s.ChangelogURL != "" || l.Channels["beta"].ChangelogURL != "https://github.com/HolyArrs/HolyArrs/releases/tag/v1.3.0-rc1" {
		t.Errorf("links: %q %q", s.ChangelogURL, l.Channels["beta"].ChangelogURL)
	}
	for _, bad := range []string{"http://github.com/HolyArrs/HolyArrs/x", "https://github.com/HolyArrs/HolyArrs-evil/x", "https://github.com/HolyArrs/HolyArrs/../x", "javascript:alert(1)", "https://docs.holyarrs.dev/changes"} {
		if updates.Changelog(bad) != "" {
			t.Errorf("kept %q", bad)
		}
	}
}

func TestOffer(t *testing.T) {
	k := signtest.New(t)
	parse := func(extra, revoked string) *updates.List {
		data := []byte(strings.Replace(string(listJSON("1", "2026-10-23T11:00:00Z", extra)), `"revoked":[]`, `"revoked":[`+revoked+`]`, 1))
		l, err := updates.Parse(data, k.Sign(data, "t"), []updates.PublicKey{k.Pub}, now, 0)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	if _, ok := parse("", "").Offer("stable", 99); !ok {
		t.Error("no rollout_percent means everyone")
	}
	l := parse(`,"rollout_percent":30`, "")
	if _, ok := l.Offer("stable", 29); !ok {
		t.Error("bucket 29 under 30")
	}
	if _, ok := l.Offer("stable", 30); ok {
		t.Error("bucket 30 not under 30")
	}
	if _, ok := parse(`,"hold":true`, "").Offer("stable", 0); ok {
		t.Error("held release offered")
	}
	if _, ok := parse("", `"v1.2.0"`).Offer("stable", 0); ok {
		t.Error("revoked release offered")
	}
	if _, ok := parse("", "").Offer("beta", 0); ok {
		t.Error("missing channel offered")
	}
}
