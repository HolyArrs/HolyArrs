package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// DataCompat is this build's data format number (see Channel.DataCompat).
const DataCompat = 1

// downloadHosts are the only hosts a program is downloaded from.
var downloadHosts = []string{"github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com", "api.holyarrs.com"}

// maxProgram caps a download, whatever the list says.
const maxProgram = 200 << 20

// DownloadAllowed says a program link is plain https on an allowed host.
func DownloadAllowed(link string) bool {
	u, err := url.Parse(link)
	return labAllowed(link) || err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && slices.Contains(downloadHosts, u.Hostname())
}

// Platform is this program's entry name in Channel.Files.
func Platform() string {
	if runtime.GOARCH == "arm" {
		return runtime.GOOS + "-armv7"
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

// whyNot says why a channel's release may not be installed over running,
// or "".
func whyNot(l *List, c Channel, running string, skip []string) string {
	switch {
	case c.Version == "" || !Newer(c.Version, running):
		return "no newer version"
	case slices.Contains(l.Revoked, c.Version):
		return "this version was withdrawn"
	case slices.Contains(skip, c.Version):
		return "you went back from this version, so it is skipped"
	case c.ImageRequired:
		return "this version needs a new image: pull it to update"
	case c.MinFrom != "" && Newer(c.MinFrom, running):
		return fmt.Sprintf("update to %s first (pull that image), then this one", c.MinFrom)
	case max(c.DataCompat, 1) != DataCompat:
		return "this version keeps its data in a new format: pull the new image to update"
	}
	if _, ok := c.Files[Platform()]; !ok {
		return "this version has no download for " + Platform()
	}
	return ""
}

// stage downloads c's program into its version folder and checks it: size,
// the list's sha256, its own signature and a dry run against this config
// folder. On any failure the folder is removed; nothing is installed.
func (in *Installer) stage(ctx context.Context, c Channel) (Installed, error) {
	f := in.folder()
	file := c.Files[Platform()]
	if (!DownloadAllowed(file.URL) && in.Client == nil) || file.Size <= 0 || file.Size > maxProgram {
		return Installed{}, errors.New("the download link is not allowed")
	}
	if free, err := files.DiskFree(string(f)); err == nil && free < uint64(file.Size)*2 {
		return Installed{}, fmt.Errorf("not enough free space in the config folder for the %d MB download", file.Size>>20)
	}
	dir := filepath.Join(string(f), c.Version)
	final := filepath.Join(dir, binaryName)
	fail := func(err error) (Installed, error) {
		_ = os.RemoveAll(dir)
		return Installed{}, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fail(err)
	}
	in.progress(PhaseDownloading, c.Version, "")
	sum, err := in.download(ctx, file.URL, final+tmpSuffix, file.Size)
	if err != nil {
		return fail(err)
	}
	if sum != strings.ToLower(file.SHA256) {
		return fail(errors.New("the download does not match the update list, so it was deleted"))
	}
	sig, err := in.fetchSmall(ctx, file.URL+SigSuffix)
	if err != nil {
		return fail(errors.New("the download's signature could not be read"))
	}
	data, err := os.ReadFile(final + tmpSuffix)
	if err != nil {
		return fail(err)
	}
	if _, err := Verify(data, sig, in.keys()); err != nil {
		return fail(errors.New("the download is not signed by HolyArrs, so it was deleted"))
	}
	if err := writeAtomic(final+SigSuffix, sig, 0o600); err != nil {
		return fail(err)
	}
	if err := os.Chmod(final+tmpSuffix, 0o755); err != nil {
		return fail(err)
	}
	if err := os.Rename(final+tmpSuffix, final); err != nil {
		return fail(err)
	}
	syncDir(dir)
	in.progress(PhaseChecking, c.Version, "")
	if err := in.dryRun(ctx, final); err != nil {
		return fail(fmt.Errorf("the new version could not open your data, so it was not installed: %v", err))
	}
	return Installed{Version: c.Version, Path: final, SHA256: sum, Migrates: c.Migrates}, nil
}

// download streams link into tmp (synced), capped at size, and returns
// its sha256.
func (in *Installer) download(ctx context.Context, link, tmp string, size int64) (string, error) {
	resp, err := in.get(ctx, link)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, size+1))
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", errors.New("the download stopped part way")
	}
	if n != size {
		return "", errors.New("the download has the wrong size, so it was deleted")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (in *Installer) fetchSmall(ctx context.Context, link string) ([]byte, error) {
	resp, err := in.get(ctx, link)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 8<<10))
}

// get fetches link. With the built-in client only allowed hosts are asked,
// redirects included; a test client may use a local server.
func (in *Installer) get(ctx context.Context, link string) (*http.Response, error) {
	if in.Client == nil && !DownloadAllowed(link) {
		return nil, errors.New("the download link is not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "HolyArrs/"+in.Version)
	c := http.Client{Timeout: downloadTimeout}
	if in.Client != nil {
		c = *in.Client
	}
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		if in.Client == nil && !DownloadAllowed(r.URL.String()) {
			return errors.New("the download moved to a host that is not allowed")
		}
		return nil
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.New("the download server did not answer")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("the download server answered %d", resp.StatusCode)
	}
	return resp, nil
}

// dryRun starts the new program read-only against this config folder; it
// must exit 0. A volume that cannot run programs fails here, before any
// switch.
func (in *Installer) dryRun(ctx context.Context, bin string) error {
	if in.Check != nil {
		return in.Check(ctx, bin)
	}
	cmd := exec.CommandContext(ctx, bin, CheckCommand, "-config", in.ConfigDir)
	cmd.Env = append(withoutEnv(os.Environ(), envFromVolume), envFromVolume+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return fmt.Errorf("%v: %s", err, lastLine(s))
		}
		return err
	}
	return nil
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
