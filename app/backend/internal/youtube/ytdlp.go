package youtube

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Runner runs yt-dlp. Every URL it passes was rebuilt from a checked ID and
// follows a "--", there is no shell, and every run has a time limit.
type Runner struct {
	Tools Tools
	Now   func() time.Time
	// Stall stops a download that prints nothing for this long; zero means
	// ten minutes.
	Stall time.Duration
}

// ErrNoYTDLP is returned when yt-dlp was not found.
var ErrNoYTDLP = errors.New("yt-dlp is not installed")

// Time limits for runs that print nothing until they end.
const (
	probeTimeout   = 90 * time.Second
	entriesTimeout = 15 * time.Minute
	versionTimeout = 30 * time.Second
	updateTimeout  = 2 * time.Minute
	defaultStall   = 10 * time.Minute
	// maxLine bounds one output line; -J prints a whole listing as one line.
	maxLine = 256 << 20
)

// Entry is one video in a playlist or channel listing.
type Entry struct {
	ID, Title, LiveStatus string
	Timestamp             int64 // upload time in Unix seconds, 0 when unknown
	DurationSeconds       int
	Short                 bool
}

// Probe is what a pasted link points at, with its first few entries.
type Probe struct {
	Link            Link
	Title           string
	ChannelName     string
	ChannelID       string
	DurationSeconds int
	Count           int // videos in a playlist; 0 when yt-dlp does not say
	LiveStatus      string
	Entries         []Entry
}

// Info is one video's details before it is downloaded.
type Info struct {
	Meta
	LiveStatus string
}

// Progress is how far a download has got, in bytes. Total is 0 when unknown.
type Progress struct {
	Done, Total int64
}

// rawEntry and rawInfo are the parts of yt-dlp's JSON HolyArrs reads.
type rawEntry struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	URL              string   `json:"url"`
	LiveStatus       *string  `json:"live_status"`
	Timestamp        *float64 `json:"timestamp"`
	ReleaseTimestamp *float64 `json:"release_timestamp"`
	Duration         *float64 `json:"duration"`
}

type rawInfo struct {
	rawEntry
	Channel       string     `json:"channel"`
	ChannelID     string     `json:"channel_id"`
	Uploader      string     `json:"uploader"`
	UploadDate    string     `json:"upload_date"`
	PlaylistCount *int       `json:"playlist_count"`
	Entries       []rawEntry `json:"entries"`
}

// Probe asks yt-dlp what a link is. Playlists and channels list only their
// first five entries.
func (r Runner) Probe(ctx context.Context, l Link) (Probe, error) {
	target, err := canonicalURL(l)
	if err != nil {
		return Probe{}, err
	}
	l.URL = target
	args := []string{"-J", "--no-playlist"}
	if l.Kind != KindVideo {
		args = []string{"-J", "--flat-playlist", "--playlist-end", "5"}
		if l.Kind == KindChannel {
			target += "/videos"
		}
	}
	var raw rawInfo
	if err := r.runJSON(ctx, probeTimeout, args, target, &raw); err != nil {
		return Probe{}, err
	}
	p := Probe{
		Link:            l,
		Title:           raw.Title,
		ChannelName:     channelName(raw),
		ChannelID:       raw.ChannelID,
		DurationSeconds: seconds(raw.Duration),
		LiveStatus:      deref(raw.LiveStatus),
	}
	if l.Kind == KindVideo {
		p.Count = 1
		p.Entries = toEntries([]rawEntry{raw.rawEntry}, false)
		return p, nil
	}
	if l.Kind == KindChannel && p.ChannelName != "" {
		p.Title = p.ChannelName
	}
	if raw.PlaylistCount != nil {
		p.Count = *raw.PlaylistCount
	}
	p.Entries = toEntries(raw.Entries, false)
	return p, nil
}

// Listing is what a playlist or channel holds now. Complete is set only for
// a playlist that yt-dlp listed without a single warning, with at least one
// entry and at least as many as the count it gives. yt-dlp ends well on a
// listing cut short, with only a warning, and the count it gives is then
// just the entries it got; so Complete is a hint, and what it says left the
// playlist must be seen gone twice before anything is deleted.
type Listing struct {
	Entries  []Entry
	Complete bool
}

// Entries lists every video a playlist or channel holds. A channel covers
// its Videos and Live tabs, plus Shorts when includeShorts is set, newest
// first; a playlist keeps its own order and every entry.
func (r Runner) Entries(ctx context.Context, l Link, includeShorts bool) ([]Entry, error) {
	list, err := r.Listing(ctx, l, includeShorts)
	return list.Entries, err
}

// Listing is Entries that also says whether a playlist's listing is
// complete.
func (r Runner) Listing(ctx context.Context, l Link, includeShorts bool) (Listing, error) {
	base, err := canonicalURL(l)
	if err != nil || l.Kind == KindVideo {
		return Listing{}, ErrNotYouTube
	}
	ctx, cancel := context.WithTimeout(ctx, entriesTimeout)
	defer cancel()
	if l.Kind == KindPlaylist {
		return r.listTab(ctx, base, false)
	}
	tabs := []string{"videos", "streams"}
	if includeShorts {
		tabs = append(tabs, "shorts")
	}
	seen := map[string]bool{}
	var all []Entry
	for _, tab := range tabs {
		list, err := r.listTab(ctx, base+"/"+tab, tab == "shorts")
		if err != nil && strings.Contains(err.Error(), "does not have a") {
			continue // channels without live streams or shorts have no such tab
		}
		if err != nil {
			return Listing{}, err
		}
		for _, e := range list.Entries {
			if !seen[e.ID] {
				seen[e.ID] = true
				all = append(all, e)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Timestamp > all[j].Timestamp })
	return Listing{Entries: all}, nil
}

// listTab lists one playlist or channel tab. It runs yt-dlp with warnings
// on: a page of the listing that failed is only a warning. The warning
// text is never kept, it may name videos.
func (r Runner) listTab(ctx context.Context, target string, shorts bool) (Listing, error) {
	args := append(r.baseArgs(true), "-J", "--flat-playlist", "--extractor-args", "youtubetab:approximate_date", "--", target)
	var out strings.Builder
	warned := false
	err := r.runArgs(ctx, 0, 0, args, func(line string, stderr bool) {
		if !stderr {
			out.WriteString(line)
			out.WriteByte('\n')
		} else if strings.HasPrefix(strings.TrimSpace(line), "WARNING:") {
			warned = true
		}
	})
	if err != nil {
		return Listing{}, err
	}
	var raw rawInfo
	if err := json.Unmarshal([]byte(out.String()), &raw); err != nil {
		return Listing{}, fmt.Errorf("yt-dlp printed something unexpected: %w", err)
	}
	n := len(raw.Entries)
	return Listing{Entries: toEntries(raw.Entries, shorts), Complete: !warned && raw.PlaylistCount != nil && n > 0 && n >= *raw.PlaylistCount}, nil
}

// Info looks one video up.
func (r Runner) Info(ctx context.Context, videoID string) (Info, error) {
	if !reVideoID.MatchString(videoID) {
		return Info{}, ErrNotYouTube
	}
	var raw rawInfo
	if err := r.runJSON(ctx, probeTimeout, []string{"-J", "--no-playlist"}, watchURL(videoID), &raw); err != nil {
		return Info{}, err
	}
	return Info{
		Meta: Meta{
			Title:           raw.Title,
			ChannelName:     channelName(raw),
			ChannelID:       raw.ChannelID,
			UploadedAt:      uploadedAt(raw),
			DurationSeconds: seconds(raw.Duration),
		},
		LiveStatus: deref(raw.LiveStatus),
	}, nil
}

// Version is yt-dlp's version.
func (r Runner) Version(ctx context.Context) (string, error) {
	return r.lastLine(ctx, versionTimeout, "--version")
}

// SelfUpdate runs "yt-dlp -U" and returns its last line. It returns "" and
// does nothing when the program file or its folder cannot be written here,
// as with a packaged install.
func (r Runner) SelfUpdate(ctx context.Context) (string, error) {
	if r.Tools.YTDLP == "" {
		return "", ErrNoYTDLP
	}
	if !writableFile(r.Tools.YTDLP) {
		return "", nil
	}
	return r.lastLine(ctx, updateTimeout, "-U")
}

// writableFile reports whether file is a regular file, not a link, and both
// it and its folder can be written by this process.
func writableFile(file string) bool {
	st, err := os.Lstat(file)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	f, err := os.OpenFile(file, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	probe, err := os.CreateTemp(filepath.Dir(file), ".yt-dlp-write-check-*")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}

func (r Runner) lastLine(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	last := ""
	err := r.run(ctx, timeout, 0, args, func(line string, stderr bool) {
		if !stderr && strings.TrimSpace(line) != "" {
			last = strings.TrimSpace(line)
		}
	})
	return last, err
}

// runJSON runs yt-dlp on target and decodes what it prints into v.
func (r Runner) runJSON(ctx context.Context, timeout time.Duration, args []string, target string, v any) error {
	var out strings.Builder
	err := r.run(ctx, timeout, 0, append(args, "--", target), func(line string, stderr bool) {
		if !stderr {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	})
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out.String()), v); err != nil {
		return fmt.Errorf("yt-dlp printed something unexpected: %w", err)
	}
	return nil
}

// baseArgs are the arguments every run starts with. Warnings are off but
// for a listing, which has to hear them.
func (r Runner) baseArgs(warnings bool) []string {
	args := []string{"--ignore-config", "--no-warnings", "--newline"}
	if warnings {
		args = []string{"--ignore-config", "--newline"}
	}
	if r.Tools.FFmpeg != "" {
		args = append(args, "--ffmpeg-location", r.Tools.FFmpeg)
	}
	return args
}

type outLine struct {
	text   string
	stderr bool
}

// run runs yt-dlp with the common arguments then args, handing every output
// line to onLine. A timeout above zero limits the whole run; a stall above
// zero stops it when no line arrives for that long. A failed run returns
// yt-dlp's last ERROR line, or its last line on stderr.
func (r Runner) run(ctx context.Context, timeout, stall time.Duration, args []string, onLine func(line string, stderr bool)) error {
	return r.runArgs(ctx, timeout, stall, append(r.baseArgs(false), args...), onLine)
}

// runArgs is run with every argument given, the common ones included.
func (r Runner) runArgs(ctx context.Context, timeout, stall time.Duration, args []string, onLine func(line string, stderr bool)) error {
	if r.Tools.YTDLP == "" {
		return ErrNoYTDLP
	}
	parent := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	cmd := exec.CommandContext(runCtx, r.Tools.YTDLP, args...)
	startOwnGroup(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 5 * time.Second
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	cmd.Stdout, cmd.Stderr = outW, errW
	if err := cmd.Start(); err != nil {
		return err
	}
	waited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		outW.Close()
		errW.Close()
		waited <- err
	}()

	lines := make(chan outLine)
	var readErr error
	var readMu sync.Mutex
	var wg sync.WaitGroup
	read := func(rd io.Reader, stderr bool) {
		defer wg.Done()
		sc := bufio.NewScanner(rd)
		sc.Buffer(make([]byte, 64<<10), maxLine)
		for sc.Scan() {
			lines <- outLine{sc.Text(), stderr}
		}
		if err := sc.Err(); err != nil {
			readMu.Lock()
			readErr = err
			readMu.Unlock()
			stop()
			io.Copy(io.Discard, rd)
		}
	}
	wg.Add(2)
	go read(outR, false)
	go read(errR, true)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	var watchdog <-chan time.Time
	var timer *time.Timer
	if stall > 0 {
		timer = time.NewTimer(stall)
		defer timer.Stop()
		watchdog = timer.C
	}
	stalled := false
	lastErr, lastStderr := "", ""
loop:
	for {
		select {
		case l := <-lines:
			if timer != nil && !stalled {
				timer.Reset(stall)
			}
			if l.stderr {
				if t := strings.TrimSpace(l.text); t != "" {
					lastStderr = t
					if strings.HasPrefix(t, "ERROR:") {
						lastErr = t
					}
				}
			}
			onLine(l.text, l.stderr)
		case <-watchdog:
			stalled = true
			watchdog = nil
			stop()
		case <-done:
			break loop
		}
	}
	waitErr := <-waited

	switch {
	case stalled:
		return fmt.Errorf("no progress from yt-dlp for %s", stall)
	case parent.Err() != nil:
		return parent.Err()
	case ctx.Err() != nil:
		return fmt.Errorf("yt-dlp took longer than %s: %w", timeout, ctx.Err())
	case readErr != nil:
		return fmt.Errorf("reading yt-dlp output: %w", readErr)
	case waitErr != nil && lastErr != "":
		return errors.New(lastErr)
	case waitErr != nil && lastStderr != "":
		return errors.New(lastStderr)
	}
	return waitErr
}

func toEntries(raw []rawEntry, shorts bool) []Entry {
	var out []Entry
	for _, e := range raw {
		if !reVideoID.MatchString(e.ID) {
			continue
		}
		ts := e.Timestamp
		if ts == nil {
			ts = e.ReleaseTimestamp
		}
		var stamp int64
		if ts != nil {
			stamp = int64(*ts)
		}
		out = append(out, Entry{
			ID:              e.ID,
			Title:           e.Title,
			LiveStatus:      deref(e.LiveStatus),
			Timestamp:       stamp,
			DurationSeconds: seconds(e.Duration),
			Short:           shorts || strings.Contains(e.URL, "/shorts/"),
		})
	}
	return out
}

func channelName(raw rawInfo) string {
	if raw.Channel != "" {
		return raw.Channel
	}
	return raw.Uploader
}

func uploadedAt(raw rawInfo) *time.Time {
	if raw.Timestamp != nil && *raw.Timestamp > 0 {
		t := time.Unix(int64(*raw.Timestamp), 0).UTC()
		return &t
	}
	if t, err := time.Parse("20060102", raw.UploadDate); err == nil {
		return &t
	}
	return nil
}

func seconds(f *float64) int {
	if f == nil || *f < 0 {
		return 0
	}
	return int(*f + 0.5)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
