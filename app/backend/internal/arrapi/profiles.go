package arrapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The profile routes take the profiles Profilarr pushes and keep each as
// Pick rules of their own. The profile is kept as sent, so its qualities,
// made-up group ids and raw scores list back unchanged. Pick rules made in
// HolyArrs and the ready-made ones are listed but never changed or
// removed here.

// wireProfile is a profile as Profilarr sends it.
type wireProfile struct {
	Name                  string      `json:"name"`
	UpgradeAllowed        bool        `json:"upgradeAllowed"`
	Cutoff                int64       `json:"cutoff"`
	Items                 []wireItem  `json:"items"`
	MinFormatScore        int         `json:"minFormatScore"`
	CutoffFormatScore     int         `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int         `json:"minUpgradeFormatScore"`
	FormatItems           []wireScore `json:"formatItems"`
	Language              *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"language"`
}

// wireItem is one quality, or a group of them (its own id and name).
type wireItem struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Quality *wireQuality `json:"quality"`
	Items   []wireItem   `json:"items"`
	Allowed bool         `json:"allowed"`
}

// wireScore is one format's score in a profile.
type wireScore struct {
	Format int64  `json:"format"`
	Name   string `json:"name"`
	Score  int    `json:"score"`
}

// sideKind is the title kind of a side's Pick rules.
func (h *handler) sideKind() string {
	if h.side == "sonarr" {
		return "show"
	}
	return "movie"
}

// qualityName names a profile entry's quality by this side's table when
// the entry left the name out.
func (h *handler) qualityName(q *wireQuality) string {
	if q == nil {
		return ""
	}
	if q.Name != "" {
		return q.Name
	}
	for _, s := range sideQualities[h.side] {
		if s.ID == q.ID {
			return s.Name
		}
	}
	return ""
}

// oldProfile reads a sent profile as Pick rules read an old app's: the
// allowed qualities with groups opened, the cutoff's, scores and language.
func (h *handler) oldProfile(p wireProfile) library.OldProfile {
	out := library.OldProfile{UpgradeAllowed: p.UpgradeAllowed, MinScore: p.MinFormatScore,
		CutoffScore: p.CutoffFormatScore, MinUpgradeScore: p.MinUpgradeFormatScore}
	cutFound := false
	for _, it := range p.Items {
		var names []string
		id := it.ID
		if len(it.Items) > 0 {
			for _, s := range it.Items {
				if n := h.qualityName(s.Quality); n != "" {
					names = append(names, n)
				}
			}
		} else if n := h.qualityName(it.Quality); n != "" {
			names, id = []string{n}, it.Quality.ID
		}
		if it.Allowed {
			out.Qualities = append(out.Qualities, names...)
		}
		if id == p.Cutoff && !cutFound {
			out.CutoffQualities, cutFound = names, true
		}
	}
	for _, q := range out.CutoffQualities {
		out.FourK = out.FourK || strings.HasPrefix(library.QualityStep(q), "2160p")
	}
	// Sonarr profiles carry no language: Sonarr takes every language there.
	if h.side == "radarr" && p.Language != nil {
		out.Language = p.Language.Name
		if n, ok := sideLanguages["radarr"][p.Language.ID]; ok && out.Language == "" {
			out.Language = n
		}
		out.LanguageCode = languageCode(out.Language)
	}
	return out
}

// pickRulesOf builds the Pick rules body a sent profile asks for: the
// old-app reading plus the traits of the formats it scores. notes say
// what did not carry over.
func (h *handler) pickRulesOf(ctx context.Context, p wireProfile) (mediafiles.PickRulesBody, []string, error) {
	body, notes := library.OldProfileRules(h.oldProfile(p))
	formats, err := listFormatRows(ctx, h.Store.Read(), h.side)
	if err != nil {
		return body, nil, err
	}
	traits, err := library.PushedTraits(ctx, h.Store)
	if err != nil {
		return body, nil, err
	}
	byID := map[int64]pushedFormat{}
	for _, f := range formats {
		byID[f.ID] = f
	}
	seen := map[int64]bool{}
	tps := []mediafiles.TraitPoints{}
	names := map[string]string{}
	for _, s := range p.FormatItems {
		f, ok := byID[s.Format]
		if s.Score == 0 || !ok || seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		if f.TraitID == 0 || traits[f.TraitID] == "" {
			notes = append(notes, fmt.Sprintf("%s is left out: %s", clipName(f.Name), cmpOr(f.Skipped, "it became no trait")))
			continue
		}
		pts := library.ClampPoints(s.Score)
		if pts != s.Score {
			notes = append(notes, fmt.Sprintf("%s scored %d there; points here stop at %d", clipName(f.Name), s.Score, pts))
		}
		key := mediafiles.OwnTraitKey(f.TraitID)
		names[key] = f.Name
		tps = append(tps, mediafiles.TraitPoints{Trait: key, Points: pts})
	}
	if len(tps) > maxPushedTraits {
		sort.SliceStable(tps, func(i, j int) bool { return abs(tps[i].Points) > abs(tps[j].Points) })
		for _, tp := range tps[maxPushedTraits:] {
			notes = append(notes, clipName(names[tp.Trait])+" is left out: Pick rules hold at most 64 traits; the strongest scores were kept")
		}
		tps = tps[:maxPushedTraits]
	}
	body.Traits = tps
	return body, notes, nil
}

// maxPushedTraits matches the Pick rules limit on listed traits.
const maxPushedTraits = 64

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// listedProfiles are the Pick rules this side lists, with the profile an
// app sent for each (when one did). Music rules and rules sent to the
// other side are left out.
func (h *handler) listedProfiles(ctx context.Context) ([]mediafiles.PickRules, map[int64]pushedProfile, error) {
	ps, err := database.Select[mediafiles.PickRules](ctx, h.Store.Read(), "pick_rules", "1=1 ORDER BY id")
	if err != nil {
		return nil, nil, err
	}
	pushed, err := listProfileRows(ctx, h.Store.Read())
	if err != nil {
		return nil, nil, err
	}
	out := ps[:0]
	for _, p := range ps {
		if mediafiles.IsMusicRules(p.Body) {
			continue
		}
		if pp, ok := pushed[p.ID]; ok && pp.Side != h.side {
			continue
		}
		out = append(out, p)
	}
	return out, pushed, nil
}

// listedName is the name a side lists Pick rules under: the sent one.
func listedName(p mediafiles.PickRules, pushed map[int64]pushedProfile) string {
	if pp, ok := pushed[p.ID]; ok {
		return pp.Name
	}
	return p.Name
}

// profileNameTaken reports whether other listed Pick rules (not id) go by
// this name on this side.
func (h *handler) profileNameTaken(ctx context.Context, name string, id int64) (bool, error) {
	ps, pushed, err := h.listedProfiles(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if p.ID != id && listedName(p, pushed) == name {
			return true, nil
		}
	}
	return false, nil
}

// qualityProfiles lists the Pick rules for movies and shows as quality
// profiles, with every field Profilarr reads back.
func (h *handler) qualityProfiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ps, pushed, err := h.listedProfiles(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	formats, err := listFormatRows(ctx, h.Store.Read(), h.side)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		if pp, ok := pushed[p.ID]; ok {
			out = append(out, profileShape(p.ID, pp, formats))
			continue
		}
		out = append(out, h.rulesShape(p, formats))
	}
	writeJSON(w, http.StatusOK, out)
}

// qualityProfile answers one profile as the list shows it; apps such as
// Seerr and Recyclarr read a single profile by id.
func (h *handler) qualityProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ps, pushed, err := h.listedProfiles(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, p := range ps {
		if p.ID != id {
			continue
		}
		formats, err := listFormatRows(ctx, h.Store.Read(), h.side)
		if err != nil {
			internalError(w, err)
			return
		}
		if pp, ok := pushed[p.ID]; ok {
			writeJSON(w, http.StatusOK, profileShape(p.ID, pp, formats))
			return
		}
		writeJSON(w, http.StatusOK, h.rulesShape(p, formats))
		return
	}
	notFound(w)
}

// profileShape is a sent profile as listed: as sent, under its Pick rules
// id, with a raw score for every format this side has (0 when it gave none).
func profileShape(id int64, pp pushedProfile, formats []pushedFormat) map[string]any {
	m := asMap(pp.Body)
	var sent wireProfile
	_ = json.Unmarshal([]byte(pp.Body), &sent)
	score := map[int64]int{}
	for _, s := range sent.FormatItems {
		score[s.Format] = s.Score
	}
	m["id"], m["name"] = id, pp.Name
	m["formatItems"] = formatItems(formats, func(f pushedFormat) int { return score[f.ID] })
	if m["items"] == nil {
		m["items"] = []any{}
	}
	return m
}

func formatItems(formats []pushedFormat, score func(pushedFormat) int) []map[string]any {
	out := make([]map[string]any, 0, len(formats))
	for _, f := range formats {
		out = append(out, map[string]any{"format": f.ID, "name": f.Name, "score": score(f)})
	}
	return out
}

// rulesShape writes Pick rules made in HolyArrs as a profile: this side's
// qualities in ladder order, best last, allowed where their step is.
func (h *handler) rulesShape(p mediafiles.PickRules, formats []pushedFormat) map[string]any {
	b := p.Body
	allowed := map[string]bool{}
	for _, s := range b.Steps {
		allowed[s.Quality] = s.Allowed
	}
	qs := slices.Clone(sideQualities[h.side])
	sort.SliceStable(qs, func(i, j int) bool {
		return slices.Index(mediafiles.Ladder, library.QualityStep(qs[i].Name)) < slices.Index(mediafiles.Ladder, library.QualityStep(qs[j].Name))
	})
	items := make([]map[string]any, 0, len(qs))
	cutoff := int64(0)
	for _, q := range qs {
		step := library.QualityStep(q.Name)
		items = append(items, map[string]any{"quality": q, "items": []any{}, "allowed": allowed[step]})
		if step != "" && step == b.GoodEnoughAt {
			cutoff = q.ID
		}
	}
	points := map[string]int{}
	for _, tp := range b.Traits {
		points[tp.Trait] = tp.Points
	}
	good := 0
	if b.GoodEnoughPoints != nil {
		good = *b.GoodEnoughPoints
	}
	m := map[string]any{"id": p.ID, "name": p.Name, "upgradeAllowed": true, "cutoff": cutoff, "items": items,
		"minFormatScore": b.MinPoints, "cutoffFormatScore": good, "minUpgradeFormatScore": b.UpgradeMinPoints,
		"formatItems": formatItems(formats, func(f pushedFormat) int {
			if f.TraitID == 0 {
				return 0
			}
			return points[mediafiles.OwnTraitKey(f.TraitID)]
		})}
	if h.side == "radarr" {
		lang := map[string]any{"id": -1, "name": "Any"}
		switch {
		case len(b.Language.Wanted) == 1:
			if id, n, ok := languageByCode("radarr", b.Language.Wanted[0]); ok {
				lang = map[string]any{"id": id, "name": n}
			}
		case len(b.Language.Wanted) == 0 && b.Language.OriginalOK:
			lang = map[string]any{"id": -2, "name": "Original"}
		}
		m["language"] = lang
	}
	return m
}

func (h *handler) addProfile(w http.ResponseWriter, r *http.Request) { h.saveProfile(w, r, 0) }

func (h *handler) putProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.saveProfile(w, r, id)
}

// ownPushed reads Pick rules this side may change: ones an app sent to it.
// Others answer 409 (made in HolyArrs, or ready-made) or 404 (missing,
// music, or sent to the other side). It returns false after answering.
func (h *handler) ownPushed(ctx context.Context, w http.ResponseWriter, id int64) (mediafiles.PickRules, pushedProfile, bool) {
	var p mediafiles.PickRules
	err := database.Get(ctx, h.Store.Read(), "pick_rules", id, &p)
	if err == nil && mediafiles.IsMusicRules(p.Body) {
		err = database.ErrNotFound
	}
	if err != nil {
		internalError(w, err)
		return p, pushedProfile{}, false
	}
	pp, err := profileRow(ctx, h.Store.Read(), id)
	switch {
	case errors.Is(err, database.ErrNotFound):
		message(w, http.StatusConflict, "these Pick rules were made in HolyArrs; only profiles sent through this API can be changed or removed here")
		return p, pp, false
	case err != nil:
		internalError(w, err)
		return p, pp, false
	case pp.Side != h.side:
		notFound(w)
		return p, pp, false
	}
	return p, pp, true
}

// saveProfile makes Pick rules from a sent profile (id 0) or replaces the
// ones it made before.
func (h *handler) saveProfile(w http.ResponseWriter, r *http.Request, id int64) {
	if !mayWriteRules(w, r) {
		return
	}
	ctx := r.Context()
	var old mediafiles.PickRules
	var oldRow pushedProfile
	if id != 0 {
		var ok bool
		if old, oldRow, ok = h.ownPushed(ctx, w, id); !ok {
			return
		}
	}
	var in wireProfile
	raw, ok := readRaw(w, r, &in)
	if !ok {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "Name must not be empty", AttemptedValue: in.Name})
		return
	}
	if taken, err := h.profileNameTaken(ctx, in.Name, id); err != nil || taken {
		if err != nil {
			internalError(w, err)
			return
		}
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "Must be unique", AttemptedValue: in.Name})
		return
	}
	body, notes, err := h.pickRulesOf(ctx, in)
	if err != nil {
		internalError(w, err)
		return
	}
	cfg, err := h.loadConfig(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	cfg.fold(&body)
	body.Synced = &mediafiles.SyncedProfile{From: mediafiles.SourceArrAPI, App: h.side, ProfileID: h.side + ":" + in.Name,
		Name: clipName(in.Name), At: h.now().UTC().Format(time.RFC3339)}
	by := actor(r)
	var saved mediafiles.PickRules
	if id != 0 {
		body.Tags = old.Body.Tags
		name := old.Name
		if oldRow.Name != in.Name {
			name, err = h.freeRulesName(ctx, in.Name, id)
		}
		if err == nil {
			saved, err = library.UpdatePickRules(ctx, h.Store, id, library.PickRulesInput{Name: name, Kinds: old.Kinds, Body: body}, by)
		}
	} else {
		var name string
		if name, err = h.freeRulesName(ctx, in.Name, 0); err == nil {
			saved, err = library.CreatePickRules(ctx, h.Store, library.PickRulesInput{Name: name, Kinds: []string{h.sideKind()}, Body: body}, by)
		}
	}
	if err != nil {
		h.rulesError(w, err)
		return
	}
	pp := pushedProfile{PickRulesID: saved.ID, Side: h.side, Name: in.Name, Body: string(raw), Notes: notes}
	if err := saveProfileRow(ctx, h.Store, pp, by); err != nil {
		if id == 0 {
			_ = library.DeletePickRules(ctx, h.Store, saved.ID, by)
		}
		internalError(w, err)
		return
	}
	formats, err := listFormatRows(ctx, h.Store.Read(), h.side)
	if err != nil {
		internalError(w, err)
		return
	}
	code := http.StatusAccepted
	if id == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, profileShape(saved.ID, pp, formats))
}

// freeRulesName finds a Pick rules name no other rules (not self) hold:
// the sent name, then with the side added, then numbered.
func (h *handler) freeRulesName(ctx context.Context, name string, self int64) (string, error) {
	base := clipName(strings.TrimSpace(name))
	ps, err := database.Select[mediafiles.PickRules](ctx, h.Store.Read(), "pick_rules", "id != ?", self)
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, p := range ps {
		taken[strings.ToLower(p.Name)] = true
	}
	for n := 1; n < 100; n++ {
		try := base
		switch {
		case n == 2:
			try = base + " (" + h.side + ")"
		case n > 2:
			try = fmt.Sprintf("%s (%s %d)", base, h.side, n-1)
		}
		if !taken[strings.ToLower(try)] {
			return try, nil
		}
	}
	return "", library.ErrPickRulesName
}

// rulesError answers a refused Pick rules change.
func (h *handler) rulesError(w http.ResponseWriter, err error) {
	var fe *mediafiles.FieldError
	switch {
	case errors.As(err, &fe):
		refuse(w, failure{PropertyName: fe.Field, ErrorMessage: fe.Text})
	case errors.Is(err, library.ErrPickRulesName):
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "Must be unique"})
	case errors.Is(err, library.ErrPickRulesInUse), errors.Is(err, library.ErrPresetLocked):
		message(w, http.StatusConflict, err.Error())
	default:
		internalError(w, err)
	}
}

// deleteProfile removes Pick rules an app sent to this side. Rules still in
// use answer 500, which Profilarr reads as "in use" and skips, as the old
// apps refuse to remove a profile titles use.
func (h *handler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if !mayWriteRules(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, _, ok := h.ownPushed(ctx, w, id); !ok {
		return
	}
	err := library.DeletePickRules(ctx, h.Store, id, actor(r))
	if errors.Is(err, library.ErrPickRulesInUse) {
		message(w, http.StatusInternalServerError, "these Pick rules are still used by titles, Library folders, lists or settings")
		return
	}
	if err != nil {
		h.rulesError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
