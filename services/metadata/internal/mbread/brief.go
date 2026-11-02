package mbread

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
)

// Brief is an artist's name, home country and URL links: what a picture
// lookup reads. It skips the full answer's release groups, which cost a
// third of the picture queue's CPU (2026-10-29 profile).
type Brief struct {
	Name, Country string
	Links         []string
}

// ArtistBrief answers an artist's Brief from the slim copy; false when the
// copy lacks it or there is no copy.
func (r *Reader) ArtistBrief(ctx context.Context, mbid string) (Brief, bool) {
	c := r.acquire()
	if c == nil {
		return Brief{}, false
	}
	defer c.mu.RUnlock()
	q := &query{ctx: ctx, c: c, credits: map[int64][]any{}, areas: map[int64]map[string]any{}}
	var id int64
	var name sql.NullString
	var area sql.NullInt64
	err := c.db.QueryRowContext(ctx, `SELECT id, name, area FROM artist WHERE gid=?`, strings.ToLower(mbid)).Scan(&id, &name, &area)
	if err != nil {
		return Brief{}, false
	}
	b := Brief{Name: name.String, Country: country(q.area(area))}
	rows, err := q.rows(`SELECT u.url FROM l_artist_url x JOIN url u ON u.id=x.entity1 WHERE x.entity0=? ORDER BY u.url`, id)
	if err == nil {
		for rows.Next() {
			var u sql.NullString
			if rows.Scan(&u) == nil && u.String != "" {
				b.Links = append(b.Links, u.String)
			}
		}
		rows.Close()
	}
	return b, true
}

// HomonymIDs answers the ids of the other artists in the slim copy that
// carry exactly this name (at most 50; none when there is no copy).
func (r *Reader) HomonymIDs(ctx context.Context, name, except string) []string {
	c := r.acquire()
	if c == nil {
		return nil
	}
	defer c.mu.RUnlock()
	if name == "" {
		return nil
	}
	rows, err := c.db.QueryContext(ctx, `SELECT gid FROM artist WHERE name=? AND gid<>? LIMIT 50`, name, strings.ToLower(except))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// TitleArtists answers how many artists in the slim copy are credited on a
// release group of this title, case aside (0 when there is no copy). The
// title has no index: one call reads the release groups once (about half a
// second), so it is asked only for a match resting on one title.
func (r *Reader) TitleArtists(ctx context.Context, title string) int {
	c := r.acquire()
	if c == nil {
		return 0
	}
	defer c.mu.RUnlock()
	var n int
	if c.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT acn.artist) FROM release_group rg JOIN artist_credit_name acn ON acn.artist_credit=rg.artist_credit WHERE rg.name=? COLLATE NOCASE`, title).Scan(&n) != nil {
		return 0
	}
	return n
}

// ArtistBarcodes answers up to n barcodes of releases credited to the
// artist alone (one-artist credits), each once; the artist matchers look
// the albums up at the outside sources to find the artist's id there.
func (r *Reader) ArtistBarcodes(ctx context.Context, mbid string, n int) []string {
	c := r.acquire()
	if c == nil {
		return nil
	}
	defer c.mu.RUnlock()
	rows, err := c.db.QueryContext(ctx, `SELECT r.barcode FROM artist a
JOIN artist_credit_name acn ON acn.artist=a.id
JOIN artist_credit ac ON ac.id=acn.artist_credit AND ac.artist_count=1
JOIN release_group rg ON rg.artist_credit=ac.id
JOIN release r ON r.release_group=rg.id AND r.artist_credit=ac.id
WHERE a.gid=? AND r.barcode IS NOT NULL AND r.barcode<>'' GROUP BY r.barcode LIMIT ?`, strings.ToLower(mbid), n)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		if rows.Scan(&b) == nil {
			out = append(out, b)
		}
	}
	return out
}

// ArtistNames answers an artist's name, sort name and alias names (and
// their sort names): one read, nothing else of the artist.
func (r *Reader) ArtistNames(ctx context.Context, mbid string) []string {
	c := r.acquire()
	if c == nil {
		return nil
	}
	defer c.mu.RUnlock()
	rows, err := c.db.QueryContext(ctx, `SELECT a.name, a.sort_name FROM artist a WHERE a.gid=?
UNION SELECT al.name, al.sort_name FROM artist_alias al JOIN artist a ON a.id=al.artist WHERE a.gid=?`, strings.ToLower(mbid), strings.ToLower(mbid))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n, s sql.NullString
		if rows.Scan(&n, &s) == nil {
			for _, v := range []sql.NullString{n, s} {
				if v.String != "" {
					out = append(out, v.String)
				}
			}
		}
	}
	return out
}

// ArtistByBarcode answers the one artist a release of this barcode credits
// alone ("" when none, or when the barcode's releases credit several).
func (r *Reader) ArtistByBarcode(ctx context.Context, code string) string {
	c := r.acquire()
	if c == nil {
		return ""
	}
	defer c.mu.RUnlock()
	rows, err := c.db.QueryContext(ctx, `SELECT DISTINCT a.gid FROM release r
JOIN artist_credit ac ON ac.id=r.artist_credit AND ac.artist_count=1
JOIN artist_credit_name acn ON acn.artist_credit=ac.id
JOIN artist a ON a.id=acn.artist
WHERE r.barcode=? LIMIT 2`, code)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		if rows.Scan(&g) == nil {
			out = append(out, g)
		}
	}
	if len(out) != 1 {
		return ""
	}
	return out[0]
}

// ArtistLinks answers every artist link (artist id, address) whose address
// holds one of the parts: one read of the links table.
func (r *Reader) ArtistLinks(ctx context.Context, parts []string) [][2]string {
	c := r.acquire()
	if c == nil || len(parts) == 0 {
		if c != nil {
			c.mu.RUnlock()
		}
		return nil
	}
	defer c.mu.RUnlock()
	where := make([]string, len(parts))
	args := make([]any, len(parts))
	for i, p := range parts {
		where[i] = "u.url LIKE ?"
		args[i] = "%" + p + "%"
	}
	rows, err := c.db.QueryContext(ctx, `SELECT a.gid, u.url FROM l_artist_url l JOIN url u ON u.id=l.entity1 JOIN artist a ON a.id=l.entity0 WHERE `+strings.Join(where, " OR "), args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var g, u string
		if rows.Scan(&g, &u) == nil {
			out = append(out, [2]string{g, u})
		}
	}
	return out
}

// NameHolders counts the artists in the slim copy carrying a name as
// written, in lower, upper or title case, or with a capital first letter
// (the name index is used; no full scan).
func (r *Reader) NameHolders(ctx context.Context, name string) int {
	c := r.acquire()
	if c == nil {
		return 0
	}
	defer c.mu.RUnlock()
	if name == "" {
		return 0
	}
	low := strings.ToLower(name)
	upFirst := func(w string) string {
		r := []rune(w)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		return string(r)
	}
	first := upFirst(low)
	words := strings.Fields(low)
	for i, w := range words {
		words[i] = upFirst(w)
	}
	title := strings.Join(words, " ")
	var n int
	if c.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT id) FROM artist WHERE name IN (?,?,?,?,?)`, name, low, first, title, strings.ToUpper(name)).Scan(&n) != nil {
		return 0
	}
	return n
}
