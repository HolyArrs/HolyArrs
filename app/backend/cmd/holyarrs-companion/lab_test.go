package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/api"
	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// This opt-in lab uses the real API and auth with disposable state. Its only
// external process is the owned companion container, with one secret-file mount.
func TestCompanionContainerLab(t *testing.T) {
	if os.Getenv("HOLYARRS_COMPANION_LAB") != "1" {
		t.Skip("set HOLYARRS_COMPANION_LAB=1 with the locally built companion image")
	}
	old := log.Writer()
	log.SetOutput(io.Discard) // first-run setup writes a private one-time code
	t.Cleanup(func() { log.SetOutput(old) })
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal("disposable store could not start")
	}
	defer st.Close()
	a := auth.New(st, auth.Options{AllowedHosts: []string{"host.docker.internal"}})
	listener, err := net.Listen("tcp", ":18740")
	if err != nil {
		t.Fatal("owned fixture port 18740 is unavailable; nothing was replaced")
	}
	server := &http.Server{Handler: api.New(api.Server{Store: st, Auth: a}).Handler()}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	base := "http://127.0.0.1:18740"
	jar, _ := cookiejar.New(nil)
	owner := &http.Client{Jar: jar}
	request := func(c *http.Client, method, route string, body any, headers map[string]string, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, base+route, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal("owned API fixture could not be reached")
		}
		defer resp.Body.Close()
		answer, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != want {
			t.Fatalf("%s %s status %d, expected %d", method, route, resp.StatusCode, want)
		}
		return answer
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), auth.SetupCodeFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a setup code file was written; first run has no setup code")
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal(request(owner, "POST", "/api/v1/auth/setup", map[string]string{
		"name": "synthetic companion owner", "password": rand.Text(),
	}, nil, http.StatusCreated), &session); err != nil || session.CSRF == "" {
		t.Fatal("owned owner setup did not return a session")
	}
	var key struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(request(owner, "POST", "/api/v1/auth/keys", map[string]any{
		"label": "owned health companion lab", "scopes": []string{auth.ScopeHealthRead},
	}, map[string]string{auth.CSRFHeader: session.CSRF}, http.StatusCreated), &key); err != nil || key.ID < 1 || key.Key == "" {
		t.Fatal("task-only key could not be issued")
	}
	headers := map[string]string{auth.APIKeyHeader: key.Key}
	request(http.DefaultClient, "GET", "/api/v1/health/states", nil, headers, http.StatusOK)
	for _, route := range []string{"/api/v1/titles", "/api/v1/settings/naming", "/api/v1/auth/keys"} {
		request(http.DefaultClient, "GET", route, nil, headers, http.StatusForbidden)
	}
	request(http.DefaultClient, "POST", "/api/v1/titles", map[string]string{"name": "must not exist"}, headers, http.StatusForbidden)
	request(http.DefaultClient, "POST", "/api/v1/auth/keys", map[string]any{"label": "must not exist", "scopes": []string{"all"}}, headers, http.StatusForbidden)
	var titles, keys int
	if err := st.Read().QueryRow("SELECT count(*) FROM titles").Scan(&titles); err != nil || titles != 0 {
		t.Fatal("unwanted mutation changed the library")
	}
	if err := st.Read().QueryRow("SELECT count(*) FROM api_keys").Scan(&keys); err != nil || keys != 1 {
		t.Fatal("unwanted key creation was not denied")
	}
	keyPath := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyPath, []byte(key.Key), 0o600); err != nil {
		t.Fatal("owned key file could not be written")
	}
	create := exec.Command("docker", "create", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--mount", "type=bind,source="+keyPath+",target=/run/secrets/holyarrs-key,readonly",
		"-e", "HOLYARRS_URL=http://host.docker.internal:18740", "holyarrs-health-companion:continuation-20261015")
	created, err := create.Output()
	if err != nil {
		t.Fatal("owned companion container could not be created")
	}
	id := strings.TrimSpace(string(created))
	if len(id) != 64 || strings.ContainsAny(id, " /\\\r\n") {
		t.Fatal("Docker did not return an owned container id")
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })
	inspected, err := exec.Command("docker", "inspect", id).Output()
	if err != nil {
		t.Fatal("owned container could not be inspected")
	}
	var configuration []struct {
		Config     struct{ User string }
		HostConfig struct {
			ReadonlyRootfs bool
			Privileged     bool
			CapDrop        []string
		}
		Mounts []struct {
			Destination string
			RW          bool
		}
	}
	if json.Unmarshal(inspected, &configuration) != nil || len(configuration) != 1 {
		t.Fatal("owned container configuration unavailable")
	}
	c := configuration[0]
	if c.Config.User != "1000:1000" || !c.HostConfig.ReadonlyRootfs || c.HostConfig.Privileged || len(c.HostConfig.CapDrop) != 1 || !strings.EqualFold(c.HostConfig.CapDrop[0], "ALL") || len(c.Mounts) != 1 || c.Mounts[0].Destination != "/run/secrets/holyarrs-key" || c.Mounts[0].RW {
		t.Fatal("companion isolation does not match the recipe")
	}
	output, err := exec.Command("docker", "start", "--attach", id).Output()
	if err != nil || bytes.Contains(output, []byte(key.Key)) {
		t.Fatal("owned companion did not complete safely")
	}
	var summary struct {
		States map[string]int `json:"states"`
	}
	if json.Unmarshal(output, &summary) != nil || summary.States == nil {
		t.Fatal("companion did not return state counts")
	}
	request(owner, "DELETE", fmt.Sprintf("/api/v1/auth/keys/%d", key.ID), nil, map[string]string{auth.CSRFHeader: session.CSRF}, http.StatusNoContent)
	request(http.DefaultClient, "GET", "/api/v1/health/states", nil, headers, http.StatusUnauthorized)
	t.Log("container health read passed; other reads, mutation and scope escalation denied; key revoked; owned container and temporary state cleaned")
}
