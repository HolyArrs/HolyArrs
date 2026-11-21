package youtube

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// forgetWait is how long ForgetPerson waits for a person's running jobs to
// stop before it removes their folder anyway; forgetPoll is how often it
// looks.
var (
	forgetWait = 30 * time.Second
	forgetPoll = 50 * time.Millisecond
)

// ForgetPerson removes everything the YouTube library keeps for a deleted
// person: their waiting and running jobs are stopped, and once they have
// stopped (or forgetWait has passed) their follow and video rows go, and
// so does their "<id>-<name>" folder in the YouTube folder. Waiting first
// keeps a download that was mid-way from making the folder again after it
// went. Only entries directly inside the YouTube folder are removed; a
// link there is removed as a link, never followed. It carries on past a
// failed step and returns every failure.
func (s *Service) ForgetPerson(ctx context.Context, userID int64) error {
	var errs []error
	if err := s.stopPersonJobs(ctx, userID); err != nil {
		errs = append(errs, err)
	}
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error { return DeletePersonRows(tx, userID) }); err != nil {
		errs = append(errs, fmt.Errorf("youtube: rows of user %d: %w", userID, err))
	}
	if err := removePersonFolder(LoadSettings(ctx, s.Store.Read()).Folder, userID); err != nil {
		errs = append(errs, fmt.Errorf("youtube: folder of user %d: %w", userID, err))
	}
	return errors.Join(errs...)
}

// stopPersonJobs cancels every queued and running job of a person and
// waits until none is left, up to forgetWait. It cancels again on each
// look, so a job a stopping check queued at the last moment goes too.
func (s *Service) stopPersonJobs(ctx context.Context, userID int64) error {
	prefix := fmt.Sprintf("youtube:%d:", userID)
	deadline := time.Now().Add(forgetWait)
	for {
		ids, err := jobs.LiveIDsWithKeyPrefix(ctx, s.Store.Read(), prefix)
		if err != nil {
			return fmt.Errorf("youtube: jobs of user %d: %w", userID, err)
		}
		if len(ids) == 0 {
			return nil
		}
		var cancelErr error
		for _, id := range ids {
			if err := s.Queue.Cancel(ctx, id); err != nil {
				cancelErr = fmt.Errorf("youtube: cancel job %d: %w", id, err)
			}
		}
		if time.Now().After(deadline) {
			return errors.Join(fmt.Errorf("youtube: %d jobs of user %d still running", len(ids), userID), cancelErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(forgetPoll):
		}
	}
}

// removePersonFolder removes every "<id>-" folder or link directly inside
// root. Nothing happens when root is not set or not there.
func removePersonFolder(root string, userID int64) error {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("%d-", userID)
	var errs []error
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) || !(e.IsDir() || e.Type()&os.ModeSymlink != 0) {
			continue
		}
		// RemoveAll unlinks a link without following it, and never
		// follows links found inside the folder.
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			errs = append(errs, withoutPath(err))
		}
	}
	return errors.Join(errs...)
}

// withoutPath drops the file path from err: paths under a person's folder
// carry video titles, which never go in a log line.
func withoutPath(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s: %w", pe.Op, pe.Err)
	}
	return err
}
