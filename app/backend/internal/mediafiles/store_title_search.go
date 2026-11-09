package mediafiles

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"golang.org/x/text/unicode/norm"
)

var ErrTitleSearchQuery = errors.New("find by name accepts at most 256 characters and 16 words")

// SearchTitles matches literal word prefixes in local names, original names, aliases and years.
func SearchTitles(ctx context.Context, s *database.Store, query string) ([]Title, error) {
	if utf8.RuneCountInString(query) > 256 {
		return nil, ErrTitleSearchQuery
	}
	if strings.TrimSpace(query) == "" {
		return database.Select[Title](ctx, s.Read(), "titles", "state != 'removed' ORDER BY sort_name, id")
	}
	words := strings.FieldsFunc(norm.NFC.String(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	// Combining marks stay with their word, but cannot form a word on their own.
	terms := words[:0]
	for _, word := range words {
		if strings.ContainsFunc(word, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			terms = append(terms, word)
		}
	}
	words = terms
	if len(words) > 16 {
		return nil, ErrTitleSearchQuery
	}
	if len(words) == 0 {
		return []Title{}, nil
	}
	for i, word := range words {
		// A quoted phrase keeps FTS operators and punctuation out of the query.
		words[i] = `"` + word + `"*`
	}
	return database.Select[Title](ctx, s.Read(), "titles",
		"state != 'removed' AND id IN (SELECT rowid FROM title_search WHERE title_search MATCH ?) ORDER BY sort_name, id",
		strings.Join(words, " AND "))
}

// RebuildTitleSearch repairs the index through the same bounded writer as edits.
func RebuildTitleSearch(ctx context.Context, s *database.Store) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM title_search"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO title_search(rowid, name, sort_name, aliases, year)
			SELECT id, name, sort_name,
				trim(coalesce(original_name, '') || ' ' || coalesce((SELECT group_concat(value, ' ') FROM json_each(titles.aliases)), '')), year
			FROM titles`)
		return err
	})
}
