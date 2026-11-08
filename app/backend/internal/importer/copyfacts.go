package importer

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// EditionFileName adds the edition to a file name.
func EditionFileName(name, edition string) string {
	if edition == "" {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
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

// InspectFacts reads the media facts of a file.
func InspectFacts(file string) map[string]any {
	i, err := mediafiles.Inspect(file)
	if err != nil {
		i = mediafiles.Info{Status: "error", AudioUnknown: true}
	}
	if i.Status != "ok" && i.AudioTracks == 0 {
		i.AudioUnknown = true
	}
	if i.AudioLanguages == nil {
		i.AudioLanguages = []string{}
	}
	facts := factsOf(i)
	if fi, err := os.Stat(file); err == nil {
		facts["inspection_size"] = fi.Size()
		facts["inspection_modified"] = strconv.FormatInt(fi.ModTime().UnixNano(), 10)
	}
	return facts
}

// FactsVersion is bumped when the stored fact keys change, so old facts get read again.
const FactsVersion = 2

// HeaderFactKeys lists the fact keys that come from the file header.
var HeaderFactKeys = []string{"resolution", "width", "height", "display_width", "runtime_seconds", "audio_channels", "audio_codec",
	"hdr", "bit_depth", "frame_rate", "bitrate"}

func factsOf(i mediafiles.Info) map[string]any {
	facts := map[string]any{"inspection_status": i.Status, "container": i.Container, "video_codec": i.VideoCodec,
		"audio_languages": i.AudioLanguages, "audio_tracks": i.AudioTracks, "audio_unknown": i.AudioUnknown,
		"facts_version": FactsVersion}
	if r := i.Resolution(); r > 0 {
		facts["resolution"] = strconv.Itoa(r) + "p"
		facts["width"], facts["height"] = i.Width, i.Height
		if i.DisplayWidth > 0 {
			facts["display_width"] = i.DisplayWidth
		}
	}
	if i.Duration > 0 {
		facts["runtime_seconds"] = int(i.Duration.Round(time.Second) / time.Second)
	}
	if len(i.AudioChannels) > 0 && i.AudioChannels[0] > 0 {
		facts["audio_channels"] = channelWords(i.AudioChannels[0])
	}
	if len(i.AudioCodecs) > 0 {
		facts["audio_codec"] = i.AudioCodecs[0]
	}
	subs := []string{}
	seen := map[string]bool{}
	for _, t := range i.SubtitleTracks {
		if t.Language != "" && !seen[t.Language] {
			seen[t.Language] = true
			subs = append(subs, t.Language)
		}
	}
	facts["subtitle_languages"] = subs
	if i.HDR != "" {
		facts["hdr"] = i.HDR
	}
	if i.BitDepth > 0 {
		facts["bit_depth"] = i.BitDepth
	}
	if i.FrameRate > 0 {
		facts["frame_rate"] = math.Round(i.FrameRate*1000) / 1000
	}
	if i.Bitrate > 0 {
		facts["bitrate"] = i.Bitrate
	}
	return facts
}

func channelWords(n int) string {
	switch n {
	case 6:
		return "5.1"
	case 7:
		return "6.1"
	case 8:
		return "7.1"
	}
	return strconv.Itoa(n) + ".0"
}

func factInt(facts map[string]any, key string) int {
	switch n := facts[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		v, _ := n.Int64()
		return int(v)
	}
	return 0
}

// NeedsFacts reports whether facts are missing or out of date.
func NeedsFacts(facts map[string]any) bool {
	return factInt(facts, "facts_version") < FactsVersion
}

// EditionKey returns a comparable key for the edition in a name.
func EditionKey(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}

// ParsedEdition returns the edition found in a release name.
func ParsedEdition(name string) string {
	editions := releasename.Parse(name, releasename.Hints{}).Edition
	if len(editions) != 1 {
		return ""
	}
	return editions[0]
}

// AudioMeets reports whether the audio facts satisfy the wanted languages.
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

func copyRefusal(v mediafiles.Version, fileName, releaseName, confirmed string, facts map[string]any) string {
	fileEditions := releasename.Parse(fileName, releasename.Hints{}).Edition
	releaseEditions := releasename.Parse(releaseName, releasename.Hints{}).Edition
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

// InspectedLanguages lists the audio languages found in the facts.
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
