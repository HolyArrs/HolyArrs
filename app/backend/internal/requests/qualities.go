package requests

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// What people can ask for: the Quality buttons on the movie and show request
// sheets, one list per kind, in order. Each line is a name people see, the
// Pick rules it asks for and whether it is the title's main copy or kept as
// its own copy.
//
// It is stored as one setting (ask_qualities), only what the owner changed:
//
//	{"movie": [{"id": "standard", "name": "HD"}, {"id": "4k", "hidden": true}], "show": [...]}
//
// The two built-in lines take their Pick rules and folder from the Request
// settings (the plain ones, and the four 4K ones), so they never move. A line
// the owner added (its id starts with "own-") carries its name and its own
// pick_rules_id and asks for the title's main copy. Who may pick which line
// is set per person (PersonQualities). The array order is the order people see, so
// reordering is rewriting it. Asks keep the line id, so none of this moves
// stored asks.

// The built-in lines.
const (
	// QualityStandard is the title's main copy on the Request settings' Pick
	// rules and folder.
	QualityStandard = "standard"
	// QualityFourK is the 4K copy on the 4K Pick rules and folder, kept as
	// its own copy.
	QualityFourK = "4k"
)

// fourKName is the 4K line's name when the owner gave none.
const fourKName = "4K"

// qualityNameMax is the longest name a line may have, in characters.
const qualityNameMax = 30

// qualityLinesMax is the most lines one kind may have.
const qualityLinesMax = 8

// ownPrefix starts the id of a line the owner added.
const ownPrefix = "own-"

// ownLine says id is a line the owner added: "own-" and up to 20 lowercase
// letters and digits.
func ownLine(id string) bool {
	rest, ok := strings.CutPrefix(id, ownPrefix)
	if !ok || rest == "" || len(rest) > 20 {
		return false
	}
	for _, c := range rest {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// Errors from saving the lines and from asking for one.
var (
	ErrQualityName       = errors.New("a quality name can be at most 30 characters")
	ErrQualityAllHidden  = errors.New("keep at least one quality shown for movies and one for shows")
	ErrQualityOwnLine    = errors.New("give every quality you added a name")
	ErrQualityTooMany    = errors.New("movies and shows can have at most 8 qualities each")
	ErrQualityNotOffered = errors.New("that quality cannot be asked for")
	ErrQualityTaken      = errors.New("someone already asked for this movie in that quality; decide their ask instead")
)

// QualityLine is one line of what people can ask for.
type QualityLine struct {
	ID string `json:"id"`
	// Name is what people see; empty shows the built-in name (Standard, 4K).
	Name string `json:"name"`
	// Label is the owner's own name for it, seen only where lines are set
	// up; empty shows Name.
	Label  string `json:"label"`
	Hidden bool   `json:"hidden"`
	// Read from the Request settings, never stored for the built-in lines:
	// the Pick rules (0: the folder's own) and Library folder (0: the
	// request folder, then the title's), whether it is kept as its own copy,
	// and whether it can be asked for now (the 4K line needs 4K Pick rules).
	// A line the owner added stores its Pick rules and is ready while they
	// are still there.
	PickRulesID     int64 `json:"pick_rules_id"`
	LibraryFolderID int64 `json:"library_folder_id"`
	Extra           bool  `json:"extra"`
	Ready           bool  `json:"ready"`
}

// QualityLists are the lines for movies and for shows.
type QualityLists struct {
	Movie []QualityLine `json:"movie"`
	Show  []QualityLine `json:"show"`
}

// of is the list for an ask kind ("movie" or "show").
func (q QualityLists) of(kind string) []QualityLine {
	if kind == "show" {
		return q.Show
	}
	return q.Movie
}

// defaultQualities are the lines an owner who changed nothing has.
func defaultQualities() QualityLists {
	both := func() []QualityLine { return []QualityLine{{ID: QualityStandard}, {ID: QualityFourK}} }
	return QualityLists{Movie: both(), Show: both()}
}

// storedLine is what is stored of a line.
type storedLine struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Label       string `json:"label,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	PickRulesID int64  `json:"pick_rules_id,omitempty"`
}

// stored is what is stored of l: the Pick rules only for a line the owner added.
func stored(l QualityLine) storedLine {
	s := storedLine{ID: l.ID, Name: strings.TrimSpace(l.Name), Label: strings.TrimSpace(l.Label), Hidden: l.Hidden}
	if ownLine(l.ID) {
		s.PickRulesID = l.PickRulesID
	}
	return s
}

// parseQualities reads the stored setting; anything it cannot read gives the
// built-in lines. Unknown or repeated lines are dropped and a missing
// built-in line comes back at the end.
func parseQualities(v string) QualityLists {
	var in struct {
		Movie []storedLine `json:"movie"`
		Show  []storedLine `json:"show"`
	}
	if v == "" || json.Unmarshal([]byte(v), &in) != nil {
		return defaultQualities()
	}
	fix := func(ls []storedLine) []QualityLine {
		out := []QualityLine{}
		seen := map[string]bool{}
		for _, l := range ls {
			if (l.ID != QualityStandard && l.ID != QualityFourK && !ownLine(l.ID)) || seen[l.ID] {
				continue
			}
			seen[l.ID] = true
			line := QualityLine{ID: l.ID, Name: strings.TrimSpace(l.Name), Label: strings.TrimSpace(l.Label), Hidden: l.Hidden}
			if ownLine(l.ID) {
				line.PickRulesID = l.PickRulesID
			}
			out = append(out, line)
		}
		for _, id := range []string{QualityStandard, QualityFourK} {
			if !seen[id] {
				out = append(out, QualityLine{ID: id})
			}
		}
		return out
	}
	return QualityLists{Movie: fix(in.Movie), Show: fix(in.Show)}
}

// storeQualities is the setting's stored value.
func storeQualities(q QualityLists) string {
	keep := func(ls []QualityLine) []storedLine {
		out := []storedLine{}
		for _, l := range ls {
			out = append(out, stored(l))
		}
		return out
	}
	b, _ := json.Marshal(map[string][]storedLine{"movie": keep(q.Movie), "show": keep(q.Show)})
	return string(b)
}

// checkQualities checks the lines an owner sends: the built-in lines and the
// owner's own (each with a name and Pick rules), names of at most 30
// characters, at most 8 lines a kind, and for each kind at least one line
// shown.
func checkQualities(q QualityLists) (QualityLists, error) {
	sent := func(ls []QualityLine) []storedLine {
		out := []storedLine{}
		for _, l := range ls {
			if utf8.RuneCountInString(strings.TrimSpace(l.Name)) > qualityNameMax || utf8.RuneCountInString(strings.TrimSpace(l.Label)) > qualityNameMax {
				return nil
			}
			out = append(out, stored(l))
		}
		return out
	}
	m, s := sent(q.Movie), sent(q.Show)
	if m == nil || s == nil {
		return q, ErrQualityName
	}
	b, _ := json.Marshal(map[string][]storedLine{"movie": m, "show": s})
	out := parseQualities(string(b))
	for _, ls := range [][]QualityLine{out.Movie, out.Show} {
		if len(ls) > qualityLinesMax {
			return out, ErrQualityTooMany
		}
		shown := false
		for _, l := range ls {
			if ownLine(l.ID) && (l.Name == "" || l.PickRulesID <= 0) {
				return out, ErrQualityOwnLine
			}
			shown = shown || !l.Hidden
		}
		if !shown {
			return out, ErrQualityAllHidden
		}
	}
	return out, nil
}

// qualityLines fills the lines of one kind with what the Request settings say
// they ask for.
func qualityLines(ctx context.Context, db querier, set Settings, kind string) ([]QualityLine, error) {
	plainRules, plainFolder := set.PickRulesID, set.LibraryFolderID
	rules, folder := set.FourKPickRulesID, set.FourKLibraryFolderID
	if kind == "show" {
		plainRules, plainFolder = set.ShowPickRulesID, set.ShowLibraryFolderID
		rules, folder = set.ShowFourKPickRulesID, set.ShowFourKLibraryFolderID
	}
	if rules == 0 {
		var err error
		if rules, err = fourKRules(ctx, db); err != nil {
			return nil, err
		}
	}
	if folder == 0 {
		folder = plainFolder
	}
	out := []QualityLine{}
	for _, l := range set.Qualities.of(kind) {
		switch l.ID {
		case QualityStandard:
			l.PickRulesID, l.LibraryFolderID, l.Extra, l.Ready = plainRules, plainFolder, false, true
		case QualityFourK:
			l.PickRulesID, l.LibraryFolderID, l.Extra, l.Ready = rules, folder, true, rules != 0
		default:
			var n int
			if err := db.QueryRowContext(ctx, askedRulesSQL, l.PickRulesID).Scan(&n); err != nil {
				return nil, err
			}
			l.LibraryFolderID, l.Extra, l.Ready = plainFolder, false, n > 0
		}
		out = append(out, l)
	}
	return out, nil
}

// resolveQualities fills both lists (for the Request rules page).
func resolveQualities(ctx context.Context, db querier, set Settings) (QualityLists, error) {
	m, err := qualityLines(ctx, db, set, "movie")
	if err != nil {
		return QualityLists{}, err
	}
	s, err := qualityLines(ctx, db, set, "show")
	return QualityLists{Movie: m, Show: s}, err
}

// offered are the lines of kind p may ask for: shown, ready and allowed by
// p's own choice, in order. A Kid gets the first one only. With none left
// and no choice of p's own, the first line stands in; with a choice, nothing.
func offered(lines []QualityLine, p Person, kind string) []QualityLine {
	out := []QualityLine{}
	for _, l := range lines {
		if !l.Hidden && l.Ready && p.mayPick(kind, l.ID) {
			out = append(out, l)
		}
	}
	if len(out) == 0 && len(lines) > 0 && p.mayPick(kind, "") {
		out = lines[:1]
	}
	if p.MaxRating != "" && len(out) > 1 {
		out = out[:1]
	}
	return out
}

// QualityChoice is a line as the Ask sheets show it: its id, the owner's name
// for it (empty: the built-in name), whether it is kept as its own copy, and
// its Pick rules (so an approver's row does not show them twice).
type QualityChoice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Extra       bool   `json:"extra"`
	PickRulesID int64  `json:"pick_rules_id"`
}

func choicesOf(lines []QualityLine) []QualityChoice {
	out := []QualityChoice{}
	for _, l := range lines {
		out = append(out, QualityChoice{ID: l.ID, Name: l.Name, Extra: l.Extra, PickRulesID: l.PickRulesID})
	}
	return out
}

// lineByID finds a line, false when there is none.
func lineByID(lines []QualityLine, id string) (QualityLine, bool) {
	for _, l := range lines {
		if l.ID == id {
			return l, true
		}
	}
	return QualityLine{}, false
}

// shownName is a line's name as alerts and the History say it.
func shownName(l QualityLine) string {
	if l.Name != "" {
		return l.Name
	}
	if l.ID == QualityFourK {
		return fourKName
	}
	return "Standard"
}

// pickQuality is the line p asks for: id, or p's first line when id is
// empty. A line p may not use is ErrQualityNotOffered.
func pickQuality(ctx context.Context, db querier, p Person, kind, id string) (QualityLine, error) {
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return QualityLine{}, err
	}
	lines, err := qualityLines(ctx, db, set, kind)
	if err != nil {
		return QualityLine{}, err
	}
	if p.Qualities == nil {
		if err := loadPersonQualities(ctx, db, &p); err != nil {
			return QualityLine{}, err
		}
	}
	mine := offered(lines, p, kind)
	if id == "" && len(mine) > 0 {
		return mine[0], nil
	}
	if l, ok := lineByID(mine, id); ok {
		return l, nil
	}
	return QualityLine{}, ErrQualityNotOffered
}

// builtInLine is a built-in line as settings say it now, for asks made by the
// app itself (watchlists, imports) and the older calls: 4K or Standard.
func builtInLine(ctx context.Context, db querier, kind string, fourK bool) (QualityLine, error) {
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return QualityLine{}, err
	}
	lines, err := qualityLines(ctx, db, set, kind)
	if err != nil {
		return QualityLine{}, err
	}
	id := QualityStandard
	if fourK {
		id = QualityFourK
	}
	l, _ := lineByID(lines, id)
	return l, nil
}

// ImportedQuality puts the built-in line on an ask brought in from an older
// app, plain or 4K (Pick rules 0: the Request settings' when approved).
func ImportedQuality(a *domain.Ask) {
	if a.Kind != "movie" && a.Kind != "show" {
		return
	}
	l := QualityLine{ID: QualityStandard}
	if a.FourK {
		l = QualityLine{ID: QualityFourK, Extra: true}
	}
	withQuality(a, l)
}

// withQuality puts the line on an ask.
func withQuality(a *domain.Ask, l QualityLine) {
	a.Quality, a.QualityName, a.QualityRulesID, a.QualityExtra = l.ID, shownName(l), l.PickRulesID, l.Extra
	a.FourK = l.ID == QualityFourK
}
