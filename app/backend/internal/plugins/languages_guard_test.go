package plugins_test

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func TestVerifyListLanguagesNeedsMinBlocklist(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	ok := plugins.Entry{ID: "lang-de", Version: "1.2.1", SHA256: strings.Repeat("a", 64)}
	raw, _ := plugins.SignList(priv, plugins.List{Kind: plugins.KindLanguages, Counter: 5, Entries: []plugins.Entry{ok}})
	if _, err := plugins.VerifyList(pub, raw, plugins.KindLanguages, 0); !errors.Is(err, plugins.ErrListFormat) {
		t.Fatalf("languages list without min_blocklist_counter accepted: %v", err)
	}
}

func TestLanguageBlocklistBelowListMinimumRefused(t *testing.T) {
	ps := newPackServer(t)
	ps.listsMin(4, 4, 3, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "de"); !errors.Is(err, plugins.ErrStaleBlock) {
		t.Fatalf("stale blocklist accepted: %v", err)
	}
	if err := l.Recheck(context.Background()); err != nil {
		t.Fatalf("recheck with nothing installed: %v", err)
	}
}

func TestLanguageCountersStartFromTheEngine(t *testing.T) {
	ps := newPackServer(t)
	ps.listsMin(1, 1, 3, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	st, err := plugins.NewStore(l.Engine.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCounters(0, 5, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := l.Install(context.Background(), "de"); !errors.Is(err, plugins.ErrListOlder) {
		t.Fatalf("blocklist older than the plugin engine's accepted: %v", err)
	}
	ps.listsMin(1, 1, 5, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	if err := l.Install(context.Background(), "de"); err != nil {
		t.Fatal(err)
	}
}

func TestLanguageReadErrorDoesNotDropThePack(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "de"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(l.Dir, "de", "1.2.1.json.bz2")
	good, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// A path that can't be read as a file stands in for a busy disk.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Pack("de"); err == nil {
		t.Fatal("unreadable pack served")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Pack("de"); err == nil {
		t.Fatal("missing pack served")
	}
	if err := os.WriteFile(p, good, 0o600); err != nil {
		t.Fatal(err)
	}
	plain(t, l, "de")
	if rows := l.List(); rows[indexOf(rows, "de")].State != "installed" {
		t.Fatal("pack no longer installed after a read error")
	}
}

func TestLanguageKeepRechecksEvenWhenEnsureFails(t *testing.T) {
	ps := newPackServer(t)
	de := ps.publish(t, "de", "1.2.1")
	ps.lists(1, []plugins.Entry{de})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "de"); err != nil {
		t.Fatal(err)
	}
	// fr is wanted but not listed, so Ensure fails; de is now blocked.
	ps.lists(2, []plugins.Entry{de}, plugins.Entry{ID: "lang-de"})
	if err := l.KeepCycle(context.Background(), []string{"fr"}); err == nil {
		t.Fatal("ensure for an unlisted language succeeded")
	}
	if _, _, err := l.Pack("de"); !errors.Is(err, plugins.ErrNotInstalled) {
		t.Fatalf("blocked pack still served after a failed ensure: %v", err)
	}
}

func TestLanguageInstallDropsABlockedInstalledPack(t *testing.T) {
	ps := newPackServer(t)
	de := ps.publish(t, "de", "1.2.1")
	ps.lists(1, []plugins.Entry{de})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "de"); err != nil {
		t.Fatal(err)
	}
	ps.lists(2, []plugins.Entry{de}, plugins.Entry{SHA256: de.SHA256})
	if err := l.Install(context.Background(), "de"); !errors.Is(err, plugins.ErrBlocked) {
		t.Fatalf("blocked pack installed: %v", err)
	}
	if _, _, err := l.Pack("de"); !errors.Is(err, plugins.ErrNotInstalled) {
		t.Fatalf("blocked pack still served: %v", err)
	}
}
