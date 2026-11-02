package mbdump

import "strings"

// SchemaSequence is the dump schema the column lists below are written
// for. A dump with another sequence is refused.
const SchemaSequence = 31

// table is one kept dump table: its columns in dump order (name and
// SQLite type) and the indexes made after the load.
type table struct {
	cols    []string
	indexes []string
}

func dates(prefix string) []string {
	return []string{prefix + "_year INTEGER", prefix + "_month INTEGER", prefix + "_day INTEGER"}
}

func cols(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func s(c ...string) []string { return c }

var typeCols = s("id INTEGER PRIMARY KEY", "name TEXT", "parent INTEGER", "child_order INTEGER", "description TEXT", "gid TEXT")

var linkCols = s("id INTEGER PRIMARY KEY", "link INTEGER", "entity0 INTEGER", "entity1 INTEGER", "edits_pending INTEGER",
	"last_updated TEXT", "link_order INTEGER", "entity0_credit TEXT", "entity1_credit TEXT")

var tagCols = func(entity string) []string {
	return s(entity+" INTEGER", "tag INTEGER", "count INTEGER", "last_updated TEXT")
}

// Tables are the dump tables the slim copy keeps; everything else in the
// dump (edit history, votes, collections, other entities) is skipped.
// Tables from the derived dump (tags) are kept when that archive is read.
var Tables = map[string]table{
	"artist": {cols(s("id INTEGER PRIMARY KEY", "gid TEXT", "name TEXT", "sort_name TEXT"), dates("begin_date"), dates("end_date"),
		s("type INTEGER", "area INTEGER", "gender INTEGER", "comment TEXT", "edits_pending INTEGER", "last_updated TEXT", "ended TEXT",
			"begin_area INTEGER", "end_area INTEGER")),
		s("gid", "name")},
	"artist_alias": {cols(s("id INTEGER PRIMARY KEY", "artist INTEGER", "name TEXT", "locale TEXT", "edits_pending INTEGER",
		"last_updated TEXT", "type INTEGER", "sort_name TEXT"), dates("begin_date"), dates("end_date"),
		s("primary_for_locale TEXT", "ended TEXT")),
		s("artist", "name")},
	"artist_credit": {s("id INTEGER PRIMARY KEY", "name TEXT", "artist_count INTEGER", "ref_count INTEGER", "created TEXT",
		"edits_pending INTEGER", "gid TEXT"), nil},
	"artist_credit_name": {s("artist_credit INTEGER", "position INTEGER", "artist INTEGER", "name TEXT", "join_phrase TEXT"),
		s("artist_credit", "artist")},
	"artist_type": {typeCols, nil},
	"area": {cols(s("id INTEGER PRIMARY KEY", "gid TEXT", "name TEXT", "type INTEGER", "edits_pending INTEGER", "last_updated TEXT"),
		dates("begin_date"), dates("end_date"), s("ended TEXT", "comment TEXT")), nil},
	"iso_3166_1": {s("area INTEGER", "code TEXT"), s("area")},
	"release_group": {s("id INTEGER PRIMARY KEY", "gid TEXT", "name TEXT", "artist_credit INTEGER", "type INTEGER", "comment TEXT",
		"edits_pending INTEGER", "last_updated TEXT"), s("gid", "artist_credit")},
	"release_group_primary_type":        {typeCols, nil},
	"release_group_secondary_type":      {typeCols, nil},
	"release_group_secondary_type_join": {s("release_group INTEGER", "secondary_type INTEGER", "created TEXT"), s("release_group")},
	"release": {s("id INTEGER PRIMARY KEY", "gid TEXT", "name TEXT", "artist_credit INTEGER", "release_group INTEGER", "status INTEGER",
		"packaging INTEGER", "language INTEGER", "script INTEGER", "barcode TEXT", "comment TEXT", "edits_pending INTEGER",
		"quality INTEGER", "last_updated TEXT"), s("gid", "release_group", "barcode")},
	"release_status":          {typeCols, nil},
	"release_country":         {cols(s("release INTEGER", "country INTEGER"), dates("date")), s("release")},
	"release_unknown_country": {cols(s("release INTEGER"), dates("date")), s("release")},
	"medium": {s("id INTEGER PRIMARY KEY", "release INTEGER", "position INTEGER", "format INTEGER", "name TEXT", "edits_pending INTEGER",
		"last_updated TEXT", "track_count INTEGER", "gid TEXT"), s("release")},
	"medium_format": {s("id INTEGER PRIMARY KEY", "name TEXT", "parent INTEGER", "child_order INTEGER", "year INTEGER",
		"has_discids TEXT", "description TEXT", "gid TEXT"), nil},
	"track": {s("id INTEGER PRIMARY KEY", "gid TEXT", "recording INTEGER", "medium INTEGER", "position INTEGER", "number TEXT",
		"name TEXT", "artist_credit INTEGER", "length INTEGER", "edits_pending INTEGER", "last_updated TEXT", "is_data_track TEXT"),
		s("medium", "recording")},
	"recording": {s("id INTEGER PRIMARY KEY", "gid TEXT", "name TEXT", "artist_credit INTEGER", "length INTEGER", "comment TEXT",
		"edits_pending INTEGER", "last_updated TEXT", "video TEXT"), s("gid", "artist_credit")},
	"url": {s("id INTEGER PRIMARY KEY", "gid TEXT", "url TEXT", "edits_pending INTEGER", "last_updated TEXT"), s("url")},
	"link": {cols(s("id INTEGER PRIMARY KEY", "link_type INTEGER"), dates("begin_date"), dates("end_date"),
		s("attribute_count INTEGER", "created TEXT", "ended TEXT")), nil},
	"link_type": {s("id INTEGER PRIMARY KEY", "parent INTEGER", "child_order INTEGER", "gid TEXT", "entity_type0 TEXT",
		"entity_type1 TEXT", "name TEXT", "description TEXT", "link_phrase TEXT", "reverse_link_phrase TEXT", "long_link_phrase TEXT",
		"last_updated TEXT", "is_deprecated TEXT", "has_dates TEXT", "entity0_cardinality INTEGER", "entity1_cardinality INTEGER"), nil},
	"l_artist_url":        {linkCols, s("entity0", "entity1")},
	"l_release_group_url": {linkCols, s("entity0", "entity1")},
	"l_release_url":       {linkCols, s("entity0", "entity1")},
	"genre":               {s("id INTEGER PRIMARY KEY", "gid TEXT", "name TEXT", "comment TEXT", "edits_pending INTEGER", "last_updated TEXT"), nil},
	"tag":                 {s("id INTEGER PRIMARY KEY", "name TEXT", "ref_count INTEGER"), s("name")},
	"artist_tag":          {tagCols("artist"), s("artist")},
	"release_group_tag":   {tagCols("release_group"), s("release_group")},
}

func (t table) names() []string {
	out := make([]string, len(t.cols))
	for i, c := range t.cols {
		out[i], _, _ = strings.Cut(c, " ")
	}
	return out
}
