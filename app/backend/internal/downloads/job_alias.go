package downloads

// jobIndex maps each job by its id and, when it has one, its second id, so a
// hybrid torrent sent by its v1 hash is found although qBittorrent lists it by
// its v2 hash. A job's own id always wins over another job's second id.
func jobIndex(jobs []Job) map[string]Job {
	have := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		have[j.ID] = j
	}
	for _, j := range jobs {
		if _, taken := have[j.AltID]; j.AltID != "" && !taken {
			have[j.AltID] = j
		}
	}
	return have
}
