package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// A made-up secrets file; every value starts with "your-" and is
// invented for this test.
const cliFile = `version: 1
tvdb:
  api_key: your-tvdb-key-1
trakt:
  client_id: your-trakt-id-2
indexers:
  - name: Lab feed
    address: http://127.0.0.1:18799/api
    api_key: your-feed-key-3
    allow_local_network: true
qbittorrent:
  address: http://127.0.0.1:18708
  api_key: your-qbit-key-4
  allow_local_network: true
radarr:
  address: http://127.0.0.1:18702
  api_key: your-radarr-key-5
holyarrs:
  address: http://127.0.0.1:18730
  owner_api_key: your-owner-key-6
`

var cliValues = []string{"your-tvdb-key-1", "your-trakt-id-2", "your-feed-key-3", "your-qbit-key-4", "your-radarr-key-5", "your-owner-key-6", ":18799", ":18708", ":18702", ":18730"}

// configDir makes a config folder with a database and a master key, as a
// first start leaves it.
func configDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if _, err := secrets.LoadKey(dir, ""); err != nil {
		t.Fatal(err)
	}
	return dir
}

func secretsFile(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "holyarrs-secrets.yaml")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// runImport runs the command and checks that no value of the file shows
// in its output or in the log. Ports are checked with their colon: a bare
// number can turn up in a random temp folder name.
func runImport(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errb, logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	code := importSecrets(args, &out, &errb)
	all := out.String() + errb.String() + logs.String()
	for _, v := range cliValues {
		if strings.Contains(all, v) {
			t.Fatalf("a value from the file was said (%s):\n%s", v, all)
		}
	}
	return code, out.String() + errb.String()
}

func TestImportSecretsOffline(t *testing.T) {
	dir := configDir(t)
	p := secretsFile(t, cliFile)
	code, out := runImport(t, "-file", p, "-config", dir, "-dry-run")
	if code != 0 || !strings.Contains(out, "would set") || !strings.Contains(out, "dry run: nothing was changed") {
		t.Fatalf("dry %d\n%s", code, out)
	}
	code, out = runImport(t, "-file", p, "-config", dir)
	if code != 0 || !strings.Contains(out, "mode: offline") || !strings.Contains(out, "indexers[1]") || strings.Count(out, " set") < 4 {
		t.Fatalf("run %d\n%s", code, out)
	}
	code, out = runImport(t, "-file", p, "-config", dir, "-delete")
	if code != 0 || strings.Contains(out, " set") || !strings.Contains(out, "unchanged") {
		t.Fatalf("again %d\n%s", code, out)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("the file was not deleted")
	}
	code, out = runImport(t, "-forget-move-credentials", "-config", dir)
	if code != 0 || !strings.Contains(out, "1 forgotten") {
		t.Fatalf("forget %d %s", code, out)
	}
}

func TestImportSecretsRefuses(t *testing.T) {
	dir := configDir(t)
	if code, out := runImport(t, "-file", secretsFile(t, strings.Replace(cliFile, "client_id:", "clientid:", 1)), "-config", dir); code != 2 || !strings.Contains(out, `unknown key "clientid"`) {
		t.Fatalf("unknown key %d %s", code, out)
	}
	if code, _ := runImport(t, "-config", dir); code != 2 {
		t.Fatal("no file")
	}
	if code, out := runImport(t, "-file", secretsFile(t, cliFile), "-config", t.TempDir()); code != 2 || !strings.Contains(out, "no HolyArrs database") {
		t.Fatalf("empty config %d %s", code, out)
	}
	big := secretsFile(t, cliFile+"#"+strings.Repeat("x", 300<<10)+"\n")
	if code, out := runImport(t, "-file", big, "-config", dir); code != 2 || !strings.Contains(out, "KB") {
		t.Fatalf("big %d %s", code, out)
	}
	// Other users may read it: refused on Unix, a warning on Windows.
	open := secretsFile(t, cliFile)
	_ = os.Chmod(open, 0o644)
	code, out := runImport(t, "-file", open, "-config", dir, "-dry-run")
	if runtime.GOOS == "windows" {
		if code != 0 || !strings.Contains(out, "cannot check who may read") {
			t.Fatalf("windows %d %s", code, out)
		}
	} else {
		if code != 2 || !strings.Contains(out, "chmod 600") {
			t.Fatalf("readable %d %s", code, out)
		}
		if code, _ := runImport(t, "-file", open, "-config", dir, "-dry-run", "-allow-readable-file"); code != 0 {
			t.Fatal("allow-readable-file")
		}
	}
	// One bad item: exit 1, the file is kept even with -delete.
	bad := secretsFile(t, strings.Replace(cliFile, "http://127.0.0.1:18799/api", "ftp://127.0.0.1/api", 1))
	code, out = runImport(t, "-file", bad, "-config", dir, "-delete")
	if code != 1 || !strings.Contains(out, "indexers[1]") || !strings.Contains(out, "failed:") || !strings.Contains(out, "the file was kept") {
		t.Fatalf("partial %d %s", code, out)
	}
	if _, err := os.Stat(bad); err != nil {
		t.Fatal("a file with a failed item was deleted")
	}
}

// While HolyArrs runs (it holds the config folder's lock), the import goes
// through its API with the owner key in a header.
func TestImportSecretsThroughAPI(t *testing.T) {
	dir := configDir(t)
	unlock, err := files.LockDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	var got struct {
		DryRun bool           `json:"dry_run"`
		File   map[string]any `json:"file"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(accounts.APIKeyHeader) != "your-owner-key-6" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.Contains(r.URL.String(), "your-") {
			t.Error("a value in the address")
		}
		switch r.URL.Path {
		case "/api/v1/settings/import-secrets":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got)
			_, _ = w.Write([]byte(`{"lines":[{"item":"tvdb","result":"set"},{"item":"radarr (old app)","result":"set (saved for the Move tool; used once, then forgotten)"}],"ok":true}`))
		case "/api/v1/migrate/saved-keys":
			_, _ = w.Write([]byte(`{"forgotten":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	text := strings.Replace(cliFile, "http://127.0.0.1:18730", srv.URL, 1)
	code, out := runImport(t, "-file", secretsFile(t, text), "-config", dir)
	if code != 0 || !strings.Contains(out, "mode: through the running HolyArrs's API") || !strings.Contains(out, "tvdb") {
		t.Fatalf("api %d %s", code, out)
	}
	if h, _ := got.File["holyarrs"].(map[string]any); h["owner_api_key"] != "" || h["address"] != "" {
		t.Fatalf("the owner key was sent in the body: %v", h)
	}
	if code, out := runImport(t, "-file", secretsFile(t, text), "-config", dir, "-forget-move-credentials"); code != 0 || !strings.Contains(out, "1 forgotten") {
		t.Fatalf("forget %d %s", code, out)
	}
	// A wrong key, plain http off the local network, no key at all.
	wrong := strings.Replace(text, "your-owner-key-6", "your-other-7", 1)
	if code, out := runImport(t, "-file", secretsFile(t, wrong), "-config", dir); code != 1 || !strings.Contains(out, "refused the owner API key") || strings.Contains(out, "your-other-7") {
		t.Fatalf("wrong key %d %s", code, out)
	}
	public := strings.Replace(cliFile, "http://127.0.0.1:18730", "http://holyarrs.example.com", 1)
	if code, out := runImport(t, "-file", secretsFile(t, public), "-config", dir); code != 2 || !strings.Contains(out, "must be https") {
		t.Fatalf("public http %d %s", code, out)
	}
	none := strings.Replace(cliFile, "owner_api_key: your-owner-key-6", `owner_api_key: ""`, 1)
	if code, out := runImport(t, "-file", secretsFile(t, none), "-config", dir); code != 2 || !strings.Contains(out, "owner_api_key") {
		t.Fatalf("no key %d %s", code, out)
	}
}
