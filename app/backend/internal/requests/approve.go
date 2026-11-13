package requests

import (
	"context"
	"database/sql"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"log"
	"net"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ApproveOptions are the approver's choices; zero values take Requests
// settings (then the add-a-movie defaults).
type ApproveOptions struct {
	LibraryFolderID int64  `json:"library_folder_id"`
	PickRulesID     int64  `json:"pick_rules_id"`
	Note            string `json:"note"`
	// AudioLanguages are the audio languages the approver wants on the
	// movie or show (base codes such as en or ja); empty leaves the
	// Version as it is. Music asks ignore them.
	AudioLanguages []string `json:"audio_languages"`
	// Seasons (show asks) and Albums (artist asks, MusicBrainz ids) approve
	// only part of the ask; empty approves all of it.
	Seasons []int    `json:"seasons"`
	Albums  []string `json:"albums"`
	// Episodes (show asks) keeps single episodes of seasons the ask is
	// for, besides the whole Seasons; with Seasons it is the final pick.
	Episodes []domain.AskEpisodes `json:"episodes"`
	// Songs (artist asks) keeps only some songs of albums: the album's
	// MusicBrainz id and the recording ids of the songs to keep. Once the
	// album's tracks are in the Library the other songs are switched off.
	Songs []AlbumSongs `json:"songs"`
	// Quality approves a movie or show ask as another line of what people
	// can ask for (empty: as asked). Going from the main copy to an extra
	// copy (4K) or back changes which copy is added.
	Quality string `json:"quality"`

	// chosen: the approver picked the quality or the Pick rules, so a copy
	// already there that cannot take them is said, not passed over.
	chosen bool
}

// AlbumSongs are the songs of one album an approver keeps.
type AlbumSongs struct {
	Album      string   `json:"album"`
	Recordings []string `json:"recordings"`
}

// Approve says yes to an ask: the movie is added with the chosen Library
// folder and Pick rules and, unless the setting is off, searched for.
func (s *Service) Approve(ctx context.Context, id int64, by Person, opt ApproveOptions) (domain.Ask, error) {
	a, _, err := s.decide(ctx, id, by, opt, false)
	return a, err
}

// ApproveSaying approves like Approve and also answers a line for the
// approver when the quality they picked was not used because the title
// already had that copy (empty otherwise).
func (s *Service) ApproveSaying(ctx context.Context, id int64, by Person, opt ApproveOptions) (domain.Ask, string, error) {
	return s.decide(ctx, id, by, opt, false)
}

// CopyKept is said to an approver whose quality was not used because the
// title already had that copy.
const CopyKept = "The quality you picked was not used: this title already has that copy."

func (s *Service) decide(ctx context.Context, id int64, by Person, opt ApproveOptions, auto bool) (domain.Ask, string, error) {
	a, kept, err := s.decideAsk(ctx, id, by, opt, auto)
	if kept {
		return a, CopyKept, err
	}
	return a, "", err
}

func (s *Service) decideAsk(ctx context.Context, id int64, by Person, opt ApproveOptions, auto bool) (domain.Ask, bool, error) {
	if len(opt.Note) > 500 {
		return domain.Ask{}, false, ErrNoteLength
	}
	db := s.Store.Read()
	a, err := get(ctx, db, id)
	if err != nil {
		return a, false, err
	}
	if a.State != domain.AskAsked && a.State != domain.AskTurnedDown {
		return a, false, ErrNotOpen
	}
	if a.Kind == "artist" {
		opt.AudioLanguages, opt.Quality = nil, ""
	} else if err := library.CheckAudioLanguages(a.Kind, opt.AudioLanguages); err != nil {
		return a, false, err
	}
	narrowed, err := s.narrow(ctx, a, opt)
	if err != nil {
		return a, false, err
	}
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return a, false, err
	}
	// The line approved: the one asked for, or the approver's pick (any line
	// of the kind, shown or not), with its name as it is now.
	var line QualityLine
	hasLine := false
	if a.Kind != "artist" {
		lines, err := qualityLines(ctx, db, set, a.Kind)
		if err != nil {
			return a, false, err
		}
		want := a.Quality
		if opt.Quality != "" && opt.Quality != a.Quality {
			want, opt.chosen = opt.Quality, true
			if _, ok := lineByID(lines, want); !ok {
				return a, false, ErrQualityNotOffered
			}
		}
		line, hasLine = lineByID(lines, want)
	}
	opt.chosen = opt.chosen || opt.PickRulesID != 0
	// Claim the ask first: only one decision wins, and only the winner adds
	// the movie and tells anyone.
	now := s.now()
	var before domain.Ask
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		cur, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		quality := cur.Quality
		if hasLine {
			quality = line.ID
		}
		if cur.State == domain.AskTurnedDown {
			// Another open ask for the same thing means it was asked again.
			// Artist asks have no TMDB id: they match by MusicBrainz id.
			q, args := decideSQL, []any{cur.Kind, cur.TMDBID, quality, id}
			if cur.Kind == "artist" {
				q = decideSQL2
				args = []any{cur.SourceIDs["musicbrainz"], id}
			}
			var n int
			if err := tx.QueryRow(q, args...).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return ErrAskedAgain
			}
		}
		res, err := tx.Exec(decideSQL3, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotOpen
		}
		before = cur
		a = cur
		a.State, a.TitleID, a.DecidedBy, a.DecidedAt, a.Note, a.AutoApproved = domain.AskApproved, nil, &by.UserID, &now, strings.TrimSpace(opt.Note), auto
		if len(opt.Seasons) > 0 || len(opt.Episodes) > 0 {
			a.Seasons, a.Episodes = narrowed.Seasons, narrowed.Episodes
		}
		if len(opt.Albums) > 0 || len(opt.Songs) > 0 {
			a.Albums = narrowed.Albums
		}
		if hasLine && line.ID != cur.Quality {
			withQuality(&a, line)
		} else if hasLine {
			a.QualityName = shownName(line)
		}
		err = database.Update(tx, "asks", &a, by.actor())
		if openAskConflict(err) {
			return ErrQualityTaken
		}
		return err
	})
	if errors.Is(err, database.ErrNotFound) {
		err = ErrNotOpen
	}
	if err != nil {
		return a, false, err
	}
	// The add runs apart from the approver's request, with its own limit: a
	// closed tab or a page change ends that request, and must not stop an
	// approval halfway.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), approveAddLimit)
	defer cancel()
	// The audio languages only go on a title this approval adds: changing a
	// title already in the Library is a Library edit, not an approval.
	if len(opt.AudioLanguages) > 0 {
		var had int64
		var herr error
		if a.Kind == "show" {
			had, herr = library.ShowByIDs(ctx, db, a.SourceIDs)
		} else {
			had, herr = library.TitleByTMDB(ctx, db, a.TMDBID)
		}
		if herr != nil || had != 0 {
			opt.AudioLanguages = nil
		}
	}
	var tid int64
	var kept bool
	switch a.Kind {
	case "show":
		tid, kept, err = s.addShowForAsk(ctx, a, set, opt, by)
	case "artist":
		tid, err = s.addArtistForAsk(ctx, a, set, opt, by)
	default:
		tid, kept, err = s.addForAsk(ctx, a, set, opt, by)
	}
	if err != nil {
		return before, false, s.notAdded(ctx, before, by, err)
	}
	// The title is in the Library now. The rest must finish even if the
	// approver's request ends, or the ask stays approved with no title.
	ctx = context.WithoutCancel(ctx)
	if len(opt.AudioLanguages) > 0 { // only for a title this approval added (see above)
		if err := s.setAudio(ctx, tid, opt.AudioLanguages, by); err != nil {
			log.Printf("asks: audio languages of ask %d: %v", id, err)
		}
	}
	a.TitleID = &tid
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(decideSQL4, tid, a.ID)
		return err
	}); err != nil {
		return a, kept, err
	}
	if err := s.followAskers(ctx, a.ID, tid); err != nil {
		log.Printf("asks: follows of ask %d: %v", a.ID, err)
	}
	// A movie is searched as it is added; a show's episodes are kept up to
	// date above, then looked for at once too, not at the next missing
	// search (hours away). Artists are searched in addArtistForAsk, after
	// the MusicBrainz refresh when it brings the asked albums' tracks.
	if set.SearchOnApprove && a.Kind == "show" && s.Library != nil && s.Library.Queue != nil {
		if err := s.Library.ScanThenSearch(ctx, tid, by.actor()); err != nil {
			log.Printf("asks: search for ask %d: %v", a.ID, err)
		}
	}
	people, _ := s.people(ctx, a.ID)
	how := "approved by " + by.Name
	if auto {
		how = "approved straight away (" + by.Name + " may auto-approve)"
	}
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventAskApproved, TitleID: &tid, Summary: label(a) + ": " + how,
		Data: map[string]any{"ask_id": a.ID, "asked_by": names(people), "approved_by": by.Name, "auto": auto, "note": a.Note}, GroupKey: groupKey(a.ID)})
	text := label(a) + " was approved"
	if !auto {
		text += " by " + s.shownAs(ctx, by)
	}
	switch a.Kind {
	case "show":
		text += ". HolyArrs keeps it up to date; you will hear when the episodes you asked for are ready."
	case "artist":
		text += ". HolyArrs keeps the albums you asked for up to date; you will hear when every track is ready to listen to."
	default:
		text += ". HolyArrs is looking for it; you will hear when it is ready."
	}
	if a.Note != "" {
		text += " Note: " + a.Note
	}
	s.tell(ctx, a, people, alerts.MomentApproved, "Approved: "+label(a), text)
	s.changed(ctx, a.ID)
	if _, err := s.CheckReady(ctx); err != nil {
		log.Printf("asks: check ready: %v", err)
	}
	got, err := get(ctx, s.Store.Read(), a.ID)
	return got, kept, err
}

// approveAddLimit is how long adding an approved title may take. A lookup the
// metadata server has not kept yet can take half a minute.
const approveAddLimit = 2 * time.Minute

// ErrNotAddedSlow answers an approval whose title could not be added because
// the metadata server did not answer in time.
var ErrNotAddedSlow = errors.New("The metadata server did not answer in time. It is back in Waiting; approve it again.")

// notAdded puts an ask whose title could not be added back as it was, so it
// can be decided again, and says so: a log line, a History line with the
// reason and a live change, so the Requests page shows it waiting at once.
// It answers the error for the approver.
func (s *Service) notAdded(ctx context.Context, before domain.Ask, by Person, err error) error {
	ctx = context.WithoutCancel(ctx)
	if rerr := s.Store.Write(ctx, func(tx *sql.Tx) error {
		return database.Update(tx, "asks", &before, by.actor())
	}); rerr != nil {
		log.Printf("asks: put back ask %d: %v", before.ID, rerr)
	}
	slow := errors.Is(err, metadata.ErrServiceSlow) || errors.Is(err, context.DeadlineExceeded)
	// A connection failure keeps its raw text (with the address) in the
	// log only; History says it in plain words.
	var ne net.Error
	down := !slow && (errors.Is(err, metadata.ErrServiceUnavailable) || errors.As(err, &ne) || strings.Contains(err.Error(), "dial tcp"))
	why := err.Error()
	switch {
	case slow:
		why = "the metadata server did not answer in time"
	case down:
		why = "the metadata server could not be reached"
	}
	log.Printf("asks: %s was approved by %s but could not be added, so it is back in Waiting: %v", label(before), by.Name, err)
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventAskNotAdded, Summary: label(before) + ": could not add it, back in Waiting",
		Data: map[string]any{"ask_id": before.ID, "approved_by": by.Name, "reason": why, "slow": slow, "down": down}, GroupKey: groupKey(before.ID)})
	s.changed(ctx, before.ID)
	if slow {
		return ErrNotAddedSlow
	}
	return err
}

// setAudio puts the approver's audio languages on the title's main Version.
func (s *Service) setAudio(ctx context.Context, titleID int64, langs []string, by Person) error {
	var vid int64
	if err := s.Store.Read().QueryRowContext(ctx, setAudioSQL, titleID).Scan(&vid); err != nil {
		return err
	}
	_, err := library.EditVersion(ctx, s.Store, titleID, vid, library.VersionChange{AudioLanguages: &langs}, by.actor())
	return err
}

// addForAsk adds the asked-for movie to the Library, or finds it there. A
// 4K ask uses the 4K Pick rules and folder: on a movie already in the
// Library it adds the 4K Version (unless one is there), otherwise the
// movie is added with its main copy as the 4K copy. A plain ask on a movie
// whose only copy is the 4K one adds a plain copy beside it. It says true
// when the approver's pick was not used because that copy was there.
func (s *Service) addForAsk(ctx context.Context, a domain.Ask, set Settings, opt ApproveOptions, by Person) (int64, bool, error) {
	db := s.Store.Read()
	tid, err := library.TitleByTMDB(ctx, db, a.TMDBID)
	if err != nil {
		return 0, false, err
	}
	if err := askedRules(ctx, db, a, &opt, set.FourKLibraryFolderID, set.FourKPickRulesID, set.LibraryFolderID); err != nil {
		return 0, false, err
	}
	if tid != 0 {
		kept, err := s.addCopy(ctx, tid, a.FourK, opt, set.LibraryFolderID, set.PickRulesID, by)
		return tid, kept, err
	}
	m, err := s.Metadata.Movie(ctx, "tmdb:"+a.TMDBID)
	if err != nil {
		return 0, false, err
	}
	o := library.AddOptions{LibraryFolderID: opt.LibraryFolderID, PickRulesID: opt.PickRulesID, SearchOnAdd: set.SearchOnApprove, WhyAdded: "ask", MainFourK: a.FourK}
	if o.LibraryFolderID == 0 {
		o.LibraryFolderID = set.LibraryFolderID
	}
	if o.PickRulesID == 0 {
		o.PickRulesID = set.PickRulesID
	}
	tid, _, err = s.Library.Add(ctx, m, o, by.actor())
	var dup *library.AlreadyAddedError
	if errors.As(err, &dup) {
		// Added by someone else meanwhile: make sure this ask's copy is there.
		kept, err := s.addCopy(ctx, dup.TitleID, a.FourK, opt, set.LibraryFolderID, set.PickRulesID, by)
		return dup.TitleID, kept, err
	}
	return tid, false, err
}

// askedRules puts what the ask's line asked for on opt: the Pick rules it
// stood for when asked (while they are still there; else the settings'),
// and for a 4K ask the 4K folder too. On a main copy the approver's own
// Pick rules win.
func askedRules(ctx context.Context, db database.Querier, a domain.Ask, opt *ApproveOptions, fourKFolder, fourKRules, plainFolder int64) error {
	asked := int64(0)
	if a.QualityRulesID != 0 {
		var n int
		if err := db.QueryRowContext(ctx, askedRulesSQL, a.QualityRulesID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			asked = a.QualityRulesID
		}
	}
	if a.FourK {
		if asked != 0 {
			fourKRules = asked
		}
		return fourKPlace(ctx, db, opt, fourKFolder, fourKRules, plainFolder)
	}
	if opt.PickRulesID == 0 {
		opt.PickRulesID = asked
	}
	return nil
}

// fourKPlace puts a 4K ask's Pick rules and Library folder on opt: the
// 4K settings, else the Pick rules named 4K and the approver's folder (then
// the plain ask folder). No 4K Pick rules at all is ErrNoFourKRules.
func fourKPlace(ctx context.Context, db database.Querier, opt *ApproveOptions, folder, rules, plainFolder int64) error {
	if rules == 0 {
		var err error
		if rules, err = fourKRules(ctx, db); err != nil {
			return err
		}
	}
	if rules == 0 {
		return ErrNoFourKRules
	}
	opt.PickRulesID = rules
	if opt.LibraryFolderID == 0 {
		opt.LibraryFolderID = folder
	}
	if opt.LibraryFolderID == 0 {
		opt.LibraryFolderID = plainFolder
	}
	return nil
}

// FourKReady says a 4K ask of kind ("movie" or "show") could be approved:
// 4K Pick rules are set for it or named 4K.
func FourKReady(ctx context.Context, db querier, kind string) (bool, error) {
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return false, err
	}
	rules := set.FourKPickRulesID
	if kind == "show" {
		rules = set.ShowFourKPickRulesID
	}
	if rules == 0 {
		if rules, err = fourKRules(ctx, db); err != nil {
			return false, err
		}
	}
	return rules != 0, nil
}

// plainCopyLabel labels a plain copy added beside a 4K main copy.
const plainCopyLabel = "Main HD"

// addCopy gives a title in the Library the ask's own copy when it has none:
// the 4K Version on opt's 4K Pick rules and folder, or a plain copy on the
// approver's (else the ask settings', else the folder's default) Pick
// rules. The main copy is left as it is. When the copy is there already and
// the approver picked a quality it is not on, it says true: their pick was
// not used.
func (s *Service) addCopy(ctx context.Context, tid int64, fourK bool, opt ApproveOptions, plainFolder, plainRules int64, by Person) (bool, error) {
	db := s.Store.Read()
	if vid, err := copyOf(ctx, db, tid, fourK); err != nil {
		return false, err
	} else if vid != 0 {
		if !opt.chosen {
			return false, nil
		}
		var rules int64
		if err := db.QueryRowContext(ctx, addCopySQL, vid).Scan(&rules); err != nil {
			return false, err
		}
		return opt.PickRulesID == 0 || rules != opt.PickRulesID, nil
	}
	in := library.VersionInput{Label: mediafiles.FourKName, PickRulesID: opt.PickRulesID, LibraryFolderID: opt.LibraryFolderID, FourK: fourK}
	if !fourK {
		in.Label = plainCopyLabel
		if in.LibraryFolderID == 0 {
			in.LibraryFolderID = plainFolder
		}
		if in.LibraryFolderID == 0 {
			if err := db.QueryRowContext(ctx, mainFolderSQL, tid).Scan(&in.LibraryFolderID); err != nil {
				return false, err
			}
		}
		if in.PickRulesID == 0 {
			in.PickRulesID = plainRules
		}
		if in.PickRulesID == 0 {
			// Not the main copy's rules: those are the 4K ones.
			if err := db.QueryRowContext(ctx, plainRulesSQL, in.LibraryFolderID).Scan(&in.PickRulesID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return false, err
			}
		}
		if in.PickRulesID == 0 {
			if err := db.QueryRowContext(ctx, plainRulesSQL2, mediafiles.GoodEverydayName).Scan(&in.PickRulesID); err != nil {
				return false, err
			}
		}
	}
	v, err := library.AddVersion(ctx, s.Store, tid, in, nil, s.Library.Clock(), by.actor())
	if errors.Is(err, library.ErrFourKTaken) {
		return false, nil // added at the same moment by another approval
	}
	if err != nil {
		return false, err
	}
	var t mediafiles.Title
	if database.Get(ctx, db, "titles", tid, &t) == nil {
		s.Library.VersionEvent(ctx, t, "Added the "+v.Label+" Version", v, by.actor())
	}
	return false, nil
}

// TurnDown says no to an ask, with a note for the people who asked.
func (s *Service) TurnDown(ctx context.Context, id int64, by Person, note string) (domain.Ask, error) {
	note = strings.TrimSpace(note)
	if len(note) > 500 {
		return domain.Ask{}, ErrNoteLength
	}
	now := s.now()
	var a domain.Ask
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		// An approved ask whose title failed to add can still be turned down.
		res, err := tx.Exec(turnDownSQL, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var n int
			if err := tx.QueryRow(turnDownSQL2, id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return database.ErrNotFound
			}
			return ErrNotOpen
		}
		if a, err = get(ctx, tx, id); err != nil {
			return err
		}
		a.State, a.DecidedBy, a.DecidedAt, a.Note = domain.AskTurnedDown, &by.UserID, &now, note
		return database.Update(tx, "asks", &a, by.actor())
	})
	if err != nil {
		return a, err
	}
	people, _ := s.people(ctx, a.ID)
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventAskTurnedDown, Summary: label(a) + ": turned down by " + by.Name,
		Data: map[string]any{"ask_id": a.ID, "asked_by": names(people), "turned_down_by": by.Name, "note": note}, GroupKey: groupKey(a.ID)})
	text := label(a) + " was turned down by " + s.shownAs(ctx, by) + "."
	if note != "" {
		text += " Note: " + note
	}
	s.tell(ctx, a, people, alerts.MomentTurnedDown, "Turned down: "+label(a), text)
	s.changed(ctx, a.ID)
	return a, nil
}
