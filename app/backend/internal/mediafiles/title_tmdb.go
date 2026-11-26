package mediafiles

// TrustedTMDB is the Title's TMDB id for asking TMDB about it. For a show
// whose TMDB id is the same number as its own TVDB or TVmaze id it is "":
// that number was copied into the wrong field and on TMDB names another
// show, so its pictures, cast and episode orders would be that show's. A
// refresh looks the real id up.
func (t Title) TrustedTMDB() string {
	v := t.SourceIDs["tmdb"]
	if t.Kind == "show" && v != "" && (v == t.SourceIDs["tvdb"] || v == t.SourceIDs["tvmaze"]) {
		return ""
	}
	return v
}
