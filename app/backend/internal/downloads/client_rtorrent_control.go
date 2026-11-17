package downloads

import (
	"context"
	"errors"
	"log"
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
	rows, err := r.rows(ctx, "main")
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
	rows, err := r.rows(ctx, "main")
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

// deleteTarget decides whether HolyArrs may delete a torrent's files after
// rTorrent lets go of it, through the shared rules in safeDeleteTarget.
func (r *RTorrent) deleteTarget(row rtRow, all []rtRow, keep []string) (filesTarget, string) {
	others := []string{}
	for _, o := range all {
		if o.hash != row.hash {
			others = append(others, o.content())
		}
	}
	return safeDeleteTarget(deleteAsk{client: "rTorrent", content: row.content(), name: row.name, others: others,
		pathMap: r.PathMap, saveRoot: r.SaveRoot, cats: append([]string{r.Category}, r.Others...)}, keep)
}
