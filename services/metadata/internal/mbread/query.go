package mbread

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// query is one answer's reads on one copy, with the artist credits and
// areas it has read already.
type query struct {
	ctx     context.Context
	c       *copyDB
	credits map[int64][]any
	areas   map[int64]map[string]any
	// evs and meds are release events and track-less media read ahead
	// for a whole release list (prefetch).
	evs  map[int64][]evRow
	meds map[int64][]medRow
}

type evRow struct {
	area    sql.NullInt64
	y, m, d sql.NullInt64
	known   bool // from release_country (else release_unknown_country)
}

type medRow struct {
	id, pos, count int64
	format, name   sql.NullString
}

// inList is ids as an SQL list.
func inList(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ",")
}

// prefetch reads the events and media of many releases in two queries
// each, so a long release list is not two reads a release.
func (q *query) prefetch(ids []int64) {
	if len(ids) == 0 {
		return
	}
	in := inList(ids)
	if q.evs == nil {
		q.evs, q.meds = map[int64][]evRow{}, map[int64][]medRow{}
	}
	for _, id := range ids {
		q.evs[id], q.meds[id] = nil, nil
	}
	if rows, err := q.rows(`SELECT release, country, date_year, date_month, date_day, 1 FROM release_country WHERE release IN (` + in + `)
UNION ALL SELECT release, NULL, date_year, date_month, date_day, 0 FROM release_unknown_country WHERE release IN (` + in + `)`); err == nil {
		for rows.Next() {
			var rel int64
			var e evRow
			if rows.Scan(&rel, &e.area, &e.y, &e.m, &e.d, &e.known) == nil {
				q.evs[rel] = append(q.evs[rel], e)
			}
		}
		rows.Close()
	}
	if rows, err := q.rows(`SELECT m.release, m.id, m.position, m.track_count, f.name, m.name FROM medium m LEFT JOIN medium_format f ON f.id=m.format
WHERE m.release IN (` + in + `) ORDER BY m.release, m.position`); err == nil {
		for rows.Next() {
			var rel int64
			var m medRow
			if rows.Scan(&rel, &m.id, &m.pos, &m.count, &m.format, &m.name) == nil {
				q.meds[rel] = append(q.meds[rel], m)
			}
		}
		rows.Close()
	}
}

func (q *query) rows(sqlText string, args ...any) (*sql.Rows, error) {
	return q.c.db.QueryContext(q.ctx, sqlText, args...)
}

// id answers the row id of a gid in table.
func (q *query) id(table, gid string) (int64, bool) {
	var id int64
	err := q.c.db.QueryRowContext(q.ctx, `SELECT id FROM "`+table+`" WHERE gid=?`, gid).Scan(&id)
	return id, err == nil
}

// date formats a dump date as MusicBrainz does: YYYY, YYYY-MM or
// YYYY-MM-DD ("" without a year).
func date(y, m, d sql.NullInt64) string {
	if !y.Valid {
		return ""
	}
	s := fmt.Sprintf("%04d", y.Int64)
	if m.Valid {
		s += fmt.Sprintf("-%02d", m.Int64)
		if d.Valid {
			s += fmt.Sprintf("-%02d", d.Int64)
		}
	}
	return s
}

// dateKey orders dates with missing parts after known ones.
func dateKey(y, m, d sql.NullInt64) [3]int64 {
	k := [3]int64{9999, 99, 99}
	if y.Valid {
		k[0] = y.Int64
	}
	if m.Valid {
		k[1] = m.Int64
	}
	if d.Valid {
		k[2] = d.Int64
	}
	return k
}

func less(a, b [3]int64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func str(n sql.NullString) string { return n.String }

// flag reads a dump boolean ("t"/"f").
func flag(n sql.NullString) bool { return n.String == "t" }

func lifeSpan(by, bm, bd, ey, em, ed sql.NullInt64, ended sql.NullString) map[string]any {
	ls := map[string]any{"begin": nil, "end": nil, "ended": flag(ended)}
	if s := date(by, bm, bd); s != "" {
		ls["begin"] = s
	}
	if s := date(ey, em, ed); s != "" {
		ls["end"] = s
	}
	return ls
}

// genders are MusicBrainz's gender names (the gender table is not kept).
var genders = map[int64]string{1: "Male", 2: "Female", 3: "Other", 4: "Not applicable", 5: "Non-binary"}

// area answers an area as MusicBrainz nests it (nil when unknown).
func (q *query) area(id sql.NullInt64) map[string]any {
	if !id.Valid {
		return nil
	}
	if a, ok := q.areas[id.Int64]; ok {
		return a
	}
	var gid, name sql.NullString
	if q.c.db.QueryRowContext(q.ctx, `SELECT gid, name FROM area WHERE id=?`, id.Int64).Scan(&gid, &name) != nil {
		q.areas[id.Int64] = nil
		return nil
	}
	a := map[string]any{"id": gid.String, "name": name.String, "sort-name": name.String, "disambiguation": ""}
	codes := []string{}
	if q.c.iso {
		if rows, err := q.rows(`SELECT code FROM iso_3166_1 WHERE area=? ORDER BY code`, id.Int64); err == nil {
			for rows.Next() {
				var c string
				if rows.Scan(&c) == nil {
					codes = append(codes, c)
				}
			}
			rows.Close()
		}
	}
	a["iso-3166-1-codes"] = codes
	q.areas[id.Int64] = a
	return a
}

func country(a map[string]any) string {
	if a == nil {
		return ""
	}
	if codes, _ := a["iso-3166-1-codes"].([]string); len(codes) > 0 {
		return codes[0]
	}
	return ""
}

// credit answers an artist credit as MusicBrainz's "artist-credit" list.
func (q *query) credit(id sql.NullInt64) []any {
	if !id.Valid {
		return []any{}
	}
	if c, ok := q.credits[id.Int64]; ok {
		return c
	}
	out := []any{}
	rows, err := q.rows(`SELECT acn.name, acn.join_phrase, a.gid, a.name, a.sort_name, a.comment, t.name, a.area
FROM artist_credit_name acn JOIN artist a ON a.id=acn.artist LEFT JOIN artist_type t ON t.id=a.type
WHERE acn.artist_credit=? ORDER BY acn.position`, id.Int64)
	if err == nil {
		type row struct {
			name, join, gid, aname, sort, comment, typ sql.NullString
			area                                       sql.NullInt64
		}
		var rs []row
		for rows.Next() {
			var x row
			if rows.Scan(&x.name, &x.join, &x.gid, &x.aname, &x.sort, &x.comment, &x.typ, &x.area) == nil {
				rs = append(rs, x)
			}
		}
		rows.Close()
		for _, x := range rs {
			a := map[string]any{"id": x.gid.String, "name": x.aname.String, "sort-name": x.sort.String, "disambiguation": x.comment.String,
				"type": nilIfEmpty(x.typ.String)}
			if c := country(q.area(x.area)); c != "" {
				a["country"] = c
			}
			out = append(out, map[string]any{"name": x.name.String, "joinphrase": x.join.String, "artist": a})
		}
	}
	q.credits[id.Int64] = out
	return out
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// tags answers an entity's tags and genres (the tags whose name is a
// genre), most used first.
func (q *query) tags(table, col string, id int64) (tags, genres []any) {
	tags, genres = []any{}, []any{}
	rows, err := q.rows(`SELECT t.name, x.count FROM "`+table+`" x JOIN tag t ON t.id=x.tag
WHERE x."`+col+`"=? AND x.count>0 ORDER BY x.count DESC, t.name`, id)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name sql.NullString
		var n int64
		if rows.Scan(&name, &n) != nil {
			continue
		}
		tags = append(tags, map[string]any{"name": name.String, "count": n})
		if g, ok := q.c.genres[name.String]; ok {
			genres = append(genres, map[string]any{"id": g[0], "name": name.String, "count": n, "disambiguation": g[1]})
		}
	}
	return
}

// urlRels answers an entity's URL relations as MusicBrainz's "relations".
func (q *query) urlRels(table string, id int64) []any {
	out := []any{}
	rows, err := q.rows(`SELECT lt.name, lt.gid, u.gid, u.url, l.ended, l.begin_date_year, l.begin_date_month, l.begin_date_day,
 l.end_date_year, l.end_date_month, l.end_date_day
FROM "`+table+`" x JOIN link l ON l.id=x.link JOIN link_type lt ON lt.id=l.link_type JOIN url u ON u.id=x.entity1
WHERE x.entity0=? ORDER BY lt.name, u.url`, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var typ, typeID, ugid, u, ended sql.NullString
		var by, bm, bd, ey, em, ed sql.NullInt64
		if rows.Scan(&typ, &typeID, &ugid, &u, &ended, &by, &bm, &bd, &ey, &em, &ed) != nil {
			continue
		}
		rel := map[string]any{"type": typ.String, "type-id": typeID.String, "target-type": "url", "direction": "forward",
			"ended": flag(ended), "begin": nilIfEmpty(date(by, bm, bd)), "end": nilIfEmpty(date(ey, em, ed)),
			"attributes": []any{}, "attribute-values": map[string]any{}, "attribute-ids": map[string]any{},
			"source-credit": "", "target-credit": "", "url": map[string]any{"id": ugid.String, "resource": u.String}}
		out = append(out, rel)
	}
	return out
}

// artist answers an artist's source_data (nil when the copy lacks it):
// the lookup's fields with aliases, tags, genres and URL relations, and
// every release group with their count (the lean answer's second call).
func (q *query) artist(gid string) (map[string]any, error) {
	var id int64
	var name, sortName, comment, typ, ended sql.NullString
	var by, bm, bd, ey, em, ed, gender, area, beginArea, endArea sql.NullInt64
	err := q.c.db.QueryRowContext(q.ctx, `SELECT a.id, a.name, a.sort_name, a.comment, t.name, a.gender, a.area, a.begin_area, a.end_area,
 a.begin_date_year, a.begin_date_month, a.begin_date_day, a.end_date_year, a.end_date_month, a.end_date_day, a.ended
FROM artist a LEFT JOIN artist_type t ON t.id=a.type WHERE a.gid=?`, gid).Scan(&id, &name, &sortName, &comment, &typ, &gender, &area,
		&beginArea, &endArea, &by, &bm, &bd, &ey, &em, &ed, &ended)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	src := map[string]any{"id": gid, "name": name.String, "sort-name": sortName.String, "disambiguation": comment.String,
		"type": nilIfEmpty(typ.String), "gender": nil, "country": nil,
		"area": q.area(area), "begin-area": q.area(beginArea), "end-area": q.area(endArea),
		"life-span": lifeSpan(by, bm, bd, ey, em, ed, ended), "ipis": []any{}, "isnis": []any{}}
	if gender.Valid {
		src["gender"] = nilIfEmpty(genders[gender.Int64])
	}
	if c := country(q.area(area)); c != "" {
		src["country"] = c
	}
	aliases := []any{}
	if rows, err := q.rows(`SELECT name, sort_name, locale, primary_for_locale, ended FROM artist_alias WHERE artist=? ORDER BY name`, id); err == nil {
		for rows.Next() {
			var n, sn, loc, prim, end sql.NullString
			if rows.Scan(&n, &sn, &loc, &prim, &end) == nil {
				aliases = append(aliases, map[string]any{"name": n.String, "sort-name": sn.String, "locale": nilIfEmpty(loc.String),
					"primary": nilBool(prim), "ended": flag(end), "type": nil, "begin": nil, "end": nil})
			}
		}
		rows.Close()
	}
	src["aliases"] = aliases
	src["tags"], src["genres"] = q.tags("artist_tag", "artist", id)
	src["relations"] = q.urlRels("l_artist_url", id)
	groups, err := q.groups(id)
	if err != nil {
		return nil, err
	}
	src["release-groups"] = groups
	src["release-group-count"] = len(groups)
	return src, nil
}

func nilBool(n sql.NullString) any {
	if !n.Valid || n.String == "" {
		return nil
	}
	return n.String == "t"
}

// groupInfo is one release group row with its types and first date.
type groupInfo struct {
	id                   int64
	gid, title, comment  string
	primary, primaryID   string
	secondary, secondIDs []string
	credit               sql.NullInt64
	first                string
}

func (g groupInfo) json() map[string]any {
	return map[string]any{"id": g.gid, "title": g.title, "disambiguation": g.comment, "primary-type": nilIfEmpty(g.primary),
		"primary-type-id": nilIfEmpty(g.primaryID), "secondary-types": g.secondary, "secondary-type-ids": g.secondIDs,
		"first-release-date": g.first}
}

// loadGroups reads release groups chosen by where (on rg) with their
// types and first release dates.
func (q *query) loadGroups(where string, arg any) ([]*groupInfo, error) {
	rows, err := q.rows(`SELECT DISTINCT rg.id, rg.gid, rg.name, rg.comment, pt.name, pt.gid, rg.artist_credit FROM release_group rg
LEFT JOIN release_group_primary_type pt ON pt.id=rg.type WHERE `+where+` LIMIT ?`, arg, maxGroups)
	if err != nil {
		return nil, err
	}
	var out []*groupInfo
	byID := map[int64]*groupInfo{}
	for rows.Next() {
		g := &groupInfo{secondary: []string{}, secondIDs: []string{}}
		var gid, title, comment, pt, ptID sql.NullString
		if err := rows.Scan(&g.id, &gid, &title, &comment, &pt, &ptID, &g.credit); err != nil {
			rows.Close()
			return nil, err
		}
		g.gid, g.title, g.comment, g.primary, g.primaryID = gid.String, title.String, comment.String, pt.String, ptID.String
		out = append(out, g)
		byID[g.id] = g
	}
	rows.Close()
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(out))
	for _, g := range out {
		ids = append(ids, fmt.Sprint(g.id))
	}
	in := strings.Join(ids, ",")
	if rows, err := q.rows(`SELECT j.release_group, st.name, st.gid FROM release_group_secondary_type_join j
JOIN release_group_secondary_type st ON st.id=j.secondary_type WHERE j.release_group IN (` + in + `) ORDER BY st.child_order, st.name`); err == nil {
		for rows.Next() {
			var rg int64
			var n, g sql.NullString
			if rows.Scan(&rg, &n, &g) == nil && byID[rg] != nil {
				byID[rg].secondary = append(byID[rg].secondary, n.String)
				byID[rg].secondIDs = append(byID[rg].secondIDs, g.String)
			}
		}
		rows.Close()
	}
	best := map[int64][3]int64{}
	firsts := map[int64]string{}
	if rows, err := q.rows(`SELECT r.release_group, c.date_year, c.date_month, c.date_day FROM release r JOIN release_country c ON c.release=r.id
WHERE r.release_group IN (` + in + `) AND c.date_year IS NOT NULL
UNION ALL SELECT r.release_group, u.date_year, u.date_month, u.date_day FROM release r JOIN release_unknown_country u ON u.release=r.id
WHERE r.release_group IN (` + in + `) AND u.date_year IS NOT NULL`); err == nil {
		for rows.Next() {
			var rg int64
			var y, m, d sql.NullInt64
			if rows.Scan(&rg, &y, &m, &d) != nil {
				continue
			}
			k := dateKey(y, m, d)
			if b, ok := best[rg]; !ok || less(k, b) {
				best[rg], firsts[rg] = k, date(y, m, d)
			}
		}
		rows.Close()
	}
	for _, g := range out {
		g.first = firsts[g.id]
	}
	return out, nil
}

// groups answers an artist's release groups (those whose credit names
// the artist), oldest first, as the release-group browse lists them.
func (q *query) groups(artist int64) ([]any, error) {
	gs, err := q.loadGroups(`rg.artist_credit IN (SELECT artist_credit FROM artist_credit_name WHERE artist=?)`, artist)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(gs, func(i, j int) bool {
		a, b := gs[i].first, gs[j].first
		if (a == "") != (b == "") {
			return b == ""
		}
		if a != b {
			return a < b
		}
		return gs[i].title < gs[j].title
	})
	out := make([]any, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.json())
	}
	return out, nil
}

// releaseRow is one release with what its JSON needs.
type releaseRow struct {
	id                                    int64
	gid, title, status, statusID, barcode string
	comment                               string
	credit, group                         sql.NullInt64
	quality                               sql.NullInt64
}

func (q *query) scanReleases(rows *sql.Rows) ([]releaseRow, error) {
	defer rows.Close()
	var out []releaseRow
	for rows.Next() {
		var r releaseRow
		var gid, title, st, stID, bc, cm sql.NullString
		if err := rows.Scan(&r.id, &gid, &title, &st, &stID, &bc, &cm, &r.credit, &r.group, &r.quality); err != nil {
			return nil, err
		}
		r.gid, r.title, r.status, r.statusID, r.barcode, r.comment = gid.String, title.String, st.String, stID.String, bc.String, cm.String
		out = append(out, r)
	}
	return out, rows.Err()
}

const releaseCols = `SELECT r.id, r.gid, r.name, s.name, s.gid, r.barcode, r.comment, r.artist_credit, r.release_group, r.quality
FROM release r LEFT JOIN release_status s ON s.id=r.status `

// events answers a release's release events, earliest first, and its
// date and country (the earliest event's).
func (q *query) events(rel int64) (evs []any, first, ctry string) {
	evs = []any{}
	type ev struct {
		k    [3]int64
		d    string
		area map[string]any
	}
	var list []ev
	rs, ok := q.evs[rel]
	if !ok {
		q.prefetch([]int64{rel})
		rs = q.evs[rel]
	}
	for _, x := range rs {
		var a map[string]any
		if x.known {
			a = q.area(x.area)
		}
		list = append(list, ev{dateKey(x.y, x.m, x.d), date(x.y, x.m, x.d), a})
	}
	sort.SliceStable(list, func(i, j int) bool { return less(list[i].k, list[j].k) })
	for i, e := range list {
		m := map[string]any{"date": e.d}
		if e.area != nil {
			m["area"] = e.area
		}
		evs = append(evs, m)
		if i == 0 {
			first, ctry = e.d, country(e.area)
		}
	}
	return evs, first, ctry
}

// media answers a release's media; with tracks, each medium's tracks
// with their recordings.
func (q *query) media(rel int64, withTracks bool) []any {
	out := []any{}
	ms, ok := q.meds[rel]
	if !ok {
		q.prefetch([]int64{rel})
		ms = q.meds[rel]
	}
	offset := int64(0)
	for _, m := range ms {
		mm := map[string]any{"position": m.pos, "format": nilIfEmpty(m.format.String), "title": m.name.String, "track-count": m.count,
			"track-offset": offset}
		offset += m.count
		if withTracks {
			mm["tracks"] = q.tracks(m.id)
		}
		out = append(out, mm)
	}
	return out
}

func (q *query) tracks(medium int64) []any {
	out := []any{}
	type tr struct {
		gid, number, name, rgid, rname, rcomment, video sql.NullString
		pos, length, rlength, credit, rcredit           sql.NullInt64
	}
	var ts []tr
	rows, err := q.rows(`SELECT t.gid, t.position, t.number, t.name, t.length, t.artist_credit,
 r.gid, r.name, r.length, r.comment, r.video, r.artist_credit
FROM track t JOIN recording r ON r.id=t.recording WHERE t.medium=? ORDER BY t.position`, medium)
	if err != nil {
		return out
	}
	for rows.Next() {
		var t tr
		if rows.Scan(&t.gid, &t.pos, &t.number, &t.name, &t.length, &t.credit, &t.rgid, &t.rname, &t.rlength, &t.rcomment, &t.video, &t.rcredit) == nil {
			ts = append(ts, t)
		}
	}
	rows.Close()
	for _, t := range ts {
		rec := map[string]any{"id": t.rgid.String, "title": t.rname.String, "length": nullInt(t.rlength), "disambiguation": t.rcomment.String,
			"video": flag(t.video), "artist-credit": q.credit(t.rcredit)}
		out = append(out, map[string]any{"id": t.gid.String, "position": t.pos.Int64, "number": t.number.String, "title": t.name.String,
			"length": nullInt(t.length), "artist-credit": q.credit(t.credit), "recording": rec})
	}
	return out
}

func nullInt(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

// groupBrief is the release group as a release nests it.
func (q *query) groupBrief(id sql.NullInt64) map[string]any {
	if !id.Valid {
		return nil
	}
	gs, err := q.loadGroups(`rg.id=?`, id.Int64)
	if err != nil || len(gs) == 0 {
		return nil
	}
	m := gs[0].json()
	m["artist-credit"] = q.credit(gs[0].credit)
	return m
}

// releaseJSON makes one release's source_data.
func (q *query) releaseJSON(r releaseRow, group map[string]any, withTracks, withRels bool) map[string]any {
	evs, first, ctry := q.events(r.id)
	m := map[string]any{"id": r.gid, "title": r.title, "status": nilIfEmpty(r.status), "status-id": nilIfEmpty(r.statusID),
		"date": first, "country": nilIfEmpty(ctry), "barcode": nilIfEmpty(r.barcode), "disambiguation": r.comment, "packaging": nil,
		"quality": quality(r.quality), "release-events": evs, "artist-credit": q.credit(r.credit), "media": q.media(r.id, withTracks),
		"label-info": []any{}}
	if group != nil {
		m["release-group"] = group
	}
	if withRels {
		m["relations"] = q.urlRels("l_release_url", r.id)
	}
	return m
}

func quality(n sql.NullInt64) any {
	switch {
	case !n.Valid:
		return nil
	case n.Int64 == 2:
		return "high"
	case n.Int64 == 0:
		return "low"
	}
	return "normal"
}

// release answers one release's source_data (nil when missing): with
// tracks, its media with recordings (the tracklist call's answer).
func (q *query) release(gid string, withTracks bool) (map[string]any, error) {
	rows, err := q.rows(releaseCols+`WHERE r.gid=?`, strings.ToLower(gid))
	if err != nil {
		return nil, err
	}
	rs, err := q.scanReleases(rows)
	if err != nil || len(rs) == 0 {
		return nil, err
	}
	return q.releaseJSON(rs[0], q.groupBrief(rs[0].group), withTracks, true), nil
}

// groupReleases answers a release group's releases (up to the edition
// cap) with media but no tracks, as the release browse answers them, and
// how many it has in all.
func (q *query) groupReleases(group int64, withGroup bool) ([]map[string]any, int, error) {
	var count int
	if err := q.c.db.QueryRowContext(q.ctx, `SELECT COUNT(*) FROM release WHERE release_group=?`, group).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.rows(releaseCols+`WHERE r.release_group=? ORDER BY r.id LIMIT ?`, group, maxReleases)
	if err != nil {
		return nil, 0, err
	}
	rs, err := q.scanReleases(rows)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, len(rs))
	for i, r := range rs {
		ids[i] = r.id
	}
	q.prefetch(ids)
	var brief map[string]any
	if withGroup {
		brief = q.groupBrief(sql.NullInt64{Int64: group, Valid: true})
	}
	out := make([]map[string]any, 0, len(rs))
	for _, r := range rs {
		out = append(out, q.releaseJSON(r, brief, false, false))
	}
	return out, count, nil
}

// releaseGroup answers a release group's source_data (nil when missing):
// the lookup's fields with its releases, tags, genres and URL relations,
// and the media of its earliest official release (the lean answer's
// tracklist call, "tracklist_release").
func (q *query) releaseGroup(gid string) (map[string]any, error) {
	gs, err := q.loadGroups(`rg.gid=?`, gid)
	if err != nil || len(gs) == 0 {
		return nil, err
	}
	g := gs[0]
	src := g.json()
	src["artist-credit"] = q.credit(g.credit)
	rels, _, err := q.groupReleases(g.id, false)
	if err != nil {
		return nil, err
	}
	list := make([]any, 0, len(rels))
	best, bestDate, bestOfficial := "", "", false
	for _, r := range rels {
		// Each release keeps its total track count (the Deezer matcher
		// compares against the smallest official edition).
		if ms, ok := r["media"].([]any); ok {
			var n int64
			for _, m := range ms {
				if mm, ok := m.(map[string]any); ok {
					c, _ := mm["track-count"].(int64)
					n += c
				}
			}
			r["track-count"] = n
		}
		delete(r, "media")
		delete(r, "artist-credit")
		list = append(list, r)
		id, _ := r["id"].(string)
		d, _ := r["date"].(string)
		official := r["status"] == "Official"
		if best == "" || (official && !bestOfficial) || (official == bestOfficial && d != "" && (bestDate == "" || d < bestDate)) {
			best, bestDate, bestOfficial = id, d, official
		}
	}
	src["releases"] = list
	src["tags"], src["genres"] = q.tags("release_group_tag", "release_group", g.id)
	src["relations"] = q.urlRels("l_release_group_url", g.id)
	if best != "" {
		if rel, err := q.release(best, true); err == nil && rel != nil {
			src["media"] = rel["media"]
			src["tracklist_release"] = best
		}
	}
	return src, nil
}

// recording answers a recording's source_data (nil when missing): its
// fields, artist credit, first release date and the releases it is on.
func (q *query) recording(gid string) (map[string]any, error) {
	var id int64
	var name, comment, video sql.NullString
	var length, credit sql.NullInt64
	err := q.c.db.QueryRowContext(q.ctx, `SELECT id, name, length, comment, video, artist_credit FROM recording WHERE gid=?`, gid).
		Scan(&id, &name, &length, &comment, &video, &credit)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	src := map[string]any{"id": gid, "title": name.String, "length": nullInt(length), "disambiguation": comment.String,
		"video": flag(video), "artist-credit": q.credit(credit), "isrcs": []any{}, "tags": []any{}, "genres": []any{}, "relations": []any{}}
	rows, err := q.rows(releaseCols+`WHERE r.id IN (SELECT m.release FROM track t JOIN medium m ON m.id=t.medium WHERE t.recording=?) ORDER BY r.id LIMIT ?`, id, maxRecRels)
	if err != nil {
		return nil, err
	}
	rs, err := q.scanReleases(rows)
	if err != nil {
		return nil, err
	}
	list := make([]any, 0, len(rs))
	first, firstKey := "", [3]int64{9999, 99, 99}
	for _, r := range rs {
		m := q.releaseJSON(r, q.groupBrief(r.group), false, false)
		delete(m, "media")
		list = append(list, m)
		if d, _ := m["date"].(string); d != "" {
			if k := keyOf(d); less(k, firstKey) {
				first, firstKey = d, k
			}
		}
	}
	src["releases"] = list
	src["first-release-date"] = first
	return src, nil
}

// keyOf is dateKey for a formatted date.
func keyOf(d string) [3]int64 {
	k := [3]int64{9999, 99, 99}
	var y, m, dd int64
	n, _ := fmt.Sscanf(d, "%d-%d-%d", &y, &m, &dd)
	if n >= 1 {
		k[0] = y
	}
	if n >= 2 {
		k[1] = m
	}
	if n >= 3 {
		k[2] = dd
	}
	return k
}
