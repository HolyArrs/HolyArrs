package shows

import "strings"

// StatusIssue carries the recorded cause of a failed or blocked episode.
// Summaries retain at most eight distinct issues, rather than every episode.
type StatusIssue struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

func addStatusIssue(issues []StatusIssue, state, reason string) []StatusIssue {
	if state != "failed" && state != "needs_a_decision" {
		return issues
	}
	issue := StatusIssue{State: state, Reason: strings.TrimSpace(reason)}
	for _, old := range issues {
		if old == issue {
			return issues
		}
	}
	if len(issues) < 8 {
		return append(issues, issue)
	}
	// A late failure must still outrank decisions when the summary is full.
	if state == "failed" {
		for i, old := range issues {
			if old.State == "needs_a_decision" {
				issues[i] = issue
				break
			}
		}
	}
	return issues
}
