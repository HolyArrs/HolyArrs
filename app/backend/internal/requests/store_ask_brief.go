package requests

// Statements of ask_brief.go. The text is the exact SQL the server runs.
const (
	openMovieAskSQL = `SELECT a.id, a.state, EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?),
		(SELECT count(*) FROM ask_people p WHERE p.ask_id = a.id) FROM asks a WHERE a.kind = 'movie' AND a.tmdb_id = ? AND a.four_k = ? AND a.state IN ('asked', 'approved')`
	openShowAskSQL = `SELECT a.id, a.state, a.seasons, EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?),
		(SELECT count(*) FROM ask_people p WHERE p.ask_id = a.id) FROM asks a WHERE a.kind = 'show' AND a.state IN ('asked', 'approved') AND (`
	titleAsksMineSQL  = "a.title_id = ? AND EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)"
	openArtistAsksSQL = `SELECT a.id, a.state, a.albums, EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?),
		(SELECT count(*) FROM ask_people p WHERE p.ask_id = a.id) FROM asks a WHERE a.kind = 'artist' AND a.state IN ('asked', 'approved')
		AND json_extract(a.source_ids, '$.musicbrainz') = ? ORDER BY a.id`
)
