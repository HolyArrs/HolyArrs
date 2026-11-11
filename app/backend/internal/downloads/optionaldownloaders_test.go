package downloads

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOptionalDownloaderSecretsRoundTrip(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	for _, kind := range []string{"transmission", "nzbget", "deluge", "rtorrent"} {
		t.Run(kind, func(t *testing.T) {
			secret := "optional-downloader-test-password"
			in := map[string]any{"name": kind, "kind": kind, "url": "http://downloader:9091", "username": "task-user", "secret": secret}
			b := do(t, "POST", srv.URL+"/api/v1/downloaders", body(in), 201)
			var d DownloaderView
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), secret) || d.Secret == "" || d.Secret == secret || d.Kind != kind {
				t.Fatalf("unsafe or incomplete view: %s", b)
			}
			in["secret"], in["priority"] = d.Secret, 7
			b = do(t, "PUT", srv.URL+"/api/v1/downloaders/"+jsonNumber(d.ID), body(in), 200)
			if strings.Contains(string(b), secret) {
				t.Fatal("edit exposed the password")
			}
			in["url"], in["secret"] = "http://another-downloader:9091", ""
			do(t, "PUT", srv.URL+"/api/v1/downloaders/"+jsonNumber(d.ID), body(in), 409)
		})
	}
}

func jsonNumber(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}
