package downloads

import (
	"context"
	"math"
	"path"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// aria2ListMax is how many waiting or stopped jobs one listing asks for.
const aria2ListMax = 100000

// a2Keys are the job fields HolyArrs reads.
var a2Keys = []string{"gid", "status", "infoHash", "dir", "totalLength", "completedLength", "uploadLength", "downloadSpeed", "uploadSpeed",
	"connections", "numSeeders", "seeder", "errorCode", "errorMessage", "followedBy", "following", "files", "bittorrent"}

// a2Status is one aria2 job. Every number comes as a string.
type a2Status struct {
	GID             string      `json:"gid"`
	Status          string      `json:"status"`
	InfoHash        string      `json:"infoHash"`
	Dir             string      `json:"dir"`
	TotalLength     string      `json:"totalLength"`
	CompletedLength string      `json:"completedLength"`
	UploadLength    string      `json:"uploadLength"`
	DownloadSpeed   string      `json:"downloadSpeed"`
	UploadSpeed     string      `json:"uploadSpeed"`
	Connections     string      `json:"connections"`
	NumSeeders      string      `json:"numSeeders"`
	Seeder          string      `json:"seeder"`
	ErrorCode       string      `json:"errorCode"`
	ErrorMessage    string      `json:"errorMessage"`
	FollowedBy      []string    `json:"followedBy"`
	Following       string      `json:"following"`
	Files           []a2FileRow `json:"files"`
	Bittorrent      struct {
		Info struct {
			Name string `json:"name"`
		} `json:"info"`
		Mode string `json:"mode"`
	} `json:"bittorrent"`
}

// a2FileRow is one file of a job, with its full path on the aria2 machine.
type a2FileRow struct {
	Path            string `json:"path"`
	Length          string `json:"length"`
	CompletedLength string `json:"completedLength"`
	Selected        string `json:"selected"`
}

const a2MetaPrefix = "[METADATA]"

// all lists every job aria2 holds: running, waiting or paused, and stopped.
func (a *Aria2) all(ctx context.Context) ([]a2Status, error) {
	answers, errs, err := a.multicall(ctx, []a2Call{
		{"aria2.tellActive", []any{a2Keys}},
		{"aria2.tellWaiting", []any{0, aria2ListMax, a2Keys}},
		{"aria2.tellStopped", []any{0, aria2ListMax, a2Keys}},
	})
	if err != nil {
		return nil, err
	}
	out := []a2Status{}
	for i, raw := range answers {
		if errs[i] != nil {
			return nil, errs[i]
		}
		var rows []a2Status
		if err := jsonInto(raw, &rows); err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// find returns the job HolyArrs reads for a hash, every job of that hash, and
// every job aria2 holds.
func (a *Aria2) find(ctx context.Context, id string) (a2Status, []a2Status, []a2Status, error) {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return a2Status{}, nil, nil, errBadHash
	}
	rows, err := a.all(ctx)
	if err != nil {
		return a2Status{}, nil, nil, err
	}
	group := byHash(rows)[id]
	p, ok := pick(group)
	if !ok {
		return a2Status{}, nil, nil, ErrNotFound
	}
	return p, group, rows, nil
}

// byHash groups the torrent jobs by lower-case info hash. Jobs without one
// (direct downloads) are left out.
func byHash(rows []a2Status) map[string][]a2Status {
	out := map[string][]a2Status{}
	for _, s := range rows {
		if h := strings.ToLower(s.InfoHash); releasename.ValidInfoHash(h) {
			out[h] = append(out[h], s)
		}
	}
	return out
}

// pick chooses the one job that stands for a hash. A magnet first runs as a
// metadata job that hands over to a child with the same hash; a second add
// of a live hash leaves a job in error 12 beside the real one.
func pick(group []a2Status) (a2Status, bool) {
	kept := []a2Status{}
	for _, s := range group {
		if s.Status == "removed" || s.Status == "complete" && len(s.FollowedBy) > 0 {
			continue
		}
		kept = append(kept, s)
	}
	fresh := []a2Status{}
	for _, s := range kept {
		if !(s.Status == "error" && s.ErrorCode == "12") {
			fresh = append(fresh, s)
		}
	}
	if len(fresh) > 0 {
		kept = fresh
	}
	rank := func(s a2Status) int {
		switch s.Status {
		case "active":
			return 4
		case "waiting", "paused":
			return 3
		case "complete":
			return 2
		}
		return 1
	}
	var best a2Status
	found := false
	for _, s := range kept {
		if !found || rank(s) > rank(best) || rank(s) == rank(best) && isMeta(best) && !isMeta(s) {
			best, found = s, true
		}
	}
	return best, found
}

func isMeta(s a2Status) bool {
	return len(s.Files) > 0 && strings.HasPrefix(s.Files[0].Path, a2MetaPrefix) || s.Bittorrent.Info.Name == ""
}

func live(s a2Status) bool {
	return s.Status == "active" || s.Status == "waiting" || s.Status == "paused"
}

// a2Sep is the separator a folder of the aria2 machine uses.
func a2Sep(dir string) string {
	if strings.Contains(dir, `\`) && !strings.Contains(dir, "/") {
		return `\`
	}
	return "/"
}

func a2Join(dir, name string) string {
	return strings.TrimRight(dir, `/\`) + a2Sep(dir) + name
}

// a2Folder is the last element of a folder: the category of a job.
func a2Folder(dir string) string {
	t := strings.TrimRight(dir, `/\`)
	return t[strings.LastIndexAny(t, `/\`)+1:]
}

func a2Base(p string) string { return a2Folder(p) }

// content is where a job's data is on the aria2 machine: the file of a
// single-file torrent, or the torrent's folder. It is empty unless it is a
// full path inside the job's folder.
func content(s a2Status) string {
	dir := s.Dir
	if !absPath(dir) {
		return ""
	}
	sep := a2Sep(dir)
	p := ""
	if s.Bittorrent.Mode == "multi" {
		n := s.Bittorrent.Info.Name
		if n == "" || n == "." || n == ".." || strings.ContainsAny(n, `/\`) {
			return ""
		}
		p = a2Join(dir, n)
	} else if len(s.Files) > 0 {
		p = s.Files[0].Path
	}
	if !absPath(p) {
		return ""
	}
	if sep == "/" {
		p, dir = path.Clean(p), path.Clean(dir)
	}
	dir = strings.TrimRight(dir, sep)
	if p == dir || !strings.HasPrefix(p, dir+sep) {
		return ""
	}
	return p
}

// a2State maps an aria2 job onto ours. An errored job stays Paused with its
// message: aria2 tries it again after a restart, so it is never failed here.
func a2State(s a2Status) (string, string) {
	switch {
	case live(s) && isMeta(s):
		return StateDownloading, "metadata"
	case s.Status == "active" && (s.Seeder == "true" || a2Num(s.TotalLength) > 0 && a2Num(s.CompletedLength) == a2Num(s.TotalLength)):
		return StateFinished, "seeding"
	case s.Status == "active":
		return StateDownloading, "downloading"
	case s.Status == "waiting":
		return StateQueued, "waiting"
	case s.Status == "paused":
		return StatePaused, "paused"
	case s.Status == "complete":
		return StateFinished, "complete"
	case s.Status == "error" && s.ErrorCode == "12":
		return StatePaused, "error: the same torrent is already in aria2"
	case s.Status == "error":
		msg := redactURLs(cleanDetail(s.ErrorMessage))
		if msg == "" {
			msg = "aria2 error " + s.ErrorCode
		}
		return StatePaused, "error: " + msg
	}
	return StatePaused, s.Status
}

func (a *Aria2) job(s a2Status) Job {
	total, done := max(0, a2Num(s.TotalLength)), max(0, a2Num(s.CompletedLength))
	down, up := max(0, a2Num(s.DownloadSpeed)), max(0, a2Num(s.UploadLength))
	state, detail := a2State(s)
	name := s.Bittorrent.Info.Name
	if name == "" && len(s.Files) > 0 && strings.HasPrefix(s.Files[0].Path, a2MetaPrefix) {
		name = strings.TrimPrefix(s.Files[0].Path, a2MetaPrefix)
	}
	if name == "" {
		name = strings.ToLower(s.InfoHash)
	}
	progress := float64(0)
	if total > 0 {
		progress = max(0, min(1, float64(done)/float64(total)))
	}
	if s.Status == "complete" {
		progress = 1
	}
	eta := int64(0)
	if down > 0 && total > done {
		eta = (total - done) / down
	}
	ratio := float64(0)
	if done > 0 {
		ratio = float64(up) / float64(done)
	}
	seeds := min(max(0, a2Num(s.NumSeeders)), math.MaxInt32)
	return Job{ID: strings.ToLower(s.InfoHash), Name: name, Category: a2Folder(s.Dir), SavePath: s.Dir, ContentPath: content(s),
		Size: total, Progress: progress, Speed: down, UpSpeed: max(0, a2Num(s.UploadSpeed)), Seeds: int(seeds),
		Peers: int(min(max(0, a2Num(s.Connections)-seeds), math.MaxInt32)), ETA: eta, State: state, Detail: detail, Ratio: ratio, Uploaded: up}
}

// files are the selected files of a job, named from its folder.
func (a *Aria2) files(s a2Status) []File {
	sep := a2Sep(s.Dir)
	prefix := strings.TrimRight(s.Dir, `/\`) + sep
	out := []File{}
	for _, f := range s.Files {
		if f.Selected == "false" || !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		name := safeRelName(strings.TrimPrefix(f.Path, prefix))
		if name == "" || name == "." {
			continue
		}
		progress := float64(0)
		if n := a2Num(f.Length); n > 0 {
			progress = max(0, min(1, float64(a2Num(f.CompletedLength))/float64(n)))
		}
		out = append(out, File{Name: name, Size: max(0, a2Num(f.Length)), Progress: progress})
	}
	return out
}
