package migrate

import "slices"

func (b *builder) exclusions(s *Snapshot) {
	b.cover("exclusions")
	kind := map[Kind]string{Radarr: "movie", Sonarr: "show", Lidarr: "artist"}[s.Kind]
	// A live read whose exclusions route failed says so in the reader's
	// notes (a read kept before live reads had the backup-only note).
	if slices.Contains(s.Notes, "the app has no exclusions list; skipped") || slices.Contains(s.Notes, "exclusion lists are read from a backup only") {
		b.not(GroupNeverAdd, b.from("exclusions", -1), "exclusions", "they could not be read through the API; read a backup to bring them")
	}
	for i, e := range s.Exclusions {
		from := b.from("exclusions", i)
		ids := map[string]string{}
		var key string
		for _, src := range []string{"tmdb", "tvdb", "musicbrainz", "imdb"} {
			if v := clip(e.IDs[src], 60); v != "" {
				ids[src] = v
				if key == "" {
					key = src + ":" + v
				}
			}
		}
		if kind == "" || key == "" || (kind == "movie" && ids["tmdb"] == "") {
			b.not(GroupNeverAdd, from, e.Name, "no id HolyArrs can match")
			continue
		}
		b.add(GroupNeverAdd, Item{Key: "never_add:" + kind + ":" + key, Name: e.Name, From: from, Detail: "kept off the Library",
			NeverAdd: &NeverAddPlan{Kind: kind, IDs: ids}})
	}
}
