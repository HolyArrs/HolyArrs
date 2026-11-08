package mediafiles

import (
	"errors"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// OwnTrait is a trait a person makes: a test on a Release's name or
// on its release group, used in Pick rules like the built-in traits under
// the key OwnTraitKey(id). Each term is plain words, matched anywhere in
// the name (or as the whole group) ignoring case, or a /pattern/.
type OwnTrait struct {
	database.Meta
	Name  string   `db:"name"`
	Match string   `db:"match"` // name, group
	Terms []string `db:"terms,json"`
}

// OwnTraitPrefix starts the key of an own trait in Pick rules.
const OwnTraitPrefix = "own:"

// OwnTraitKey is the Pick rules key of the own trait with this id.
func OwnTraitKey(id int64) string { return OwnTraitPrefix + strconv.FormatInt(id, 10) }

// OwnTraitID reads the id out of a Pick rules key; ok is false for any other key.
func OwnTraitID(key string) (id int64, ok bool) {
	rest, found := strings.CutPrefix(key, OwnTraitPrefix)
	if !found {
		return 0, false
	}
	// Only the plain form counts (own:7, never own:07 or own:+7): removing
	// a trait looks for exactly that key.
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil && id > 0 && rest == strconv.FormatInt(id, 10)
}

// TermPattern is a term written as /pattern/: its pattern, and whether it is one.
func TermPattern(term string) (string, bool) {
	if len(term) >= 3 && strings.HasPrefix(term, "/") && strings.HasSuffix(term, "/") {
		return term[1 : len(term)-1], true
	}
	return "", false
}

// CheckOwnTrait checks a trait before it is saved and cleans its terms.
func CheckOwnTrait(t *OwnTrait) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || !utf8.ValidString(t.Name) || utf8.RuneCountInString(t.Name) > 60 {
		return fieldErr("name", "The name must hold 1 to 60 characters.")
	}
	if t.Match != "name" && t.Match != "group" {
		return fieldErr("match", "Match the Release name or its release group.")
	}
	var terms []string
	for _, raw := range t.Terms {
		term := strings.TrimSpace(raw)
		if term == "" {
			continue
		}
		if utf8.RuneCountInString(term) > 200 {
			return fieldErr("terms", "Each term holds at most 200 characters.")
		}
		if p, ok := TermPattern(term); ok {
			if _, err := regexp.Compile("(?i)" + p); err != nil {
				// Only what is wrong: the raw error names the (?i) added above.
				why := "it cannot be read"
				var se *syntax.Error
				if errors.As(err, &se) {
					why = string(se.Code)
				}
				return fieldErr("terms", "%s is not a pattern HolyArrs can read (%s).", term, why)
			}
		}
		terms = append(terms, term)
	}
	if len(terms) == 0 || len(terms) > 50 {
		return fieldErr("terms", "List 1 to 50 terms.")
	}
	t.Terms = terms
	return nil
}
