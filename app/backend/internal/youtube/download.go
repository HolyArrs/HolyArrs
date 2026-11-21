package youtube

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// Download timings and limits.
const (
	// downloadAttempts is how many times a video is tried before it fails.
	downloadAttempts = 3
	// liveEvery spaces out progress events and job progress writes.
	liveEvery = 2 * time.Second
	// stateWriteTime bounds a state write made after the job's own context
	// ended, so a cancel or stop still settles the row.
	stateWriteTime = 10 * time.Second
	// downloadRounds is how many times one job starts a video over because
	// it was removed and added again while the job ran.
	downloadRounds = 3
)

// LiveVideo is the live event about one of a person's videos, sent only to
// that person.
type LiveVideo struct {
	ID      int64  `json:"id"`
	State   string `json:"state"`
	Percent int    `json:"percent"`
	Error   string `json:"error,omitempty"`
}

// finalError ends a job without the queue trying it again.
type finalError struct{ error }

func (finalError) Final() bool     { return true }
func (e finalError) Unwrap() error { return e.error }

// errRowGone means a video was removed or unfollowed while its job ran.
var errRowGone = errors.New("video gone while downloading")

// HandleDownload runs a youtube_download job, params {"row_id"}. It skips a
// video that is gone or no longer queued, waits (leaving it queued) while
// the disk is low or the video has not aired, and otherwise downloads it to
// "<folder>/<id>-<name>/<follow or channel>/<base>.mp4" with its .jpg. A
// failed attempt puts the video back in the queue; the third marks it
// failed. yt-dlp's error goes on the video row only: job errors and results
// carry row ids, never titles or YouTube ids.
//
// A video removed and added again while its job ran is queued with no job
// of its own, since the new job was the same as this one. So when the
// video went from under the job and is queued again, the job starts over.
func (s *Service) HandleDownload(ctx context.Context, r *jobs.Run) error {
	id, ok := paramID(r.Job.Params, "row_id")
	if !ok {
		return finalError{errors.New("the job has no video row id")}
	}
	// A job queued behind another job for the same video only takes a video
	// that job left queued.
	if after, _ := r.Job.Params["after"].(bool); after {
		if v, err := VideoByID(ctx, s.Store.Read(), id); errors.Is(err, sql.ErrNoRows) || (err == nil && v.State != StateQueued) {
			r.SetResult(map[string]any{"skipped": "settled"})
			return nil
		} else if err != nil {
			return err
		}
	}
	for round := 1; ; round++ {
		if round > 1 {
			r.SetResult(map[string]any{})
		}
		err := s.download(ctx, r, id)
		if !errors.Is(err, errRowGone) {
			return err
		}
		r.SetResult(map[string]any{"skipped": "gone"})
		if round >= downloadRounds || ctx.Err() != nil {
			return nil
		}
		if v, err := VideoByID(ctx, s.Store.Read(), id); err != nil || v.State != StateQueued {
			return nil
		}
	}
}

// download is one round of a youtube_download job. errRowGone when the
// video was removed, unfollowed or put back in the queue under it; what
// that round downloaded is deleted by then.
func (s *Service) download(ctx context.Context, r *jobs.Run, id int64) error {
	v, err := VideoByID(ctx, s.Store.Read(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return errRowGone
	}
	if err != nil {
		return err
	}
	// A downloading row is one an earlier run of this job left when the app
	// stopped: only one live job per row exists.
	if v.State != StateQueued && v.State != StateDownloading {
		r.SetResult(map[string]any{"skipped": v.State})
		return nil
	}
	if ok, err := s.mayUse()(ctx, v.UserID); err != nil {
		return err
	} else if !ok {
		r.SetResult(map[string]any{"skipped": "no_access"})
		return nil
	}
	run := s.runner()
	if len(run.Tools.Missing()) > 0 {
		return finalError{ErrNeedsTools}
	}
	set := LoadSettings(ctx, s.Store.Read())
	if set.Folder == "" {
		r.SetResult(map[string]any{"paused": "no_folder"})
		return nil
	}
	if s.FreeSpace != nil {
		if free := s.FreeSpace(set.Folder); free >= 0 && free < set.floorBytes() {
			r.SetResult(map[string]any{"paused": "low_space"})
			return nil
		}
	}

	info, err := run.Info(ctx, v.VideoID)
	if err != nil {
		return s.attemptFailed(ctx, r, v, err)
	}
	if info.LiveStatus == "is_live" || info.LiveStatus == "is_upcoming" || info.LiveStatus == "post_live" {
		r.SetResult(map[string]any{"paused": "not_aired"})
		return nil
	}
	base, err := s.videoBase(ctx, set.Folder, v, info)
	if err != nil {
		return err
	}
	// Only a row still queued or downloading starts: the person may have
	// removed the video while yt-dlp looked it up.
	if err := s.setState(ctx, v, StateDownloading, "", 0); err != nil {
		pruneEmptyDir(set.Folder, base)
		return err
	}
	var last time.Time
	file, err := run.Download(ctx, v.VideoID, v.MaxHeight, base, func(p Progress) {
		if p.Total <= 0 || time.Since(last) < liveEvery {
			return
		}
		last = time.Now()
		pct := int(min(max(p.Done*100/p.Total, 0), 100))
		_ = r.Progress(ctx, float64(pct)/100, fmt.Sprintf("%d%%", pct))
		s.publish(v.UserID, LiveVideo{ID: v.ID, State: StateDownloading, Percent: pct})
	})
	if err != nil {
		pruneEmptyDir(set.Folder, base)
		return s.attemptFailed(ctx, r, v, err)
	}
	if err := s.finish(ctx, set.Folder, v, file, info.Meta); errors.Is(err, errRowGone) {
		return err
	} else if err != nil {
		return s.attemptFailed(ctx, r, v, err)
	}
	return s.keepLast(ctx, set.Folder, v.ID)
}

// videoBase makes the folders for a video and returns its path without
// extension. A followed video goes in a folder named after the follow that
// took it first: the playlist title, or the channel name as it was when
// followed, so a renamed channel keeps one folder. Any other video goes in
// one named after its channel.
func (s *Service) videoBase(ctx context.Context, root string, v Video, info Info) (string, error) {
	p, err := s.Person(ctx, v.UserID)
	if err != nil {
		return "", err
	}
	folder := info.ChannelName
	follows, err := FollowsHolding(ctx, s.Store.Read(), v.UserID, v.ID)
	if err != nil {
		return "", err
	}
	if len(follows) > 0 && follows[0].Title != "" {
		folder = follows[0].Title
	}
	user, err := UserDir(root, v.UserID, p.Name)
	if err != nil {
		return "", pathFree(err)
	}
	dir := filepath.Join(user, SafeName(folder, NameBytes))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", pathFree(err)
	}
	return filepath.Join(dir, VideoBase(info.UploadedAt, info.Title, v.VideoID)), nil
}

// finish records a downloaded file. When the video was removed, unfollowed
// or queued again meanwhile, or the write does not go through (the job was
// cancelled just then), the new file and thumbnail are deleted.
func (s *Service) finish(ctx context.Context, root string, v Video, file string, m Meta) error {
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) {
		os.Remove(file)
		os.Remove(thumbOf(file))
		return errors.New("yt-dlp wrote the video outside the YouTube folder")
	}
	st, err := os.Stat(file)
	if err != nil {
		return pathFree(err)
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		now, err := VideoByID(ctx, tx, v.ID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && now.State != StateDownloading) {
			return errRowGone
		}
		if err != nil {
			return err
		}
		return FinishVideo(tx, v.ID, filepath.ToSlash(rel), st.Size(), m)
	})
	if errors.Is(err, errRowGone) {
		dropDownload(root, file)
		return err
	}
	if err != nil {
		s.dropUnlessKept(root, v.ID, file)
		return err
	}
	s.publish(v.UserID, LiveVideo{ID: v.ID, State: StateDone, Percent: 100})
	return nil
}

// dropUnlessKept deletes a downloaded file whose row was not written,
// unless the row says downloaded after all: a write can land though its
// caller heard otherwise. It runs on its own context, since the job's may
// be the one that ended. When the row cannot be read the file stays.
func (s *Service) dropUnlessKept(root string, rowID int64, file string) {
	ctx, cancel := context.WithTimeout(context.Background(), stateWriteTime)
	defer cancel()
	now, err := VideoByID(ctx, s.Store.Read(), rowID)
	if (err != nil && !errors.Is(err, sql.ErrNoRows)) || (err == nil && now.State == StateDone) {
		return
	}
	dropDownload(root, file)
}

// keepLast lets every follow that holds a video and keeps only its newest
// go of its downloaded videos beyond those. A video that is manual or that
// another follow holds keeps its files; any other is deleted, files first,
// and marked removed so no check takes it again.
func (s *Service) keepLast(ctx context.Context, root string, rowID int64) error {
	v, err := VideoByID(ctx, s.Store.Read(), rowID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	follows, err := FollowsHolding(ctx, s.Store.Read(), v.UserID, v.ID)
	if err != nil {
		return err
	}
	for _, f := range follows {
		if f.KeepLast == nil || f.Mirror {
			continue
		}
		old, err := KeepLastExcess(ctx, s.Store.Read(), v.UserID, f.ID, *f.KeepLast)
		if err != nil {
			return err
		}
		for _, o := range old {
			// The follow is read again each time: keep-last switched off
			// meanwhile stops it.
			if stopped, err := s.letGo(ctx, root, o, f.ID, false, false); err != nil {
				return err
			} else if stopped {
				break
			}
		}
	}
	return nil
}

// attemptFailed settles a video whose job did not finish. A cancelled or
// stopping job puts it back in the queue for the next run. Otherwise
// yt-dlp's error stays on the video and its owner is told: it goes back in
// the queue for the job's next attempt, or fails on the last one. The job's
// own error names the row only. A video removed or unfollowed meanwhile
// keeps that state: errRowGone.
func (s *Service) attemptFailed(ctx context.Context, r *jobs.Run, v Video, cause error) error {
	// The write runs on its own context: the job's may be the one that
	// ended, and a cancelled job is never tried again.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stateWriteTime)
	defer cancel()
	if ctx.Err() != nil {
		_ = s.setState(wctx, v, StateQueued, "", 0)
		return ctx.Err()
	}
	final := r.Job.Attempt >= downloadAttempts
	state := StateQueued
	if final {
		state = StateFailed
	}
	if err := s.setState(wctx, v, state, cause.Error(), 0); err != nil {
		return err
	}
	err := fmt.Errorf("youtube video %d: attempt %d of %d failed; the reason is on the video", v.ID, r.Job.Attempt, downloadAttempts)
	if final {
		return finalError{err}
	}
	return err
}

// dropDownload deletes a downloaded file and its thumbnail, if any, and
// the folder made for them when that leaves it empty.
func dropDownload(root, file string) {
	if file != "" {
		os.Remove(file)
		os.Remove(thumbOf(file))
		pruneEmptyDir(root, file)
	}
}

// setState moves a video its job holds, queued or downloading, to state and
// tells its owner. errRowGone when the row is gone or in another state: the
// person removed it or unfollowed meanwhile, and that stands.
func (s *Service) setState(ctx context.Context, v Video, state, errText string, pct int) error {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error { return MoveJobVideo(tx, v.ID, state, errText) })
	if errors.Is(err, sql.ErrNoRows) {
		return errRowGone
	}
	if err != nil {
		return err
	}
	s.publish(v.UserID, LiveVideo{ID: v.ID, State: state, Percent: pct, Error: errText})
	return nil
}

// publish sends a video event to its owner only.
func (s *Service) publish(userID int64, ev LiveVideo) {
	if s.Live != nil {
		s.Live.PublishOwn(LiveTopic, ev, userID)
	}
}

// pathFree drops the path from a file error: video paths hold titles.
func pathFree(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s in the YouTube folder: %w", pe.Op, pe.Err)
	}
	return err
}
