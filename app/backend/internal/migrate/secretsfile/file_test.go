package secretsfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFile is a made-up secrets file in the template's shape. Every value
// is invented for tests and starts with "your-" so no scanner mistakes it
// for a real one.
const fakeFile = `# HolyArrs secrets file (made up for tests)
version: 1
tmdb:
  read_token: "your-tmdb-read-1"   # a comment after a value
  api_key: ""
tvdb:
  api_key: your-tvdb-key-2
trakt:
  client_id: 'your-trakt-id-3'
opensubtitles:
  api_key: "your-os-key-4"
  username: lab-person
  password: 'your-os-word-5 # not a comment'
subdl:
  api_key: your-subdl-key-6
addic7ed:
  username: ""
  password: ""
jackett:
  address: ""
  api_key: ""
prowlarr:
  address: http://127.0.0.1:9696
  api_key: your-prowlarr-key-7
  allow_local_network: true
indexers:
  - name: Lab feed
    address: http://127.0.0.1:9696/1/api
    api_key: ""
    allow_local_network: true
  - name: "Lab news"
    address: "http://127.0.0.1:18799/api"
    api_key: "your-news-key-8"
    format: newznab
    allow_local_network: yes
qbittorrent:
  address: http://127.0.0.1:18708
  api_key: "your-qbit-key-9"
  allow_local_network: true
sabnzbd:
  address: http://127.0.0.1:18707
  api_key: your-sab-key-10
  allow_local_network: true
nzbget: {address: "", username: "", password: ""}
transmission:
  address: ""
  username: ""
  password: ""
deluge:
  address: ""
  password: ""
jellyfin:
  address: http://127.0.0.1:18709
  api_key: your-jf-key-11
  allow_local_network: true
emby:
  address:
  api_key:
plex:
  address: ""
  token: ""
radarr:
  address: http://127.0.0.1:18702
  api_key: your-radarr-key-12
  allow_local_network: true
sonarr:
  address: ""
  api_key: ""
lidarr:
  address: ""
  api_key: ""
bazarr:
  address: ""
  api_key: ""
seerr:
  address: ""
  api_key: ""
smtp:
  host: mailpit
  port: 1025
  username: ""
  password: ""
  from: holyarrs@lab.test
  allow_local_network: true
discord:
  webhook_url: ""
webhook:
  url: ""
  signing_secret: "your-sign-13"
holyarrs:
  address: http://127.0.0.1:18730
  owner_api_key: your-owner-key-14
`

func TestParseFakeFile(t *testing.T) {
	f, err := Parse([]byte(fakeFile))
	if err != nil {
		t.Fatal(err)
	}
	if f.TMDB.ReadToken != "your-tmdb-read-1" || f.TVDB.APIKey != "your-tvdb-key-2" || f.Trakt.ClientID != "your-trakt-id-3" {
		t.Errorf("links %+v %+v %+v", f.TMDB, f.TVDB, f.Trakt)
	}
	if f.OpenSubtitles.Password != "your-os-word-5 # not a comment" || f.OpenSubtitles.Username != "lab-person" {
		t.Errorf("quoted # kept: %+v", f.OpenSubtitles)
	}
	if len(f.Indexers) != 2 || f.Indexers[0].Name != "Lab feed" || !f.Indexers[0].AllowLocalNetwork || f.Indexers[1].Format != "newznab" ||
		!f.Indexers[1].AllowLocalNetwork || f.Indexers[1].APIKey != "your-news-key-8" {
		t.Errorf("indexers %+v", f.Indexers)
	}
	if !f.Prowlarr.AllowLocalNetwork || f.SMTP.Port != "1025" || f.Emby.Address != "" || f.Webhook.SigningSecret != "your-sign-13" {
		t.Errorf("blocks %+v %+v", f.Prowlarr, f.SMTP)
	}
	if len(f.Values()) < 14 {
		t.Errorf("values %d", len(f.Values()))
	}
}

func TestParseRefuses(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"unknown key", "version: 1\ntmdb:\n  read_tokn: your-x-1\n", "unknown key \"read_tokn\""},
		{"unknown block", "version: 1\nsonar:\n  api_key: your-x-1\n", "unknown key \"sonar\""},
		{"twice", "version: 1\ntvdb:\n  api_key: your-x-1\n  api_key: your-x-2\n", "line 4: key api_key appears twice"},
		{"tab", "version: 1\ntvdb:\n\tapi_key: your-x-1\n", "line 3: use spaces"},
		{"version", "version: 2\n", "version must be 1"},
		{"no version", "tvdb:\n  api_key: your-x-1\n", "version must be 1"},
		{"bool", "version: 1\nradarr:\n  allow_local_network: maybe\n", "radarr.allow_local_network must be true or false"},
		{"anchor", "version: 1\ntvdb:\n  api_key: &a your-x-1\n", "line 3: put this value in quotes"},
		{"open quote", "version: 1\ntrakt:\n  client_id: \"your-x-1\n", "line 3: a quoted value is not closed"},
		{"block text", "version: 1\ntvdb:\n  api_key: |\n    your-x-1\n", "line 3: put this value in quotes"},
		{"deeper", "version: 1\ntvdb:\n  api_key: your-x-1\n    more: your-x-2\n", "line 4: indented more"},
		{"wrong kind", "version: 1\ntvdb: your-x-1\n", "key tvdb has the wrong kind of value"},
		{"list of text", "version: 1\nindexers:\n  - your-x-1\n", "wrong kind of value"},
		{"top list", "- your-x-1\n", "must be a list of keys"},
	} {
		_, err := Parse([]byte(c.text))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if strings.Contains(err.Error(), "your-x") {
			t.Errorf("%s: a value is in the error: %v", c.name, err)
		}
	}
	if _, err := Parse(make([]byte, MaxSize+1)); err != ErrTooBig {
		t.Errorf("size cap: %v", err)
	}
	// A byte-order mark, Windows line ends, "---" and a list at the key's
	// own indent are all fine.
	f, err := Parse([]byte("\xef\xbb\xbf---\r\nversion: 1\r\nindexers:\r\n- name: a\r\n  address: http://x\r\n"))
	if err != nil || len(f.Indexers) != 1 || f.Indexers[0].Address != "http://x" {
		t.Fatalf("%+v %v", f, err)
	}
	if f, err := Parse([]byte("version: 1\nindexers: []\n")); err != nil || len(f.Indexers) != 0 {
		t.Fatalf("empty list %v", err)
	}
	if f, err := Parse([]byte("version: 1\ntvdb:\n  api_key: 'it''s-your-x'\n")); err != nil || f.TVDB.APIKey != "it's-your-x" {
		t.Fatalf("single quotes %v", err)
	}
}

func TestDecodeJSONRefusesUnknown(t *testing.T) {
	if _, err := DecodeJSON([]byte(`{"version":1,"tvdb":{"api_key":"your-x-1","extra":"your-x-2"}}`)); err == nil || strings.Contains(err.Error(), "your-x") {
		t.Fatalf("%v", err)
	}
}

// The template in docs (the same one beside the owner's file) reads, and
// importing it as it is skips every item.
func TestTemplateParses(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "docs", "migrate", "holyarrs-secrets.template.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Values()) != 1 || f.Indexers[0].Format != "torznab" { // only the format default is filled
		t.Fatalf("values %v", f.Values())
	}
	for _, l := range Apply(context.Background(), deps(t), f, true) {
		if !strings.HasPrefix(l.Result, "skipped") {
			t.Errorf("%s: %s", l.Item, l.Result)
		}
	}
}
