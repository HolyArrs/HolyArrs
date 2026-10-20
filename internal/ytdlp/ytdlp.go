// Package ytdlp finds the yt-dlp program HolyArrs uses for trailers and
// keeps a copy of it up to date from the official GitHub releases.
//
// Lookup order: HOLYARRS_YTDLP, then the updated copy in the config
// folder (written only by a checked update that passed the canary), then
// the copy bundled next to the holyarrs program, then PATH.
// The weekly update downloads the newest release's SHA2-256SUMS and
// SHA2-256SUMS.sig, checks the signature against yt-dlp's pinned public
// key, then checks the program's SHA-256 against the signed sums, swaps it
// in atomically and
// keeps the previous copy; if the canary lookup then fails, the previous
// copy comes back.
package ytdlp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/openpgp"
)

// Env names an explicit yt-dlp program.
const Env = "HOLYARRS_YTDLP"

// Pinned is the release bundled with HolyArrs builds.
const Pinned = "2026.08.19"

// DefaultReleaseBase is the official "latest release" download location.
const DefaultReleaseBase = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"

// Every is how often the update runs.
const Every = 7 * 24 * time.Hour

func exe() string {
	if runtime.GOOS == "windows" {
		return "yt-dlp.exe"
	}
	return "yt-dlp"
}

// Asset is the release file for this machine ("" when there is none).
func Asset() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "yt-dlp.exe"
	case "linux/amd64":
		return "yt-dlp_linux"
	case "linux/arm64":
		return "yt-dlp_linux_aarch64"
	case "darwin/amd64", "darwin/arm64":
		return "yt-dlp_macos"
	}
	return ""
}

func regular(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// Tool finds and updates yt-dlp.
type Tool struct {
	// DataDir is where the updated copy lives (<config>/bin).
	DataDir string
	// Self is the holyarrs program (os.Executable); its folder holds the
	// bundled copy.
	Self string
	// ReleaseBase is where releases are read (tests use a fake server).
	ReleaseBase string
	HTTP        *http.Client
	// Keyring is trusted to sign releases (nil: yt-dlp's pinned key).
	Keyring openpgp.EntityList
	// Canary checks a freshly installed copy (nil: --version only).
	Canary func(ctx context.Context, bin string) error

	mu       sync.Mutex
	result   string // last update result, plain words
	resultAt time.Time
}

// Find returns the yt-dlp to run, or "" when there is none.
func (t *Tool) Find() string {
	if p := strings.TrimSpace(os.Getenv(Env)); p != "" {
		if regular(p) {
			return p
		}
		return ""
	}
	// The updated copy only exists after a checked update passed the
	// canary, so it wins over the bundled one (else updates never apply).
	if t.DataDir != "" {
		if p := filepath.Join(t.DataDir, exe()); regular(p) {
			return p
		}
	}
	if t.Self != "" {
		if p := filepath.Join(filepath.Dir(t.Self), exe()); regular(p) {
			return p
		}
	}
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		return p
	}
	return ""
}

// Version runs yt-dlp --version.
func Version(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := Run(ctx, bin, 30*time.Second, "--version")
	if err != nil {
		return "", errors.New("yt-dlp does not run")
	}
	return strings.TrimSpace(string(out)), nil
}

// LastResult is the last update's result and when it ran (zero: never).
func (t *Tool) LastResult() (string, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.result, t.resultAt
}

func (t *Tool) done(msg string) {
	t.mu.Lock()
	t.result, t.resultAt = msg, time.Now()
	t.mu.Unlock()
}

// Update installs the newest official release into DataDir when it is
// different from the copy there. It returns the version now in DataDir.
func (t *Tool) Update(ctx context.Context) (string, error) {
	v, err := t.update(ctx)
	if err != nil {
		t.done("update failed: " + err.Error())
	} else {
		t.done("up to date (" + v + ")")
	}
	return v, err
}

func (t *Tool) update(ctx context.Context) (string, error) {
	asset := Asset()
	if asset == "" || t.DataDir == "" {
		return "", errors.New("no yt-dlp build for this machine")
	}
	base := t.ReleaseBase
	if base == "" {
		base = DefaultReleaseBase
	}
	cl := t.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 5 * time.Minute}
	}
	get := func(name string, limit int64) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+name, nil)
		if err != nil {
			return nil, err
		}
		resp, err := cl.Do(req)
		if err != nil {
			return nil, errors.New("release download failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("release download status " + resp.Status)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil || int64(len(b)) > limit {
			return nil, errors.New("release download failed")
		}
		return b, nil
	}
	sums, err := get("SHA2-256SUMS", 1<<20)
	if err != nil {
		return "", err
	}
	sig, err := get("SHA2-256SUMS.sig", 64<<10)
	if err != nil {
		return "", errors.New("the release has no signature")
	}
	kr := t.Keyring
	if kr == nil {
		if kr, err = pinnedKeyring(); err != nil {
			return "", err
		}
	}
	if err := verifySums(kr, sums, sig); err != nil {
		return "", err
	}
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
		}
	}
	if len(want) != 64 {
		return "", errors.New("the release lists no checksum for " + asset)
	}
	dest := filepath.Join(t.DataDir, exe())
	if b, err := os.ReadFile(dest); err == nil {
		if h := sha256.Sum256(b); hex.EncodeToString(h[:]) == want {
			return Version(ctx, dest)
		}
	}
	bin, err := get(asset, 128<<20)
	if err != nil {
		return "", err
	}
	if h := sha256.Sum256(bin); hex.EncodeToString(h[:]) != want {
		return "", errors.New("checksum mismatch")
	}
	if err := os.MkdirAll(t.DataDir, 0o755); err != nil {
		return "", err
	}
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", err
	}
	v, err := Version(ctx, tmp)
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	prev := dest + ".prev"
	hadPrev := false
	if regular(dest) {
		_ = os.Remove(prev)
		if err := os.Rename(dest, prev); err != nil {
			os.Remove(tmp)
			return "", err
		}
		hadPrev = true
	}
	if err := os.Rename(tmp, dest); err != nil {
		if hadPrev {
			_ = os.Rename(prev, dest)
		}
		os.Remove(tmp)
		return "", err
	}
	if t.Canary != nil {
		if err := t.Canary(ctx, dest); err != nil {
			if hadPrev {
				_ = os.Remove(dest)
				_ = os.Rename(prev, dest)
			} else {
				_ = os.Remove(dest)
			}
			return "", errors.New("the new yt-dlp failed the canary lookup; kept the previous copy")
		}
	}
	return v, nil
}

func pinnedKeyring() (openpgp.EntityList, error) {
	kr, err := openpgp.ReadArmoredKeyRing(strings.NewReader(publicKey))
	if err != nil || len(kr) == 0 {
		return nil, errors.New("the pinned yt-dlp key does not load")
	}
	return kr, nil
}

// verifySums checks the detached signature (binary or armored) on sums.
func verifySums(kr openpgp.EntityList, sums, sig []byte) error {
	if _, err := openpgp.CheckDetachedSignature(kr, bytes.NewReader(sums), bytes.NewReader(sig)); err == nil {
		return nil
	}
	if _, err := openpgp.CheckArmoredDetachedSignature(kr, bytes.NewReader(sums), bytes.NewReader(sig)); err == nil {
		return nil
	}
	return errors.New("release signature invalid")
}

// MaxOutput is the most yt-dlp may print; a longer answer is refused. A
// trailer with many auto-translated subtitle tracks prints over 4 MB of
// JSON (seen: 4.5 MB), so the cap leaves room for that.
const MaxOutput = 16 << 20

// EnvAllow names the only environment values yt-dlp gets (no HolyArrs
// settings or secrets): the program path, home and temp folders, the
// Windows system folder, and proxies.
var EnvAllow = []string{"PATH", "HOME", "TMP", "TEMP", "TMPDIR", "SYSTEMROOT", "SystemRoot", "WINDIR",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy"}

// MinimalEnv is the environment yt-dlp runs with.
func MinimalEnv() []string {
	var env []string
	seen := map[string]bool{}
	for _, k := range EnvAllow {
		if v, ok := os.LookupEnv(k); ok && !seen[strings.ToUpper(k)+"="+v] {
			seen[strings.ToUpper(k)+"="+v] = true
			env = append(env, k+"="+v)
		}
	}
	if _, ok := os.LookupEnv("HOME"); !ok {
		env = append(env, "HOME="+os.TempDir())
	}
	return env
}

type capped struct {
	b      bytes.Buffer
	over   bool
	cancel context.CancelFunc
}

func (c *capped) Write(p []byte) (int, error) {
	if c.over {
		return 0, errors.New("too large")
	}
	if c.b.Len()+len(p) > MaxOutput {
		c.over = true
		c.cancel()
		return 0, errors.New("too large")
	}
	return c.b.Write(p)
}

// Run runs yt-dlp with a time limit, the minimal environment and at most
// MaxOutput of output (it is stopped when it prints more), and returns
// what it printed.
func Run(ctx context.Context, bin string, limit time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = MinimalEnv()
	out := &capped{cancel: cancel}
	cmd.Stdout = out
	err := cmd.Run()
	if out.over {
		return nil, errors.New("yt-dlp answer too large")
	}
	return out.b.Bytes(), err
}
