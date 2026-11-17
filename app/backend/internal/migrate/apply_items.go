package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

func (r *run) neverAdd(it Item) (string, string) {
	n := it.NeverAdd
	db := r.m.Store.Read()
	var on bool
	switch n.Kind {
	case "movie":
		on, _ = library.OnNeverAdd(r.ctx, db, n.IDs["tmdb"])
	case "show":
		on, _ = library.ShowOnNeverAdd(r.ctx, db, n.IDs)
	default:
		on, _ = library.ArtistOnNeverAdd(r.ctx, db, n.IDs["musicbrainz"])
	}
	if on {
		return ResAlready, "already on the Never-add list"
	}
	if r.dry {
		return ResCreate, "added to the Never-add list"
	}
	row := mediafiles.NeverAdd{Kind: n.Kind, SourceIDs: n.IDs, Name: it.Name, Reason: "imported from the old app's exclusions"}
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		var err error
		if id, err = database.Insert(tx, "never_add", &row, r.by); err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "never_add", id, "", "")
	})
	return result(err, ResCreated, "added to the Never-add list")
}

// A moved Source or Downloader is always created paused and without
// local-network reach, whatever its keys: the owner checks each one, turns
// on "reach the local network" where it belongs, and resumes it. The
// link-local and metadata refusal of the dial policies stays either way.
func (r *run) source(it Item) (string, string) {
	s := it.Source
	if r.id("SELECT id FROM sources WHERE name = ?", it.Name) != 0 {
		return ResAlready, "a Source with this name is here"
	}
	what := s.Format + " at " + s.URL + ", paused until you check and resume it"
	if r.dry {
		return ResCreate, what
	}
	local := false
	prio := s.Priority
	in := sources.Input{Name: it.Name, Format: s.Format, Protocol: s.Protocol, URL: s.URL, Kinds: s.Kinds, Categories: s.Categories,
		MinSeeders: s.MinSeeders, Priority: &prio, AllowLocal: &local}
	if key := r.prowlarrKey(s.URL); key != "" {
		in.APIKey = key
		what += "; uses the key of the Prowlarr you imported from"
	}
	_, err := sources.Add(r.ctx, r.m.Store, r.m.Secrets, in, r.by, func(tx *sql.Tx, id int64) error {
		if _, err := tx.Exec("UPDATE sources SET state = 'disabled' WHERE id = ?", id); err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "source", id, "", "")
	})
	return result(err, ResCreated, what)
}

func (r *run) downloader(it Item) (string, string) {
	d := it.Downloader
	maps, dropped := safeMaps(d.PathMap, r.m.Store.Dir())
	// Checked here, not in the plan, so the plan stays the same.
	unseen := ""
	if u := unseenMaps(maps); u != "" {
		unseen = "; not visible here yet (check your mounts): " + u
	}
	if have := r.sameDownloader(d); have != "" {
		if len(maps) > 0 {
			// Nothing is changed on a Downloader that is already here.
			return ResAlready, "a Downloader at this address is here; add these folder maps to it yourself if it needs them: " + mapList(maps) + unseen
		}
		return ResAlready, "a Downloader at this address is here"
	}
	name := it.Name
	if r.id("SELECT id FROM downloaders WHERE name = ?", name) != 0 {
		name = clip(name+" (imported)", 80)
	}
	what := d.Adapter + " at " + d.URL + ", paused until you check and resume it; it downloads into its own holyarrs category"
	if len(maps) > 0 {
		what += "; folder maps: " + mapList(maps)
	}
	if len(dropped) > 0 {
		what += "; not imported (they lead into the HolyArrs config folder): " + clip(strings.Join(dropped, ", "), 500)
	}
	what += unseen
	if r.dry {
		return ResCreate, what
	}
	proto := "torrent"
	if d.Adapter == "sabnzbd" || d.Adapter == "nzbget" {
		proto = "usenet"
	}
	row := domain.Downloader{Name: name, Adapter: d.Adapter, Protocol: proto, URL: d.URL, Category: "holyarrs", Priority: d.Priority,
		PathMap: maps, AfterImport: "leave", State: "disabled", Username: d.Username, AllowLocal: false, ClearFinished: true}
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		id, err := database.Insert(tx, "downloaders", &row, r.by)
		if err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "downloader", id, "", "")
	})
	return result(err, ResCreated, what)
}

// sameDownloader names a Downloader here that reaches the same app as d
// (same kind, host and port; a path such as /sabnzbd does not count), or "".
func (r *run) sameDownloader(d *DownloaderPlan) string {
	have, err := database.Select[domain.Downloader](r.ctx, r.m.Store.Read(), "downloaders", "removed = 0 AND adapter = ? ORDER BY id", d.Adapter)
	if err != nil {
		return ""
	}
	want := domain.Downloader{Adapter: d.Adapter, URL: d.URL}
	for _, h := range have {
		if domain.SameClient(h, want) {
			return h.Name
		}
	}
	return ""
}

// setNaming puts one moved naming value on s.
func setNaming(s *mediafiles.NamingSettings, k, v string) {
	switch k {
	case "rename_on_import":
		s.RenameOnImport = v == "true"
	case "colon":
		s.Colon = v
	case "illegal":
		s.Illegal = v
	case "file_pattern":
		s.FilePattern = v
	case "folder_pattern":
		s.FolderPattern = v
	case "show_folder_pattern":
		s.ShowFolderPattern = v
	case "season_folder_pattern":
		s.SeasonFolderPattern = v
	case "episode_pattern":
		s.EpisodePattern = v
	case "daily_pattern":
		s.DailyPattern = v
	case "anime_pattern":
		s.AnimePattern = v
	case "multi_episode_style":
		s.MultiEpisodeStyle = v
	}
}

func (r *run) naming(it Item) (string, string) {
	cur, err := mediafiles.ReadNaming(r.ctx, r.m.Store.Read())
	if err != nil {
		return ResFailed, err.Error()
	}
	next := cur
	for k, v := range it.Naming {
		setNaming(&next, k, v)
	}
	if reflect.DeepEqual(next, cur) {
		return ResAlready, "naming is already set this way"
	}
	if err := next.Validate(); err != nil {
		return ResFailed, "these naming settings do not fit together: " + err.Error()
	}
	if r.dry {
		return ResChange, it.Detail + "; nothing on disk is renamed"
	}
	before, _ := json.Marshal(cur)
	after, _ := json.Marshal(it.Naming)
	err = mediafiles.PutNamingSettings(r.ctx, r.m.Store, next, r.by, func(tx *sql.Tx) error {
		return r.markTx(tx, it.Key, "naming", 0, string(before), string(after))
	})
	return result(err, ResChanged, it.Detail+"; nothing on disk is renamed")
}

func (r *run) subtitles(it Item) (string, string) {
	sp := it.Subtitles
	if sp.SiteKind != "" {
		if r.id("SELECT id FROM subtitle_sites WHERE kind = ?", sp.SiteKind) != 0 {
			return ResAlready, "a subtitle site of this kind is here"
		}
		if r.dry {
			return ResCreate, "subtitle site " + sp.SiteKind
		}
		name := it.Name
		if r.id("SELECT id FROM subtitle_sites WHERE name = ?", name) != 0 {
			name = clip(name+" (imported)", 100)
		}
		on := !sp.NeedsAccount
		_, err := subtitles.AddSite(r.ctx, r.m.Store, nil, subtitles.SiteInput{Name: name, Kind: sp.SiteKind, Enabled: &on}, r.by, r.markWith(it.Key, "subtitle_site"))
		return result(err, ResCreated, "subtitle site "+sp.SiteKind)
	}
	cur := subtitles.ReadSettings(r.ctx, r.m.Store.Read())
	next := cur
	next.Languages = nil
	for _, w := range sp.Languages {
		next.Languages = append(next.Languages, subtitles.Want{Language: w.Language, HearingImpaired: w.HearingImpaired, Forced: w.Forced})
	}
	if reflect.DeepEqual(next.Languages, cur.Languages) {
		return ResAlready, "these are already the default languages"
	}
	if err := next.Validate(); err != nil {
		return ResFailed, "the languages were not taken: " + err.Error()
	}
	if r.dry {
		return ResChange, "default languages: " + it.Detail
	}
	before, _ := json.Marshal(cur.Languages)
	after, _ := json.Marshal(next.Languages)
	err := subtitles.SaveSettings(r.ctx, r.m.Store, next, r.by, func(tx *sql.Tx) error {
		return r.markTx(tx, it.Key, "subtitle_languages", 0, string(before), string(after))
	})
	return result(err, ResChanged, "default languages: "+it.Detail)
}

func (r *run) person(it Item) (string, string) {
	pp := it.Person
	if pp.Role == "owner" {
		if pp.Email != "" || pp.MoviesPerWeek != nil || pp.ShowsPerWeek != nil {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupPeople, From: it.From, What: it.Name + ": contact and ask limits",
				Why: "the person was matched to the existing HolyArrs owner, so the owner's current email and limits are kept"})
		}
		r.people[it.Key] = r.owner
		return ResMatched, "is you, as you confirmed"
	}
	if pp.Choice == PersonSkip {
		// Keep the choice row when someone here has the same name, so the
		// owner can change it back.
		if r.choose == "" && r.id("SELECT id FROM users WHERE name = ? COLLATE NOCASE", it.Name) != 0 {
			r.choose = ChooseSameName
		}
		return ResSkipped, "you left them out; their asks come over as yours"
	}
	// Someone here signs in with the same Plex, Jellyfin or Emby account: it
	// is them, whatever their name.
	way := signInWay(pp.SignIn)
	if way != "" {
		if id, name, err := accounts.LinkedTo(r.ctx, r.m.Store.Read(), pp.SignIn, pp.SignInID); err == nil {
			r.people[it.Key] = id
			return ResMatched, "already here as " + name + ", who signs in with this " + way + " account; their requests are theirs"
		}
	}
	// A person here with the same name is never merged silently: the owner
	// chooses merge or skip on the move screen.
	if id := r.id("SELECT id FROM users WHERE name = ? COLLATE NOCASE", it.Name); id != 0 {
		if r.choose == "" {
			r.choose = ChooseSameName
		}
		if pp.Choice != PersonMerge {
			return ResSkipped, "Someone here has the same name. Until you choose, they stay behind and their requests become yours."
		}
		if pp.Email != "" || pp.MoviesPerWeek != nil || pp.ShowsPerWeek != nil {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupPeople, From: it.From, What: it.Name + ": contact and ask limits",
				Why: "the person was merged with an existing HolyArrs account, so its current email and limits are kept"})
		}
		r.people[it.Key] = id
		return ResMatched, "merged with the person of this name here, as you chose; their asks are theirs"
	}
	if pp.Choice == PersonMerge {
		// Same person merges by name: a numeric-id person whose name nobody
		// here has can only be left out.
		if r.choose == ChooseNumericID {
			return ResSkipped, "No one here has this name, so Same person can't be used. Choose Leave out."
		}
		r.choose = ChooseMergeGone
		return ResSkipped, "The person you picked is no longer here. Choose again."
	}
	email := pp.Email
	if email != "" && !alerts.CheckAddress(email) {
		r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupPeople, From: it.From, What: it.Name + ": email",
			Why: "not one plain email address, so the email is not imported"})
		email = ""
	}
	if r.dry {
		r.people[it.Key] = -1
		if way != "" {
			return ResCreate, "comes over as a " + pp.Role + "; turns on at their first " + way + " sign-in"
		}
		return ResCreate, "comes over turned off as a " + pp.Role + " until you invite them"
	}
	perms, _ := json.Marshal(pp.Perms)
	// Made turned off; with a sign-in link, their first sign-in through it
	// turns them on after that sign-in's checks (accounts.LinkImported).
	u := domain.User{Name: it.Name, Role: pp.Role, PasswordHash: "", State: "disabled", Perms: string(perms)}
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		var err error
		if id, err = database.Insert(tx, "users", &u, r.by); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO user_ask_settings (user_id, per_week, movies_per_week, shows_per_week, email) VALUES (?, NULL, ?, ?, ?)",
			id, pp.MoviesPerWeek, pp.ShowsPerWeek, email); err != nil {
			return err
		}
		if way != "" {
			if _, _, err = accounts.LinkImported(r.ctx, tx, id, pp.SignIn, pp.SignInID, pp.SignInName, time.Now()); err != nil {
				return err
			}
		}
		return r.markTx(tx, it.Key, "user", id, "", "")
	})
	if err == nil {
		r.people[it.Key] = id
	}
	if way != "" {
		return result(err, ResCreated, "came over as a "+pp.Role+"; turns on at their first "+way+" sign-in")
	}
	return result(err, ResCreated, "came over turned off as a "+pp.Role+" until you invite them")
}

// keepSignIn gives a person an earlier import made the Plex, Jellyfin or
// Emby sign-in they had in the old app, when they have none yet: imports
// before this kept it back. Their state and requests stay as they are: a
// person nobody changed since the import turns on at that first sign-in.
func (r *run) keepSignIn(it Item, id int64) (string, string) {
	pp := it.Person
	way := signInWay(pp.SignIn)
	other, name, err := accounts.LinkedTo(r.ctx, r.m.Store.Read(), pp.SignIn, pp.SignInID)
	switch {
	case err == nil && other == id:
		return ResAlready, "imported before"
	case err == nil:
		return ResAlready, "imported before; their " + way + " account signs in as " + name + " here, so it stays that way"
	}
	if r.dry {
		return ResChange, "imported before; gets their " + way + " sign-in"
	}
	var how string
	err = r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		var err error
		how, _, err = accounts.LinkImported(r.ctx, tx, id, pp.SignIn, pp.SignInID, pp.SignInName, time.Now())
		return err
	})
	if err == nil && how != accounts.LinkAdded {
		return ResAlready, "imported before"
	}
	return result(err, ResChanged, "imported before; got their "+way+" sign-in")
}

func (r *run) personID(key string) int64 {
	if key == "" {
		return 0
	}
	if id, ok := r.people[key]; ok {
		return id
	}
	if allowed, planned := r.personMarked[key]; planned && !allowed {
		return 0
	}
	if stable, planned := r.personStable[key]; planned && !stable {
		if id, ok := r.madeThisMove(key); ok {
			return id
		}
		return 0
	}
	if id, ok := r.made(key); ok {
		return id
	}
	return 0
}

func (r *run) createAsk(it Item, row *domain.Ask, by int64, asked time.Time, expectedAskerFP string, decidedBy int64, expectedDeciderFP string) (int64, error) {
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		askerFP, err := userIdentity(r.ctx, tx, by)
		if err != nil || askerFP != expectedAskerFP {
			return errUndoReplaced
		}
		if decidedBy > 0 {
			deciderFP, err := userIdentity(r.ctx, tx, decidedBy)
			if err != nil || deciderFP != expectedDeciderFP {
				return errUndoReplaced
			}
		}
		if id, err = database.Insert(tx, "asks", row, r.by); err != nil {
			return err
		}
		// A moved ask never counts against the person's weekly limit.
		if _, err = tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)", id, by, database.FormatTime(asked)); err != nil {
			return err
		}
		baseline, _ := json.Marshal(askBaseline{UserID: by, AskedAt: database.FormatTime(asked), UserFingerprint: askerFP})
		return r.markTx(tx, it.Key, "ask", id, "", string(baseline))
	})
	return id, err
}

func (r *run) ask(it Item) (string, string) {
	a := it.Ask
	if a.Kind == "show" && !r.m.partOn("shows") || a.Kind == "movie" && !r.m.partOn("movies") {
		return ResSkipped, "its part is off"
	}
	tmdb := a.IDs["tmdb"]
	existing := r.id(`SELECT id FROM asks WHERE kind = ? AND four_k = ? AND state IN ('asked', 'approved') AND (ref = ? OR (tmdb_id != '' AND tmdb_id = ?))`, a.Kind, a.FourK, a.Ref, tmdb)
	var titleID int64
	if a.Kind == "movie" {
		titleID, _ = library.TitleByTMDB(r.ctx, r.m.Store.Read(), tmdb)
	} else {
		titleID, _ = library.ShowByIDs(r.ctx, r.m.Store.Read(), a.IDs)
	}
	by := r.personID(a.By)
	if by == -2 {
		return ResSkipped, "HolyArrs can't tell who asked for this. Choose Same person or Leave out for that person, then import again."
	}
	note := a.Note
	if by == 0 {
		by = r.owner
		if a.ByName != "" {
			note = strings.TrimSpace(note + "; asked by " + a.ByName + " in " + a.App)
		}
	}
	if existing != 0 {
		expectedAskFP, askIdentityErr := askIdentity(r.ctx, r.m.Store.Read(), existing)
		expectedUserFP, userIdentityErr := r.expectedUserIdentity(a.By, by)
		if askIdentityErr != nil || userIdentityErr != nil {
			return ResFailed, "the existing request owner identity could not be verified"
		}
		if a.Approved || a.Decider != "" || a.DecidedAt != "" {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupRequests, From: it.From, What: it.Name + ": old decision",
				Why: "Requests entry already exists, so its current decision and decision time are kept"})
		}
		if by > 0 && r.id("SELECT ask_id FROM ask_people WHERE ask_id = ? AND user_id = ?", existing, by) != 0 {
			if !r.dry {
				err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
					askFP, err := askIdentity(r.ctx, tx, existing)
					if err != nil || askFP != expectedAskFP {
						return errUndoReplaced
					}
					userFP, err := userIdentity(r.ctx, tx, by)
					if err != nil || userFP != expectedUserFP {
						return errUndoReplaced
					}
					var askedAt string
					if err := tx.QueryRowContext(r.ctx, "SELECT asked_at FROM ask_people WHERE ask_id = ? AND user_id = ? AND counted=0", existing, by).Scan(&askedAt); err != nil {
						return err
					}
					mark, _ := json.Marshal(askPersonMark{AskID: existing, UserID: by, AskedAt: askedAt, AskFingerprint: askFP, UserFingerprint: userFP})
					return r.markTx(tx, it.Key, "ask_claim", existing, string(mark), "")
				})
				if err != nil {
					return ResFailed, err.Error()
				}
			}
			return ResAlready, "already on the Requests page for this person"
		}
		if r.dry {
			return ResCreate, "adds this person's request to the existing request"
		}
		if by <= 0 {
			return ResFailed, "its asker was not imported"
		}
		asked, timeErr := r.sourceOrMoveTime(a.AskedAt)
		if timeErr != nil {
			return ResFailed, "the import's stable request time was not found: " + timeErr.Error()
		}
		err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
			askFP, err := askIdentity(r.ctx, tx, existing)
			if err != nil || askFP != expectedAskFP {
				return errUndoReplaced
			}
			userFP, err := userIdentity(r.ctx, tx, by)
			if err != nil || userFP != expectedUserFP {
				return errUndoReplaced
			}
			if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)", existing, by, database.FormatTime(asked)); err != nil {
				return err
			}
			mark, _ := json.Marshal(askPersonMark{AskID: existing, UserID: by, AskedAt: database.FormatTime(asked), AskFingerprint: askFP, UserFingerprint: userFP})
			return r.markTx(tx, it.Key, "ask_person", 0, string(mark), "")
		})
		return result(err, ResCreated, "added this person's request to the existing request")
	}
	state := "asked"
	// An approved 4K request is approved here only when the 4K copy is.
	if a.FourK && titleID != 0 && r.id("SELECT id FROM versions WHERE title_id = ? AND four_k = 1", titleID) == 0 {
		titleID = 0
	}
	if a.Approved && titleID != 0 {
		state = "approved"
	} else if a.Approved {
		note = strings.TrimSpace(note + "; approved in " + a.App + ": approve it again here to add it")
	}
	note = strings.Trim(note, "; ")
	what := map[string]string{"asked": "waiting", "approved": "approved"}[state]
	if r.dry {
		if by < 0 {
			return ResCreate, what + ", by the new person"
		}
		return ResCreate, what
	}
	if by <= 0 {
		return ResFailed, "its asker was not imported"
	}
	expectedAskerFP, identityErr := r.expectedUserIdentity(a.By, by)
	if identityErr != nil || expectedAskerFP == "" {
		return ResFailed, "the request owner identity could not be verified"
	}
	asked, timeErr := r.sourceOrMoveTime(a.AskedAt)
	if timeErr != nil {
		return ResFailed, "the import's stable request time was not found: " + timeErr.Error()
	}
	row := domain.Ask{Kind: a.Kind, TMDBID: tmdb, Name: it.Name, State: state, AskedBy: by, AskedAt: asked, Note: clip(note, 300),
		Ref: a.Ref, SourceIDs: a.IDs, Seasons: append([]int{}, a.Seasons...), Albums: []domain.AskAlbum{}, FourK: a.FourK}
	if a.Kind == "movie" {
		row.Seasons = []int{}
	}
	requests.ImportedQuality(&row)
	if titleID != 0 {
		row.TitleID = &titleID
	}
	r.askDetails(&row)
	var decidedBy int64
	var expectedDeciderFP string
	if state == "approved" {
		decidedBy = r.personID(a.Decider)
		if decidedBy <= 0 {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupRequests, From: it.From, What: it.Name + ": decision owner", Why: "the source approver could not be matched, so HolyArrs records the current owner as the approver"})
			decidedBy = r.owner
		}
		expectedDeciderFP, identityErr = r.expectedUserIdentity(a.Decider, decidedBy)
		if identityErr != nil || expectedDeciderFP == "" {
			return ResFailed, "the request decision owner identity could not be verified"
		}
		decided, timeErr := r.sourceOrMoveTime(a.DecidedAt)
		if timeErr != nil {
			return ResFailed, "the import's stable decision time was not found: " + timeErr.Error()
		}
		row.DecidedBy, row.DecidedAt = &decidedBy, &decided
	}
	_, err := r.createAsk(it, &row, by, asked, expectedAskerFP, decidedBy, expectedDeciderFP)
	return result(err, ResCreated, what)
}

// askDetails gives a request its name, year and poster from the details
// source, as a request made here has them. A failed lookup keeps the
// stand-in name and stops lookups for this run.
func (r *run) askDetails(row *domain.Ask) {
	if r.noLookup || r.m.Library == nil || r.m.Library.Metadata == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	d, err := requests.LookupAskDetails(ctx, r.m.Library, row.Kind, row.Ref)
	cancel()
	if err != nil || d.Name == "" {
		r.noLookup = true
		return
	}
	if row.Name == "" || row.Name == requests.PlaceholderName(row.Ref) {
		row.Name, r.found = d.Name, d.Name
	}
	if d.Year > 0 {
		row.Year = &d.Year
	}
	row.Poster = d.Poster
}

func (r *run) alert(it Item) (string, string) {
	a := it.Alert
	if r.id("SELECT id FROM alerts WHERE name = ? COLLATE NOCASE", it.Name) != 0 {
		return ResAlready, "an Alert with this name is here"
	}
	what := a.Kind + ", off until you type " + it.Needs
	if r.dry {
		return ResCreate, what
	}
	row := domain.Alert{Name: it.Name, Kind: a.Kind, Enabled: false, Moments: append([]string{}, alerts.Moments...), SMTPHost: a.SMTPHost,
		SMTPPort: 587, SMTPSecurity: "starttls", SMTPFrom: a.SMTPFrom, SMTPUser: a.SMTPUser, StaffTo: a.StaffTo, WebhookAuth: "sign"}
	if a.SMTPPort > 0 {
		row.SMTPPort = a.SMTPPort
	}
	if a.SMTPSecurity != "" {
		row.SMTPSecurity = a.SMTPSecurity
	}
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		id, err := database.Insert(tx, "alerts", &row, r.by)
		if err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "alert", id, "", "")
	})
	return result(err, ResCreated, what)
}

// prowlarrKey is the kept Prowlarr key when addr goes through that same
// Prowlarr (scheme, host and port), else "".
func (r *run) prowlarrKey(addr string) string {
	if r.m.Secrets == nil {
		return ""
	}
	if r.prowlarr == nil {
		c, ok, err := secretsfile.ReadMoveCredential(r.ctx, r.m.Store, r.m.Secrets, string(Prowlarr))
		if err != nil || !ok {
			c = secretsfile.MoveCredential{}
		}
		r.prowlarr = &c
	}
	c := r.prowlarr
	if c.APIKey == "" || c.Address == "" || originOf(c.Address) == "" || originOf(c.Address) != originOf(addr) {
		return ""
	}
	return c.APIKey
}

// originOf is scheme://host:port in lower case, with the default port filled.
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[strings.ToLower(u.Scheme)]
	}
	return strings.ToLower(u.Scheme + "://" + u.Hostname() + ":" + port)
}
