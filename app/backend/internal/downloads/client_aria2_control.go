package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PauseItem stops one torrent at once. aria2 keeps its place and files.
func (a *Aria2) PauseItem(ctx context.Context, id string) error {
	_, group, err := a.ownJob(ctx, id)
	if err != nil {
		return err
	}
	for _, s := range group {
		if !live(s) {
			continue
		}
		if err := a.call(ctx, "aria2.forcePause", nil, s.GID); err != nil && !faultSays(err, "cannot be paused now") {
			return err
		}
	}
	return nil
}

// ResumeItem starts a paused torrent again. aria2 cannot restart a stopped
// one: that takes a new add.
func (a *Aria2) ResumeItem(ctx context.Context, id string) error {
	s, group, err := a.ownJob(ctx, id)
	if err != nil {
		return err
	}
	if !live(s) {
		return errors.New("aria2 cannot restart a stopped download; grab it again")
	}
	for _, g := range group {
		if !live(g) {
			continue
		}
		if err := a.call(ctx, "aria2.unpause", nil, g.GID); err != nil && !faultSays(err, "cannot be unpaused now") {
			return err
		}
	}
	return nil
}

// PauseAll stops every torrent in this Downloader's folders. Other jobs in
// aria2 are left alone.
func (a *Aria2) PauseAll(ctx context.Context) error {
	return a.everyOwn(ctx, "aria2.forcePause", "cannot be paused now", func(s a2Status) bool { return s.Status == "active" || s.Status == "waiting" })
}

// ResumeAll starts every paused torrent in this Downloader's folders.
func (a *Aria2) ResumeAll(ctx context.Context) error {
	return a.everyOwn(ctx, "aria2.unpause", "cannot be unpaused now", func(s a2Status) bool { return s.Status == "paused" })
}

// MoveTop is not supported.
func (a *Aria2) MoveTop(context.Context, string) error { return ErrNotSupported }

// SetSpeedLimit caps aria2's total speeds in bytes per second; 0 removes a
// cap. aria2 does not keep them across a restart.
func (a *Aria2) SetSpeedLimit(ctx context.Context, down, up int64) error {
	want := map[string]string{"max-overall-download-limit": strconv.FormatInt(max(0, down), 10), "max-overall-upload-limit": strconv.FormatInt(max(0, up), 10)}
	if err := a.call(ctx, "aria2.changeGlobalOption", nil, want); err != nil {
		return err
	}
	var got map[string]string
	if err := a.call(ctx, "aria2.getGlobalOption", &got); err != nil {
		return err
	}
	for k, v := range want {
		if a2Num(got[k]) != a2Num(v) {
			return errors.New("aria2 did not keep the speed caps")
		}
	}
	return nil
}

// Status reports aria2's speeds and caps.
func (a *Aria2) Status(ctx context.Context) (ClientStatus, error) {
	answers, errs, err := a.multicall(ctx, []a2Call{{"aria2.getGlobalStat", nil}, {"aria2.getGlobalOption", nil}})
	if err != nil {
		return ClientStatus{}, err
	}
	if err := errors.Join(errs...); err != nil {
		return ClientStatus{}, err
	}
	var stat, opts map[string]string
	if json.Unmarshal(answers[0], &stat) != nil || json.Unmarshal(answers[1], &opts) != nil {
		return ClientStatus{}, errNotAria2
	}
	st := ClientStatus{Speed: max(0, a2Num(stat["downloadSpeed"])), UpSpeed: max(0, a2Num(stat["uploadSpeed"])),
		SpeedLimit: max(0, a2Num(opts["max-overall-download-limit"])), UpLimit: max(0, a2Num(opts["max-overall-upload-limit"]))}
	rows, err := a.all(ctx)
	if err != nil {
		return ClientStatus{}, err
	}
	dirs, err := a.ourDirs(ctx)
	if err != nil {
		return ClientStatus{}, err
	}
	// Paused as a whole when there are torrents of ours and none of them runs.
	for _, group := range byHash(rows) {
		s, ok := pick(group)
		if !ok || !owns(s, dirs) {
			continue
		}
		if s.Status == "active" {
			st.Paused = false
			break
		}
		st.Paused = true
	}
	return st, nil
}

func (a *Aria2) everyOwn(ctx context.Context, method, fine string, want func(a2Status) bool) error {
	rows, err := a.all(ctx)
	if err != nil {
		return err
	}
	dirs, err := a.ourDirs(ctx)
	if err != nil {
		return err
	}
	calls := []a2Call{}
	for _, group := range byHash(rows) {
		for _, s := range group {
			if owns(s, dirs) && want(s) {
				calls = append(calls, a2Call{method, []any{s.GID}})
			}
		}
	}
	if len(calls) == 0 {
		return nil
	}
	_, errs, err := a.multicall(ctx, calls)
	if err != nil {
		return err
	}
	var bad []error
	for _, e := range errs {
		if e != nil && !errors.Is(e, ErrNotFound) && !faultSays(e, fine) {
			bad = append(bad, e)
		}
	}
	return errors.Join(bad...)
}

// ownJob reads a torrent and refuses one outside this Downloader's folders.
func (a *Aria2) ownJob(ctx context.Context, id string) (a2Status, []a2Status, error) {
	s, group, _, err := a.find(ctx, id)
	if err != nil {
		return a2Status{}, nil, err
	}
	var dirs []string
	if a.Category != "" {
		if dirs, err = a.ourDirs(ctx); err != nil {
			return a2Status{}, nil, err
		}
		if !owns(s, dirs) {
			return a2Status{}, nil, ErrNotFound
		}
	}
	mine := []a2Status{}
	for _, g := range group {
		if a.Category == "" || owns(g, dirs) {
			mine = append(mine, g)
		}
	}
	return s, mine, nil
}

// deleteTarget decides, before aria2 lets go of a job, whether HolyArrs may
// delete its files afterwards. Only what aria2 reported for this job counts,
// and only when aria2 wrote it.
func (a *Aria2) deleteTarget(ctx context.Context, s a2Status, rows []a2Status, dirs []string) (filesTarget, string) {
	if a.Keep == nil {
		return filesTarget{}, "HolyArrs does not know its Library folders here"
	}
	keep, err := a.Keep(ctx)
	if err != nil {
		return filesTarget{}, "HolyArrs could not read its Library folders"
	}
	c := content(s)
	if c == "" {
		return filesTarget{}, "aria2 did not say where they are"
	}
	if a.Category != "" && !owns(s, dirs) {
		return filesTarget{}, c + " is not in the category's folder"
	}
	sep := a2Sep(s.Dir)
	name := s.Bittorrent.Info.Name
	if s.Bittorrent.Mode == "multi" {
		for _, f := range s.Files {
			if f.Selected != "false" && !strings.HasPrefix(f.Path, c+sep) {
				return filesTarget{}, "aria2 lists a file outside " + c
			}
		}
	} else if len(s.Files) == 0 || s.Files[0].Path != c {
		return filesTarget{}, "aria2 did not say where they are"
	}
	if a2Base(c) != name {
		return filesTarget{}, c + " is not named like the torrent; aria2 may have renamed it beside another file"
	}
	if s.ErrorCode == "13" {
		return filesTarget{}, "the files were there before aria2 took the torrent"
	}
	if why := a.adopted(ctx, s.GID); why != "" {
		return filesTarget{}, why
	}
	local := filepath.Clean(mapLocal(c, a.PathMap))
	if a2Num(s.CompletedLength) == 0 {
		if fi, err := os.Lstat(local + ".aria2"); err != nil || !fi.Mode().IsRegular() {
			return filesTarget{}, "aria2 wrote nothing there yet"
		}
	}
	hash := strings.ToLower(s.InfoHash)
	others := []string{}
	for _, o := range rows {
		if strings.ToLower(o.InfoHash) != hash {
			others = append(others, content(o))
		}
	}
	return safeDeleteTarget(deleteAsk{client: "aria2", content: c, name: name, others: others, pathMap: a.PathMap,
		saveRoot: a.SaveRoot, cats: append([]string{a.Category}, a.Others...)}, keep)
}

// adopted says why a job's files may have been on disk before aria2 took it,
// or "" when aria2 had to write them itself. Without check-integrity and
// bt-seed-unverified, aria2 refuses a torrent whose file is already there
// without its control file (error 13), renames beside it, or starts it again
// from nothing; with either one it takes the files as they are. A control
// file proves nothing then, since aria2 writes one while it carries on with a
// partial file it took over, and drops it once a download is done. So the
// job's own options decide, and the files stay when aria2 does not say.
func (a *Aria2) adopted(ctx context.Context, gid string) string {
	var opts map[string]string
	if err := a.call(ctx, "aria2.getOption", &opts, gid); err != nil {
		return "aria2 did not say how it took the torrent, so the files may have been there before"
	}
	for _, k := range []string{"check-integrity", "bt-seed-unverified"} {
		if opts[k] != "false" {
			return "aria2 takes files already on disk for this torrent (" + k + "), so they may not be HolyArrs's download"
		}
	}
	return ""
}

// removeControlFile deletes aria2's control file beside a deleted target,
// when it is a plain file.
func removeControlFile(t filesTarget) string {
	rel := t.rel + ".aria2"
	if fi, err := os.Lstat(filepath.Join(t.root, rel)); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	root, err := os.OpenRoot(t.root)
	if err != nil {
		return err.Error()
	}
	defer root.Close()
	if err := root.Remove(rel); err != nil {
		return err.Error()
	}
	return ""
}
