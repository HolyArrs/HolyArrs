package downloads

import (
	"context"
	"errors"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// logf is the package log line, a variable so tests can read it.
var logf = log.Printf

// PauseItem stops one torrent. It stays open, so its files and place are kept.
func (r *RTorrent) PauseItem(ctx context.Context, id string) error {
	row, err := r.ownRow(ctx, id)
	if err != nil {
		return err
	}
	_, err = r.call(ctx, "d.stop", strings.ToUpper(row.hash))
	return rtNotFound(err)
}

// ResumeItem starts one torrent again.
func (r *RTorrent) ResumeItem(ctx context.Context, id string) error {
	row, err := r.ownRow(ctx, id)
	if err != nil {
		return err
	}
	method := "d.start"
	if row.open == 1 && row.state == 1 && row.active == 0 {
		method = "d.resume"
	}
	_, err = r.call(ctx, method, strings.ToUpper(row.hash))
	return rtNotFound(err)
}

// PauseAll stops every torrent with one of this Downloader's labels. Torrents of
// other apps are left alone.
func (r *RTorrent) PauseAll(ctx context.Context) error { return r.everyOwn(ctx, false) }

// ResumeAll starts every torrent with one of this Downloader's labels.
func (r *RTorrent) ResumeAll(ctx context.Context) error { return r.everyOwn(ctx, true) }

// MoveTop is not supported: rTorrent has no queue order.
func (r *RTorrent) MoveTop(context.Context, string) error { return ErrNotSupported }

// SetSpeedLimit caps rTorrent's total speeds in bytes per second; 0 removes a cap.
func (r *RTorrent) SetSpeedLimit(ctx context.Context, down, up int64) error {
	answers, err := r.multicall(ctx, [][]any{{"throttle.global_down.max_rate.set", "", max(0, down)}, {"throttle.global_up.max_rate.set", "", max(0, up)}})
	if err != nil {
		return err
	}
	return answerErrors(answers)
}

// Status reports rTorrent's speeds, caps and session totals.
func (r *RTorrent) Status(ctx context.Context) (ClientStatus, error) {
	answers, err := r.multicall(ctx, [][]any{
		{"throttle.global_down.rate", ""}, {"throttle.global_up.rate", ""},
		{"throttle.global_down.max_rate", ""}, {"throttle.global_up.max_rate", ""},
		{"throttle.global_down.total", ""}, {"throttle.global_up.total", ""},
	})
	if err != nil {
		return ClientStatus{}, err
	}
	if err := answerErrors(answers); err != nil {
		return ClientStatus{}, err
	}
	st := ClientStatus{Speed: asInt(answers[0]), UpSpeed: asInt(answers[1]), SpeedLimit: max(0, asInt(answers[2])),
		UpLimit: max(0, asInt(answers[3])), SessionDown: asInt(answers[4]), SessionUp: asInt(answers[5])}
	rows, err := r.rows(ctx)
	if err != nil {
		return ClientStatus{}, err
	}
	// Paused as a whole when there are torrents of ours and none of them runs.
	for _, row := range rows {
		if r.owns(row.label) {
			if row.state == 1 && row.active == 1 {
				st.Paused = false
				break
			}
			st.Paused = true
		}
	}
	return st, nil
}

func (r *RTorrent) everyOwn(ctx context.Context, start bool) error {
	rows, err := r.rows(ctx)
	if err != nil {
		return err
	}
	calls := [][]any{}
	for _, row := range rows {
		if !r.owns(row.label) {
			continue
		}
		switch {
		case !start:
			calls = append(calls, []any{"d.stop", strings.ToUpper(row.hash)})
		case row.open == 1 && row.state == 1 && row.active == 0:
			calls = append(calls, []any{"d.resume", strings.ToUpper(row.hash)})
		default:
			calls = append(calls, []any{"d.start", strings.ToUpper(row.hash)})
		}
	}
	if len(calls) == 0 {
		return nil
	}
	answers, err := r.multicall(ctx, calls)
	if err != nil {
		return err
	}
	return answerErrors(answers)
}

// ownRow reads a torrent and refuses one without this Downloader's labels.
func (r *RTorrent) ownRow(ctx context.Context, id string) (rtRow, error) {
	row, err := r.row(ctx, id)
	if err != nil {
		return rtRow{}, err
	}
	if r.Category != "" && !r.owns(row.label) {
		return rtRow{}, ErrNotFound
	}
	return row, nil
}

func (r *RTorrent) owns(label string) bool {
	return labelIs(label, r.Category) || slices.ContainsFunc(r.Others, func(c string) bool { return labelIs(label, c) })
}

func answerErrors(answers []any) error {
	var errs []error
	for _, a := range answers {
		if e, ok := a.(error); ok {
			errs = append(errs, rtNotFound(e))
		}
	}
	return errors.Join(errs...)
}

// filesTarget is a torrent's data that HolyArrs may delete: rel inside root.
type filesTarget struct{ root, rel string }

// deleteTarget decides whether HolyArrs may delete a torrent's files after
// rTorrent lets go of it. It only says yes for a file or folder named like the
// torrent, inside the save folder or a path pair, that no other torrent in
// rTorrent uses. Otherwise it says why the files stay.
func (r *RTorrent) deleteTarget(row rtRow, all []rtRow, keep []string) (filesTarget, string) {
	content := row.content()
	if content == "" {
		return filesTarget{}, "rTorrent did not say where they are"
	}
	if row.name == "" || row.name == "." || row.name == ".." || path.Base(content) != row.name {
		return filesTarget{}, content + " is not named like the torrent"
	}
	for _, o := range all {
		oc := o.content()
		if o.hash == row.hash || oc == "" {
			continue
		}
		if oc == content || strings.HasPrefix(oc, content+"/") || strings.HasPrefix(content, oc+"/") {
			return filesTarget{}, "another torrent in rTorrent uses " + content
		}
	}
	local := filepath.Clean(mapLocal(content, r.PathMap))
	if filepath.Base(local) != row.name {
		// A name holding this system's own separator would point elsewhere here.
		return filesTarget{}, local + " is not named like the torrent here"
	}
	for _, k := range keep {
		if k = strings.TrimSpace(k); k != "" && (underAny(local, []string{k}) || underAny(filepath.Clean(k), []string{local})) {
			return filesTarget{}, local + " is a Library folder or holds one"
		}
	}
	roots := []string{}
	if r.SaveRoot != "" {
		roots = append(roots, r.SaveRoot)
	}
	for _, m := range r.PathMap {
		if m.Ours != "" {
			roots = append(roots, m.Ours)
		}
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if local == root || !underAny(local, []string{root}) {
			continue
		}
		for _, c := range append([]string{r.Category}, r.Others...) {
			if c != "" && local == filepath.Join(root, c) {
				return filesTarget{}, local + " is the label's own folder"
			}
		}
		fi, err := os.Lstat(local)
		if err != nil {
			return filesTarget{}, "HolyArrs cannot see " + local + "; they stay where rTorrent put them (" + content + ")"
		}
		if !fi.Mode().IsRegular() && !fi.IsDir() {
			return filesTarget{}, local + " is a link or a special file"
		}
		rel, err := filepath.Rel(root, local)
		if err != nil {
			return filesTarget{}, err.Error()
		}
		return filesTarget{root: root, rel: rel}, ""
	}
	return filesTarget{}, "they are at " + content + " on the rTorrent machine, outside HolyArrs's save folder and path pairs; add a path pair to reach them, or delete them by hand"
}

// remove deletes the target without leaving its root. It says why on failure.
func (t filesTarget) remove() string {
	if t.root == "" || t.rel == "" || t.rel == "." {
		return "nothing to delete"
	}
	root, err := os.OpenRoot(t.root)
	if err != nil {
		return err.Error()
	}
	defer root.Close()
	if err := root.RemoveAll(t.rel); err != nil {
		return err.Error()
	}
	return ""
}
