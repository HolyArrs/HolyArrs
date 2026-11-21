package youtube

// Adopting copies videos already on disk into a person's library: a folder
// left by yt-dlp or Pinchflat. Files are copied, never moved, so the source
// stays as it was.
//
// The youtube_adopt job is the one exception to "job params carry row ids
// only": its params hold the person's id, the source folder path and any
// playlist links. Only the owner can start it, once, about the owner's own
// folder, and it carries no titles or video ids; its result, notes and
// errors are counts and ids only.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// AdoptRequest asks to adopt the videos under Path for a person. Playlists
// maps a folder name under Path to a playlist link: that folder's videos
// join a new follow of the playlist, which takes no older videos. A video
// in two folders is copied once and joins the follow of each.
type AdoptRequest struct {
	UserID    int64             `json:"user_id"`
	Path      string            `json:"path"`
	Playlists map[string]string `json:"playlists"`
	DryRun    bool              `json:"dry_run"`
}

// AdoptItem is one video file found. From is relative to the source
// folder, To to the YouTube folder, both with forward slashes. A non-empty
// Reason means it is skipped. Gone is set when YouTube no longer has it.
// A second copy of a video in a folder given a playlist is not skipped: it
// has the first copy's To, and the one file joins that playlist too.
type AdoptItem struct {
	From    string `json:"from"`
	To      string `json:"to"`
	VideoID string `json:"video_id"`
	Folder  string `json:"folder"`
	Reason  string `json:"reason"`
	Gone    bool   `json:"gone"`
}

// Adopt refusals.
var (
	ErrAdoptFolder   = errors.New("the folder to adopt from is the YouTube folder, inside it or around it")
	ErrAdoptPerson   = errors.New("that person cannot use YouTube")
	ErrAdoptNoRoot   = errors.New("set the YouTube folder before adopting videos")
	ErrAdoptPlaylist = errors.New("each playlist must be a folder name with a link to a YouTube playlist")
)

// Why an adopt item is skipped.
const (
	reasonNoID  = "no YouTube id in an .nfo, an .info.json or the name"
	reasonKnown = "already in the library"
	reasonTwice = "the same video is in the folder twice"
)

// IsAdoptRefusal reports whether err refuses an adopt request.
func IsAdoptRefusal(err error) bool {
	for _, e := range []error{ErrAdoptFolder, ErrAdoptPerson, ErrAdoptNoRoot, ErrAdoptPlaylist, library.ErrFolderPath, library.ErrFolderAbsent} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// adoptPlan is a checked request with what was found.
type adoptPlan struct {
	root, src string
	person    accounts.PersonAccess
	userDir   string   // the person's folder name under root
	dir       *os.Root // the source folder, open; close it when done
	sources   []adoptSource
}

// close lets go of the source folder.
func (a adoptPlan) close() {
	if a.dir != nil {
		a.dir.Close()
	}
}

// checkAdopt checks a request: a YouTube folder is set, the source is an
// existing absolute folder that is not a drive root, not around the data
// folder and apart from the YouTube folder, the person may use YouTube and
// every playlist is a folder name with a playlist link.
func (s *Service) checkAdopt(ctx context.Context, in AdoptRequest) (adoptPlan, error) {
	root := LoadSettings(ctx, s.Store.Read()).Folder
	if root == "" {
		return adoptPlan{}, ErrAdoptNoRoot
	}
	if in.Path == "" || !filepath.IsAbs(in.Path) {
		return adoptPlan{}, library.ErrFolderPath
	}
	src := filepath.Clean(in.Path)
	if library.UnsafeFolder(src, s.Store.Dir()) {
		return adoptPlan{}, library.ErrFolderPath
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return adoptPlan{}, library.ErrFolderAbsent
	}
	if nested(src, root) || nested(realDir(src), realDir(root)) {
		return adoptPlan{}, ErrAdoptFolder
	}
	for folder, link := range in.Playlists {
		l, err := ParseLink(link)
		if strings.TrimSpace(folder) == "" || err != nil || l.Kind != KindPlaylist {
			return adoptPlan{}, ErrAdoptPlaylist
		}
	}
	if ok, err := s.mayUse()(ctx, in.UserID); err != nil {
		return adoptPlan{}, err
	} else if !ok {
		return adoptPlan{}, ErrAdoptPerson
	}
	p, err := s.Person(ctx, in.UserID)
	if err != nil {
		return adoptPlan{}, err
	}
	return adoptPlan{root: root, src: src, person: p}, nil
}

// planAdopt checks a request and reads the source folder, which it leaves
// open for the caller to close. It looks nothing up and writes nothing.
func (s *Service) planAdopt(ctx context.Context, in AdoptRequest) (adoptPlan, error) {
	a, err := s.checkAdopt(ctx, in)
	if err != nil {
		return a, err
	}
	if a.userDir, err = userDirName(a.root, in.UserID, a.person.Name); err != nil {
		return a, pathFree(err)
	}
	if a.dir, err = os.OpenRoot(a.src); err != nil {
		return a, pathFree(err)
	}
	if err := scanAdopt(a.dir, filepath.Base(a.src), &a.sources); err != nil {
		a.close()
		return a, pathFree(err)
	}
	sort.Slice(a.sources, func(i, j int) bool { return a.sources[i].item.From < a.sources[j].item.From })
	known, err := KnownVideos(ctx, s.Store.Read(), in.UserID)
	if err != nil {
		a.close()
		return a, err
	}
	first := map[string]int{}
	for i := range a.sources {
		it := &a.sources[i].item
		if it.VideoID == "" {
			it.Reason = reasonNoID
			continue
		}
		src := a.sources[i]
		it.To = a.userDir + "/" + src.target(src.meta, "")
		at, twice := first[it.VideoID]
		_, listed := in.Playlists[it.Folder]
		switch {
		case twice && listed && (a.sources[at].item.Reason == "" || a.sources[at].item.Reason == reasonKnown):
			a.sources[i].again = true
			it.To = a.sources[at].item.To
		case twice:
			it.Reason = reasonTwice
		case known[it.VideoID].State != "" && !adoptable(known[it.VideoID].State):
			it.Reason = reasonKnown
		}
		if !twice {
			first[it.VideoID] = i
		}
	}
	return a, nil
}

// adoptable reports whether a row in state may take an adopted file: one
// with no file of its own.
func adoptable(state string) bool {
	return state == StateSkipped || state == StateRemoved || state == StateFailed
}

// PlanAdopt says what adopting would do, without looking anything up on
// YouTube and without writing anything. To uses what the files say; the
// job prefers YouTube's title, channel and date when it still has the
// video, and a playlist's title for a folder given a playlist.
func (s *Service) PlanAdopt(ctx context.Context, in AdoptRequest) ([]AdoptItem, error) {
	a, err := s.planAdopt(ctx, in)
	if err != nil {
		return nil, err
	}
	a.close()
	items := make([]AdoptItem, len(a.sources))
	for i, src := range a.sources {
		items[i] = src.item
	}
	return items, nil
}

// StartAdopt checks a request and queues the adopt job, returning its id.
func (s *Service) StartAdopt(ctx context.Context, in AdoptRequest, by database.Actor) (int64, error) {
	if _, err := s.checkAdopt(ctx, in); err != nil {
		return 0, err
	}
	if len(s.Tools().Missing()) > 0 {
		return 0, ErrNeedsTools
	}
	params := map[string]any{"user_id": in.UserID, "path": filepath.Clean(in.Path)}
	if len(in.Playlists) > 0 {
		params["playlists"] = in.Playlists
	}
	id, _, err := s.Queue.Enqueue(ctx, KindAdopt, nil, params, fmt.Sprintf("youtube:%d:adopt", in.UserID), by)
	return id, err
}

// adoptParams reads the adopt job's params back.
func adoptParams(params map[string]any) (AdoptRequest, bool) {
	uid, ok := paramID(params, "user_id")
	p, _ := params["path"].(string)
	if !ok || p == "" {
		return AdoptRequest{}, false
	}
	in := AdoptRequest{UserID: uid, Path: p}
	if m, ok := params["playlists"].(map[string]any); ok {
		in.Playlists = map[string]string{}
		for k, v := range m {
			if s, ok := v.(string); ok {
				in.Playlists[k] = s
			}
		}
	}
	return in, true
}

// HandleAdopt runs a youtube_adopt job, params {"user_id", "path"} and
// maybe {"playlists"}. Each video found is looked up on YouTube, copied
// with its thumbnail and subtitles, and recorded as downloaded. A video
// YouTube no longer has keeps what its files say and counts as gone. With
// playlists, a video joins the follow of every folder it is in, though it
// is copied once, and one already in the library joins it too.
// Result: {"copied", "skipped", "gone", "failed"}, plus "follows" and
// "linked" (videos that joined a follow without being copied) when
// playlists were given.
func (s *Service) HandleAdopt(ctx context.Context, r *jobs.Run) error {
	in, ok := adoptParams(r.Job.Params)
	if !ok {
		return finalError{errors.New("the job has no person or folder")}
	}
	run := s.runner()
	if len(run.Tools.Missing()) > 0 {
		return finalError{ErrNeedsTools}
	}
	a, err := s.planAdopt(ctx, in)
	if IsAdoptRefusal(err) {
		return finalError{err}
	}
	if err != nil {
		return err
	}
	defer a.close()
	userDir, err := UserDir(a.root, in.UserID, a.person.Name)
	if err != nil {
		return pathFree(err)
	}
	counts := map[string]any{}
	follows := map[string]*Follow{}
	if len(in.Playlists) > 0 {
		if follows, err = s.adoptFollows(ctx, run, in, a); err != nil {
			return err
		}
		counts["follows"] = len(follows)
	}
	var copied, skipped, gone, failed, linked int
	for i, src := range a.sources {
		if err := r.Progress(ctx, float64(i)/float64(len(a.sources)), fmt.Sprintf("%d of %d", i+1, len(a.sources))); err != nil {
			return err
		}
		f := follows[src.item.Folder]
		if src.again || src.item.Reason == reasonKnown {
			// Nothing to copy: the one file joins this folder's follow.
			ok, err := s.adoptLink(ctx, in.UserID, src.item.VideoID, f)
			switch {
			case err != nil:
				return err
			case ok:
				linked++
			default:
				skipped++
			}
			continue
		}
		if src.item.Reason != "" {
			skipped++
			continue
		}
		out, err := s.adoptOne(ctx, run, a.dir, a.root, userDir, in.UserID, follows[src.item.Folder], src)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch {
		case err != nil:
			failed++
		case out == adoptSkipped:
			skipped++
		case out == adoptGone:
			gone++
		default:
			copied++
		}
	}
	counts["copied"], counts["skipped"], counts["gone"], counts["failed"] = copied, skipped, gone, failed
	if len(in.Playlists) > 0 {
		counts["linked"] = linked
	}
	r.SetResult(counts)
	return nil
}

// adoptLink makes a follow hold a video the person already has a file for,
// and reports whether it did. Nothing happens without a follow or a
// downloaded row: the first copy may have failed.
func (s *Service) adoptLink(ctx context.Context, userID int64, videoID string, f *Follow) (bool, error) {
	if f == nil {
		return false, nil
	}
	ok := false
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		v, err := VideoOfYouTubeID(ctx, tx, userID, videoID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && v.State != StateDone) {
			ok = false
			return nil
		}
		if err != nil {
			return err
		}
		ok = true
		return LinkVideo(tx, userID, v.ID, f.ID, s.now())
	})
	return ok, err
}

// adoptFollows follows each playlist given for a folder that has videos to
// adopt, taking no older videos: its first check records the rest of the
// playlist as skipped. A follow the person already has is reused. A
// playlist YouTube cannot list is left out; its videos are adopted
// without a follow.
func (s *Service) adoptFollows(ctx context.Context, run Runner, in AdoptRequest, a adoptPlan) (map[string]*Follow, error) {
	used := map[string]bool{}
	for _, src := range a.sources {
		if src.item.Reason == "" || src.item.Reason == reasonKnown {
			used[src.item.Folder] = true
		}
	}
	out := map[string]*Follow{}
	for folder, link := range in.Playlists {
		if !used[folder] {
			continue
		}
		l, err := ParseLink(link)
		if err != nil {
			continue
		}
		p, err := run.Probe(ctx, l)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		f := Follow{UserID: in.UserID, Kind: KindPlaylist, YTID: l.ID, URL: p.Link.URL, Title: p.Title, MaxHeight: DefaultMaxHeight,
			Backfill: intPtr(0), Enabled: true, AddedAt: s.now()}
		err = s.Store.Write(ctx, func(tx *sql.Tx) error {
			var err error
			f.ID, err = InsertFollow(tx, f)
			return err
		})
		if errors.Is(err, ErrDuplicateFollow) {
			f, err = FollowOfYTID(ctx, s.Store.Read(), in.UserID, KindPlaylist, l.ID)
		}
		if err != nil {
			return nil, err
		}
		out[folder] = &f
	}
	return out, nil
}

// What adoptOne did with a video.
const (
	adoptCopied  = "copied"
	adoptGone    = "gone"
	adoptSkipped = "skipped"
)

// adoptOne copies one video, its thumbnail and subtitles into the person's
// folder and records it as downloaded. Its files go in a folder named
// after its playlist when one was given, else its channel, else the folder
// it came from. The follow, when there is one, holds the video; a video
// with no follow, or one YouTube no longer has, is manual, so nothing
// deletes it later. Anything copied is deleted again when the row cannot
// be written.
func (s *Service) adoptOne(ctx context.Context, run Runner, dir *os.Root, root, userDir string, userID int64, f *Follow, src adoptSource) (string, error) {
	meta, out := src.meta, adoptCopied
	if info, err := run.Info(ctx, src.item.VideoID); err == nil && info.Title != "" {
		meta = mergeMeta(info.Meta, src.meta)
	} else if ctx.Err() != nil {
		return "", ctx.Err()
	} else {
		out = adoptGone
	}
	folder := ""
	if f != nil {
		folder = f.Title
	}
	file := filepath.Join(userDir, filepath.FromSlash(src.target(meta, folder)))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", pathFree(err)
	}
	if s.FreeSpace != nil {
		if free := s.FreeSpace(root); free >= 0 && free < src.size+LoadSettings(ctx, s.Store.Read()).floorBytes() {
			return "", errors.New("not enough free space in the YouTube folder")
		}
	}
	if err := copyFile(ctx, dir, src.name, file); err != nil {
		return "", err
	}
	// ffmpeg reads the copy, never the source by path.
	if meta.DurationSeconds == 0 {
		meta.DurationSeconds = videoSeconds(ctx, run.Tools.FFmpeg, file)
	}
	copies := append([]string{file}, adoptExtras(ctx, run.Tools.FFmpeg, dir, src, file, meta.DurationSeconds)...)
	undo := func() {
		for _, c := range copies {
			os.Remove(c)
		}
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) {
		undo()
		return "", errors.New("an adopted video would leave the YouTube folder")
	}
	rel = filepath.ToSlash(rel)
	var rowID int64
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		old, err := VideoOfYouTubeID(ctx, tx, userID, src.item.VideoID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if rowID, _, err = InsertVideo(tx, Video{UserID: userID, VideoID: src.item.VideoID, State: StateDone, AddedAt: s.now()}); err != nil {
				return err
			}
		case err != nil:
			return err
		case !adoptable(old.State):
			if old.RelativePath == rel {
				copies = nil // the row already has these very files
			}
			return errRowGone
		default:
			rowID = old.ID
		}
		if f != nil {
			if err := LinkVideo(tx, userID, rowID, f.ID, s.now()); err != nil {
				return err
			}
		}
		if f == nil || out == adoptGone {
			if err := SetManual(tx, userID, rowID); err != nil {
				return err
			}
		}
		return FinishVideo(tx, rowID, rel, src.size, meta)
	})
	if errors.Is(err, errRowGone) {
		undo()
		return adoptSkipped, nil
	}
	if err != nil {
		undo()
		return "", err
	}
	s.publish(userID, LiveVideo{ID: rowID, State: StateDone, Percent: 100})
	return out, nil
}

// mergeMeta is what YouTube says, with what the files say filling gaps.
func mergeMeta(yt, local Meta) Meta {
	if yt.ChannelName == "" {
		yt.ChannelName = local.ChannelName
	}
	if yt.ChannelID == "" {
		yt.ChannelID = local.ChannelID
	}
	if yt.UploadedAt == nil {
		yt.UploadedAt = local.UploadedAt
	}
	if yt.DurationSeconds == 0 {
		yt.DurationSeconds = local.DurationSeconds
	}
	return yt
}

// nested reports whether a and b are the same folder or one holds the
// other.
func nested(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	sep := string(filepath.Separator)
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, sep)+sep) || strings.HasPrefix(b, strings.TrimSuffix(a, sep)+sep)
}

// realDir is p with symlinks followed, or p when that fails.
func realDir(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
