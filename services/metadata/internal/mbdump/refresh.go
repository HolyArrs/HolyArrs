package mbdump

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Copies live under one root: root/<dump date>/mb.sqlite, a build in
// progress in root/<dump date>.building, the copy in use named by the
// pointer file root/CURRENT and copies no longer in use listed with the
// time they were swapped out in root/RETIRED.
const (
	pointerFile = "CURRENT"
	retiredFile = "RETIRED"
)

// Index answers the newest dump date.
type Index interface {
	Latest(ctx context.Context) (string, error)
}

// HTTPIndex reads the LATEST file of a MusicBrainz dump directory.
type HTTPIndex struct {
	Base   string // e.g. https://data.metabrainz.org/pub/musicbrainz/data/fullexport
	Client *http.Client
}

// Latest answers the date in Base/LATEST.
func (x HTTPIndex) Latest(ctx context.Context) (string, error) {
	c := x.Client
	if c == nil {
		c = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(x.Base, "/")+"/LATEST", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mbdump: LATEST answered %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	d := strings.TrimSpace(string(raw))
	if d == "" || strings.ContainsAny(d, `/\. `) {
		return "", fmt.Errorf("mbdump: odd LATEST %q", d)
	}
	return d, nil
}

// Current answers the directory of the copy in use ("" when none).
func Current(root string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, pointerFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	d := strings.TrimSpace(string(raw))
	if d == "" {
		return "", nil
	}
	return filepath.Join(root, d), nil
}

func currentDate(root string) string {
	d, _ := Current(root)
	if d == "" {
		return ""
	}
	return filepath.Base(d)
}

// Refresher builds each new dump beside the copy in use, checks it and
// swaps the pointer; the old copy goes after Keep.
type Refresher struct {
	Root   string
	Index  Index
	Build  func(ctx context.Context, date, dir string) (Result, error)
	Want   []string      // tables every copy must have
	Guard  *Guard        // checked before and after a build (nil: none)
	Keep   time.Duration // old copy kept this long after a swap (0: 1 h)
	NoSwap bool          // build and verify only
	Now    func() time.Time
	Logf   func(string, ...any)
}

func (r *Refresher) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Refresher) logf(f string, a ...any) {
	if r.Logf != nil {
		r.Logf(f, a...)
	}
}

// Once builds the newest dump when it is not the copy in use. swapped is
// true when the pointer moved to it.
func (r *Refresher) Once(ctx context.Context) (date string, swapped bool, err error) {
	date, err = r.Index.Latest(ctx)
	if err != nil {
		return "", false, err
	}
	if date == currentDate(r.Root) {
		return date, false, nil
	}
	final := filepath.Join(r.Root, date)
	if _, err := os.Stat(filepath.Join(final, DBName)); err != nil {
		// A build that died earlier leaves its *.building folder behind;
		// it is removed first so it never counts against the cap.
		if err := r.dropBuilding(); err != nil {
			return date, false, err
		}
		if r.Guard != nil {
			if _, err := r.Guard.CheckBuild(ctx); err != nil {
				return date, false, err
			}
		}
		tmp := final + ".building"
		res, err := r.Build(ctx, date, tmp)
		if err != nil {
			return date, false, fmt.Errorf("mbdump: build %s: %w", date, err)
		}
		if _, err := Verify(ctx, tmp, r.Want); err != nil {
			return date, false, err
		}
		if err := os.Rename(tmp, final); err != nil {
			return date, false, err
		}
		r.logf("mbdump: built %s in %s", date, res.Took.Round(time.Second))
	} else if _, err := Verify(ctx, final, r.Want); err != nil {
		return date, false, err
	}
	if r.NoSwap {
		return date, false, nil
	}
	if err := r.swap(date); err != nil {
		return date, false, err
	}
	r.checkCap(ctx, "after swap")
	return date, true, nil
}

// dropBuilding removes every leftover *.building folder under Root.
func (r *Refresher) dropBuilding() error {
	left, err := filepath.Glob(filepath.Join(r.Root, "*.building"))
	if err != nil {
		return err
	}
	for _, dir := range left {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		r.logf("mbdump: removed leftover build %s", filepath.Base(dir))
	}
	return nil
}

// checkCap runs the 90 GB guard and logs when the music data is over it.
// It answers the guard's error (nil without a guard).
func (r *Refresher) checkCap(ctx context.Context, when string) error {
	if r.Guard == nil {
		return nil
	}
	used, err := r.Guard.Check(ctx)
	if err != nil {
		r.logf("mbdump: cap check %s: %v (%d bytes)", when, err, used)
	}
	return err
}

// swap points CURRENT at date (write beside, then rename) and lists the
// old copy as retired.
func (r *Refresher) swap(date string) error {
	old := currentDate(r.Root)
	tmp := filepath.Join(r.Root, pointerFile+".next")
	if err := os.WriteFile(tmp, []byte(date+"\n"), 0o644); err != nil {
		return err
	}
	// Rename is atomic on Linux; Windows refuses it while a reader has the
	// pointer open, so it is tried again briefly.
	var err error
	for try := 0; try < 50; try++ {
		if err = os.Rename(tmp, filepath.Join(r.Root, pointerFile)); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	r.logf("mbdump: copy in use is now %s (was %q)", date, old)
	if old == "" || old == date {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(r.Root, retiredFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s %d\n", old, r.now().Unix())
	return err
}

// Sweep removes retired copies older than Keep and leaves the rest listed.
func (r *Refresher) Sweep() error {
	keep := r.Keep
	if keep <= 0 {
		keep = time.Hour
	}
	p := filepath.Join(r.Root, retiredFile)
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var left []string
	cur := currentDate(r.Root)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		date, at, ok := strings.Cut(sc.Text(), " ")
		sec, err := strconv.ParseInt(at, 10, 64)
		if !ok || err != nil || date == "" || strings.ContainsAny(date, `/\.`) {
			continue
		}
		if date == cur {
			continue
		}
		if r.now().Sub(time.Unix(sec, 0)) < keep {
			left = append(left, sc.Text())
			continue
		}
		if err := os.RemoveAll(filepath.Join(r.Root, date)); err != nil {
			left = append(left, sc.Text())
			continue
		}
		r.logf("mbdump: dropped old copy %s", date)
	}
	f.Close()
	if err := sc.Err(); err != nil {
		return err
	}
	body := ""
	if len(left) > 0 {
		body = strings.Join(left, "\n") + "\n"
	}
	return os.WriteFile(p, []byte(body), 0o644)
}

// Watch runs Once and Sweep every interval until ctx ends. It sweeps
// before Once too, so the copy the last swap retired is gone before the
// next build counts against the cap.
func (r *Refresher) Watch(ctx context.Context, every time.Duration) {
	for {
		if err := r.Sweep(); err != nil {
			r.logf("mbdump: sweep: %v", err)
		}
		if date, swapped, err := r.Once(ctx); err != nil {
			r.logf("mbdump: refresh %s: %v", date, err)
		} else if swapped {
			r.logf("mbdump: refreshed to %s", date)
		}
		if err := r.Sweep(); err != nil {
			r.logf("mbdump: sweep: %v", err)
		}
		r.checkCap(ctx, "after sweep")
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
