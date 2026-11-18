package library

import (
	"context"
)

// MissingMovieSortColumns are the columns the movie missing list may sort on.
var MissingMovieSortColumns = map[string]bool{"t.cinema_date": true, "t.digital_date": true, "t.physical_date": true,
	"t.sort_name": true, "t.year": true, "i.last_searched_at": true}

// MissingMovies pages the movies that have no File in the main Version, out
// or not: the old apps list a movie that is not out yet too. kept false gives
// the movies that are not kept up to date instead. The sort column must be one
// of MissingMovieSortColumns; desc sorts from the biggest value; a page past
// the end is empty.
func MissingMovies(ctx context.Context, q MissingReader, kept bool, sortColumn string, desc bool, page, size int) (MissingPage, error) {
	if !MissingMovieSortColumns[sortColumn] {
		return MissingPage{}, ErrMissingSort
	}
	sqlDir := "ASC"
	if desc {
		sqlDir = "DESC"
	}
	filter := keptWhere
	if !kept {
		filter = "NOT " + keptWhere
	}
	from := `FROM items i JOIN titles t ON t.id = i.title_id JOIN versions v ON v.title_id = i.title_id AND v.is_main = 1
		JOIN slots s ON s.item_id = i.id AND s.version_id = v.id
		WHERE i.kind = 'movie' AND i.dropped_at IS NULL AND t.kind = 'movie' AND t.state != 'removed' AND s.state != 'removed'
		AND s.file_id IS NULL AND ` + filter
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(DISTINCT t.id) "+from).Scan(&total); err != nil {
		return MissingPage{}, err
	}
	offset := total // a page past the end (checked first, so a huge page cannot overflow)
	if page-1 <= total/size {
		offset = (page - 1) * size
	}
	rows, err := q.QueryContext(ctx, "SELECT DISTINCT t.id, "+sortColumn+" "+from+" ORDER BY "+sortColumn+" "+sqlDir+", t.id "+sqlDir+" LIMIT ? OFFSET ?",
		size, offset)
	if err != nil {
		return MissingPage{}, err
	}
	defer rows.Close()
	out := MissingPage{Total: total}
	for rows.Next() {
		var id int64
		var key any
		if err := rows.Scan(&id, &key); err != nil {
			return MissingPage{}, err
		}
		out.IDs = append(out.IDs, id)
	}
	return out, rows.Err()
}
