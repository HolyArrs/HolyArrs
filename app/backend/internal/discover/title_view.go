package discover

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// ViewDeps are the services the views read; field names match api.Server.
type ViewDeps struct {
	Store *database.Store
	Asks  *requests.Service
	Live  *webserver.LiveHub
}

// ViewHandlers builds views over ViewDeps.
type ViewHandlers struct{ ViewDeps }

// New returns ViewHandlers over d.
func NewView(d ViewDeps) *ViewHandlers { return &ViewHandlers{ViewDeps: d} }

// PublishView tells managers' pages a title changed. Its asks name who
// asked and who decided, with the decline note, so they stay out: pages
// read the title again from its route, which shows them to approvers only.
func (h *ViewHandlers) PublishView(v TitleView) {
	if h.Live == nil {
		return
	}
	v.Asks = nil
	h.Live.Publish("title", v)
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// SlotView is a Slot as the page shows it.
type SlotView struct {
	ID         int64            `json:"id"`
	Version    string           `json:"version"`
	VersionID  int64            `json:"version_id"`
	State      string           `json:"state"`
	StateLabel string           `json:"state_label"`
	Reason     string           `json:"reason,omitempty"`
	File       *mediafiles.File `json:"file,omitempty"`
	DownloadID *int64           `json:"active_download_id,omitempty"`
	// Download is the active (or last) Download in plain words.
	Download *downloads.DownloadView `json:"download,omitempty"`
}

// TitleView is a Title with its Slots.
type TitleView struct {
	ID               int64      `json:"id"`
	Kind             string     `json:"kind"`
	Name             string     `json:"name"`
	Year             *int       `json:"year,omitempty"`
	Overview         *string    `json:"overview,omitempty"`
	OriginalLanguage *string    `json:"original_language,omitempty"`
	IDs              any        `json:"ids"`
	KeepUpToDate     bool       `json:"keep_up_to_date"`
	WhyAdded         string     `json:"why_added"`
	ReleaseGate      string     `json:"release_gate"`
	GateDelayDays    int        `json:"gate_delay_days"`
	GateDriver       string     `json:"gate_driver"`
	GateOpensAt      *time.Time `json:"gate_opens_at,omitempty"`
	CinemaDate       *time.Time `json:"cinema_date,omitempty"`
	DigitalDate      *time.Time `json:"digital_date,omitempty"`
	PhysicalDate     *time.Time `json:"physical_date,omitempty"`
	RefreshedAt      *time.Time `json:"refreshed_at,omitempty"`
	DroppedAt        *time.Time `json:"dropped_at,omitempty"`
	Stage            string     `json:"stage"`       // announced, in_cinemas, released
	GateOpen         bool       `json:"gate_open"`   // searching may start
	GateReason       string     `json:"gate_reason"` // why it is or is not searched yet
	// The main Version: its folder and Pick rules.
	LibraryFolderID int64    `json:"library_folder_id,omitempty"`
	FolderPath      string   `json:"folder_path,omitempty"`
	PickRulesID     int64    `json:"pick_rules_id,omitempty"`
	PickRulesName   string   `json:"pick_rules_name,omitempty"`
	Artwork         []string `json:"artwork"`
	// Screen details from the metadata source: wide picture and logo URLs,
	// genre names, runtime, the US rating and people (top cast, directors).
	Backdrop       string   `json:"backdrop,omitempty"`
	Logo           string   `json:"logo,omitempty"`
	Genres         []string `json:"genres"`
	RuntimeMinutes int      `json:"runtime_minutes,omitempty"`
	Rating         string   `json:"rating,omitempty"`
	People         []string `json:"people"`
	// Score is the members' average out of 10 and Votes how many voted.
	Score float64 `json:"score,omitempty"`
	Votes int     `json:"votes,omitempty"`
	// Tags route the title's grabs and searches to the Downloaders and Sources that share one.
	Tags []string `json:"tags"`
	// AddedAt is when the title joined the Library; SizeBytes is the sum of
	// every file kept for it, across all Versions (Library table columns).
	AddedAt   time.Time `json:"added_at"`
	SizeBytes int64     `json:"size_bytes"`
	// Copy is a movie's main file (codec, audio, ids for a copy search); only
	// in the Library list, and only for people who manage the Library.
	Copy *mediafiles.TitleCopy `json:"copy,omitempty"`
	// LastActivity is the newest grab or import for the title, if any.
	LastActivity *time.Time `json:"last_activity,omitempty"`
	// Versions are the wanted copies (main first), such as HD and 4K.
	Versions []VersionView `json:"versions"`
	Slots    []SlotView    `json:"slots"`
	// Asks are the Requests that led to it: who asked and who decided.
	Asks []requests.View `json:"asks"`
	// Show holds a show's choices and season summaries (shows only; their
	// episodes and Slots come from /titles/{id}/episodes, so Slots is empty).
	Show *shows.Info `json:"show,omitempty"`
	// Artist holds an artist's choices and counts (artists only; albums,
	// Pressings and tracks come from /titles/{id}/albums and /albums/{id}).
	Artist *ArtistInfo `json:"artist,omitempty"`
}

// ArtworkLinks turns stored artwork ("poster:<cache name>") into links the
// web app can load (/api/v1/artwork/<cache name>); anything else is kept.
func ArtworkLinks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
			a = "/api/v1/artwork/" + url.PathEscape(name)
		}
		out = append(out, a)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// View builds a Title's view: its Versions, Slots, asks and, for shows and
// artists, their summaries. Batches on ctx (WithTitleFacts, WithTitleBatch)
// answer instead of per-Title reads.
func (h *ViewHandlers) View(ctx context.Context, t mediafiles.Title) (TitleView, error) {
	db := h.Store.Read()
	v := TitleView{ID: t.ID, Kind: t.Kind, Name: t.Name, Year: t.Year, Overview: t.Overview, OriginalLanguage: t.OriginalLanguage, IDs: t.SourceIDs, Slots: []SlotView{},
		KeepUpToDate: t.KeepUpToDate, WhyAdded: t.WhyAdded, ReleaseGate: t.ReleaseGate, GateDelayDays: t.GateDelayDays,
		GateDriver: t.GateDriver, GateOpensAt: t.GateOpensAt, CinemaDate: t.CinemaDate, DigitalDate: t.DigitalDate,
		PhysicalDate: t.PhysicalDate, RefreshedAt: t.RefreshedAt, DroppedAt: t.DroppedAt, Artwork: ArtworkLinks(t.Artwork),
		Backdrop: t.Backdrop, Logo: t.Logo, Genres: nonNil(t.Genres), RuntimeMinutes: t.RuntimeMinutes, Rating: t.Rating, People: nonNil(t.People),
		AddedAt: t.CreatedAt, Tags: nonNil(t.Tags), Votes: t.Votes}
	if t.Score != nil {
		v.Score = *t.Score
	}
	now := time.Now()
	var err error
	facts := factsOf(ctx)
	if facts != nil {
		v.SizeBytes = facts.sizes[t.ID]
		if c, ok := facts.copies[t.ID]; ok {
			v.Copy = &c
		}
		if at, ok := facts.activity[t.ID]; ok {
			v.LastActivity = &at
		}
	} else {
		if v.SizeBytes, err = mediafiles.TitleSize(ctx, db, t.ID); err != nil {
			return v, err
		}
		if v.LastActivity, err = mediafiles.TitleActivity(ctx, db, t.ID); err != nil {
			return v, err
		}
	}
	if t.Kind == "artist" {
		v.Stage, v.GateOpen = t.Status, true
		v.GateReason = "Each album is wanted once it is out (its release date has passed)."
		if !t.KeepUpToDate {
			v.GateReason += " Keep up to date is off for the artist, so HolyArrs does not look for it on its own."
		}
	} else if t.Kind == "show" {
		v.Stage, v.GateOpen = t.Status, true
		v.GateReason = "Each episode is wanted once it has aired."
		if !t.KeepUpToDate {
			v.GateReason += " Keep up to date is off for the show, so HolyArrs does not look for it on its own."
		}
	} else {
		v.Stage = library.Stage(library.DatesOf(t), now)
		v.GateOpen = library.GateOpen(t.GateDriver, t.GateOpensAt, now)
		v.GateReason = gateReason(t, v.GateOpen)
	}
	v.Versions = []VersionView{}
	var vs []mediafiles.Version
	if facts != nil {
		vs = facts.versions[t.ID]
	} else if vs, err = database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID); err != nil {
		return v, err
	}
	for _, ver := range vs {
		v.Versions = append(v.Versions, VersionViewOf(ctx, db, ver))
	}
	if len(v.Versions) > 0 {
		m := v.Versions[0]
		v.LibraryFolderID, v.PickRulesID, v.FolderPath, v.PickRulesName = m.LibraryFolderID, m.PickRulesID, m.FolderPath, m.PickRulesName
	}
	batch := batchOf(ctx)
	titleAsks := func() ([]requests.View, error) {
		if batch != nil {
			return batch.asksFor(t.ID), nil
		}
		return h.Asks.TitleAsks(ctx, t.ID)
	}
	if t.Kind == "show" {
		if v.Show, err = shows.InfoOf(ctx, h.Store.Read(), t); err != nil {
			return v, err
		}
		v.Asks, err = titleAsks()
		return TrimForViewer(ctx, v), err
	}
	if t.Kind == "artist" {
		if v.Artist, err = h.artistInfo(ctx, t); err != nil {
			return v, err
		}
		v.Asks, err = titleAsks()
		return TrimForViewer(ctx, v), err
	}
	var slots []mediafiles.Slot
	if batch != nil {
		slots = batch.slots[t.ID]
	} else if slots, err = database.Select[mediafiles.Slot](ctx, db, "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", t.ID); err != nil {
		return v, err
	}
	for _, s := range slots {
		var ver mediafiles.Version
		if batch != nil {
			ver = batch.labels[s.VersionID]
		} else {
			_ = database.Get(ctx, db, "versions", s.VersionID, &ver)
		}
		sv := SlotView{ID: s.ID, Version: ver.Label, VersionID: ver.ID, State: string(s.State), StateLabel: mediafiles.StateLabels[s.State], DownloadID: s.ActiveDownloadID}
		if s.LastReason != nil {
			sv.Reason = *s.LastReason
		}
		if s.ActiveDownloadID != nil {
			var d domain.Download
			var found bool
			if batch != nil {
				d, found = batch.downloads[*s.ActiveDownloadID]
			} else {
				found = database.Get(ctx, db, "downloads", *s.ActiveDownloadID, &d) == nil
			}
			if found {
				dv := downloads.DownloadViewOf(d)
				sv.Download = &dv
			}
		}
		if s.FileID != nil {
			var f mediafiles.File
			var found bool
			if batch != nil {
				f, found = batch.files[*s.FileID]
			} else {
				found = database.Get(ctx, db, "files", *s.FileID, &f) == nil
			}
			if found {
				sv.File = &f
			}
		}
		v.Slots = append(v.Slots, sv)
	}
	if v.Asks, err = titleAsks(); err != nil {
		return v, err
	}
	return TrimForViewer(ctx, v), nil
}

// gateReason says in one sentence why a movie is or is not searched yet.
func gateReason(t mediafiles.Title, open bool) string {
	date := func(at *time.Time) string { return at.Format("2006-01-02") }
	var when string
	switch t.GateDriver {
	case "announced":
		when = "the Release gate is set to announced, so searching may start any time"
	case "cinema":
		when = "the cinema date"
	case "digital":
		when = "the digital release date"
	case "physical":
		when = "the disc release date"
	case "cinema_plus_90":
		when = "90 days after the cinema date (no digital or disc date yet)"
	}
	var s string
	switch {
	case t.GateDriver == "announced":
		s = "The gate is open: " + when + "."
	case t.GateOpensAt == nil:
		s = "The gate is closed: there are no release dates yet, so HolyArrs waits until TMDB has one."
	case open:
		s = "The gate is open since " + date(t.GateOpensAt) + " (" + when + ")."
	default:
		s = "The gate is closed until " + date(t.GateOpensAt) + " (" + when + ")."
	}
	if t.GateDelayDays > 0 && t.GateDriver != "announced" && t.GateOpensAt != nil {
		s = strings.TrimSuffix(s, ".") + ", plus " + strconv.Itoa(t.GateDelayDays) + " days of delay."
	}
	if !t.KeepUpToDate {
		s += " Keep up to date is off, so HolyArrs does not look for it on its own."
	}
	return s
}

// TitleID reads the {id} Title, answering 404 itself.
func (h *ViewHandlers) TitleID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var t mediafiles.Title
	if err == nil {
		err = database.Get(r.Context(), h.Store.Read(), "titles", id, &t)
	}
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return t, false
	}
	return t, true
}

// PublishTitle sends title id's fresh view to live pages.
func (h *ViewHandlers) PublishTitle(ctx context.Context, id int64) {
	if h.Live == nil {
		return
	}
	var t mediafiles.Title
	if database.Get(ctx, h.Store.Read(), "titles", id, &t) != nil {
		return
	}
	if v, err := h.View(ctx, t); err == nil {
		h.PublishView(v)
	}
}
