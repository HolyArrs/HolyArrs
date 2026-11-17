package subtitles

import (
	"context"
	"path"
	"sort"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/container"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Scores HolyArrs writes for subtitles a machine made. They sit under every
// usual pass mark, so a file made by a person that passes always replaces
// them on a later search.
const (
	scoreTranslated        = 50
	scoreAITranslated      = 40
	scoreMachineTranslated = 30
)

// stopgapScore caps a machine-made candidate's score: AI-translated files
// above machine-translated ones, both low.
func stopgapScore(made string, total int) int {
	switch made {
	case sites.AITranslated:
		return min(total, scoreAITranslated)
	case sites.MachineTranslated:
		return min(total, scoreMachineTranslated)
	}
	return total
}

func madeRank(made string) int {
	if made == sites.AITranslated {
		return 0
	}
	return 1
}

// textFormat reports whether a subtitle file can be read as cues.
func textFormat(rel string) string {
	switch f := strings.TrimPrefix(strings.ToLower(path.Ext(rel)), "."); f {
	case subtext.SRT, subtext.VTT, subtext.ASS, subtext.SSA:
		return f
	}
	return ""
}

// sourceFits reports whether a subtitle can stand in for a want once
// translated: the same forced switch, and hearing impaired when the want asks
// for it.
func sourceFits(w Want, forced, hi bool) bool {
	return forced == w.Forced && (w.HearingImpaired != HIOnly || hi)
}

// sourceOrder sorts possible sources best first: English before other
// languages, the hearing-impaired kind the want asks for before the other,
// then files beside the video before built-in tracks, in track order.
func sourceOrder(haves []Have, w Want) []Have {
	out := append([]Have(nil), haves...)
	hi := w.HearingImpaired == HIOnly
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Language == "en") != (b.Language == "en") {
			return a.Language == "en"
		}
		if (a.HearingImpaired == hi) != (b.HearingImpaired == hi) {
			return a.HearingImpaired == hi
		}
		if a.Place != b.Place {
			return a.Place == "beside"
		}
		return a.Track < b.Track
	})
	return out
}

// stopgap runs when no file made by a person passed for a missing want. It
// queues a translation when the owner's translator and a source are there,
// and otherwise saves the best machine-made file a site's switch let in.
func (s *Service) stopgap(ctx context.Context, sc *searchCtx, t target, choices []Scored, out *Outcome) {
	if sc.translator.Ready() && !sc.opt.NoTranslate && t.want.TranslateFrom != "" {
		req, why := s.planTranslation(ctx, sc, t.want)
		if req != nil {
			if err := s.QueueTranslate(ctx, sc.fc.file.ID, *req); err == nil {
				out.Result, out.Reason = "translating", ""
				return
			} else {
				why = err.Error()
			}
		}
		out.Reason = why
	}
	var made []Scored
	for _, c := range choices {
		if c.MachineMade != "" && c.Score.Refused == "" {
			made = append(made, c)
		}
	}
	// The choices are ranked by score already; AI-translated files go first.
	sort.SliceStable(made, func(i, j int) bool { return madeRank(made[i].MachineMade) < madeRank(made[j].MachineMade) })
	for _, c := range made {
		p, err := s.fetchAndSave(ctx, sc.fc, t, c, sc.bySite[c.Site], sc.set, sc.by)
		if err != nil {
			out.Reason = err.Error()
			continue
		}
		out.Result, out.Path, out.Score, out.Site, out.Reason, out.Made = "saved", p, stopgapScore(c.MachineMade, c.Score.Total), c.Site, "", c.MachineMade
		return
	}
}

// planTranslation picks the source for translating a missing want from what
// the File has and what the sites already offered, without asking anything
// new: a text file beside the video, then a text track inside it when the
// owner lets those go without asking, then the best site file in the source
// language that passes the pass mark. It returns why when there is none.
func (s *Service) planTranslation(ctx context.Context, sc *searchCtx, w Want) (*TranslateRequest, string) {
	from := w.TranslateFrom
	req := &TranslateRequest{Language: w.Language, Forced: w.Forced, From: from}
	stops := map[string]bool{}
	if rows, err := stopgapRows(ctx, s.Store.Read(), sc.fc.file.ID); err == nil {
		for _, r := range rows {
			if r.RelativePath != nil {
				stops[*r.RelativePath] = true
			}
		}
	}
	haves := sourceOrder(sc.status.Haves, w)
	for _, h := range haves {
		if h.Place == "beside" && LangMatches(from, h.Language) && sourceFits(w, h.Forced, h.HearingImpaired) && textFormat(h.Path) != "" && !stops[h.Path] {
			req.Source, req.Path = SourceBeside, h.Path
			return req, ""
		}
	}
	why := "no " + from + " subtitle to translate from"
	for _, h := range haves {
		if h.Place != "built_in" || !LangMatches(from, h.Language) || !sourceFits(w, h.Forced, h.HearingImpaired) {
			continue
		}
		switch {
		case h.Picture || !container.TextCodec(h.Codec):
			why = ErrPicture.Error()
		case !sc.translator.BuiltInAuto:
			why = "the built-in " + from + " track is translated by hand only"
		default:
			req.Source, req.Track = SourceBuiltIn, h.Track
			return req, ""
		}
	}
	var cands []Scored
	for _, c := range sc.all {
		if c.MachineMade != "" || !LangMatches(from, c.Language) || sc.turned[c.Site+"\x00"+c.Key] || !sourceFits(w, c.Forced, c.HearingImpaired) {
			continue
		}
		c.Score = ScoreCandidate(sc.video, Want{Language: from, HearingImpaired: HIEither, Forced: c.Forced}, c.Candidate)
		cands = append(cands, c)
	}
	Rank(cands)
	if len(cands) > 0 && cands[0].Score.Refused == "" && cands[0].Score.Total >= sc.pass {
		c := cands[0]
		req.Source, req.SiteID, req.SiteKey, req.SiteLink, req.SiteFormat = SourceSite, sc.bySite[c.Site].ID, c.Key, c.Link, c.Format
		req.SiteLanguage, req.SiteHI, req.SiteScore = c.Language, c.HearingImpaired, c.Score.Total
		return req, ""
	}
	return nil, why
}

// searchCtx carries what one search knows into its stopgap step.
type searchCtx struct {
	fc         *fileCtx
	status     Status
	set        Settings
	opt        SearchOptions
	pass       int
	video      Video
	all        []Scored
	bySite     map[string]SiteRow
	turned     map[string]bool
	translator Translator
	by         domain.Actor
}
