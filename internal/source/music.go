package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/parse"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// KindSearchAlbum is the Job kind that searches one album of an artist for
// one Version (stage 3.3.3): one Release fills every track Slot it holds.
const KindSearchAlbum = "search_album"

// Music search settings (group what_to_pick), each a decision in
// decisions/log-autonomous.md (3.3.3).
const (
	SettingMissingMusic   = "missing_search_music"    // the nightly and bulk searches cover albums (default true)
	SettingDiscography    = "music_discography_packs" // a whole-discography Release may fill an album (default false)
	SettingTrackTolerance = "album_track_tolerance"   // files a Release may lack against the album's tracks (default 0; -1 off)
	SettingExtraFiles     = "album_extra_files"       // files a bare file count is taken to hold besides tracks (log, cue, cover; default 0)
)

// extraFileExts are files an album Release holds besides its tracks; they
// are not counted against the album's tracks.
var extraFileExts = map[string]bool{"log": true, "cue": true, "m3u": true, "m3u8": true, "jpg": true, "jpeg": true, "png": true,
	"gif": true, "bmp": true, "webp": true, "nfo": true, "txt": true, "pdf": true, "sfv": true, "md5": true, "accurip": true}

// trackFiles is how many files of a Release count as tracks: the file list
// without extras when the Source names the files, otherwise the count less
// the extras it is taken to hold.
func trackFiles(names []string, count, extra int) int {
	if len(names) == 0 {
		return max(count-max(extra, 0), 0)
	}
	n := 0
	for _, f := range names {
		f = strings.ToLower(strings.TrimRight(f, ". "))
		if i := strings.LastIndexByte(f, '.'); i < 0 || !extraFileExts[f[i+1:]] {
			n++
		}
	}
	return n
}

type musicRules struct {
	discography bool
	tolerance   int // -1: no file count check
	extraFiles  int // files a bare count is taken to hold besides tracks
}

func (s *Searcher) musicRules(ctx context.Context) musicRules {
	db := s.Store.Read()
	r := musicRules{discography: Setting(ctx, db, SettingDiscography) == "true"}
	if n, err := strconv.Atoi(Setting(ctx, db, SettingTrackTolerance)); err == nil && n >= -1 {
		r.tolerance = n
	}
	if n, err := strconv.Atoi(Setting(ctx, db, SettingExtraFiles)); err == nil && n >= 0 {
		r.extraFiles = n
	}
	return r
}

// handlesMusic says a Source is set up for music (no kinds means every kind).
func handlesMusic(src domain.Source) bool {
	return len(src.Kinds) == 0 || slices.Contains(src.Kinds, "music")
}

// musicCategories are the categories a Source uses for music.
func musicCategories(src domain.Source) []int {
	if c := src.Categories["music"]; len(c) > 0 {
		return c
	}
	return DefaultCategories["music"]
}

// trackSlot is one track of an album with its Slot in one Version.
type trackSlot struct {
	it   domain.Item
	sl   *domain.Slot // nil when the track has no Slot in this Version
	cur  *judge.Current
	kept bool // the track, its album, the Version and the artist are kept up to date
	disc int
}

// albumSet is an album search: the album's tracks in one Version and the
// Slots asked for.
type albumSet struct {
	ti     domain.Title
	ver    domain.Version
	pr     domain.PickRules
	part   domain.Part
	albums []judge.AlbumRef // every album of the artist, to tell them apart
	tracks []trackSlot
	target map[int64]bool // Slot ids searched for
	notOut map[int64]bool // target Slots not out yet (a hand search may skip the gate)
}

// AlbumLabel names an album for lists: "Artist – Album".
func AlbumLabel(artist, album string) string { return artist + " – " + album }

func (set albumSet) labels() map[int64]string {
	out := map[int64]string{}
	for _, t := range set.tracks {
		if t.sl != nil {
			out[t.sl.ID] = set.part.Name
		}
	}
	return out
}

// loadAlbum reads an album's tracks with their Slots in one Version.
func (s *Searcher) loadAlbum(ctx context.Context, ti domain.Title, ver domain.Version, part domain.Part) (albumSet, error) {
	db := s.Store.Read()
	set := albumSet{ti: ti, ver: ver, part: part, target: map[int64]bool{}, notOut: map[int64]bool{}}
	if err := store.Get(ctx, db, "pick_rules", ver.PickRulesID, &set.pr); err != nil {
		return set, err
	}
	parts, err := store.Select[domain.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL", ti.ID)
	if err != nil {
		return set, err
	}
	for _, p := range parts {
		a := judge.AlbumRef{PartID: p.ID, Name: p.Name}
		if p.ReleaseDate != nil {
			a.Year = p.ReleaseDate.Year()
		}
		set.albums = append(set.albums, a)
	}
	its, err := store.Select[domain.Item](ctx, db, "items", `part_id = ? AND kind = 'track' AND dropped_at IS NULL
		ORDER BY json_extract(numbers, '$.disc'), json_extract(numbers, '$.track')`, part.ID)
	if err != nil {
		return set, err
	}
	slots, err := store.Select[domain.Slot](ctx, db, "slots", "version_id = ? AND item_id IN (SELECT id FROM items WHERE part_id = ?)", ver.ID, part.ID)
	if err != nil {
		return set, err
	}
	byItem := map[int64]*domain.Slot{}
	for i := range slots {
		byItem[slots[i].ItemID] = &slots[i]
	}
	albumKept := part.KeepUpToDate && part.Included && part.DroppedAt == nil
	for _, it := range its {
		t := trackSlot{it: it, sl: byItem[it.ID], disc: 1}
		if it.Numbers.Disc != nil {
			t.disc = *it.Numbers.Disc
		}
		t.kept = it.KeepUpToDate && albumKept && ver.KeepUpToDate && ti.KeepUpToDate
		if t.sl != nil {
			t.cur = currentOf(ctx, db, *t.sl, set.pr)
		}
		set.tracks = append(set.tracks, t)
	}
	return set, nil
}

func (set albumSet) ref() judge.TitleRef {
	return judge.TitleRef{ID: set.ti.ID, Name: set.ti.Name, Aliases: set.ti.Aliases, Kind: "artist"}
}

// judgeAlbum judges every Release for an album search and ranks them.
func (s *Searcher) judgeAlbum(ctx context.Context, set albumSet, all []found, hand, skipGate bool, rules musicRules) ([]judge.Release, []judge.Verdict) {
	bad := s.dangerousSeen(ctx)
	turned := map[int64][]string{}
	turnedFor := func(slotID int64) []string {
		if l, ok := turned[slotID]; ok {
			return l
		}
		l, _ := s.turnedDown(ctx, slotID)
		turned[slotID] = l
		return l
	}
	tref := set.ref()
	checks := Checks(ctx, s.Store.Read())
	var outSince *time.Time
	if set.part.ReleaseDate != nil && (set.part.ReleaseDatePrecision == nil || *set.part.ReleaseDatePrecision == "day") {
		outSince = set.part.ReleaseDate
	}
	rels := make([]judge.Release, len(all))
	vs := make([]judge.Verdict, len(all))
	for i, f := range all {
		p := parse.Parse(f.item.Title, parse.Hints{Size: f.item.Size, Music: true})
		rels[i] = judge.Release{Name: f.item.Title, Parse: p, Size: f.item.Size, Protocol: f.item.Protocol, Seeders: f.item.Seeders,
			PublishedAt: f.item.Published, InfoHash: f.item.InfoHash, SourcePriority: f.src.Priority, Dangerous: bad.has(listingHash(f.item.InfoHash, f.item.Link), f.item.Title)}
		ss := judge.SlotSet{Match: judge.MatchTitle(p, []judge.TitleRef{tref}), MinSeeders: f.src.MinSeeders, HandSearch: hand, SkipGate: skipGate, Out: true,
			OutSince: outSince, Checks: checks}
		if ss.Match.Title != nil {
			set.lines(&ss, rels[i], f.item.Files, rules, turnedFor)
		}
		vs[i] = judge.Judge(rels[i], ss, set.pr.Body)
	}
	judge.Rank(rels, vs, set.pr.Body)
	return rels, vs
}

// lines works out which tracks and Slots an album Release fills and the
// album rules (pick-rules.md section 8.4).
func (set albumSet) lines(ss *judge.SlotSet, rel judge.Release, files int, rules musicRules, turnedFor func(int64) []string) {
	p := rel.Parse
	m := p.Music
	add := func(rule, effect, detail string) {
		ss.MatchReasons = append(ss.MatchReasons, judge.Reason{Step: "match", Rule: rule, Effect: effect, Detail: detail})
	}
	if p.Kind.Value == parse.KindDiscography {
		if !rules.discography {
			add("discography", "fail", "a whole discography; Releases with many albums are not taken (Settings > Search)")
			return
		}
		add("discography", "warn", "a whole discography, taken for "+set.part.Name)
	} else {
		a, warn, why := judge.MatchAlbum(p, set.ti.Name, set.albums)
		switch {
		case a == nil:
			add("album", "fail", why)
			return
		case a.PartID != set.part.ID:
			add("not_asked", "fail", "holds "+a.Name+", not "+set.part.Name)
			return
		}
		add("album", "pass", "holds "+set.part.Name)
		if warn != "" {
			add("album_year", "warn", warn)
		}
	}
	var covered []*trackSlot
	for i := range set.tracks {
		t := &set.tracks[i]
		if len(m.Discs) == 0 || slices.Contains(m.Discs, t.disc) {
			covered = append(covered, t)
		}
	}
	if len(covered) == 0 {
		add("disc", "fail", fmt.Sprintf("names disc %v; %s has no such disc", m.Discs, set.part.Name))
		return
	}
	if len(m.Discs) > 0 && len(covered) < len(set.tracks) {
		add("disc", "warn", fmt.Sprintf("holds disc %s only (%d of %d tracks)", joinInts(m.Discs), len(covered), len(set.tracks)))
	}
	if files > 0 || len(rel.Files) > 0 {
		files = trackFiles(rel.Files, files, rules.extraFiles)
	}
	if (files > 0 || len(rel.Files) > 0) && rules.tolerance >= 0 && files < len(covered)-rules.tolerance && !p.Has("single_image") { // an image is one file by nature; the never list says why
		add("track_count", "fail", fmt.Sprintf("holds %d track files; the album has %d tracks", files, len(covered)))
		return
	}
	for _, e := range m.Editions {
		switch e {
		case "deluxe", "expanded", "bonus":
			add("edition", "warn", "a "+e+" edition; tracks beyond the chosen Pressing are left alone")
		case "live":
			if !slices.Contains(set.part.SecondaryTypes, "live") {
				add("edition", "warn", "says live; "+set.part.Name+" is not a live album")
			}
		}
	}
	ms := 0
	for _, t := range covered {
		if t.it.LengthMS != nil {
			ms += *t.it.LengthMS
		}
	}
	if ms > 0 && p.Kind.Value != parse.KindDiscography { // a discography holds more than this album: its size is not checked
		ss.RuntimeMinutes = max(1, (ms+30_000)/60_000)
	}
	var noGain []string
	var turnedTargets []int64
	turnedAll, targetTurned := 0, false
	date := "no release date"
	if set.part.ReleaseDate != nil {
		date = set.part.ReleaseDate.Format("2 Jan 2006")
	}
	ss.GateDetail = set.part.Name + " is out (" + date + ")"
	for _, t := range covered {
		if t.sl == nil {
			continue
		}
		isTarget := set.target[t.sl.ID]
		if !isTarget && !(t.kept && slices.Contains(lookingNow, t.sl.State)) {
			continue
		}
		if l := turnedFor(t.sl.ID); slices.Contains(l, rel.Name) || rel.InfoHash != "" && slices.Contains(l, rel.InfoHash) {
			turnedAll++
			if isTarget {
				targetTurned = true
				turnedTargets = append(turnedTargets, t.sl.ID)
			}
			continue
		}
		if !judge.Gains(rel, set.pr.Body, t.cur) {
			noGain = append(noGain, trackName(t.it))
			continue
		}
		if isTarget && set.notOut[t.sl.ID] {
			ss.Out = false
			ss.GateDetail = set.part.Name + " is not out yet (" + date + ")"
		}
		ss.SlotIDs = append(ss.SlotIDs, t.sl.ID)
	}
	if len(noGain) > 0 {
		add("album_gain", "fail", fmt.Sprintf("not every track would gain: %s already %s a copy as good", shortList(noGain), hasOrHave(len(noGain))))
		ss.SlotIDs = nil
		return
	}
	switch {
	case len(ss.SlotIDs) == 0 && targetTurned:
		// Judged for those Slots, so the verdict says it was turned down.
		ss.SlotIDs, ss.TurnedDown = turnedTargets, []string{rel.Name}
	case len(ss.SlotIDs) == 0:
		add("no_gain", "fail", "no track of "+set.part.Name+" is looking for a copy")
	case turnedAll > 0:
		add("turned_down_some", "warn", fmt.Sprintf("turned down before for %d tracks; not used for those", turnedAll))
	}
}

func hasOrHave(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

func trackName(it domain.Item) string {
	if it.Name != nil && *it.Name != "" {
		return *it.Name
	}
	n := 0
	if it.Numbers.Track != nil {
		n = *it.Numbers.Track
	}
	return "track " + strconv.Itoa(n)
}

func shortList(xs []string) string {
	if len(xs) > 3 {
		return strings.Join(xs[:3], ", ") + fmt.Sprintf(" and %d more", len(xs)-3)
	}
	return strings.Join(xs, ", ")
}

func joinInts(ns []int) string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out, ", ")
}

// albumQuery is the search for one album.
func albumQuery(ti domain.Title, part domain.Part) Query {
	q := Query{Music: true, Artist: ti.Name, Album: part.Name, Title: ti.Name + " " + part.Name}
	if part.ReleaseDate != nil {
		q.Year = part.ReleaseDate.Year()
	}
	return q
}

// lookingStates for an album search: what a track Slot may be searched from.
var albumLooking = []domain.SlotState{domain.SlotWanted, domain.SlotBetterWanted, domain.SlotFailed, domain.SlotRejected, domain.SlotStalled}

// HandleAlbum searches one album of an artist for one Version: the album is
// asked for by artist and album name, every Release is judged against the
// album's track Slots, and the best is chosen (or held) for all the tracks
// it fills in one write. A Release naming one disc fills that disc only, so
// a second pick may fill the rest.
//
// Params: version_id (required), by_hand, choose (false only judges and
// stores, for a person to choose from), skip_gate, kept_only, bulk.
func (s *Searcher) HandleAlbum(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "part" {
		return errors.New("search_album needs a part subject")
	}
	db := s.Store.Read()
	by := jobActor(r)
	jobID := r.Job.ID
	var part domain.Part
	if err := store.Get(ctx, db, "parts", r.Job.Subject.ID, &part); err != nil || part.Kind != "album" {
		return fmt.Errorf("album %d: not found", r.Job.Subject.ID)
	}
	var ti domain.Title
	if err := store.Get(ctx, db, "titles", part.TitleID, &ti); err != nil {
		return err
	}
	vid, _ := r.Job.Params["version_id"].(float64)
	var ver domain.Version
	if err := store.Get(ctx, db, "versions", int64(vid), &ver); err != nil || ver.TitleID != ti.ID {
		return fmt.Errorf("search_album needs a version_id of the artist")
	}
	hand, _ := r.Job.Params["by_hand"].(bool)
	skipGate, _ := r.Job.Params["skip_gate"].(bool)
	keptOnly, _ := r.Job.Params["kept_only"].(bool)
	anyTrack := hand && !keptOnly
	show := false
	if v, ok := r.Job.Params["choose"].(bool); ok && !v {
		show = true
	}
	set, err := s.loadAlbum(ctx, ti, ver, part)
	if err != nil {
		return err
	}
	var looking []*trackSlot
	for i := range set.tracks {
		t := &set.tracks[i]
		if t.sl == nil || !(t.kept || anyTrack || show) {
			continue
		}
		notOut := t.sl.State == domain.SlotNotOutYet
		if show || slices.Contains(albumLooking, t.sl.State) || notOut && (hand && !keptOnly || skipGate) {
			looking = append(looking, t)
		}
	}
	result := map[string]any{"album": part.Name, "version_id": ver.ID}
	if len(looking) == 0 {
		result["skipped"] = "no track of " + part.Name + " is looking for a copy"
		r.SetResult(result)
		return nil
	}
	rest := func(reason string) {
		if show {
			return
		}
		ctx := context.WithoutCancel(ctx)
		for _, t := range looking {
			if !set.target[t.sl.ID] || s.slotState(ctx, t.sl.ID) != domain.SlotSearching {
				continue
			}
			to := domain.SlotWanted
			switch {
			case set.notOut[t.sl.ID]:
				to = domain.SlotNotOutYet
			case t.cur != nil || t.sl.State == domain.SlotBetterWanted:
				to = domain.SlotBetterWanted
			}
			_ = s.Store.MoveSlot(ctx, t.sl.ID, to, reason, by)
			s.publish("slot", map[string]any{"id": t.sl.ID, "state": to, "reason": reason})
		}
	}
	defer rest("the album search stopped before it finished")
	targets := []int64{}
	for _, t := range looking {
		if t.sl.State == domain.SlotNotOutYet {
			set.notOut[t.sl.ID] = true
		}
		if !show {
			if err := s.Store.MoveSlot(ctx, t.sl.ID, domain.SlotSearching, "searching "+part.Name, by); err != nil {
				continue
			}
			s.publish("slot", map[string]any{"id": t.sl.ID, "state": domain.SlotSearching})
		}
		set.target[t.sl.ID] = true
		targets = append(targets, t.sl.ID)
	}
	if len(targets) == 0 {
		result["skipped"] = "every track of " + part.Name + " got busy"
		r.SetResult(result)
		return nil
	}
	fs, failures, answered, skipped, err := s.askAll(ctx, r, albumQuery(ti, part), hand, false, ti.Tags)
	if err != nil {
		return err
	}
	if reason, data, ok, failed := noAnswer(failures, answered, skipped); ok {
		rest(reason)
		s.addEvent(ctx, by, &jobID, ti.ID, targets[0], EventSearchFailed, reason, data)
		if !failed {
			return nil
		}
		return errors.New(reason)
	}
	all := dedupe(fs)
	rels, vs := s.judgeAlbum(ctx, set, all, hand, skipGate, s.musicRules(ctx))
	now := time.Now().UTC()
	var picks []showPick
	if !show {
		picks = s.pickShow(ctx, all, vs, now)
	}
	foundBy := "auto_search"
	if hand {
		foundBy = "hand_search"
	}
	ids, err := s.storeAll(ctx, by, &jobID, foundBy, targets[0], all, rels, vs, show)
	if err != nil {
		return err
	}
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, t := range looking {
			if _, err := tx.Exec("UPDATE items SET last_searched_at = ? WHERE id = ?", store.FormatTime(now), t.it.ID); err != nil {
				return err
			}
		}
		return nil
	})
	refused := 0
	for _, v := range vs {
		if v.Outcome == "refused" {
			refused++
		}
	}
	result["releases"], result["refused"], result["sources_answered"] = len(all), refused, answered
	s.publish("releases", map[string]any{"slot_id": targets[0], "count": len(all)})
	if show {
		result["shown"] = true
		r.SetResult(result)
		return nil
	}
	if beforeSettle != nil {
		beforeSettle(targets[0])
	}
	chosen, held, took := s.settlePicks(ctx, by, &jobID, ti.ID, set.labels(), all, vs, ids, picks, "Album search")
	for _, pk := range picks {
		if !took[pk.slots[0]] {
			continue
		}
		if !pk.held && result["chosen_release_id"] == nil {
			result["chosen_release_id"] = ids[pk.idx]
		}
		if pk.held && result["held_release_id"] == nil && chosen == 0 {
			result["held_release_id"] = ids[pk.idx]
		}
	}
	result["chosen"], result["held"] = chosen, held
	left := 0
	for _, id := range targets {
		if !took[id] {
			left++
		}
	}
	if left > 0 {
		reason := fmt.Sprintf("no Release passed the Pick rules for it (%d found, %d refused)", len(all), refused)
		rest(reason)
		if chosen+held == 0 {
			s.addEvent(ctx, by, &jobID, ti.ID, targets[0], EventSearchDone, part.Name+": "+reason, nil)
		}
	}
	r.SetResult(result)
	return nil
}

// queueAlbumFor queues the album search for a track Slot (a search_slot Job
// on a track searches its whole album).
func (s *Searcher) queueAlbumFor(ctx context.Context, r *jobs.Run, c slotCtx) error {
	if s.Queue == nil || c.it.PartID == nil {
		return errors.New("searching an album needs the job queue and the track's album")
	}
	params := map[string]any{"version_id": c.ver.ID}
	key := fmt.Sprintf("search_album:%d:%d", *c.it.PartID, c.ver.ID)
	for _, k := range []string{"by_hand", "skip_gate", "bulk"} {
		if v, ok := r.Job.Params[k].(bool); ok && v {
			params[k] = true
		}
	}
	if params["by_hand"] == true {
		key = "hand_" + key
	}
	if v, ok := r.Job.Params["choose"].(bool); ok && !v {
		params["choose"] = false
		key = fmt.Sprintf("show_album:%d:%d", *c.it.PartID, c.ver.ID)
	}
	id, _, err := s.Queue.Enqueue(ctx, KindSearchAlbum, &domain.Subject{Type: "part", ID: *c.it.PartID}, params, key, jobActor(r))
	if err != nil {
		return err
	}
	r.SetResult(map[string]any{"album_search_job_id": id})
	return nil
}

// feedArtist judges a feed check's Releases for one artist, per album and
// Version: each Release is matched to its album and chosen (or held) for
// every looking track Slot it fills. It returns how many were chosen, held
// and newly stored.
func (s *Searcher) feedArtist(ctx context.Context, by domain.Actor, jobID *int64, cs []slotCtx, fs []found, storedID map[string]int64,
	keyOf func(found) string) (int, int, int, error) {
	chosen, held, stored := 0, 0, 0
	type key struct{ part, ver int64 }
	groups := map[key][]slotCtx{}
	var order []key
	for _, c := range cs {
		if c.it.PartID == nil {
			continue
		}
		k := key{*c.it.PartID, c.ver.ID}
		if len(groups[k]) == 0 {
			order = append(order, k)
		}
		groups[k] = append(groups[k], c)
	}
	rules := s.musicRules(ctx)
	for _, k := range order {
		group := groups[k]
		var part domain.Part
		if err := store.Get(ctx, s.Store.Read(), "parts", k.part, &part); err != nil {
			continue
		}
		set, err := s.loadAlbum(ctx, group[0].ti, group[0].ver, part)
		if err != nil {
			return chosen, held, stored, err
		}
		kept := map[int64]bool{}
		for _, t := range set.tracks {
			if t.sl != nil && t.kept {
				kept[t.sl.ID] = true
			}
		}
		heldNow := map[int64]int64{}
		cands := append([]found(nil), fs...)
		for _, c := range group {
			if !kept[c.sl.ID] {
				continue // feed checks look only for albums kept up to date
			}
			set.target[c.sl.ID] = true
			if c.sl.State == domain.SlotHeld && c.sl.HeldReleaseID != nil {
				heldNow[c.sl.ID] = *c.sl.HeldReleaseID
				dup := false
				for _, f := range cands {
					dup = dup || f.relID == *c.sl.HeldReleaseID
				}
				if f, ok := s.storedFound(ctx, *c.sl.HeldReleaseID); ok && !dup {
					cands = append(cands, f)
				}
			}
		}
		if len(set.target) == 0 {
			continue
		}
		for i := range cands {
			if id := storedID[keyOf(cands[i])]; id > 0 && cands[i].relID == 0 {
				cands[i].relID = id
			}
		}
		rels, vs := s.judgeAlbum(ctx, set, cands, false, false, rules)
		picks := s.pickShow(ctx, cands, vs, time.Now().UTC())
		first := int64(0)
		for id := range set.target {
			if first == 0 || id < first {
				first = id
			}
		}
		ids, err := s.storeAll(ctx, by, jobID, "feed", first, cands, rels, vs, false)
		if err != nil {
			return chosen, held, stored, err
		}
		for i := range cands {
			if cands[i].relID == 0 {
				storedID[keyOf(cands[i])] = ids[i]
				stored++
			}
		}
		keep := picks[:0]
		for _, pk := range picks {
			same := pk.held
			for _, id := range pk.slots {
				same = same && heldNow[id] == ids[pk.idx]
			}
			if !same {
				keep = append(keep, pk)
			}
		}
		if beforeSettle != nil {
			beforeSettle(first)
		}
		c1, h1, _ := s.settlePicks(ctx, by, jobID, set.ti.ID, set.labels(), cands, vs, ids, keep, "Feed check")
		chosen, held = chosen+c1, held+h1
	}
	return chosen, held, stored, nil
}
