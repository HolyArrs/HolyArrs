package youtube

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// Settings errors.
var (
	ErrFolderHasVideos = errors.New("downloaded videos are in the YouTube folder; the folder cannot change while they are there")
	ErrMinFree         = errors.New("the free space to keep must be between 1 and 100000 GB")
)

// Service is the YouTube library: settings, status, follows and downloads.
type Service struct {
	Store *database.Store
	Queue *jobs.Queue
	// Live sends an event to one person's open screens only.
	Live interface {
		PublishOwn(topic string, data any, userID int64) uint64
	}
	// Tools finds yt-dlp and ffmpeg; it runs on every call, so a program
	// installed while the app runs is found.
	Tools func() Tools
	// Run makes the yt-dlp runner for the tools found.
	Run func(Tools) Runner
	// CheckFolder checks a YouTube folder before it is saved.
	CheckFolder func(ctx context.Context, path string) error
	// Person reads what a person may do now.
	Person func(ctx context.Context, userID int64) (accounts.PersonAccess, error)
	// FreeSpace is the free bytes on the disk holding path, -1 when unknown.
	FreeSpace func(path string) int64
	Now       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) runner() Runner {
	t := s.Tools()
	if s.Run != nil {
		return s.Run(t)
	}
	return Runner{Tools: t, Now: s.Now}
}

// Status is what the YouTube page needs before anything else: the missing
// programs, whether the folder is set, and whether downloads wait for space.
type Status struct {
	Missing    []string `json:"missing"`
	FolderSet  bool     `json:"folder_set"`
	LowSpace   bool     `json:"low_space"`
	FreeBytes  int64    `json:"free_bytes"` // -1 when unknown or no folder
	FloorBytes int64    `json:"floor_bytes"`
}

// Status reports the programs, the folder and its free space.
func (s *Service) Status(ctx context.Context) Status {
	set := LoadSettings(ctx, s.Store.Read())
	st := Status{Missing: s.Tools().Missing(), FolderSet: set.Folder != "", FreeBytes: -1, FloorBytes: set.floorBytes()}
	if st.Missing == nil {
		st.Missing = []string{}
	}
	if st.FolderSet && s.FreeSpace != nil {
		st.FreeBytes = s.FreeSpace(set.Folder)
	}
	st.LowSpace = st.FreeBytes >= 0 && st.FreeBytes < st.FloorBytes
	return st
}

// SaveSettings saves the folder and the free-space floor. A new folder is
// checked first, and refused while downloaded videos are in the old one
// (their saved paths are relative to it).
func (s *Service) SaveSettings(ctx context.Context, in Settings, by database.Actor) (Settings, error) {
	if in.MinFreeGB < 1 || in.MinFreeGB > MaxFreeGB {
		return Settings{}, ErrMinFree
	}
	folder := strings.TrimSpace(in.Folder)
	if folder != "" {
		folder = filepath.Clean(folder)
	}
	old := LoadSettings(ctx, s.Store.Read())
	if folder != old.Folder && folder != "" {
		if err := s.CheckFolder(ctx, folder); err != nil {
			return Settings{}, err
		}
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		cur, err := database.Setting(ctx, tx, settingsGroup, keyFolder)
		if err != nil {
			return err
		}
		if folder != cur {
			t, err := ReadTotals(ctx, tx)
			if err != nil {
				return err
			}
			if t.Videos > 0 {
				return ErrFolderHasVideos
			}
		}
		now := s.now()
		if err := database.PutSetting(tx, settingsGroup, keyFolder, folder, by, now); err != nil {
			return err
		}
		return database.PutSetting(tx, settingsGroup, keyMinFreeGB, strconv.Itoa(in.MinFreeGB), by, now)
	})
	if err != nil {
		return Settings{}, err
	}
	return LoadSettings(ctx, s.Store.Read()), nil
}

// Errors for adding links and follows. All but ErrNeedsTools and ErrLookup
// are refusals: the person asked for something that cannot be done.
var (
	ErrNeedsTools = errors.New("YouTube needs yt-dlp and ffmpeg on the server")
	ErrLookup     = errors.New("YouTube could not be asked about this link")
	ErrNotAired   = errors.New("live streams and premieres can be added once they have aired")
	ErrNotAVideo  = errors.New("this link is a playlist or channel; follow it instead")
	ErrNotAFollow = errors.New("this link is one video; add it as a video instead")
	ErrBackfill   = errors.New("the number of videos to take must be 0 or more")
	ErrKeepLast   = errors.New("keep only the newest needs a number of 1 or more")
	ErrQuality    = errors.New("the quality cap must be between 144 and 4320")
	ErrMirrorKind = errors.New("only a playlist can be mirrored")
	ErrMirrorKeep = errors.New("a mirrored playlist keeps every video in it; turn mirroring off to keep only the newest")
	ErrMirrorList = errors.New("a mix or one of YouTube's own lists (Watch later, Liked videos, Uploads) cannot be mirrored; it changes on its own or cannot be listed in full")
)

// unmirrorable reports whether a playlist id names a list that must never
// be mirrored: mixes (RD), uploads (UL), liked videos (LL) and watch later
// (WL). They change by themselves or are not listed in full.
func unmirrorable(playlistID string) bool {
	for _, p := range []string{"RD", "UL", "LL", "WL"} {
		if strings.HasPrefix(playlistID, p) {
			return true
		}
	}
	return false
}

// IsRefusal reports whether err is a refused request rather than a fault.
func IsRefusal(err error) bool {
	for _, e := range []error{ErrNotYouTube, ErrNotAired, ErrNotAVideo, ErrNotAFollow, ErrBackfill, ErrKeepLast, ErrQuality, ErrMirrorKind, ErrMirrorKeep, ErrMirrorList} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// Follow defaults.
const (
	DefaultBackfill  = 20
	DefaultMaxHeight = 1080
)

// AddFollow is a request to follow a playlist or channel. Backfill nil
// takes the default: the newest 20 for a channel, every video for a
// playlist. KeepLast nil keeps every video. MaxHeight 0 is 1080. Mirror,
// for a playlist only, takes every video and removes what leaves it.
type AddFollow struct {
	URL           string `json:"url"`
	Backfill      *int   `json:"backfill"`
	KeepLast      *int   `json:"keep_last"`
	IncludeShorts bool   `json:"include_shorts"`
	MaxHeight     int    `json:"quality_max_height"`
	Mirror        bool   `json:"mirror"`
}

func checkHeight(h int) error {
	if h != 0 && (h < 144 || h > 4320) {
		return ErrQuality
	}
	return nil
}

// probe asks yt-dlp about a checked link.
func (s *Service) probe(ctx context.Context, l Link) (Probe, error) {
	r := s.runner()
	if len(r.Tools.Missing()) > 0 {
		return Probe{}, ErrNeedsTools
	}
	p, err := r.Probe(ctx, l)
	if err != nil {
		return Probe{}, fmt.Errorf("%w: %v", ErrLookup, err)
	}
	return p, nil
}

// Probe says what a pasted link points at, for the confirm card.
func (s *Service) Probe(ctx context.Context, raw string) (Probe, error) {
	l, err := ParseLink(raw)
	if err != nil {
		return Probe{}, err
	}
	return s.probe(ctx, l)
}

// AddVideo adds one video for a person, marks it manual and queues its
// download. Adding a video the person already has, from a follow or not,
// returns that row; a removed, failed or skipped one goes back in the
// queue. Live streams and premieres that have not aired are refused.
func (s *Service) AddVideo(ctx context.Context, userID int64, raw string, height int) (Video, error) {
	l, err := ParseLink(raw)
	if err != nil {
		return Video{}, err
	}
	if l.Kind != KindVideo {
		return Video{}, ErrNotAVideo
	}
	if err := checkHeight(height); err != nil {
		return Video{}, err
	}
	p, err := s.probe(ctx, l)
	if err != nil {
		return Video{}, err
	}
	if p.LiveStatus == "is_live" || p.LiveStatus == "is_upcoming" {
		return Video{}, ErrNotAired
	}
	var v Video
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		old, err := VideoOfYouTubeID(ctx, tx, userID, l.ID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			_, _, err = InsertVideo(tx, Video{UserID: userID, VideoID: l.ID, Title: p.Title, ChannelName: p.ChannelName,
				ChannelID: p.ChannelID, DurationSeconds: p.DurationSeconds, MaxHeight: height, State: StateQueued, Manual: true, AddedAt: s.now()})
		case err != nil:
			return err
		default:
			if err = SetManual(tx, userID, old.ID); err == nil && (old.State == StateRemoved || old.State == StateFailed || old.State == StateSkipped) {
				err = RequeueVideo(tx, userID, old.ID, height)
			}
		}
		if err != nil {
			return err
		}
		if v, err = VideoOfYouTubeID(ctx, tx, userID, l.ID); err != nil {
			return err
		}
		return s.fill(ctx, tx, &v)
	})
	if err != nil {
		return Video{}, err
	}
	if v.State == StateQueued {
		s.queueDownloadNow(ctx, userID, v.ID)
	}
	return v, nil
}

// queueDownload queues a video's download and reports whether that made a
// new job: false when one for the video was already waiting or running. A
// video left queued without a job is picked up by the next check, so a
// failure here is only logged.
func (s *Service) queueDownload(ctx context.Context, userID, rowID int64) bool {
	_, created, err := s.Queue.Enqueue(ctx, KindDownload, nil, map[string]any{"row_id": rowID},
		fmt.Sprintf("youtube:%d:%d", userID, rowID), database.Actor{Kind: "user", ID: userID})
	if err != nil {
		log.Printf("youtube: video %d: download not queued: %v", rowID, err)
	}
	return created
}

// queueDownloadNow is queueDownload for a video the person just put in the
// queue. When a job for the video is already live, that job may be about
// to end without looking at the video again, so a second job is queued
// behind it: it takes the video only if the first left it queued.
func (s *Service) queueDownloadNow(ctx context.Context, userID, rowID int64) {
	if s.queueDownload(ctx, userID, rowID) {
		return
	}
	_, _, err := s.Queue.Enqueue(ctx, KindDownload, nil, map[string]any{"row_id": rowID, "after": true},
		fmt.Sprintf("youtube:%d:%d:after", userID, rowID), database.Actor{Kind: "user", ID: userID})
	if err != nil {
		log.Printf("youtube: video %d: second download not queued: %v", rowID, err)
	}
}

// AddFollow follows a playlist or channel for a person and queues its first
// check. A channel is known by its UC id, whichever link named it. On a
// duplicate it returns the existing follow with ErrDuplicateFollow.
func (s *Service) AddFollow(ctx context.Context, userID int64, in AddFollow) (Follow, error) {
	l, err := ParseLink(in.URL)
	if err != nil {
		return Follow{}, err
	}
	if l.Kind == KindVideo {
		return Follow{}, ErrNotAFollow
	}
	if in.Backfill != nil && *in.Backfill < 0 {
		return Follow{}, ErrBackfill
	}
	if in.KeepLast != nil && *in.KeepLast < 1 {
		return Follow{}, ErrKeepLast
	}
	if err := checkHeight(in.MaxHeight); err != nil {
		return Follow{}, err
	}
	if in.Mirror && l.Kind != KindPlaylist {
		return Follow{}, ErrMirrorKind
	}
	if in.Mirror && in.KeepLast != nil {
		return Follow{}, ErrMirrorKeep
	}
	if in.Mirror && unmirrorable(l.ID) {
		return Follow{}, ErrMirrorList
	}
	p, err := s.probe(ctx, l)
	if err != nil {
		return Follow{}, err
	}
	if in.Mirror {
		in.Backfill = nil // a mirror takes every video
	}
	f := Follow{UserID: userID, Kind: l.Kind, YTID: l.ID, URL: p.Link.URL, Title: p.Title, MaxHeight: maxHeight(in.MaxHeight),
		KeepLast: in.KeepLast, Backfill: in.Backfill, Enabled: true, Mirror: in.Mirror, AddedAt: s.now()}
	if l.Kind == KindChannel {
		f.YTID = p.ChannelID
		if !reChannelID.MatchString(f.YTID) {
			return Follow{}, fmt.Errorf("%w: yt-dlp did not say which channel this is", ErrLookup)
		}
		f.URL = site + "channel/" + f.YTID
		f.IncludeShorts = in.IncludeShorts
		if f.Backfill == nil {
			f.Backfill = intPtr(DefaultBackfill)
		}
	}
	// Never download what keep_last would delete straight away.
	if f.KeepLast != nil && (f.Backfill == nil || *f.Backfill > *f.KeepLast) {
		f.Backfill = intPtr(*f.KeepLast)
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var err error
		f.ID, err = InsertFollow(tx, f)
		return err
	})
	if errors.Is(err, ErrDuplicateFollow) {
		old, rerr := FollowOfYTID(ctx, s.Store.Read(), userID, f.Kind, f.YTID)
		if rerr != nil {
			return Follow{}, rerr
		}
		return old, ErrDuplicateFollow
	}
	if err != nil {
		return Follow{}, err
	}
	s.queueCheck(ctx, userID, f.ID)
	return FollowOf(ctx, s.Store.Read(), userID, f.ID)
}

// queueCheck queues a check of one follow. The scheduled check picks the
// follow up later, so a failure here is only logged.
func (s *Service) queueCheck(ctx context.Context, userID, followID int64) {
	_, _, err := s.Queue.Enqueue(ctx, KindCheck, nil, map[string]any{"follow_id": followID},
		fmt.Sprintf("youtube:%d:follow:%d", userID, followID), database.Actor{Kind: "user", ID: userID})
	if err != nil {
		log.Printf("youtube: follow %d: check not queued: %v", followID, err)
	}
}

// fill sets the follows that hold one of a person's videos on it.
func (s *Service) fill(ctx context.Context, q database.Lister, v *Video) error {
	one := []Video{*v}
	if err := WithFollows(ctx, q, v.UserID, one); err != nil {
		return err
	}
	*v = one[0]
	return nil
}

func intPtr(n int) *int { return &n }

// UpdateFollow changes one of a person's follows; sql.ErrNoRows when it is
// not theirs. Only a playlist can be mirrored, and a mirrored playlist
// keeps every video: turning mirroring on turns keep-last off and queues
// a check, so the change shows at once.
func (s *Service) UpdateFollow(ctx context.Context, userID, id int64, c FollowChange) (Follow, error) {
	if c.MaxHeight != nil {
		if err := checkHeight(*c.MaxHeight); err != nil {
			return Follow{}, err
		}
	}
	if c.KeepLast != nil && *c.KeepLast != nil && **c.KeepLast < 1 {
		return Follow{}, ErrKeepLast
	}
	mirrorOn := false
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		ch := c
		old, err := FollowOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		mirror := old.Mirror
		if ch.Mirror != nil {
			mirror = *ch.Mirror
		}
		if mirror && old.Kind != KindPlaylist {
			return ErrMirrorKind
		}
		if mirror && unmirrorable(old.YTID) {
			return ErrMirrorList
		}
		// Marks of videos seen missing never carry over a switch.
		if mirror != old.Mirror {
			if err := ClearMissing(tx, userID, id); err != nil {
				return err
			}
		}
		if mirror && ch.KeepLast != nil && *ch.KeepLast != nil {
			return ErrMirrorKeep
		}
		if mirrorOn = mirror && !old.Mirror; mirrorOn {
			var none *int
			ch.KeepLast = &none
		}
		return UpdateFollow(tx, userID, id, ch)
	})
	if err != nil {
		return Follow{}, err
	}
	if mirrorOn {
		s.queueCheck(ctx, userID, id)
	}
	return FollowOf(ctx, s.Store.Read(), userID, id)
}

// Unfollow stops following. With deleteFiles the videos only this follow
// keeps go too, rows first and then their files and thumbnails; a video
// that is manual or that another follow holds stays. Without it every
// video the follow holds stays and becomes manual, so no other follow
// deletes it later. Which videos go, and where their files are, is read in
// the same write that removes them. sql.ErrNoRows when the follow is not
// theirs.
func (s *Service) Unfollow(ctx context.Context, userID, id int64, deleteFiles bool) error {
	var files []string
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		files = nil
		if _, err := FollowOf(ctx, tx, userID, id); err != nil {
			return err
		}
		if !deleteFiles {
			if err := KeepVideosOfFollow(tx, userID, id); err != nil {
				return err
			}
			return DeleteFollow(tx, userID, id)
		}
		videos, err := OnlyOfFollow(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		for _, v := range videos {
			files = append(files, v.RelativePath)
		}
		if err := DeleteOnlyOfFollow(tx, userID, id); err != nil {
			return err
		}
		return DeleteFollow(tx, userID, id)
	})
	if err != nil {
		return err
	}
	root := LoadSettings(ctx, s.Store.Read()).Folder
	var first error
	for _, rel := range files {
		if err := removeFiles(root, rel); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// RemoveVideo deletes one of a person's videos and its thumbnail; the row
// stays as removed so a check never brings it back. The row is marked
// first, in the write that reads where its file is, so a download ending
// just then leaves no file behind. sql.ErrNoRows when it is not theirs.
func (s *Service) RemoveVideo(ctx context.Context, userID, id int64) error {
	var rel string
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		v, err := VideoOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		rel = v.RelativePath
		return MarkRemoved(tx, userID, id)
	})
	if err != nil {
		return err
	}
	return removeFiles(LoadSettings(ctx, s.Store.Read()).Folder, rel)
}

// ErrNotFailed refuses a retry of a video that has not failed.
var ErrNotFailed = errors.New("only a failed video can be tried again")

// RetryVideo puts one of a person's failed videos back in the queue at its
// own quality cap and queues its download. sql.ErrNoRows when it is not
// theirs, ErrNotFailed when it has not failed.
func (s *Service) RetryVideo(ctx context.Context, userID, id int64) (Video, error) {
	var v Video
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		old, err := VideoOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		if old.State != StateFailed {
			return ErrNotFailed
		}
		if err := RequeueVideo(tx, userID, id, old.MaxHeight); err != nil {
			return err
		}
		if v, err = VideoOf(ctx, tx, userID, id); err != nil {
			return err
		}
		return s.fill(ctx, tx, &v)
	})
	if err != nil {
		return Video{}, err
	}
	s.queueDownloadNow(ctx, userID, v.ID)
	return v, nil
}

// SavePosition keeps where a person stopped in one of their videos and
// reports whether it is watched now: past 90 % once is watched for good.
// sql.ErrNoRows when it is not theirs.
func (s *Service) SavePosition(ctx context.Context, userID, id int64, seconds, duration int) (bool, error) {
	var v Video
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := SavePosition(tx, userID, id, seconds, watched(seconds, duration), s.now()); err != nil {
			return err
		}
		var err error
		v, err = VideoOf(ctx, tx, userID, id)
		return err
	})
	return v.WatchedAt != nil, err
}

// watched reports whether a position counts as watched: 90 % or more of a
// known length.
func watched(seconds, duration int) bool {
	return duration > 0 && seconds*10 >= duration*9
}

// removeFiles deletes a video file, its thumbnail and its subtitles under
// root, then the folder they were in when that leaves it empty. A file
// already gone is fine; a path leaving root is refused.
func removeFiles(root, rel string) error {
	if rel == "" || root == "" {
		return nil
	}
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return errors.New("a video path leaves the YouTube folder")
	}
	dir, err := os.OpenRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("the YouTube folder could not be opened: %w", pathFree(err))
	}
	defer dir.Close()
	files := []string{rel, thumbOf(rel)}
	for _, sub := range subtitlesOf(dir, rel) {
		files = append(files, sub)
	}
	for _, f := range files {
		if err := dir.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			// The path holds the title: keep it out of errors and logs.
			var pe *os.PathError
			if errors.As(err, &pe) {
				err = pe.Err
			}
			return fmt.Errorf("a video file could not be deleted: %w", err)
		}
	}
	pruneEmpty(dir, path.Dir(filepath.ToSlash(rel)))
	return nil
}

// thumbOf is the thumbnail next to a video file: the same name, .jpg.
func thumbOf(file string) string {
	return strings.TrimSuffix(file, filepath.Ext(file)) + ".jpg"
}
