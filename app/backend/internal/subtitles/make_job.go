package subtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// KindMake is the Job that asks a plugin site for one subtitle it offered.
// A plugin may take an hour to make one (transcribing a film's audio), so
// this runs in a pool of its own and searches never wait behind it.
const KindMake = "subtitle_make"

// A make whose plugin is not running (stopped, restarting, or HolyArrs
// itself just started) is asked again about once a minute, as a new Job, so
// the plugin can carry on from the pieces it kept. It gives up after a day.
var (
	makeRetryEvery = time.Minute
	makeWaitLimit  = 24 * time.Hour
)

// MakeRequest is the plugin site offer a Job asks for, and the want it is for.
type MakeRequest struct {
	Want         Want   `json:"want"`
	SiteID       int64  `json:"site_id"`
	SiteKey      string `json:"site_key"`
	SiteLanguage string `json:"site_language"`
	SiteHI       bool   `json:"site_hi,omitempty"`
	SiteForced   bool   `json:"site_forced,omitempty"`
	SiteFormat   string `json:"site_format,omitempty"`
	// ByHand is a person's pick: it may replace a subtitle the app saved, and
	// a failure is only reported.
	ByHand bool `json:"by_hand,omitempty"`
	// WaitSince is when the make first found its plugin not running.
	WaitSince string `json:"wait_since,omitempty"`
}

// MakeResult is what a make Job did.
type MakeResult struct {
	Path    string `json:"path,omitempty"`
	Site    string `json:"site,omitempty"`
	Score   int    `json:"score,omitempty"`
	Skipped string `json:"skipped,omitempty"`
}

// makeFailed is a make that ran and failed. The Job is not tried again on
// its own, so a plugin that fails is never asked for another hour of work
// straight away.
type makeFailed struct{ why string }

func (e *makeFailed) Error() string { return e.why }
func (e *makeFailed) Final() bool   { return true }
func (e *makeFailed) Code() string  { return "subtitle_make_failed" }

func makeRequest(w Want, c Scored, r SiteRow, byHand bool) MakeRequest {
	return MakeRequest{Want: w, SiteID: r.ID, SiteKey: c.Key, SiteLanguage: c.Language, SiteHI: c.HearingImpaired, SiteForced: c.Forced, SiteFormat: c.Format, ByHand: byHand}
}

// QueueMake asks for one plugin site subtitle. Each File and language has at
// most one live make Job.
func (s *Service) QueueMake(ctx context.Context, fileID int64, req MakeRequest) error {
	if s.Enqueue == nil {
		return errors.New("no job queue")
	}
	return s.Enqueue(ctx, KindMake, &domain.Subject{Type: "file", ID: fileID}, makeParams(req), makeKey(fileID, req.Want))
}

func makeKey(fileID int64, w Want) string {
	return fmt.Sprintf("%s:%d:%s:%v", KindMake, fileID, w.Language, w.Forced)
}

func makeParams(req MakeRequest) map[string]any {
	b, _ := json.Marshal(req)
	var params map[string]any
	_ = json.Unmarshal(b, &params)
	return params
}

func (s *Service) handleMake(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "file" {
		return errors.New("subtitle_make needs a file subject")
	}
	var req MakeRequest
	b, _ := json.Marshal(r.Job.Params)
	if err := json.Unmarshal(b, &req); err != nil || req.SiteID <= 0 || req.SiteKey == "" {
		return &makeFailed{why: "the request cannot be read"}
	}
	fileID, by := r.Job.Subject.ID, jobActor(r)
	res, err := s.Make(ctx, fileID, req, by)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotMain) || errors.Is(err, ErrMusic) || errors.Is(err, mediafiles.ErrLookOnly) {
		r.SetResult(map[string]any{"skipped": err.Error()})
		return nil
	}
	if err == nil {
		r.SetResult(resultMap(res))
		return nil
	}
	if ctx.Err() != nil {
		return err // stopped: the queue runs it again after the restart
	}
	if errors.Is(err, sites.ErrPluginGone) {
		return s.waitForPlugin(ctx, r, req, err)
	}
	return s.makeGaveUp(ctx, fileID, req, by, err)
}

// waitForPlugin queues the make again a minute on, while its plugin is not
// running, and ends this Job. Each try is a new Job: the queue keeps at most
// one live Job per key, and this one is still live, so the next try takes
// the other of two keys. A search can't queue a third meanwhile, as a plugin
// that is not running offers nothing.
func (s *Service) waitForPlugin(ctx context.Context, r *jobs.Run, req MakeRequest, gone error) error {
	fileID, since := r.Job.Subject.ID, now()
	if t, err := time.Parse(database.TimeLayout, req.WaitSince); err == nil {
		since = t
	}
	if s.EnqueueAt == nil || now().Sub(since) >= makeWaitLimit {
		return s.makeGaveUp(ctx, fileID, req, jobActor(r), fmt.Errorf("the plugin did not come back within a day: %w", gone))
	}
	req.WaitSince = database.FormatTime(since)
	key := makeKey(fileID, req.Want)
	if r.Job.DedupeKey != nil && *r.Job.DedupeKey == key {
		key += ":wait"
	}
	at := now().Add(makeRetryEvery)
	if err := s.EnqueueAt(ctx, KindMake, r.Job.Subject, makeParams(req), key, at); err != nil {
		return err // the queue tries this Job again
	}
	r.SetResult(map[string]any{"waiting": gone.Error(), "next_try": database.FormatTime(at)})
	return nil
}

// makeGaveUp ends a make that failed. Unless a person picked it, it falls
// back to a plain search, which may still save another machine-made file,
// and holds the language back for a day.
func (s *Service) makeGaveUp(ctx context.Context, fileID int64, req MakeRequest, by domain.Actor, err error) error {
	if !req.ByHand {
		if _, serr := s.Search(ctx, fileID, SearchOptions{NoMake: true}, by); serr != nil {
			err = errors.Join(err, serr)
		}
		s.holdTranslation(ctx, fileID, req.Want)
	}
	return &makeFailed{why: err.Error()}
}

// Make asks the plugin site for the subtitle it offered and saves it beside
// the video, marked as made by a machine. It skips when the site was
// removed or turned off, and, unless a person picked it, when a subtitle
// for the want arrived before or while the plugin made it.
func (s *Service) Make(ctx context.Context, fileID int64, req MakeRequest, by domain.Actor) (MakeResult, error) {
	var res MakeResult
	row, err := siteByID(ctx, s.Store.Read(), req.SiteID)
	if err != nil || row.Kind != "plugin" || row.State == "disabled" {
		res.Skipped = "the plugin subtitle site was removed or turned off"
		return res, nil
	}
	res.Site = row.Name
	fc, err := s.load(ctx, fileID)
	if err != nil {
		return res, err
	}
	if fc.look {
		return res, mediafiles.ErrLookOnly
	}
	if why, err := s.makeStillWanted(ctx, fc, req, by); err != nil || why != "" {
		res.Skipped = why
		return res, err
	}
	c := Scored{Candidate: fromThisFile(sites.Candidate{Site: row.Name, Key: req.SiteKey, Language: req.SiteLanguage, HearingImpaired: req.SiteHI,
		Forced: req.SiteForced, Format: req.SiteFormat, Link: strconv.FormatInt(fileID, 10)}, fc.videoFacts())}
	c.Score = ScoreCandidate(fc.videoFacts(), req.Want, c.Candidate)
	if c.Score.Refused != "" {
		return res, fmt.Errorf("that subtitle cannot be used: %s", c.Score.Refused)
	}
	site, err := s.client(ctx, row)
	if err != nil {
		return res, err
	}
	if !s.downloadsLeft(ctx, row) {
		return res, errors.New("the site's downloads for today are used up")
	}
	data, err := site.Download(ctx, c.Candidate)
	if err != nil {
		if !errors.Is(err, sites.ErrPluginGone) && ctx.Err() == nil {
			s.failed(ctx, fc, req.Want, row.Name, err, by)
		}
		return res, err
	}
	s.countDownload(ctx, row) // only a file the plugin made counts
	// The plugin may have taken an hour: read the File and its subtitles
	// again, so a replaced video or a subtitle that arrived meanwhile wins.
	if fc, err = s.load(ctx, fileID); err != nil {
		return res, err
	}
	why, err := s.makeStillWanted(ctx, fc, req, by)
	if err != nil || why != "" {
		res.Skipped = why
		return res, err
	}
	t := target{want: req.Want, beat: -1}
	if req.ByHand {
		t.old = s.savedFor(ctx, fileID, req.Want)
	}
	set := ReadSettings(ctx, s.Store.Read())
	p, err := s.saveDownloaded(ctx, fc, t, c, row, data, set, by)
	if err != nil {
		return res, err
	}
	if req.ByHand {
		s.markByHand(ctx, fileID, p)
	}
	res.Path, res.Score = p, stopgapScore(sites.Transcribed, c.Score.Total)
	out := Outcome{Want: req.Want, Result: "saved", Path: p, Score: res.Score, Site: row.Name, Made: sites.Transcribed}
	if t.old != nil {
		out.Result = "upgraded"
	}
	s.afterTarget(ctx, fc, t, &out, set, by)
	return res, nil
}

// fromThisFile marks a plugin site's offer as written from the video's own
// audio. The hook sends no title, year, episode or release name, but the
// plugin works from this very File, so the offer matches all of them and is
// timed to it. Its saved score is still capped (stopgapScore).
func fromThisFile(c sites.Candidate, v Video) sites.Candidate {
	c.MachineMade = sites.Transcribed
	c.MatchedByID = true
	c.Title, c.Year, c.Season, c.Episode = v.Title, v.Year, v.Season, v.Episode
	c.ReleaseNames = []string{v.ReleaseName}
	c.Corrected = true
	return c
}

// makeStillWanted is why a make for req is no longer needed, or "": a
// timed one only while its want is still missing.
func (s *Service) makeStillWanted(ctx context.Context, fc *fileCtx, req MakeRequest, by domain.Actor) (string, error) {
	if req.ByHand {
		return "", nil
	}
	st, err := s.look(ctx, fc, by)
	if err != nil {
		return "", err
	}
	for _, m := range st.Missing {
		if m.Language == req.Want.Language && m.Forced == req.Want.Forced {
			return "", nil
		}
	}
	return "the File has a " + fmtWant(req.Want) + " subtitle now", nil
}
