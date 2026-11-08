package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// importSecrets is `holyarrs import-secrets`: it reads the owner's secrets
// file and stores each filled-in item, straight into the config folder when
// HolyArrs is stopped, or through the running app's API (holyarrs.address
// and holyarrs.owner_api_key in the file) when it runs. It prints one line
// per item and never a value. Exit 0 when every item went well, 1 when one
// failed, 2 for a bad command line or file.
func importSecrets(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("holyarrs import-secrets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "the secrets file (YAML)")
	dir := fs.String("config", "/config", "HolyArrs's config folder (used when HolyArrs is stopped)")
	dry := fs.Bool("dry-run", false, "only say what would be set")
	del := fs.Bool("delete", false, "after a fully successful run, overwrite the file with zeros and delete it")
	forget := fs.Bool("forget-move-credentials", false, "delete every arr key saved for the Move tool, then stop")
	shared := fs.Bool("allow-readable-file", false, "accept a file other users may read (Docker Desktop shows every file as readable)")
	httpLAN := fs.Bool("allow-http-lan", false, "send the owner key and the file over plain http to a local-network address (not only this machine)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "import-secrets: unexpected arguments; use -file <path>")
		return 2
	}
	say := func(format string, a ...any) { fmt.Fprintf(stdout, format+"\n", a...) }
	warn := func(format string, a ...any) { fmt.Fprintf(stderr, "import-secrets: "+format+"\n", a...) }

	var f *secretsfile.File
	if *file != "" {
		var err error
		f, err = readSecretsFile(*file, *shared, warn)
		if err != nil {
			warn("%v", err)
			return 2
		}
	} else if !*forget {
		warn("name the secrets file with -file <path>")
		return 2
	}
	hide := func(msg string) string {
		if f != nil {
			for _, v := range f.Values() {
				if len(v) >= 3 {
					msg = strings.ReplaceAll(msg, v, "[hidden]")
				}
			}
		}
		return msg
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	unlock, lockErr := files.LockDir(*dir)
	var lines []secretsfile.Line
	switch {
	case lockErr == nil:
		defer unlock()
		if _, err := os.Stat(filepath.Join(*dir, database.MainFile)); err != nil {
			warn("no HolyArrs database in the config folder; start HolyArrs once first, or pass -config <folder>")
			return 2
		}
		keyFile := os.Getenv(secrets.EnvKeyFile)
		if keyFile == "" {
			if _, err := os.Stat(filepath.Join(*dir, secrets.KeyFile)); err != nil {
				warn("no master key in the config folder (or %s); the vault cannot be opened", secrets.EnvKeyFile)
				return 2
			}
		}
		s, err := database.Open(*dir, database.Options{})
		if err != nil {
			warn("opening the config folder: %s", hide(err.Error()))
			return 2
		}
		defer s.Close()
		key, err := secrets.LoadKey(*dir, keyFile)
		if err != nil {
			warn("reading the master key: %s", hide(err.Error()))
			return 2
		}
		vault, err := secrets.Open(ctx, s, key)
		if err != nil {
			warn("opening the vault: %s", hide(err.Error()))
			return 2
		}
		if *forget {
			n, err := secretsfile.ForgetMoveCredential(ctx, s, "")
			if err != nil {
				warn("%s", hide(err.Error()))
				return 1
			}
			say("move credentials: %d forgotten", n)
			return 0
		}
		say("mode: offline (HolyArrs is stopped; writing to the config folder)")
		lines = secretsfile.Apply(ctx, secretsfile.Deps{Store: s, Vault: vault, By: domain.System}, f, *dry)
	case errors.Is(lockErr, files.ErrDirLocked):
		if f == nil {
			warn("HolyArrs is running: pass -file <path> so its address and owner key can be read from the secrets file")
			return 2
		}
		base, key, err := appTarget(f, *httpLAN, warn)
		if err != nil {
			warn("HolyArrs is running, so the import goes through its API: %v", err)
			return 2
		}
		if *forget {
			var out struct {
				Forgotten int `json:"forgotten"`
			}
			if err := callApp(ctx, http.MethodDelete, base+"/api/v1/migrate/saved-keys", key, nil, &out); err != nil {
				warn("%s", hide(err.Error()))
				return 1
			}
			say("move credentials: %d forgotten", out.Forgotten)
			return 0
		}
		say("mode: through the running HolyArrs's API")
		send := *f
		send.HolyArrs.Address, send.HolyArrs.OwnerAPIKey = "", ""
		var out struct {
			Lines []secretsfile.Line `json:"lines"`
		}
		if err := callApp(ctx, http.MethodPost, base+"/api/v1/settings/import-secrets", key, map[string]any{"dry_run": *dry, "file": send}, &out); err != nil {
			warn("%s", hide(err.Error()))
			return 1
		}
		lines = out.Lines
	default:
		warn("the config folder cannot be used: %s", hide(lockErr.Error()))
		return 2
	}
	for _, l := range lines {
		say("%-26s %s", l.Item, hide(l.Result))
	}
	if !secretsfile.AllOK(lines) {
		say("some items failed; fix them in the file and run the import again (items already set say unchanged)")
		if *del {
			say("the file was kept because not every item went well")
		}
		return 1
	}
	if *dry {
		say("dry run: nothing was changed")
		return 0
	}
	if *del {
		if err := wipe(*file); err != nil {
			warn("the file could not be deleted (%s); delete it yourself", hide(err.Error()))
			return 1
		}
		say("the secrets file was overwritten with zeros and deleted")
	}
	return 0
}

// readSecretsFile checks and reads the file: a regular file, not a link,
// of at most MaxSize that other users may not read (Unix; Windows gets a
// warning). The checks run on the opened file, so it cannot be swapped
// between the check and the read.
func readSecretsFile(path string, shared bool, warn func(string, ...any)) (*secretsfile.File, error) {
	lst, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("the secrets file cannot be found or opened")
	}
	if lst.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("the secrets file is a link; name the file itself")
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, errors.New("the secrets file cannot be opened")
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil || !os.SameFile(lst, st) {
		return nil, errors.New("the secrets file changed while it was opened")
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("the secrets file is not a regular file")
	}
	if st.Size() > secretsfile.MaxSize {
		return nil, secretsfile.ErrTooBig
	}
	if runtime.GOOS == "windows" {
		warn("warning: on Windows HolyArrs cannot check who may read the file; keep it in a folder only you can open")
	} else if st.Mode().Perm()&0o077 != 0 {
		if !shared {
			return nil, fmt.Errorf("other users may read the secrets file; run chmod 600 on it (or pass -allow-readable-file when Docker Desktop shows every file as readable)")
		}
		warn("warning: other users may read the secrets file")
	}
	b, err := io.ReadAll(io.LimitReader(fh, secretsfile.MaxSize+1))
	if err != nil {
		return nil, errors.New("the secrets file cannot be read")
	}
	f, err := secretsfile.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("the secrets file: %w", err)
	}
	return f, nil
}

// appTarget is the running app's address and owner key from the file.
// Plain http carries the owner key and the whole file unencrypted, so it
// goes only to this machine (loopback) unless allowLAN (-allow-http-lan)
// lets it reach a local-network address, with a warning. https may go
// anywhere. A cloud metadata or link-local address is always refused.
func appTarget(f *secretsfile.File, allowLAN bool, warn func(string, ...any)) (string, string, error) {
	raw, key := strings.TrimRight(strings.TrimSpace(f.HolyArrs.Address), "/"), strings.TrimSpace(f.HolyArrs.OwnerAPIKey)
	if raw == "" || key == "" {
		return "", "", errors.New("fill in holyarrs.address and holyarrs.owner_api_key in the file (an owner API key from My account)")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", errors.New("holyarrs.address must look like https://host:port")
	}
	h := strings.ToLower(u.Hostname())
	if metadataHost(h) {
		return "", "", errors.New("holyarrs.address is a cloud metadata or link-local address; refused")
	}
	if u.Scheme == "http" && !loopbackHost(h) {
		if !allowLAN || !localHost(h) {
			return "", "", errors.New("holyarrs.address must be https unless HolyArrs is on this machine (run the import inside its container: docker exec ... holyarrs import-secrets, with http://127.0.0.1:port), or pass -allow-http-lan for a local-network address")
		}
		warn("warning: the owner key and the whole file go over plain http to a local-network address (-allow-http-lan); anyone on that network could read them")
	}
	return raw, key, nil
}

// loopbackHost says whether a host is this machine.
func loopbackHost(h string) bool {
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return h == "localhost"
}

// metadataHost says whether a host is a cloud metadata service or any
// link-local address.
func metadataHost(h string) bool {
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLinkLocalUnicast() || ip.Equal(net.ParseIP("fd00:ec2::254")) || ip.Equal(net.ParseIP("100.100.100.200"))
	}
	return h == "metadata.google.internal" || h == "metadata"
}

// localHost says whether a host name is the local network: a private
// address, or a name without dots (a container or LAN name) or under
// .local, .lan or .home.arpa.
func localHost(h string) bool {
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsPrivate()
	}
	h = strings.ToLower(h)
	return !strings.Contains(h, ".") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".lan") || strings.HasSuffix(h, ".home.arpa")
}

// callApp sends one call with the key in its header (never in the address)
// and reads the JSON answer. Errors never hold the address or the key.
func callApp(ctx context.Context, method, addr, key string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return errors.New("the file could not be packed for sending")
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, addr, rd)
	if err != nil {
		return errors.New("holyarrs.address is not a usable address")
	}
	req.Header.Set(accounts.APIKeyHeader, key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("the running HolyArrs could not be reached (%v)", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return errors.New("the running HolyArrs refused the owner API key")
		case http.StatusForbidden:
			return fmt.Errorf("the running HolyArrs refused: %s (the key must be the owner's, with the admin scope)", e.Error)
		}
		return fmt.Errorf("the running HolyArrs answered %d: %s", resp.StatusCode, e.Error)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return errors.New("the running HolyArrs sent an answer that could not be read")
	}
	return nil
}

// wipe overwrites a file with zeros, syncs it and removes it (best effort
// on disks that keep old blocks).
func wipe(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	zero := make([]byte, 32<<10)
	for left := st.Size(); left > 0; {
		n := min(left, int64(len(zero)))
		if _, err := fh.Write(zero[:n]); err != nil {
			fh.Close()
			return err
		}
		left -= n
	}
	if err := errors.Join(fh.Sync(), fh.Close()); err != nil {
		return err
	}
	return os.Remove(path)
}
