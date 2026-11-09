package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Reports: anyone signed in tells the owner and admins about a
// problem with a title in the Library. The staff get an Alert; marking it
// fixed (with an optional reply) leaves the reporter a message and sends
// them an Alert. There is no thread: one optional reply closes it.

// Errors from reporting and closing reports.
var (
	ErrReportKind   = errors.New("kind must be picture, sound, subtitles, wont_play or other")
	ErrNotInLibrary = errors.New("nothing of this title is in the Library yet, so there is nothing to report")
	ErrReportPart   = errors.New("the album, season, episode, track or Version is not part of this title")
	// ErrReportedAlready: the same person has this problem open already.
	ErrReportedAlready = errors.New("this problem is already reported and still open")
	ErrReportFixed     = errors.New("this report is already fixed")
	ErrReportOpen      = errors.New("this report is still open")
	// ErrTooManyReports: the person has maxOpenReports open already.
	ErrTooManyReports = errors.New("you have 20 open reports; wait until some are fixed")
)

// maxOpenReports caps one person's open reports, so a loop cannot flood the
// staff Alerts.
const maxOpenReports = 20

// ReportKindLabels are the words people see for problem kinds.
var ReportKindLabels = map[string]string{
	"picture": "Picture", "sound": "Sound", "subtitles": "Subtitles", "wont_play": "Won't play", "other": "Other",
}

// ReportStateLabels are the words people see for report states.
var ReportStateLabels = map[string]string{domain.ReportOpen: "Open", domain.ReportFixed: "Fixed"}

// ReportInput is what a person reports. Only TitleID and Kind are needed;
// the rest narrows it (0 or nil: not narrowed).
type ReportInput struct {
	TitleID   int64  `json:"title_id"`
	AlbumID   int64  `json:"album_id"`
	Season    *int   `json:"season"`
	ItemID    int64  `json:"item_id"` // an episode or track (a movie's own item is found)
	VersionID int64  `json:"version_id"`
	Kind      string `json:"kind"`
	Note      string `json:"note"`
}

// ReportView is a report as pages show it.
type ReportView struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	TitleID   int64     `json:"title_id"`
	TitleName string    `json:"title_name"`
	TitleKind string    `json:"title_kind"` // movie, show, artist
	Year      *int      `json:"year,omitempty"`
	Poster    string    `json:"poster,omitempty"`
	AlbumID   *int64    `json:"album_id,omitempty"`
	AlbumName string    `json:"album_name,omitempty"`
	Season    *int      `json:"season,omitempty"`
	ItemID    *int64    `json:"item_id,omitempty"`
	Episode   *int      `json:"episode,omitempty"`
	Track     *int      `json:"track,omitempty"`
	ItemName  string    `json:"item_name,omitempty"`
	VersionID *int64    `json:"version_id,omitempty"`
	Version   string    `json:"version,omitempty"` // the Version's label
	FileID    *int64    `json:"file_id,omitempty"`
	// Where says what part it is about in words: S02E03, Season 2, an
	// album, Track 4 ("" for the whole title).
	Where      string     `json:"where,omitempty"`
	Kind       string     `json:"kind"`
	KindLabel  string     `json:"kind_label"`
	Note       string     `json:"note,omitempty"`
	State      string     `json:"state"`
	StateLabel string     `json:"state_label"`
	MadeBy     *Who       `json:"made_by,omitempty"`
	ClosedBy   *Who       `json:"closed_by,omitempty"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	Reply      string     `json:"reply,omitempty"`
	// Mine says the person reading made it (set by the API).
	Mine bool `json:"mine"`
	// Rating is the title's rating, for the API's Kid check.
	Rating string `json:"-"`
}

// label names the report's title and part: "Severance (2022), S02E03 (4K)".
func (v ReportView) label() string {
	l := v.TitleName
	if v.Year != nil {
		l += " (" + strconv.Itoa(*v.Year) + ")"
	}
	if v.Where != "" {
		l += ", " + v.Where
	}
	return l
}

func reportKey(id int64) string { return "report:" + strconv.FormatInt(id, 10) }

// openReportConflict says err is the one-open-report index refusing a
// second open report for the same problem.
func openReportConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "reports")
}

// Report records p's report of a problem with a title in the Library and
// tells the staff.
func (s *Service) Report(ctx context.Context, p Person, in ReportInput) (ReportView, error) {
	in.Kind = strings.TrimSpace(in.Kind)
	if !slices.Contains(domain.ReportKinds, in.Kind) {
		return ReportView{}, ErrReportKind
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > 500 {
		return ReportView{}, ErrNoteLength
	}
	db := s.Store.Read()
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", in.TitleID, &t); err != nil {
		return ReportView{}, err
	}
	var have int
	if err := db.QueryRowContext(ctx, reportSQL, t.ID).Scan(&have); err != nil {
		return ReportView{}, err
	}
	if t.State == "removed" || have == 0 {
		return ReportView{}, ErrNotInLibrary
	}
	r := domain.Report{TitleID: t.ID, Kind: in.Kind, Note: in.Note, State: domain.ReportOpen, MadeBy: &p.UserID}
	if in.ItemID == 0 && t.Kind == "movie" {
		_ = db.QueryRowContext(ctx, reportSQL2, t.ID).Scan(&in.ItemID)
	}
	if in.ItemID != 0 {
		var it mediafiles.Item
		if err := database.Get(ctx, db, "items", in.ItemID, &it); errors.Is(err, database.ErrNotFound) || (err == nil && it.TitleID != t.ID) {
			return ReportView{}, ErrReportPart
		} else if err != nil {
			return ReportView{}, err
		}
		if it.Kind == "episode" && it.Numbers.Season != nil {
			if in.Season != nil && *in.Season != *it.Numbers.Season {
				return ReportView{}, ErrReportPart
			}
			in.Season = it.Numbers.Season
		}
		if it.Kind == "track" && it.PartID != nil {
			if in.AlbumID != 0 && in.AlbumID != *it.PartID {
				return ReportView{}, ErrReportPart
			}
			in.AlbumID = *it.PartID
		}
		r.ItemID = &it.ID
	}
	if in.Season != nil {
		if t.Kind != "show" {
			return ReportView{}, ErrReportPart
		}
		var n int
		if err := db.QueryRowContext(ctx, reportSQL3, t.ID, *in.Season).Scan(&n); err != nil {
			return ReportView{}, err
		}
		if n == 0 {
			return ReportView{}, ErrReportPart
		}
		r.Season = in.Season
	}
	if in.AlbumID != 0 {
		var n int
		if err := db.QueryRowContext(ctx, reportSQL4, in.AlbumID, t.ID).Scan(&n); err != nil {
			return ReportView{}, err
		}
		if n == 0 {
			return ReportView{}, ErrReportPart
		}
		r.AlbumID = &in.AlbumID
	}
	if in.VersionID != 0 {
		var n int
		if err := db.QueryRowContext(ctx, reportSQL5, in.VersionID, t.ID).Scan(&n); err != nil {
			return ReportView{}, err
		}
		if n == 0 {
			return ReportView{}, ErrReportPart
		}
		r.VersionID = &in.VersionID
	}
	// The file that was there: when the item has one copy in the Library
	// (or the Version picks one), keep it and its Version.
	var slotID *int64
	if r.ItemID != nil {
		rows, err := db.QueryContext(ctx, reportSQL6, *r.ItemID, in.VersionID, in.VersionID)
		if err != nil {
			return ReportView{}, err
		}
		type copyRow struct{ slot, version, file int64 }
		var found []copyRow
		for rows.Next() {
			var c copyRow
			if err := rows.Scan(&c.slot, &c.version, &c.file); err != nil {
				rows.Close()
				return ReportView{}, err
			}
			found = append(found, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ReportView{}, err
		}
		if len(found) == 1 {
			c := found[0]
			slotID, r.VersionID, r.FileID = &c.slot, &c.version, &c.file
		}
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var open int
		if err := tx.QueryRow(reportSQL7, p.UserID).Scan(&open); err != nil {
			return err
		}
		if open >= maxOpenReports {
			return ErrTooManyReports
		}
		_, err := database.Insert(tx, "reports", &r, p.actor())
		return err
	})
	if openReportConflict(err) {
		return ReportView{}, ErrReportedAlready
	}
	if err != nil {
		return ReportView{}, err
	}
	v, err := s.reportView(ctx, r.ID)
	if err != nil {
		return ReportView{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventReportMade, TitleID: &r.TitleID, SlotID: slotID,
		Summary: p.Name + " reported a problem with " + v.label() + ": " + v.KindLabel,
		Data:    map[string]any{"report_id": r.ID, "kind": r.Kind, "note": r.Note, "made_by": p.Name}, GroupKey: reportKey(r.ID)})
	text := p.Name + " reported a problem with " + v.label() + ": " + v.KindLabel + "."
	if r.Note != "" {
		text += " Note: " + r.Note
	}
	s.send(ctx, alerts.Message{Moment: alerts.MomentReported, Subject: p.Name + " reported a problem: " + v.label(),
		Text: text + " See it on the Reports page.", Report: v.alert()})
	return v, nil
}

func (v ReportView) alert() *alerts.Report {
	return &alerts.Report{ID: v.ID, TitleID: v.TitleID, Title: v.TitleName, Where: v.Where, Kind: v.Kind, Note: v.Note, Reply: v.Reply}
}

// FixReport marks an open report fixed. The reporter gets a message and an
// Alert, with the reply when there is one.
func (s *Service) FixReport(ctx context.Context, by Person, id int64, reply string) (ReportView, error) {
	reply = strings.TrimSpace(reply)
	if utf8.RuneCountInString(reply) > 500 {
		return ReportView{}, ErrNoteLength
	}
	now := s.now()
	var r domain.Report
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "reports", id, &r); err != nil {
			return err
		}
		if r.State != domain.ReportOpen {
			return ErrReportFixed
		}
		r.State, r.ClosedBy, r.ClosedAt, r.Reply = domain.ReportFixed, &by.UserID, &now, reply
		return database.UpdateWhere(tx, "reports", &r, by.actor(), "state = 'open'")
	})
	if err != nil {
		return ReportView{}, err
	}
	v, err := s.reportView(ctx, id)
	if err != nil {
		return ReportView{}, err
	}
	who := ""
	if v.MadeBy != nil {
		who = v.MadeBy.Name
	}
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventReportFixed, TitleID: &r.TitleID, SlotID: s.reportSlot(ctx, r),
		Summary: v.label() + ": " + v.KindLabel + " problem fixed by " + by.Name,
		Data:    map[string]any{"report_id": r.ID, "kind": r.Kind, "note": r.Note, "reply": reply, "made_by": who, "fixed_by": by.Name}, GroupKey: reportKey(r.ID)})
	if r.MadeBy != nil {
		s.tellReporter(ctx, r, v, by)
	}
	return v, nil
}

// ReopenReport opens a fixed report again (the problem is back).
func (s *Service) ReopenReport(ctx context.Context, by Person, id int64) (ReportView, error) {
	var r domain.Report
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "reports", id, &r); err != nil {
			return err
		}
		if r.State != domain.ReportFixed {
			return ErrReportOpen
		}
		r.State, r.ClosedBy, r.ClosedAt, r.Reply = domain.ReportOpen, nil, nil, ""
		return database.UpdateWhere(tx, "reports", &r, by.actor(), "state = 'fixed'")
	})
	if openReportConflict(err) {
		return ReportView{}, ErrReportedAlready
	}
	if err != nil {
		return ReportView{}, err
	}
	v, err := s.reportView(ctx, id)
	if err != nil {
		return ReportView{}, err
	}
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventReportReopened, TitleID: &r.TitleID, SlotID: s.reportSlot(ctx, r),
		Summary: v.label() + ": " + v.KindLabel + " problem opened again by " + by.Name,
		Data:    map[string]any{"report_id": r.ID, "kind": r.Kind, "note": r.Note, "reopened_by": by.Name}, GroupKey: reportKey(r.ID)})
	return v, nil
}

// reportSlot is the Slot of the report's item and Version, if both are set.
func (s *Service) reportSlot(ctx context.Context, r domain.Report) *int64 {
	if r.ItemID == nil || r.VersionID == nil {
		return nil
	}
	var id int64
	if err := s.Store.Read().QueryRowContext(ctx, reportSlotSQL, *r.ItemID, *r.VersionID).Scan(&id); err != nil {
		return nil
	}
	return &id
}

// tellReporter leaves the reporter a message and sends them the Alert.
func (s *Service) tellReporter(ctx context.Context, r domain.Report, v ReportView, by Person) {
	var p person
	err := s.Store.Read().QueryRowContext(ctx, tellReporterSQL, *r.MadeBy).Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID)
	if err != nil {
		return
	}
	text := "The problem you reported with " + v.label() + " (" + v.KindLabel + ") was fixed by " + s.shownAs(ctx, by) + "."
	if r.Reply != "" {
		text += " Reply: " + r.Reply
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(tellReporterSQL2,
			database.FormatTime(s.now()), p.ID, text, r.TitleID, r.ID)
		return err
	})
	if err != nil {
		log.Printf("asks: report %d message: %v", r.ID, err)
	}
	s.send(ctx, alerts.Message{Moment: alerts.MomentFixed, Subject: "Fixed: " + v.label(), Text: text,
		People: []alerts.Person{{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID}}, Report: v.alert()})
}

// ListReports lists reports, newest first, open ones before fixed ones.
// Without all only p's own are listed; state ("open", "fixed" or "") and
// titleID (0: any) narrow the list.
func (s *Service) ListReports(ctx context.Context, p Person, all bool, state string, titleID int64, limit int) ([]ReportView, error) {
	where, args := []string{"1=1"}, []any{}
	if !all {
		where, args = append(where, "made_by = ?"), append(args, p.UserID)
	}
	if state != "" {
		where, args = append(where, "state = ?"), append(args, state)
	}
	if titleID != 0 {
		where, args = append(where, "title_id = ?"), append(args, titleID)
	}
	return s.ReportViews(ctx, strings.Join(where, " AND ")+" ORDER BY CASE state WHEN 'open' THEN 0 ELSE 1 END, id DESC LIMIT ?", append(args, limit)...)
}

// OpenReports counts the reports still open.
func OpenReports(ctx context.Context, db database.Querier) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, openReportsSQL).Scan(&n)
	return n, err
}

func (s *Service) reportView(ctx context.Context, id int64) (ReportView, error) {
	vs, err := s.ReportViews(ctx, "id = ?", id)
	if err != nil {
		return ReportView{}, err
	}
	if len(vs) == 0 {
		return ReportView{}, database.ErrNotFound
	}
	return vs[0], nil
}

// ReportViews reads reports matching where (SQL on reports, may end with
// ORDER BY and LIMIT) with the names pages show.
func (s *Service) ReportViews(ctx context.Context, where string, args ...any) ([]ReportView, error) {
	db := s.Store.Read()
	rs, err := database.Select[domain.Report](ctx, db, "reports", where, args...)
	if err != nil {
		return nil, err
	}
	out := []ReportView{}
	titles := map[int64]mediafiles.Title{}
	versions := map[int64]int{}
	names := map[int64]string{}
	who := func(id *int64) *Who {
		if id == nil {
			return nil
		}
		n, ok := names[*id]
		if !ok {
			_ = db.QueryRowContext(ctx, reportViewsSQL, *id).Scan(&n)
			names[*id] = n
		}
		return &Who{ID: *id, Name: n}
	}
	for _, r := range rs {
		t, ok := titles[r.TitleID]
		if !ok {
			_ = database.Get(ctx, db, "titles", r.TitleID, &t)
			titles[r.TitleID] = t
			var n int
			_ = db.QueryRowContext(ctx, reportViewsSQL2, r.TitleID).Scan(&n)
			versions[r.TitleID] = n
		}
		v := ReportView{ID: r.ID, CreatedAt: r.CreatedAt, TitleID: r.TitleID, TitleName: t.Name, TitleKind: t.Kind, Year: t.Year, Rating: t.Rating,
			AlbumID: r.AlbumID, Season: r.Season, ItemID: r.ItemID, VersionID: r.VersionID, FileID: r.FileID, Kind: r.Kind, KindLabel: ReportKindLabels[r.Kind],
			Note: r.Note, State: r.State, StateLabel: ReportStateLabels[r.State], MadeBy: who(r.MadeBy), ClosedBy: who(r.ClosedBy), ClosedAt: r.ClosedAt, Reply: r.Reply}
		if len(t.Artwork) > 0 {
			v.Poster = t.Artwork[0]
		}
		if r.AlbumID != nil {
			_ = db.QueryRowContext(ctx, reportViewsSQL3, *r.AlbumID).Scan(&v.AlbumName)
		}
		if r.ItemID != nil {
			var it mediafiles.Item
			if database.Get(ctx, db, "items", *r.ItemID, &it) == nil {
				v.Episode, v.Track = it.Numbers.Episode, it.Numbers.Track
				if it.Name != nil {
					v.ItemName = *it.Name
				}
			}
		}
		if r.VersionID != nil {
			_ = db.QueryRowContext(ctx, reportViewsSQL4, *r.VersionID).Scan(&v.Version)
		}
		v.Where = reportWhere(v, versions[r.TitleID] > 1)
		out = append(out, v)
	}
	return out, nil
}

// reportWhere says what part a report is about: "S02E03", "Season 2",
// "Album name, track 4"; the Version's label is added when the title has
// more than one.
func reportWhere(v ReportView, manyVersions bool) string {
	var w string
	switch {
	case v.Season != nil && v.Episode != nil:
		w = fmt.Sprintf("S%02dE%02d", *v.Season, *v.Episode)
	case v.Season != nil:
		w = "Season " + strconv.Itoa(*v.Season)
	case v.AlbumName != "" && v.Track != nil:
		w = v.AlbumName + ", track " + strconv.Itoa(*v.Track)
	case v.AlbumName != "":
		w = v.AlbumName
	case v.Track != nil:
		w = "Track " + strconv.Itoa(*v.Track)
	}
	if v.TitleKind == "artist" && v.ItemName != "" {
		if w != "" {
			w += ": "
		}
		w += v.ItemName
	}
	if manyVersions && v.Version != "" {
		if w != "" {
			w += " "
		}
		w += "(" + v.Version + ")"
	}
	return w
}
