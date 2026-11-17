package subtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/container"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/translate"
)

// KindTranslate is the Job that translates one subtitle for one File.
const KindTranslate = "subtitle_translate"

// Event kinds of translations.
const (
	EventTranslated      = "subtitle_translated"
	EventTranslateFailed = "subtitle_translate_failed"
)

// Where a translation takes its text from.
const (
	SourceBeside  = "beside"   // a subtitle file beside the video
	SourceBuiltIn = "built_in" // a text track inside the video
	SourceSite    = "site"     // a file a subtitle site offered
)

// Errors a translation can stop with.
var (
	ErrPicture     = errors.New("picture subtitles such as PGS and VobSub are skipped: they cannot be read as text")
	ErrNoSource    = errors.New("no text subtitle to translate from was found")
	ErrHasSubtitle = errors.New("the video already has a subtitle in that language, and a translation never replaces one")
)

// TranslateRequest says what to translate. Language is the language to make;
// From the one to translate from, or empty for the wish's rule (or, by hand,
// any text subtitle, English first). Source and its fields pin the source;
// empty lets the Job find one.
type TranslateRequest struct {
	Language     string `json:"language"`
	Forced       bool   `json:"forced,omitempty"`
	From         string `json:"from,omitempty"`
	Source       string `json:"source,omitempty"`
	Path         string `json:"path,omitempty"`
	Track        int    `json:"track,omitempty"`
	SiteID       int64  `json:"site_id,omitempty"`
	SiteKey      string `json:"site_key,omitempty"`
	SiteLink     string `json:"site_link,omitempty"`
	SiteFormat   string `json:"site_format,omitempty"`
	SiteLanguage string `json:"site_language,omitempty"`
	SiteHI       bool   `json:"site_hi,omitempty"`
	SiteScore    int    `json:"site_score,omitempty"`
	// ByHand is a person's ask: built-in tracks are always allowed, a failure
	// is only reported, and a machine-made subtitle may be replaced.
	ByHand bool `json:"by_hand,omitempty"`
}

// TranslateResult is what a translation made.
type TranslateResult struct {
	Path       string `json:"path,omitempty"`
	Language   string `json:"language"`
	From       string `json:"from,omitempty"`
	Source     string `json:"source,omitempty"`
	Lines      int    `json:"lines"`
	Translated int    `json:"translated"`
	Partial    bool   `json:"partial,omitempty"`
	Skipped    string `json:"skipped,omitempty"`
}

// translateFailed is a translation that ran and failed. The Job is not tried
// again on its own, so a broken translator never spends twice in a row.
type translateFailed struct{ why string }

func (e *translateFailed) Error() string { return e.why }
func (e *translateFailed) Final() bool   { return true }
func (e *translateFailed) Code() string  { return "subtitle_translate_failed" }

// QueueTranslate asks for a translation of one subtitle of one File. Each
// File and language has at most one live translation Job.
func (s *Service) QueueTranslate(ctx context.Context, fileID int64, req TranslateRequest) error {
	if s.Enqueue == nil {
		return errors.New("no job queue")
	}
	b, _ := json.Marshal(req)
	var params map[string]any
	_ = json.Unmarshal(b, &params)
	return s.Enqueue(ctx, KindTranslate, &domain.Subject{Type: "file", ID: fileID}, params, translateKey(fileID, req))
}

func translateKey(fileID int64, req TranslateRequest) string {
	return fmt.Sprintf("%s:%d:%s:%v", KindTranslate, fileID, req.Language, req.Forced)
}

func (s *Service) handleTranslate(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "file" {
		return errors.New("subtitle_translate needs a file subject")
	}
	var req TranslateRequest
	b, _ := json.Marshal(r.Job.Params)
	if err := json.Unmarshal(b, &req); err != nil {
		return &translateFailed{why: "the translation request cannot be read"}
	}
	fileID, by := r.Job.Subject.ID, jobActor(r)
	res, err := s.Translate(ctx, fileID, req, by)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotMain) || errors.Is(err, ErrMusic) {
		r.SetResult(map[string]any{"skipped": err.Error()})
		return nil
	}
	if err == nil {
		r.SetResult(resultMap(res))
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err() // stopped: nothing more is spent
	}
	if !req.ByHand {
		// A failed translation falls back to a plain search, which may still
		// save a machine-made file, and holds the language back for a day.
		if _, serr := s.Search(ctx, fileID, SearchOptions{NoTranslate: true}, by); serr != nil {
			err = errors.Join(err, serr)
		}
		s.holdTranslation(ctx, fileID, Want{Language: NormLanguage(req.Language), Forced: req.Forced})
	}
	var tf *translateFailed
	if errors.As(err, &tf) {
		return err
	}
	return &translateFailed{why: err.Error()}
}

func resultMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// source is the text a translation starts from.
type source struct {
	kind     string
	language string
	hi       bool
	path     string // beside: relative to the Library folder
	track    int
	site     SiteRow
	cand     sites.Candidate
}

// Translate makes one subtitle with the owner's translator and saves it
// beside the video as SRT. It never replaces a subtitle made by a person:
// when one is there, before or after the translation, it skips. Lines the
// translator never sent back keep their source text, and such a file is
// marked partial; one with less than half its lines translated is a failure.
func (s *Service) Translate(ctx context.Context, fileID int64, req TranslateRequest, by domain.Actor) (TranslateResult, error) {
	res := TranslateResult{}
	fc, err := s.load(ctx, fileID)
	if err != nil {
		return res, err
	}
	if req.Language = NormLanguage(req.Language); req.Language == "" {
		return res, ErrBadLanguage
	}
	res.Language = req.Language
	if req.From != "" {
		if req.From = NormLanguage(req.From); req.From == "" {
			return res, ErrBadLanguage
		}
	}
	client, tr, err := s.translatorClient(ctx)
	if err != nil {
		return res, err
	}
	st, err := s.look(ctx, fc, by)
	if err != nil {
		return res, err
	}
	for _, w := range st.Wish.Wants {
		if req.From == "" && w.Language == req.Language && w.Forced == req.Forced {
			req.From = w.TranslateFrom
		}
	}
	if req.From == req.Language {
		return res, fmt.Errorf("%s cannot be translated from itself", req.Language)
	}
	stops, err := stopgapRows(ctx, s.Store.Read(), fc.file.ID)
	if err != nil {
		return res, err
	}
	old, err := realSubtitle(st, req, stops)
	if err != nil {
		if !req.ByHand {
			res.Skipped = err.Error()
			return res, nil
		}
		return res, err
	}
	src, err := s.findSource(ctx, fc, st, req, tr, stops)
	if err != nil {
		return res, err
	}
	res.From, res.Source = src.language, src.kind
	cues, err := s.readSource(ctx, fc, src)
	if err != nil {
		return res, err
	}
	job := translate.Job{Batch: translate.Batch{From: src.language, To: req.Language, FromName: LanguageName(src.language), ToName: LanguageName(req.Language),
		Title: fc.title.Name, Kind: fc.item.Kind, MediaID: fc.title.ID}}
	for i, c := range cues {
		job.Lines = append(job.Lines, translate.Line{N: i + 1, Text: c.Text})
	}
	out, terr := translate.Run(ctx, client, job)
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	res.Lines, res.Translated, res.Partial = out.Total, out.Translated, out.Partial()
	if out.Translated == 0 || out.Translated*2 < out.Total {
		why := "the translator sent back too few lines"
		if terr != nil {
			why = translatorWords(terr)
		}
		s.translateEvent(ctx, fc, req, src, res, why, by)
		return res, &translateFailed{why: why}
	}
	for i := range cues {
		cues[i].Text = out.Lines[i]
	}
	// A subtitle made by a person may have landed while the translator worked.
	if st, err = s.look(ctx, fc, by); err != nil {
		return res, err
	}
	if _, err := realSubtitle(st, req, stops); err != nil {
		res.Skipped = "a subtitle arrived while translating: " + err.Error()
		return res, nil
	}
	made, name, score := MadeTranslated, tr.Name(), scoreTranslated
	if res.Partial {
		made = MadeTranslatedPartial
	}
	row := Subtitle{FileID: fc.file.ID, Language: req.Language, Forced: req.Forced, HearingImpaired: src.hi, Place: "beside",
		SiteName: &name, SiteKey: &src.language, Score: &score, ScoreParts: []Part{}, Origin: "fetched", State: "present", Sync: map[string]any{}, Made: &made}
	p, err := s.save(ctx, fc, &row, subtext.WriteSRT(cues), subtext.SRT, ReadSettings(ctx, s.Store.Read()), old, by)
	if err != nil {
		return res, err
	}
	res.Path = p
	s.translateEvent(ctx, fc, req, src, res, "", by)
	return res, nil
}

// realSubtitle refuses a translation when the File already has a subtitle in
// the language. Only by hand may a machine-made one be replaced; it is then
// returned as the subtitle to hold aside.
func realSubtitle(st Status, req TranslateRequest, stops []Subtitle) (*Subtitle, error) {
	w := Want{Language: req.Language, Forced: req.Forced, HearingImpaired: HIEither}
	var old *Subtitle
	for _, h := range st.Haves {
		if !meets(w, h) {
			continue
		}
		if h.Place == "beside" && req.ByHand {
			if row := stopgapAt(stops, h.Path); row != nil {
				old = row
				continue
			}
		}
		return nil, ErrHasSubtitle
	}
	return old, nil
}

func stopgapAt(stops []Subtitle, rel string) *Subtitle {
	for i := range stops {
		if stops[i].RelativePath != nil && *stops[i].RelativePath == rel {
			return &stops[i]
		}
	}
	return nil
}

func (s *Service) translateEvent(ctx context.Context, fc *fileCtx, req TranslateRequest, src source, res TranslateResult, why string, by domain.Actor) {
	titleID, slotID, fileID := fc.title.ID, fc.file.SlotIDs[0], fc.file.ID
	want := fmtWant(Want{Language: req.Language, Forced: req.Forced})
	from := LanguageName(src.language)
	data := map[string]any{"file_id": fileID, "language": req.Language, "from": src.language, "source": src.kind, "lines": res.Lines,
		"translated": res.Translated, "by_hand": req.ByHand}
	group := "subtitles:" + itoa(fileID)
	var e *domain.Event
	switch {
	case why != "":
		data["reason"] = why
		e = &domain.Event{Actor: by, Kind: EventTranslateFailed, TitleID: &titleID, SlotID: &slotID, Data: data, GroupKey: group,
			Summary: fmt.Sprintf("Could not translate the %s subtitle from %s: %s", want, from, why)}
	case res.Partial:
		data["path"], data["partial"] = res.Path, true
		e = &domain.Event{Actor: by, Kind: EventTranslated, TitleID: &titleID, SlotID: &slotID, Data: data, GroupKey: group,
			Summary: fmt.Sprintf("Translated part of the %s subtitle from %s (%d of %d lines)", want, from, res.Translated, res.Lines)}
	default:
		data["path"] = res.Path
		e = &domain.Event{Actor: by, Kind: EventTranslated, TitleID: &titleID, SlotID: &slotID, Data: data, GroupKey: group,
			Summary: fmt.Sprintf("Translated the %s subtitle from %s", want, from)}
	}
	s.event(ctx, e)
}

// findSource resolves the request's source, or finds one: a text file beside
// the video, then a text track inside it (by hand, or when the owner lets
// them go without asking), then the best site file in the source language.
// Without a source language, by hand takes any text subtitle, English first.
func (s *Service) findSource(ctx context.Context, fc *fileCtx, st Status, req TranslateRequest, tr Translator, stops []Subtitle) (source, error) {
	w := Want{Language: req.Language, Forced: req.Forced, HearingImpaired: HINormal}
	langOK := func(l string) bool { return l != req.Language && (req.From == "" || LangMatches(req.From, l)) }
	switch req.Source {
	case SourceSite:
		var row SiteRow
		if err := database.Get(ctx, s.Store.Read(), "subtitle_sites", req.SiteID, &row); err != nil {
			return source{}, ErrNoSource
		}
		return source{kind: SourceSite, language: req.SiteLanguage, hi: req.SiteHI, site: row,
			cand: sites.Candidate{Site: row.Name, Key: req.SiteKey, Link: req.SiteLink, Format: req.SiteFormat, Language: req.SiteLanguage}}, nil
	case SourceBeside, SourceBuiltIn, "":
	default:
		return source{}, fmt.Errorf("source must be %s, %s or %s", SourceBeside, SourceBuiltIn, SourceSite)
	}
	haves := sourceOrder(st.Haves, w)
	picture := false
	for _, h := range haves {
		switch {
		case req.Source == SourceBeside && h.Path != req.Path, req.Source == SourceBuiltIn && (h.Place != "built_in" || h.Track != req.Track):
			continue
		case req.Source == "" && (!langOK(h.Language) || !sourceFits(w, h.Forced, h.HearingImpaired)):
			continue
		}
		switch h.Place {
		case "beside":
			if textFormat(h.Path) != "" && stopgapAt(stops, h.Path) == nil {
				return source{kind: SourceBeside, language: h.Language, hi: h.HearingImpaired, path: h.Path}, nil
			}
		case "built_in":
			if h.Picture || !container.TextCodec(h.Codec) {
				picture = true
				continue
			}
			if req.ByHand || tr.BuiltInAuto {
				return source{kind: SourceBuiltIn, language: h.Language, hi: h.HearingImpaired, track: h.Track}, nil
			}
		}
	}
	if picture && req.Source != "" {
		return source{}, ErrPicture
	}
	if req.Source == "" && req.From != "" {
		if src, ok := s.siteSource(ctx, fc, req, w); ok {
			return src, nil
		}
	}
	if picture {
		return source{}, ErrPicture
	}
	return source{}, ErrNoSource
}

// siteSource asks the usable sites once for the source language and keeps
// the best file made by a person that passes the pass mark.
func (s *Service) siteSource(ctx context.Context, fc *fileCtx, req TranslateRequest, w Want) (source, bool) {
	rows, err := usable(ctx, s.Store.Read(), now())
	if err != nil {
		return source{}, false
	}
	v := fc.videoFacts()
	q := sites.Query{Kind: fc.item.Kind, Title: v.Title, Year: v.Year, Season: v.Season, Episode: v.Episode, ReleaseName: v.ReleaseName,
		IMDb: fc.title.SourceIDs["imdb"], TMDB: fc.title.SourceIDs["tmdb"], TVDB: fc.title.SourceIDs["tvdb"], Languages: []string{req.From}}
	turned := s.turnedDown(ctx, fc.file.ID)
	pass := ReadSettings(ctx, s.Store.Read()).PassMark(fc.item.Kind)
	var best *Scored
	var bestRow SiteRow
	for order, r := range rows {
		k, _ := sites.KindOf(r.Kind)
		if (fc.item.Kind == "movie" && !k.Movies) || (fc.item.Kind == "episode" && !k.Shows) {
			continue
		}
		site, err := s.client(ctx, r)
		if err != nil {
			continue
		}
		cands, err := site.Search(ctx, q)
		if err != nil {
			s.pause(ctx, r, err)
			continue
		}
		for _, c := range cands[:min(len(cands), MaxCandidatesPerSite)] {
			c.Site = r.Name
			if c.MachineMade != "" || !LangMatches(req.From, c.Language) || turned[c.Site+"\x00"+c.Key] || !sourceFits(w, c.Forced, c.HearingImpaired) {
				continue
			}
			sc := Scored{Candidate: c, Priority: r.Priority, Order: order, Score: ScoreCandidate(v, Want{Language: req.From, HearingImpaired: HIEither, Forced: c.Forced}, c)}
			if sc.Score.Refused != "" || sc.Score.Total < pass {
				continue
			}
			if best == nil || (sc.Score.Total > best.Score.Total) {
				best, bestRow = &sc, r
			}
		}
	}
	if best == nil {
		return source{}, false
	}
	return source{kind: SourceSite, language: best.Language, hi: best.HearingImpaired, site: bestRow, cand: best.Candidate}, true
}

// readSource reads a source's cues: a beside file through the safe reader, a
// built-in track through the MKV reader, or a site file through the usual
// download checks.
func (s *Service) readSource(ctx context.Context, fc *fileCtx, src source) ([]subtext.Cue, error) {
	switch src.kind {
	case SourceBuiltIn:
		cues, err := container.ExtractText(ctx, fc.video(), src.track)
		if errors.Is(err, container.ErrNotText) {
			return nil, ErrPicture
		}
		return cues, err
	case SourceSite:
		site, err := s.client(ctx, src.site)
		if err != nil {
			return nil, err
		}
		if !s.countDownload(ctx, src.site) {
			return nil, errors.New("the site's downloads for today are used up")
		}
		data, err := site.Download(ctx, src.cand)
		if err != nil {
			s.pause(ctx, src.site, err)
			return nil, err
		}
		entries, err := subtext.Unpack(data, src.cand.Key+"."+src.cand.Format)
		if err != nil {
			return nil, err
		}
		e := pickEntry(entries)
		text, err := subtext.Clean(e.Data, e.Format)
		if err != nil {
			return nil, err
		}
		return subtext.ParseCues(text, e.Format)
	}
	full := filepath.Join(fc.lib, filepath.FromSlash(src.path))
	if !importer.InsideReal(fc.lib, full) {
		return nil, fmt.Errorf("%s is outside the Library folder", src.path)
	}
	fi, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > subtext.MaxText {
		return nil, errors.New("the subtitle file is not a regular file under 5 MB")
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	format := textFormat(src.path)
	text, err := subtext.Clean(b, format)
	if err != nil {
		return nil, err
	}
	return subtext.ParseCues(text, format)
}
