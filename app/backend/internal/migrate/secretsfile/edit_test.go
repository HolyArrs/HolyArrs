package secretsfile

import (
	"context"
	"strings"
	"testing"
)

// An empty key keeps the stored one (clear_key: true removes it); an item
// whose address or key changes is paused again.
func TestEditKeepsKeyAndRepauses(t *testing.T) {
	d := deps(t)
	ctx := context.Background()
	if ls := Apply(ctx, d, parse(t, fakeFile), false); !AllOK(ls) {
		t.Fatal(ls)
	}
	// The owner resumes everything after checking.
	mustExec(t, d, "UPDATE sources SET state = 'enabled'")
	mustExec(t, d, "UPDATE downloaders SET state = 'ok'")
	keyOf := func() string {
		var ref *int64
		_ = d.Store.Read().QueryRow("SELECT secret_ref FROM sources WHERE name = 'Lab news'").Scan(&ref)
		if ref == nil {
			return ""
		}
		k, _ := d.Vault.Get(ctx, *ref)
		return k
	}

	// Empty key in the file: the stored key stays, nothing changes.
	noKey := strings.Replace(fakeFile, `api_key: "your-news-key-8"`, `api_key: ""`, 1)
	r := results(Apply(ctx, d, parse(t, noKey), false))
	if r["indexers[2]"] != Unchanged || keyOf() != "your-news-key-8" {
		t.Fatalf("empty key erased or changed: %q key %q", r["indexers[2]"], keyOf())
	}

	// A new key: changed and paused again.
	newKey := strings.Replace(fakeFile, "your-news-key-8", "your-news-key-9", 1)
	r = results(Apply(ctx, d, parse(t, newKey), false))
	if !strings.Contains(r["indexers[2]"], "changed, paused") || keyOf() != "your-news-key-9" {
		t.Fatalf("new key %q", r["indexers[2]"])
	}
	if n := count(t, d, "SELECT count(*) FROM sources WHERE name = 'Lab news' AND state = 'disabled'"); n != 1 {
		t.Error("not paused after a key change")
	}
	if n := count(t, d, "SELECT count(*) FROM sources WHERE name = 'Lab feed' AND state = 'enabled'"); n != 1 {
		t.Error("an unchanged item was paused")
	}

	// clear_key: true removes it.
	clear := strings.Replace(fakeFile, `api_key: "your-news-key-8"`, "api_key: \"\"\n    clear_key: true", 1)
	r = results(Apply(ctx, d, parse(t, clear), false))
	if keyOf() != "" || !strings.HasPrefix(r["indexers[2]"], Set) {
		t.Fatalf("clear_key %q key left %v", r["indexers[2]"], keyOf() != "")
	}

	// A Downloader with a new address: changed and paused.
	moved := strings.Replace(fakeFile, "http://127.0.0.1:18708", "http://127.0.0.1:18798", 1)
	if !strings.Contains(moved, "18798") {
		t.Fatal("qbittorrent address not found in the test file")
	}
	r = results(Apply(ctx, d, parse(t, moved), false))
	if !strings.Contains(r["qbittorrent"], "changed, paused") {
		t.Fatalf("qbittorrent %q", r["qbittorrent"])
	}
	if n := count(t, d, "SELECT count(*) FROM downloaders WHERE adapter = 'qbittorrent' AND state = 'disabled'"); n != 1 {
		t.Error("downloader not paused")
	}
	if n := count(t, d, "SELECT count(*) FROM downloaders WHERE adapter = 'sabnzbd' AND state = 'ok'"); n != 1 {
		t.Error("unchanged downloader paused")
	}
}
