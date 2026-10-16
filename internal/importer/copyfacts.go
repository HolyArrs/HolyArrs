package importer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/mediainfo"
	"github.com/CoderLuii/holyarrs/internal/parse"
)

// EditionFileName emits Plex's documented edition tag. Other server mappings
// are described in docs/movie-editions.md; the tag is not scan evidence.
func EditionFileName(name, edition string) string {
	if edition == "" {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	// A previous rename may already have emitted this exact tag.
	tag := " {edition-" + edition + "}"
	if strings.Contains(base, "{edition-") {
		if start := strings.Index(base, "{edition-"); start >= 0 {
			if end := strings.Index(base[start:], "}"); end >= 0 {
				base = strings.TrimSpace(base[:start] + base[start+end+1:])
			}
		}
	}
	return base + tag + strings.ToLower(ext)
}

// InspectFacts keeps inspected container facts separate from filename quality.
// Errors expose a status, never a path or a raw parser error to a client.
func InspectFacts(file string) map[string]any {
	i, err := mediainfo.Inspect(file)
	if err != nil {
		i = mediainfo.Info{Status: "error", AudioUnknown: true}
	}
	if i.Status != "ok" && i.AudioTracks == 0 {
		i.AudioUnknown = true
	}
	if i.AudioLanguages == nil {
		i.AudioLanguages = []string{}
	}
	facts := map[string]any{"inspection_status": i.Status, "container": i.Container, "video_codec": i.VideoCodec,
		"audio_languages": i.AudioLanguages, "audio_tracks": i.AudioTracks, "audio_unknown": i.AudioUnknown}
	if fi, err := os.Stat(file); err == nil {
		facts["inspection_size"] = fi.Size()
		facts["inspection_modified"] = strconv.FormatInt(fi.ModTime().UnixNano(), 10)
	}
	return facts
}

// EditionKey compares explicit edition labels without punctuation differences.
func EditionKey(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}

// ParsedEdition returns one unambiguous edition stated by the name. Empty is
// unknown or ambiguous; callers that accept a correction require confirmation.
func ParsedEdition(name string) string {
	editions := parse.Parse(name, parse.Hints{}).Edition
	if len(editions) != 1 {
		return ""
	}
	return editions[0]
}

// AudioMeets requires each chosen language to be present in inspected tracks.
// Extra unlabelled tracks stay visible but cannot supply a missing language.
func AudioMeets(facts map[string]any, wanted []string) bool {
	if len(wanted) == 0 {
		return true
	}
	actual := map[string]bool{}
	switch langs := facts["audio_languages"].(type) {
	case []string:
		for _, l := range langs {
			actual[l] = true
		}
	case []any:
		for _, l := range langs {
			if s, ok := l.(string); ok {
				actual[s] = true
			}
		}
	}
	for _, l := range wanted {
		if !actual[l] {
			return false
		}
	}
	return facts["inspection_status"] != "error" && facts["inspection_status"] != "unsupported"
}

func copyRefusal(v domain.Version, fileName, releaseName, confirmed string, facts map[string]any) string {
	fileEditions := parse.Parse(fileName, parse.Hints{}).Edition
	releaseEditions := parse.Parse(releaseName, parse.Hints{}).Edition
	edition, releaseEdition := "", ""
	if len(fileEditions) == 1 {
		edition = fileEditions[0]
	}
	if len(releaseEditions) == 1 {
		releaseEdition = releaseEditions[0]
	}
	if confirmed != "" {
		edition = confirmed
	} else if v.Edition != "" && (len(fileEditions) > 1 || len(releaseEditions) > 1) {
		return "edition is ambiguous; confirm it explicitly"
	} else if edition == "" {
		edition = releaseEdition
	}
	if v.Edition != "" {
		if edition == "" {
			return "edition is unknown; confirm the edition or choose an unconstrained Version"
		}
		if EditionKey(edition) != EditionKey(v.Edition) {
			return "edition does not match the selected Version"
		}
		if confirmed == "" && releaseEdition != "" && EditionKey(releaseEdition) != EditionKey(edition) {
			return "file and release name disagree about the edition; confirm it explicitly"
		}
	}
	if !AudioMeets(facts, v.AudioLanguages) {
		return "inspected audio does not establish every language required by this Version; choose a suitable copy or review the requirement"
	}
	facts["edition"] = edition
	facts["edition_source"] = "name"
	if confirmed != "" {
		facts["edition_source"] = "confirmed"
	}
	if edition == "" {
		facts["edition_source"] = "unknown"
	}
	return ""
}

// InspectedLanguages returns actual track languages, never filename hints.
func InspectedLanguages(facts map[string]any) []string {
	out := []string{}
	switch langs := facts["audio_languages"].(type) {
	case []string:
		out = append(out, langs...)
	case []any:
		for _, value := range langs {
			if lang, ok := value.(string); ok {
				out = append(out, lang)
			}
		}
	}
	return out
}
