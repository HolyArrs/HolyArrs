package sources

import (
	"encoding/json"
	"testing"
)

// 4.3.5: the early and fake switches in Settings > Search.
// route: GET /api/v1/settings/search
// route: PUT /api/v1/settings/search
func TestReleaseSafetySettings(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	u := srv.URL + "/api/v1"
	var st SearchSettings
	_ = json.Unmarshal(do(t, "GET", u+"/settings/search", "", 200), &st)
	if !st.RefuseEarlyReleases || st.EarlyReleaseGraceHours != 24 || !st.RefuseTinyReleases || st.ImportMinVideoMB != 0 || st.ImportMinTrackKB != 0 ||
		!st.ImportSizeFloors || st.HoldUnknownReleases || st.EpisodeDefaultMinutes != 20 {
		t.Fatalf("defaults %+v", st)
	}
	_ = json.Unmarshal(do(t, "PUT", u+"/settings/search", `{"refuse_early_releases":false,"early_release_grace_hours":48,"refuse_tiny_releases":false,"import_min_video_mb":50,"import_min_track_kb":200,"import_size_floors":false,"hold_unknown_releases":true,"episode_default_minutes":45}`, 200), &st)
	if st.RefuseEarlyReleases || st.EarlyReleaseGraceHours != 48 || st.RefuseTinyReleases || st.ImportMinVideoMB != 50 || st.ImportMinTrackKB != 200 ||
		st.ImportSizeFloors || !st.HoldUnknownReleases || st.EpisodeDefaultMinutes != 45 {
		t.Fatalf("changed %+v", st)
	}
	do(t, "PUT", u+"/settings/search", `{"early_release_grace_hours":721}`, 400)
	do(t, "PUT", u+"/settings/search", `{"import_min_video_mb":-1}`, 400)
	do(t, "PUT", u+"/settings/search", `{"episode_default_minutes":0}`, 400)
	do(t, "PUT", u+"/settings/search", `{"episode_default_minutes":241}`, 400)
}
