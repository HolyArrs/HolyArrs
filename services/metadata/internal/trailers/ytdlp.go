package trailers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// YTDLP finds and updates yt-dlp. The image ships a pinned release at
// Builtin; the weekly update writes newer releases to Dir (on the data
// volume, since the root is read-only) and those win once they work.
type YTDLP struct {
	Builtin string // e.g. /usr/local/bin/yt-dlp
	Dir     string // e.g. /data/bin
	// ReleaseBase is the GitHub "latest release" download address.
	ReleaseBase string
	HTTP        *http.Client
	// CanaryKey is a long-lived official trailer resolved by the canary.
	CanaryKey string
	// LatestURL is GitHub's latest-release API address for yt-dlp.
	LatestURL string

	etag     string // last Latest answer, reused on 304
	tag      string
	released time.Time
}

// DefaultLatestURL is yt-dlp's latest release on GitHub's API (read
// unauthenticated, once a day).
const DefaultLatestURL = "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"

// Latest reads yt-dlp's newest release tag and date, sending the last
// ETag so an unchanged answer is a cheap 304.
func (y *YTDLP) Latest(ctx context.Context) (string, time.Time, error) {
	cl := y.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.LatestURL, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "holyarrs-metadata")
	if y.etag != "" {
		req.Header.Set("If-None-Match", y.etag)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && y.tag != "" {
		return y.tag, y.released, nil
	}
	if resp.StatusCode != 200 {
		return "", time.Time{}, errors.New("latest release status " + resp.Status)
	}
	var j struct {
		Tag       string    `json:"tag_name"`
		Published time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&j); err != nil || j.Tag == "" {
		return "", time.Time{}, errors.New("latest release unreadable")
	}
	y.etag, y.tag, y.released = resp.Header.Get("ETag"), j.Tag, j.Published
	return j.Tag, j.Published, nil
}

// CanaryKey is an official trailer that has stayed up for years (Big Buck
// Bunny, the Blender Foundation's open film).
const CanaryKey = "aqz-KE-bpKQ"

// DefaultReleaseBase is the official yt-dlp release location.
const DefaultReleaseBase = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"

func (y *YTDLP) asset() string {
	switch runtime.GOARCH {
	case "arm64":
		return "yt-dlp_linux_aarch64"
	case "amd64":
		return "yt-dlp_linux"
	}
	return ""
}

// Path is the yt-dlp to run: the updated one when present, else the built-in.
func (y *YTDLP) Path() string {
	if y.Dir != "" {
		p := filepath.Join(y.Dir, "yt-dlp")
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	if _, err := os.Stat(y.Builtin); err == nil {
		return y.Builtin
	}
	return ""
}

func run(ctx context.Context, bin string, limit time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+os.TempDir())
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if out.Len() > 16<<20 {
		return nil, errors.New("yt-dlp answer too large")
	}
	if strings.Contains(errOut.String(), "not a bot") {
		return out.Bytes(), ErrBotCheck
	}
	return out.Bytes(), err
}

// ErrBotCheck: YouTube asks this server's address to sign in ("confirm
// you're not a bot"); yt-dlp itself may be fine.
var ErrBotCheck = errors.New("YouTube bot check for this server's address")

// Version runs yt-dlp --version.
func (y *YTDLP) Version(ctx context.Context, bin string) (string, error) {
	out, err := run(ctx, bin, 30*time.Second, "--version")
	if err != nil {
		return "", errors.New("yt-dlp does not run")
	}
	return strings.TrimSpace(string(out)), nil
}

// Canary resolves CanaryKey and checks that it gives a playable address
// (an HLS manifest or a direct MP4). It returns yt-dlp's version.
func (y *YTDLP) Canary(ctx context.Context) (string, error) {
	bin := y.Path()
	if bin == "" {
		return "", errors.New("yt-dlp missing")
	}
	v, err := y.Version(ctx, bin)
	if err != nil {
		return "", err
	}
	out, err := run(ctx, bin, 60*time.Second, "-J", "--no-playlist", "--no-cache-dir", "--no-check-formats", "--skip-download", "--ignore-no-formats-error",
		"--extractor-args", "youtube:player_client=visionos;player_skip=configs", "--", "https://www.youtube.com/watch?v="+y.CanaryKey)
	if errors.Is(err, ErrBotCheck) {
		return v, err
	}
	if err != nil {
		return v, errors.New("canary lookup failed")
	}
	var j struct {
		Formats []struct {
			FormatID    string `json:"format_id"`
			URL         string `json:"url"`
			ManifestURL string `json:"manifest_url"`
		} `json:"formats"`
	}
	if json.Unmarshal(out, &j) != nil {
		return v, errors.New("canary answer unreadable")
	}
	for _, f := range j.Formats {
		if strings.Contains(f.ManifestURL, "/hls_variant/") || strings.Contains(f.URL, ".googlevideo.com/") {
			return v, nil
		}
	}
	return v, errors.New("canary found no playable format")
}

// Update downloads the newest official release, checks it against the
// release's SHA2-256SUMS, makes sure it runs and puts it in Dir. It does
// nothing when the newest is already in use.
func (y *YTDLP) Update(ctx context.Context) (string, error) {
	asset := y.asset()
	if asset == "" || y.Dir == "" {
		return "", errors.New("no yt-dlp build for this machine")
	}
	cl := y.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 5 * time.Minute}
	}
	get := func(name string, limit int64) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.ReleaseBase+name, nil)
		if err != nil {
			return nil, err
		}
		resp, err := cl.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
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
		return "", errors.New("release has no checksum for " + asset)
	}
	if cur := y.Path(); cur != "" {
		if b, err := os.ReadFile(cur); err == nil {
			h := sha256.Sum256(b)
			if hex.EncodeToString(h[:]) == want {
				return y.Version(ctx, cur)
			}
		}
	}
	bin, err := get(asset, 128<<20)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(bin)
	if hex.EncodeToString(h[:]) != want {
		return "", errors.New("release checksum mismatch")
	}
	if err := os.MkdirAll(y.Dir, 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(y.Dir, "yt-dlp.new")
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", err
	}
	v, err := y.Version(ctx, tmp)
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, filepath.Join(y.Dir, "yt-dlp")); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return v, nil
}
