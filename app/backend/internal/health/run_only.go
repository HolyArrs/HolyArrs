package health

import (
	"context"
	"errors"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrNoGroup is a re-run asked for a group of checks that does not exist.
var ErrNoGroup = errors.New("no such group of checks: send backups, library_folder, source or downloader")

// group is the checks one fix can change, and the kinds they store.
type group struct {
	run   func(*Engine) []func(context.Context) ([]Result, error)
	kinds []string
}

var groups = map[string]group{
	"backups": {func(e *Engine) []func(context.Context) ([]Result, error) {
		return []func(context.Context) ([]Result, error){e.backups}
	}, []string{"backups"}},
	"library_folder": {func(e *Engine) []func(context.Context) ([]Result, error) {
		return []func(context.Context) ([]Result, error){e.folders}
	}, []string{"library_folder", "disk_space"}},
	"source": {func(e *Engine) []func(context.Context) ([]Result, error) {
		return []func(context.Context) ([]Result, error){e.sources}
	}, []string{"source"}},
	"downloader": {func(e *Engine) []func(context.Context) ([]Result, error) {
		return []func(context.Context) ([]Result, error){e.downloaders, e.downloaderKinds, e.sameDownloaders, e.saveFolders}
	}, []string{"downloader", "no_downloader", "same_downloader", "save_folder"}},
}

// RunOnly runs one group of checks right after the fix for it, so its
// warning goes out now instead of at the next timed run. Rows of other
// groups stay as the last full run left them.
func (e *Engine) RunOnly(ctx context.Context, name string) ([]domain.HealthCheck, error) {
	g, ok := groups[name]
	if !ok {
		return nil, ErrNoGroup
	}
	e.run.Lock()
	defer e.run.Unlock()
	var rs []Result
	for _, f := range g.run(e) {
		got, err := f(ctx)
		if err != nil {
			return nil, err
		}
		rs = append(rs, got...)
	}
	kinds := map[string]bool{}
	for _, k := range g.kinds {
		kinds[k] = true
	}
	return e.keep(ctx, rs, kinds)
}
