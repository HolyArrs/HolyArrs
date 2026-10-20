// Package ytdlp finds the yt-dlp program HolyArrs uses for trailers and
// keeps a copy of it up to date from the official GitHub releases.
//
// Lookup order: HOLYARRS_YTDLP, then the updated copy in the config
// folder (written only by a checked update that passed the canary), then
// the copy bundled next to the holyarrs program, then PATH.
// The weekly update downloads the newest release, checks its SHA-256
// against the release's SHA2-256SUMS file, swaps it in atomically and
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
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
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

// Run runs yt-dlp with a time limit and returns what it printed.
func Run(ctx context.Context, bin string, limit time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if out.Len() > 16<<20 {
		return nil, errors.New("yt-dlp answer too large")
	}
	return out.Bytes(), err
}
