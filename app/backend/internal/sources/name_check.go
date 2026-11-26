package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Name check: a person pastes a Release name and sees how HolyArrs
// reads it, which Library title it matches and how each copy's Pick rules
// judge it. It runs the same reader, matcher and judge an automatic search
// uses, stores nothing and never sends anything to a downloader.

// nameCheckIn is the request body.
type nameCheckIn struct {
	Name string `json:"name"`
	// Size in bytes, optional: size limits are judged only when given.
	Size int64 `json:"size"`
	// PickRulesID, optional: judge with these Pick rules instead of the
	// matched title's copies (no file on disk is taken into account).
	PickRulesID int64 `json:"pick_rules_id"`
	// TitleID, optional: match only against this Library title.
	TitleID int64 `json:"title_id"`
}

// NameTitle is the Library title a name matched.
type NameTitle struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Year   int    `json:"year,omitempty"`
	Sure   bool   `json:"sure"`
	Detail string `json:"detail"`
}

// NameCopy is one judged copy (or the chosen Pick rules).
type NameCopy struct {
	Version    string   `json:"version,omitempty"`
	PickRules  string   `json:"pick_rules"`
	Episodes   string   `json:"episodes,omitempty"`
	Album      string   `json:"album,omitempty"`
	Outcome    string   `json:"outcome"`
	Passed     bool     `json:"passed"`
	Points     int      `json:"points"`
	Step       string   `json:"step,omitempty"`
	GoodEnough bool     `json:"good_enough"`
	HasFile    bool     `json:"has_file"`
	Reasons    []Reason `json:"reasons"`
	// slots are the Slots the verdict covers, for the episode labels.
	slots []int64
}

// NameCheck is the answer.
type NameCheck struct {
	Name   string             `json:"name"`
	Parsed releasename.Result `json:"parsed"`
	Read   *ReleaseRead       `json:"read,omitempty"`
	Title  *NameTitle         `json:"title"`
	Copies []NameCopy         `json:"copies"`
	// Note says why nothing was judged: "no_title" (no match and no Pick
	// rules chosen), "no_album" (an artist matched but no album of it).
	Note string `json:"note,omitempty"`
}

func (h *Handlers) nameCheck(w http.ResponseWriter, r *http.Request) {
	var in nameCheckIn
	if !webserver.Decode(w, r, &in, `body is {"name": string, "size": int, "pick_rules_id": int, "title_id": int}; only name is needed`) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 1000 {
		webserver.Fail(w, http.StatusBadRequest, "name must be 1 to 1000 characters")
		return
	}
	in.Size = max(in.Size, 0)
	s := h.search()
	out, status, err := s.CheckName(r.Context(), in)
	switch {
	case err != nil && status == 0:
		webserver.Internal(w, "sources", err)
	case err != nil:
		webserver.Fail(w, status, err.Error())
	default:
		for i := range out.Copies {
			if ids := out.Copies[i].slots; len(ids) > 0 && h.Catalog != nil && out.Title != nil && out.Title.Kind == "show" {
				out.Copies[i].Episodes = h.Catalog.EpisodeLabels(r.Context(), ids)
			}
		}
		webserver.WriteJSON(w, http.StatusOK, out)
	}
}

// CheckName reads, matches and judges one name. A non-zero status with an
// error is the person's mistake; a zero status with an error is the server's.
func (s *Searcher) CheckName(ctx context.Context, in nameCheckIn) (out NameCheck, status int, err error) {
	db := s.Store.Read()
	where, args := "state != 'removed'", []any{}
	if in.TitleID > 0 {
		where, args = "id = ? AND state != 'removed'", []any{in.TitleID}
	}
	titles, err := database.Select[mediafiles.Title](ctx, db, "titles", where, args...)
	if err != nil {
		return out, 0, err
	}
	if in.TitleID > 0 && len(titles) == 0 {
		return out, http.StatusNotFound, errors.New("no such Title")
	}
	refs := titleRefs(titles)
	hasShow := slices.ContainsFunc(refs, func(r mediafiles.TitleRef) bool { return r.Kind == "show" })
	hasArtist := slices.ContainsFunc(refs, func(r mediafiles.TitleRef) bool { return r.Kind == "artist" })
	item := Item{Title: in.Name, Size: in.Size}
	m := matchFeedItem(item, refs, hasShow, hasArtist)

	var ti *mediafiles.Title
	if m.Title != nil {
		for i := range titles {
			if titles[i].ID == m.Title.ID {
				ti = &titles[i]
			}
		}
	}
	// The name is read the way the title's search reads it.
	hints := releasename.Hints{Size: in.Size}
	switch {
	case ti != nil && ti.Kind == "show":
		hints.Show = true
	case ti != nil && ti.Kind == "artist":
		hints.Music = true
	}
	out.Name = in.Name
	out.Parsed = releasename.Parse(in.Name, hints)
	out.Read = readOfResult(out.Parsed)
	out.Copies = []NameCopy{}
	if ti != nil {
		kind := ti.Kind
		if kind == "" {
			kind = "movie"
		}
		out.Title = &NameTitle{ID: ti.ID, Name: ti.Name, Kind: kind, Sure: m.Conf == releasename.Sure, Detail: m.Detail}
		if ti.Year != nil {
			out.Title.Year = *ti.Year
		}
	}

	f := found{item: item}
	if in.PickRulesID > 0 {
		var pr mediafiles.PickRules
		if err := mediafiles.LoadPickRules(ctx, db, in.PickRulesID, &pr); err != nil {
			return out, http.StatusNotFound, errors.New("no such Pick rules")
		}
		ref := mediafiles.TitleRef{Name: out.Parsed.Title.Value, Year: out.Parsed.Year.Value}
		match := mediafiles.Match{Title: &ref, Conf: releasename.Sure, Detail: "judged as if it were the title it names"}
		if m.Title != nil {
			match = m
		}
		rel := mediafiles.ReleaseFacts{Name: in.Name, Parse: out.Parsed, Size: in.Size}
		v := mediafiles.Judge(rel, mediafiles.SlotSet{Match: match, SlotIDs: []int64{0}, Out: true, Checks: Checks(ctx, db)}, pr.Body)
		out.Copies = append(out.Copies, nameCopy(v, "", pr.Name, false))
		return out, 0, nil
	}
	if ti == nil {
		out.Note = "no_title"
		return out, 0, nil
	}
	vers, err := mediafiles.VersionsOfTitle(ctx, db, ti.ID)
	if err != nil {
		return out, 0, err
	}
	switch out.Title.Kind {
	case "show":
		rules := s.showRules(ctx)
		for _, ver := range vers {
			set, err := s.loadShow(ctx, *ti, ver)
			if err != nil {
				return out, 0, err
			}
			has := false
			for _, e := range set.eps {
				if e.sl != nil {
					set.target[e.sl.ID] = true
				}
			}
			_, vs, err := s.judgeShow(ctx, set, []found{f}, false, false, rules)
			if err != nil {
				return out, 0, err
			}
			for _, e := range set.eps {
				if e.sl != nil && e.cur != nil && slices.Contains(vs[0].SlotIDs, e.sl.ID) {
					has = true
				}
			}
			c := nameCopy(vs[0], ver.Label, set.pr.Name, has)
			c.slots = vs[0].SlotIDs
			out.Copies = append(out.Copies, c)
		}
	case "artist":
		parts, err := mediafiles.PartsOfTitle(ctx, db, ti.ID)
		if err != nil {
			return out, 0, err
		}
		rules := s.musicRules(ctx)
		for _, ver := range vers {
			for _, part := range parts {
				if part.Kind != "album" {
					continue
				}
				set, err := s.loadAlbum(ctx, *ti, ver, part)
				if err != nil {
					return out, 0, err
				}
				has := false
				for _, t := range set.tracks {
					if t.sl != nil {
						set.target[t.sl.ID] = true
						has = has || t.cur != nil
					}
				}
				_, vs := s.judgeAlbum(ctx, set, []found{f}, false, false, rules)
				// Only the album the name is for is shown.
				if matchFailed(vs[0]) {
					continue
				}
				c := nameCopy(vs[0], ver.Label, set.pr.Name, has)
				c.Album = part.Name
				out.Copies = append(out.Copies, c)
			}
		}
		if len(out.Copies) == 0 {
			out.Note = "no_album"
		}
	default:
		for _, ver := range vers {
			slots, err := mediafiles.VersionSlotsOfTitle(ctx, db, ver.ID, ti.ID)
			if err != nil {
				return out, 0, err
			}
			if len(slots) == 0 {
				continue
			}
			c, err := s.loadSlot(ctx, slots[0].ID)
			if err != nil {
				return out, 0, err
			}
			_, vs, err := s.judgeAll(ctx, c, []found{f}, true, false, false)
			if err != nil {
				return out, 0, err
			}
			out.Copies = append(out.Copies, nameCopy(vs[0], ver.Label, c.pr.Name, c.cur != nil))
		}
	}
	return out, 0, nil
}

// matchFailed says the verdict refused the name at the match step.
func matchFailed(v mediafiles.Verdict) bool {
	for _, r := range v.Reasons {
		if r.Step == "match" && r.Effect == "fail" {
			return true
		}
	}
	return false
}

// nameCopy turns a verdict into one judged copy.
func nameCopy(v mediafiles.Verdict, version, rules string, hasFile bool) NameCopy {
	c := NameCopy{Version: version, PickRules: rules, Outcome: v.Outcome, Passed: v.Passed, Points: v.Points,
		GoodEnough: v.GoodEnough, HasFile: hasFile, Reasons: reasonsOf(v.Reasons)}
	if v.Step != nil {
		c.Step = v.Step.Name
	}
	return c
}

// reasonsOf puts verdict lines in the words the release lists use.
func reasonsOf(rs []mediafiles.Reason) []Reason {
	out := make([]Reason, 0, len(rs))
	for _, r := range rs {
		text := r.Detail
		// The total line already says its points.
		if r.Points != 0 && r.Code() != "points.total" {
			text = fmt.Sprintf("%s (%+d points)", text, r.Points)
		}
		out = append(out, Reason{Code: r.Code(), Effect: r.Effect, Text: text})
	}
	return out
}

// readOfResult is readOf for a fresh reading.
func readOfResult(p releasename.Result) *ReleaseRead {
	r := &ReleaseRead{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value,
		Audio: p.Audio.Format, Channels: p.Audio.Channels, Atmos: p.Audio.Atmos, Languages: p.Languages, Edition: p.Edition,
		Group: p.Group.Value, Proper: p.Fix > 0}
	if m := p.Music; m != nil {
		r.Format, r.Bitrate, r.BitDepth, r.Lossless = m.Format, m.Bitrate, m.BitDepth, m.Lossless
	}
	return r
}
