package downloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// aria2Confirm is how often and how long an add is looked for after it was sent.
var aria2Confirm = struct {
	tries int
	gap   time.Duration
}{10, 500 * time.Millisecond}

// aria2Settle is how long a remove waits for aria2 to stop a running job.
var aria2Settle = struct {
	tries int
	gap   time.Duration
}{10, 200 * time.Millisecond}

// Aria2 is a Client for aria2's JSON-RPC interface. aria2 has no labels, so
// the category is a folder: HolyArrs saves into the save folder or aria2's
// download folder plus the category, and a job is ours only in that folder.
type Aria2 struct {
	Category string
	Others   []string // more categories that are also ours
	// PathMap and SaveRoot tell where HolyArrs sees the files, so a remove that
	// asks for the files can delete them here. aria2 never deletes them.
	PathMap  []domain.PathMap
	SaveRoot string
	// Keep lists folders that are never deleted, nor anything in or around
	// them: the Library folders. Without it nothing is deleted.
	Keep func(ctx context.Context) ([]string, error)

	endpoint, secret string
	http             *http.Client
}

// NewAria2 returns a client for the aria2 at base. The path follows Aria2Path.
func NewAria2(base, secret string, h *http.Client) *Aria2 {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	base = strings.TrimSpace(base)
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		u.Path, u.RawPath = Aria2Path(u.Path), ""
		base = u.String()
	}
	return &Aria2{endpoint: base, secret: secret, http: h}
}

func jsonInto(raw json.RawMessage, out any) error {
	if json.Unmarshal(raw, out) != nil {
		return errNotAria2
	}
	return nil
}

// Health reports whether aria2 answers, accepts the secret and can take torrents.
func (a *Aria2) Health(ctx context.Context) (Health, error) {
	answers, errs, err := a.multicall(ctx, []a2Call{{"aria2.getVersion", nil}, {"aria2.getGlobalOption", nil}})
	if err != nil {
		return Health{}, err
	}
	if err := errors.Join(errs...); err != nil {
		if errors.Is(err, ErrAuth) {
			return Health{}, ErrAuth
		}
		return Health{}, err
	}
	var v struct {
		Version  string   `json:"version"`
		Features []string `json:"enabledFeatures"`
	}
	var opts map[string]string
	if json.Unmarshal(answers[0], &v) != nil || v.Version == "" || json.Unmarshal(answers[1], &opts) != nil {
		return Health{}, errors.New("the address answered, but not as aria2 JSON-RPC")
	}
	if !slices.Contains(v.Features, "BitTorrent") {
		return Health{}, errors.New("this aria2 was built without BitTorrent")
	}
	major, rest, _ := strings.Cut(v.Version, ".")
	minor, _, _ := strings.Cut(rest, ".")
	if x, err := strconv.Atoi(major); err == nil {
		if y, err := strconv.Atoi(minor); err == nil && (x < 1 || x == 1 && y < 34) {
			return Health{}, fmt.Errorf("aria2 %s is too old; HolyArrs needs 1.34 or newer", v.Version)
		}
	}
	if !absPath(opts["dir"]) {
		return Health{}, fmt.Errorf("aria2's download folder %q is not a full path", opts["dir"])
	}
	if _, err := a.all(ctx); err != nil {
		return Health{}, err
	}
	notes := []string{}
	if a.secret == "" {
		notes = append(notes, "aria2 has no rpc-secret, so anyone who can reach it can control it")
	}
	if opts["on-download-stop"] != "" || opts["on-download-complete"] != "" {
		notes = append(notes, "aria2 runs a script when a download stops; some images use it to delete the files of removed or failed downloads")
	}
	if strings.TrimSpace(opts["bt-tracker"]) != "" {
		notes = append(notes, "aria2 adds its own tracker list to every torrent, private ones included")
	}
	return Health{Version: v.Version, At: time.Now().UTC(), Note: strings.Join(notes, "; ")}, nil
}

// List returns the torrents in the category's folder. An empty category
// returns every torrent.
func (a *Aria2) List(ctx context.Context, category string) ([]Job, error) {
	rows, err := a.all(ctx)
	if err != nil {
		return nil, err
	}
	dir := ""
	if category != "" {
		folder, err := a.folder(ctx, a.SaveRoot, category)
		if err != nil {
			return nil, err
		}
		dir = a2DirKey(folder)
	}
	groups := byHash(rows)
	jobs := []Job{}
	seen := map[string]bool{}
	for _, s := range rows {
		h := strings.ToLower(s.InfoHash)
		if seen[h] || groups[h] == nil {
			continue
		}
		seen[h] = true
		if p, ok := pick(groups[h]); ok && (category == "" || a2DirKey(p.Dir) == dir) {
			jobs = append(jobs, a.job(p))
		}
	}
	return jobs, nil
}

// Get returns the torrent with the given hash, with its files.
func (a *Aria2) Get(ctx context.Context, id string) (Job, error) {
	s, _, _, err := a.find(ctx, id)
	if err != nil {
		return Job{}, err
	}
	j := a.job(s)
	j.Files = a.files(s)
	return j, nil
}

// Add sends a torrent into the category's folder and returns its hash.
// HolyArrs fetches a .torrent itself and sends its bytes, so aria2 never
// contacts the Source.
func (a *Aria2) Add(ctx context.Context, req AddRequest) (string, error) {
	if req.Category == "" {
		return "", ErrCategory
	}
	if req.Category == "." || req.Category == ".." || strings.ContainsAny(req.Category, `/\`) {
		return "", ErrAria2Folder
	}
	expected := strings.ToLower(req.InfoHash)
	if expected != "" && !releasename.ValidInfoHash(expected) {
		return "", errBadHash
	}
	if req.ReconcileOnly {
		if expected == "" || req.Skip[expected] {
			return "", ErrUncertain
		}
		s, _, _, err := a.find(ctx, expected)
		if err != nil || a2Folder(s.Dir) != req.Category {
			return "", errors.Join(ErrUncertain, err)
		}
		return expected, nil
	}
	var torrent []byte
	if h, ok := magnetHash(req.URL); ok {
		if expected != "" && expected != h {
			return "", errors.New("the magnet and the expected info hash do not match")
		}
		expected = h
	} else {
		b, err := req.File, error(nil)
		if len(b) == 0 {
			b, err = fetchRelease(ctx, req.URL, req.AllowLocal, torrentFetchMax)
		}
		if err != nil {
			return "", err
		}
		if !req.AllowLocal {
			if b, err = safeTorrent(b); err != nil {
				return "", err
			}
		}
		h, err := torrentFileHash(b)
		if err != nil {
			return "", err
		}
		if expected != "" && expected != h {
			return "", errors.New("the torrent file does not match its info hash")
		}
		expected, torrent = h, b
	}
	if req.Skip[expected] {
		return "", errors.New("the torrent belongs to another download")
	}
	folder, err := a.folder(ctx, req.SavePath, req.Category)
	if err != nil {
		return "", err
	}
	if done, err := a.duplicate(ctx, expected, folder); done || err != nil {
		if err != nil {
			return "", err
		}
		return expected, nil
	}
	opts := map[string]string{"dir": folder}
	if torrent != nil && torrentPrivate(torrent) {
		// aria2's own tracker list would otherwise be added to a private torrent.
		opts["bt-tracker"] = ""
	}
	var gid string
	var sendErr error
	if torrent == nil {
		sendErr = a.call(ctx, "aria2.addUri", &gid, []string{req.URL}, opts)
	} else {
		sendErr = a.call(ctx, "aria2.addTorrent", &gid, base64.StdEncoding.EncodeToString(torrent), []string{}, opts)
	}
	var fault *a2Fault
	var big aria2TooLarge
	if errors.As(sendErr, &fault) || errors.Is(sendErr, ErrAuth) || errors.Is(sendErr, outbound.ErrAddressRefused) || errors.As(sendErr, &big) {
		return "", sendErr
	}
	if sendErr != nil {
		gid = ""
	}
	return a.confirm(ctx, gid, expected, folder, sendErr)
}

// duplicate looks for the hash before an add, since a second add of a live
// hash makes aria2 start an errored copy that shares the files. It reports
// true when the torrent is already in the category's folder.
func (a *Aria2) duplicate(ctx context.Context, hash, folder string) (bool, error) {
	rows, err := a.all(ctx)
	if err != nil {
		return false, err
	}
	group := byHash(rows)[hash]
	for _, s := range group {
		if live(s) {
			if a2DirKey(s.Dir) == a2DirKey(folder) {
				return true, nil
			}
			return false, errors.New("the torrent is already in aria2 in another folder")
		}
	}
	stale := []string{}
	for _, s := range group {
		if a2DirKey(s.Dir) != a2DirKey(folder) {
			continue
		}
		if s.Status == "complete" && len(s.FollowedBy) == 0 {
			return true, nil
		}
		stale = append(stale, s.GID)
	}
	for _, gid := range stale {
		if err := a.clearResult(ctx, gid); err != nil {
			return false, err
		}
	}
	return false, nil
}

// folder is where an add goes: the save folder, or aria2's own download
// folder when the Downloader has path pairs, plus the category.
func (a *Aria2) folder(ctx context.Context, save, category string) (string, error) {
	base, err := a.baseDir(ctx, save)
	if err != nil {
		return "", err
	}
	return a2Join(base, category), nil
}

// baseDir is the folder the categories sit in: the save folder, or aria2's
// own download folder when the Downloader has path pairs.
func (a *Aria2) baseDir(ctx context.Context, save string) (string, error) {
	base, ok := a.saveDir(save)
	if !ok {
		var opts map[string]string
		if err := a.call(ctx, "aria2.getGlobalOption", &opts); err != nil {
			return "", err
		}
		base = opts["dir"]
	}
	if !absPath(base) {
		return "", fmt.Errorf("aria2's download folder %q is not a full path", base)
	}
	return base, nil
}

// ourDirs are the exact folders a job of this Downloader sits in: the base
// folder joined with the category and with each of the others.
func (a *Aria2) ourDirs(ctx context.Context) ([]string, error) {
	base, err := a.baseDir(ctx, a.SaveRoot)
	if err != nil {
		return nil, err
	}
	dirs := []string{}
	for _, c := range append([]string{a.Category}, a.Others...) {
		if c != "" {
			dirs = append(dirs, a2DirKey(a2Join(base, c)))
		}
	}
	return dirs, nil
}

// a2DirKey is a folder of the aria2 machine in a form that two spellings of
// the same folder share.
func a2DirKey(dir string) string {
	if a2Sep(dir) == `\` {
		dir = strings.ReplaceAll(dir, `\`, "/")
	}
	return path.Clean(dir)
}

// saveDir is HolyArrs's save folder when aria2 sees the same paths.
func (a *Aria2) saveDir(p string) (string, bool) {
	if p == "" || len(a.PathMap) > 0 || !path.IsAbs(p) || strings.ContainsAny(p, "\"\\") {
		return "", false
	}
	for _, c := range p {
		if c < 0x20 || c == 0x7f {
			return "", false
		}
	}
	return p, true
}

// confirm looks for the add: by the GID aria2 answered, or by hash when the
// answer was lost.
func (a *Aria2) confirm(ctx context.Context, gid, hash, folder string, sendErr error) (string, error) {
	for i := 0; i < aria2Confirm.tries && ctx.Err() == nil; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(aria2Confirm.gap):
			}
		}
		if gid == "" {
			rows, err := a.all(ctx)
			if err != nil {
				continue
			}
			s, ok := pick(byHash(rows)[hash])
			if !ok {
				continue
			}
			if a2DirKey(s.Dir) != a2DirKey(folder) {
				return "", errors.Join(ErrUncertain, errors.New("the torrent is in aria2 in another folder"))
			}
			return hash, nil
		}
		var s a2Status
		err := a.call(ctx, "aria2.tellStatus", &s, gid, a2Keys)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return "", errors.Join(ErrUncertain, err)
		}
		if h := strings.ToLower(s.InfoHash); h != hash {
			if err := a.forget(ctx, s); err != nil {
				return "", errors.Join(ErrUncertain, err)
			}
			return "", fmt.Errorf("aria2 took the add as another torrent (%s), so HolyArrs removed it again", h)
		}
		if s.Status == "error" {
			if err := a.clearResult(ctx, gid); err != nil {
				return "", errors.Join(ErrUncertain, err)
			}
			if s.ErrorCode == "12" {
				return "", errors.Join(ErrUncertain, errors.New("aria2 already holds this torrent"))
			}
			return "", fmt.Errorf("aria2 refused the torrent: %s", redactURLs(cleanDetail(s.ErrorMessage)))
		}
		return hash, nil
	}
	return "", errors.Join(ErrUncertain, sendErr, errors.New("aria2 did not show the torrent yet"))
}

// Remove takes the torrent out of aria2. aria2 never deletes files, so when
// deleteFiles is set HolyArrs deletes them itself where it can see them.
func (a *Aria2) Remove(ctx context.Context, id string, deleteFiles bool) error {
	s, group, rows, err := a.find(ctx, id)
	if err != nil {
		return err
	}
	var dirs []string
	if a.Category != "" {
		if dirs, err = a.ourDirs(ctx); err != nil {
			return err
		}
		if !owns(s, dirs) {
			return errors.New("refusing to remove a torrent outside the configured category")
		}
	}
	var target filesTarget
	why := ""
	if deleteFiles {
		target, why = a.deleteTarget(ctx, s, rows, dirs)
	}
	for _, g := range group {
		if a.Category != "" && !owns(g, dirs) {
			continue
		}
		if err := a.forget(ctx, g); err != nil {
			return err
		}
	}
	after, err := a.all(ctx)
	if err != nil {
		return err
	}
	for _, g := range byHash(after)[strings.ToLower(s.InfoHash)] {
		if live(g) {
			return errors.New("aria2 still runs the torrent after the remove")
		}
	}
	if !deleteFiles {
		return nil
	}
	if why == "" {
		if why = target.remove(); why == "" {
			why = removeControlFile(target)
		}
	}
	if why != "" {
		logf("aria2: removed %s but kept its files: %s", strings.ToLower(s.InfoHash), why)
	}
	return nil
}

// forget stops a job and clears its result. aria2 deletes no files doing so.
func (a *Aria2) forget(ctx context.Context, s a2Status) error {
	if live(s) {
		err := a.call(ctx, "aria2.forceRemove", nil, s.GID)
		if err != nil && !errors.Is(err, ErrNotFound) && !faultSays(err, "Active Download not found") {
			return err
		}
		for i := 0; i < aria2Settle.tries; i++ {
			var st a2Status
			err := a.call(ctx, "aria2.tellStatus", &st, s.GID, []string{"status"})
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if !live(st) {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(aria2Settle.gap):
			}
		}
	}
	return a.clearResult(ctx, s.GID)
}

// clearResult drops a stopped job's result. A job already gone, or still
// stopping, is left to the caller's own check.
func (a *Aria2) clearResult(ctx context.Context, gid string) error {
	err := a.call(ctx, "aria2.removeDownloadResult", nil, gid)
	if err == nil || errors.Is(err, ErrNotFound) || faultSays(err, "Could not remove download result") {
		return nil
	}
	return err
}

// owns says whether a job sits in one of dirs, from ourDirs.
func owns(s a2Status, dirs []string) bool {
	return s.Dir != "" && slices.Contains(dirs, a2DirKey(s.Dir))
}
