package mediafiles

// FileLanguages is what language trait tests read for a File in the
// Library: its audio track languages, else the Version's audio languages;
// nil when neither is known (the tests then read the name).
func FileLanguages(mediaInfo map[string]any, version []string) []string {
	var out []string
	if list, ok := mediaInfo["audio_languages"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 && len(version) > 0 {
		out = version
	}
	return out
}
