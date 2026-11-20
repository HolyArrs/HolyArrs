package library

import (
	"context"
	"path"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// prefetcher is a metadata source that fetches many lookups in few requests
// (the metadata server's batch route) and answers them later from memory.
type prefetcher interface {
	Prefetch(ctx context.Context, reads []metadata.TMDBRead) int
}

// scanWindow is how many folders' lookups one batch carries.
const scanWindow = 40

// folderRead is the first lookup matchFolder makes for a folder.
func folderRead(folder string) metadata.TMDBRead {
	folder = matchingLookup(folder)
	if m := reTMDBTag.FindStringSubmatch(folder); m != nil {
		if n, err := strconv.ParseInt(m[1], 10, 64); err == nil && n > 0 {
			return metadata.MovieDetailRead(n)
		}
	}
	name, year := FolderName(strings.TrimSpace(reTMDBTag.ReplaceAllString(folder, "")))
	return metadata.MovieSearchRead(name, year)
}

// scanAhead fetches the lookups of a scan's folders one window ahead of
// the scan, so each folder's lookup is answered from memory.
type scanAhead struct {
	index map[string]int // full relative paths: basenames can repeat across groups
	ready []chan struct{}
	free  chan struct{}
	stop  chan struct{}
	at    int
}

// close stops fetching ahead (the scan ended).
func (a *scanAhead) close() {
	if a != nil {
		close(a.stop)
	}
}

// startAhead starts fetching folders' lookups; nil when md cannot batch.
func startAhead(ctx context.Context, md metadata.Provider, folders []string) *scanAhead {
	p, ok := md.(prefetcher)
	if !ok || len(folders) == 0 {
		return nil
	}
	a := &scanAhead{index: map[string]int{}, free: make(chan struct{}, 2), stop: make(chan struct{}), at: -1}
	for i, f := range folders {
		a.index[f] = i
	}
	for range (len(folders) + scanWindow - 1) / scanWindow {
		a.ready = append(a.ready, make(chan struct{}))
	}
	a.free <- struct{}{}
	go func() {
		for w := range a.ready {
			select {
			case <-a.free:
			case <-a.stop:
			case <-ctx.Done():
			}
			if ctx.Err() == nil && !a.stopped() {
				end := min(len(folders), (w+1)*scanWindow)
				reads := make([]metadata.TMDBRead, 0, end-w*scanWindow)
				for _, f := range folders[w*scanWindow : end] {
					reads = append(reads, folderRead(path.Base(f)))
				}
				p.Prefetch(ctx, reads)
			}
			close(a.ready[w])
		}
	}()
	return a
}

func (a *scanAhead) stopped() bool {
	select {
	case <-a.stop:
		return true
	default:
		return false
	}
}

// wait waits until folder's lookup was fetched (or tried); a folder not in
// the scan's list does not wait.
func (a *scanAhead) wait(ctx context.Context, folder string) {
	if a == nil {
		return
	}
	i, ok := a.index[folder]
	if !ok {
		return
	}
	w := i / scanWindow
	if w > a.at {
		a.at = w
		// Entering a window lets the next one be fetched meanwhile.
		select {
		case a.free <- struct{}{}:
		default:
		}
	}
	select {
	case <-a.ready[w]:
	case <-ctx.Done():
	}
}

// scanLookups lists the relative folder paths a scan will look up, in scan order: those
// it reads (at most limit), less those already in the Library. names are paths
// inside the Library folder; a lookup goes by the last part.
func scanLookups(names []string, limit int, known map[string]int64, fold bool) []string {
	var out []string
	for i, n := range names {
		if i >= limit {
			break
		}
		if _, ok := known[FolderKey(n, fold)]; !ok {
			out = append(out, n)
		}
	}
	return out
}
