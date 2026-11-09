package secretsfile

import (
	"context"
	"strings"
	"testing"
)

func TestSABnzbdNZBKeyIsNotTheAPIKey(t *testing.T) {
	ctx := context.Background()
	d := deps(t)
	before := count(t, d, "SELECT count(*) FROM secrets")
	f := parse(t, "version: 1\nsabnzbd:\n  address: http://sabnzbd:8080\n  api_key: your-sab-full-key\n  nzb_key: your-sab-add-only-key\n  allow_local_network: true\n")
	wantSkip := "skipped (not used: SABnzbd needs its full api_key)"
	dry := Apply(ctx, d, f, true)
	if !AllOK(dry) || results(dry)["sabnzbd.nzb_key"] != wantSkip {
		t.Fatalf("dry run: %v", dry)
	}
	if count(t, d, "SELECT count(*) FROM downloaders") != 0 || count(t, d, "SELECT count(*) FROM secrets") != before {
		t.Fatal("dry run wrote credentials")
	}
	ls := Apply(ctx, d, f, false)
	if !AllOK(ls) || results(ls)["sabnzbd.nzb_key"] != wantSkip {
		t.Fatalf("import: %v", ls)
	}
	var ref int64
	var state string
	if err := d.Store.Read().QueryRow("SELECT secret_ref, state FROM downloaders WHERE adapter = 'sabnzbd'").Scan(&ref, &state); err != nil {
		t.Fatal(err)
	}
	key, err := d.Vault.Get(ctx, ref)
	if err != nil || key != "your-sab-full-key" || state != "disabled" {
		t.Fatal("SABnzbd must stay paused and use only its full API key")
	}
	if count(t, d, "SELECT count(*) FROM secrets") != before+1 {
		t.Fatal("the unused NZB key was stored")
	}
	again := Apply(ctx, d, f, true)
	if !AllOK(again) || results(again)["sabnzbd"] != Unchanged {
		t.Fatalf("repeat: %v", again)
	}
	noValues(t, d, f, append(append(dry, ls...), again...))
}

func TestSABnzbdNZBKeyCannotReplaceMissingAPIKey(t *testing.T) {
	d := deps(t)
	before := count(t, d, "SELECT count(*) FROM secrets")
	f := parse(t, "version: 1\nsabnzbd:\n  address: http://sabnzbd:8080\n  nzb_key: your-sab-add-only-key\n  allow_local_network: true\n")
	ls := Apply(context.Background(), d, f, false)
	if AllOK(ls) || !strings.HasPrefix(results(ls)["sabnzbd"], "failed: needs an address and an api_key") {
		t.Fatal("an NZB key must not substitute for the full API key")
	}
	if count(t, d, "SELECT count(*) FROM downloaders") != 0 || count(t, d, "SELECT count(*) FROM secrets") != before {
		t.Fatal("incomplete SABnzbd credentials were stored")
	}
	noValues(t, d, f, ls)
}

func TestSABnzbdNZBKeyJSON(t *testing.T) {
	f, err := DecodeJSON([]byte(`{"version":1,"sabnzbd":{"api_key":"your-sab-full-key","nzb_key":"your-sab-add-only-key"}}`))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range f.Values() {
		found = found || value == "your-sab-add-only-key"
	}
	if !found {
		t.Fatal("redaction must include the unused NZB key")
	}
	if _, err := DecodeJSON([]byte(`{"version":1,"sabnzbd":{"nzb_key":"your-sab-add-only-key","extra":"your-unknown-value"}}`)); err == nil || strings.Contains(err.Error(), "your-") {
		t.Fatal("other unknown fields must still be refused without values")
	}
}

func TestSABnzbdLoginFieldsAreUnused(t *testing.T) {
	ctx := context.Background()
	d := deps(t)
	before := count(t, d, "SELECT count(*) FROM secrets")
	f := parse(t, "version: 1\nsabnzbd:\n  address: http://sabnzbd:8080\n  api_key: your-sab-full-key\n  username: your-sab-user\n  password: your-sab-web-password\n  allow_local_network: true\n")
	ls := Apply(ctx, d, f, false)
	if !AllOK(ls) || results(ls)["sabnzbd.login"] != "skipped (not used: SABnzbd uses its api_key)" {
		t.Fatalf("web login should be unused: %v", ls)
	}
	var user string
	if err := d.Store.Read().QueryRow("SELECT username FROM downloaders WHERE adapter = 'sabnzbd'").Scan(&user); err != nil || user != "" {
		t.Fatal("web login was stored as a Downloader login")
	}
	if count(t, d, "SELECT count(*) FROM secrets") != before+1 {
		t.Fatal("unused web credentials were stored")
	}
	noValues(t, d, f, ls)
	f.SABnzbd.APIKey = ""
	if AllOK(Apply(ctx, deps(t), f, false)) {
		t.Fatal("web credentials must not replace the full API key")
	}
}

func TestUnusedMetadataAccountFields(t *testing.T) {
	ctx := context.Background()
	d := deps(t)
	before := count(t, d, "SELECT count(*) FROM secrets")
	f := parse(t, "version: 1\ntvdb:\n  api_key: your-tvdb-api-key\n  unique_user_key: your-tvdb-user-key\ntrakt:\n  client_id: your-trakt-client-id\n  redirect_urls: https://your-redirect.invalid/callback\n")
	ls := Apply(ctx, d, f, false)
	r := results(ls)
	if !AllOK(ls) || !strings.HasPrefix(r["tvdb.unique_user_key"], "skipped (not used:") || !strings.HasPrefix(r["trakt.redirect_urls"], "skipped (not used:") {
		t.Fatalf("unused account fields: %v", ls)
	}
	for key, want := range map[string]string{TraktClientKey: "your-trakt-client-id"} {
		value, err := d.Vault.Setting(ctx, LinksGroup, key)
		if err != nil || value != want {
			t.Fatalf("wrong credential selected for %s", key)
		}
	}
	if count(t, d, "SELECT count(*) FROM secrets") != before+1 {
		t.Fatal("unused account values were stored")
	}
	noValues(t, d, f, ls)
	again := results(Apply(ctx, d, f, true))
	if again["tvdb"] != tvdbOff || again["trakt"] != Unchanged {
		t.Fatal("unused account fields changed repeat-import behavior")
	}
}
