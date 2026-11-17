package health

import "context"

// peopleChecks is one check per Jellyfin or Emby sign-in server whose user
// list the removed-people check reads: a warning while it cannot be read.
func (e *Engine) peopleChecks(ctx context.Context) ([]Result, error) {
	if e.UnreadUserLists == nil {
		return nil, nil
	}
	var out []Result
	for kind, msg := range e.UnreadUserLists(ctx) {
		r := Result{Key: "people_check:" + kind, Kind: "people_check", Subject: kindName[kind], State: OK}
		if msg != "" {
			r.State, r.Message = Warning, msg
		}
		out = append(out, r)
	}
	return out, nil
}

var kindName = map[string]string{"jellyfin": "Jellyfin", "emby": "Emby"}
