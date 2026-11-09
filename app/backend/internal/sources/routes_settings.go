package sources

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// SearchSettings are the search choices a person can change.
type SearchSettings struct {
	FeedCheckMinutes    int  `json:"feed_check_minutes"`    // 0 is off, else 10 to 120
	MissingSearchCount  int  `json:"missing_search_count"`  // per daily run; 0 is off, at most 500
	MissingSearchBetter bool `json:"missing_search_better"` // the daily run also looks for better copies
	TryNextDownloader   bool `json:"try_next_downloader"`
	// Shows.
	MissingSearchShows bool   `json:"missing_search_shows"` // the daily and bulk searches cover episodes
	SeasonPackUpgrade  string `json:"season_pack_upgrade"`  // all: every episode a pack holds must gain; any: one is enough
	SeasonPackPrefer   string `json:"season_pack_prefer"`   // rank: a pack wins ties; always: a passing pack wins
	MultiSeasonPacks   bool   `json:"multi_season_packs"`   // Releases with several seasons may be taken
	// Music.
	MissingSearchMusic    bool `json:"missing_search_music"`    // the daily and bulk searches cover albums
	MusicDiscographyPacks bool `json:"music_discography_packs"` // a whole-discography Release may fill an album
	AlbumTrackTolerance   int  `json:"album_track_tolerance"`   // files a Release may lack against the album's tracks; -1 off
	AlbumExtraFiles       int  `json:"album_extra_files"`       // files a bare file count is taken to hold besides tracks; 0 to 20
	ArtistSearchCap       int  `json:"artist_search_cap"`       // most album searches one Search now on an artist queues; 1 to 2000
	// Early and fake Releases.
	RefuseEarlyReleases    bool `json:"refuse_early_releases"`     // a Release posted before its movie, episode or album came out is refused
	EarlyReleaseGraceHours int  `json:"early_release_grace_hours"` // 0 to 720
	RefuseTinyReleases     bool `json:"refuse_tiny_releases"`      // far too small for what it holds, or an empty or old-container file at import
	ImportMinVideoMB       int  `json:"import_min_video_mb"`       // least video file placed; 0 uses the fixed floor (1 MB a minute, 20 MB unknown length)
	ImportMinTrackKB       int  `json:"import_min_track_kb"`       // least track file placed; 0 uses the fixed floor (512 KB)
	ImportSizeFloors       bool `json:"import_size_floors"`        // the fixed least sizes at import and the judged-size check; off leaves empty files and the sizes set above
	HoldUnknownReleases    bool `json:"hold_unknown_releases"`     // automatic searches hold a Release of unknown size or dates for a person; off warns only
	EpisodeDefaultMinutes  int  `json:"episode_default_minutes"`   // length taken for an episode of unknown length by the size floor; 1 to 240
}

func (h *Handlers) readSearchSettings(ctx context.Context) (SearchSettings, error) {
	db := h.Store.Read()
	var st SearchSettings
	every, enabled, err := jobs.ScheduleEvery(ctx, db, jobs.KindFeedCheck)
	if err != nil {
		return st, err
	}
	if enabled {
		st.FeedCheckMinutes = every / 60
	}
	missingOn := jobs.ScheduleEnabled(ctx, db, jobs.KindSearchMissing)
	st.MissingSearchCount, _ = strconv.Atoi(Setting(ctx, db, SettingMissingCount))
	if !missingOn {
		st.MissingSearchCount = 0
	}
	st.MissingSearchBetter = Setting(ctx, db, SettingMissingBetter) == "true"
	st.TryNextDownloader = Setting(ctx, db, "try_next_downloader") != "false"
	st.MissingSearchShows = Setting(ctx, db, SettingMissingShows) != "false"
	st.SeasonPackUpgrade = "all"
	if Setting(ctx, db, SettingPackUpgrade) == "any" {
		st.SeasonPackUpgrade = "any"
	}
	st.SeasonPackPrefer = "rank"
	if Setting(ctx, db, SettingPackPrefer) == "always" {
		st.SeasonPackPrefer = "always"
	}
	st.MultiSeasonPacks = Setting(ctx, db, SettingMultiSeason) == "true"
	st.MissingSearchMusic = Setting(ctx, db, SettingMissingMusic) != "false"
	st.MusicDiscographyPacks = Setting(ctx, db, SettingDiscography) == "true"
	if n, err := strconv.Atoi(Setting(ctx, db, SettingTrackTolerance)); err == nil {
		st.AlbumTrackTolerance = n
	}
	st.AlbumExtraFiles, _ = strconv.Atoi(Setting(ctx, db, SettingExtraFiles))
	st.ArtistSearchCap = ArtistSearchCap(ctx, db)
	ch := Checks(ctx, db)
	st.RefuseEarlyReleases, st.EarlyReleaseGraceHours, st.RefuseTinyReleases = ch.Early, int(ch.Grace.Hours()), ch.Tiny
	st.ImportMinVideoMB, _ = strconv.Atoi(Setting(ctx, db, "import_min_video_mb"))
	st.ImportMinTrackKB, _ = strconv.Atoi(Setting(ctx, db, "import_min_track_kb"))
	st.ImportSizeFloors = Setting(ctx, db, SettingImportFloors) != "false"
	st.HoldUnknownReleases, st.EpisodeDefaultMinutes = ch.HoldUnknown, ch.EpisodeMinutes
	return st, nil
}

func (h *Handlers) getSearchSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.readSearchSettings(r.Context())
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, st)
}

func (h *Handlers) putSearchSettings(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their value.
	in, err := h.readSearchSettings(r.Context())
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	if !webserver.Decode(w, r, &in, "body is the search settings") {
		return
	}
	if in.SeasonPackUpgrade != "all" && in.SeasonPackUpgrade != "any" {
		webserver.Fail(w, http.StatusBadRequest, "season_pack_upgrade is all or any")
		return
	}
	if in.SeasonPackPrefer != "rank" && in.SeasonPackPrefer != "always" {
		webserver.Fail(w, http.StatusBadRequest, "season_pack_prefer is rank or always")
		return
	}
	if in.AlbumTrackTolerance < -1 || in.AlbumTrackTolerance > 50 {
		webserver.Fail(w, http.StatusBadRequest, "album_track_tolerance is -1 (off) or 0 to 50")
		return
	}
	if in.AlbumExtraFiles < 0 || in.AlbumExtraFiles > 20 {
		webserver.Fail(w, http.StatusBadRequest, "album_extra_files is 0 to 20")
		return
	}
	if in.ArtistSearchCap < 1 || in.ArtistSearchCap > 2000 {
		webserver.Fail(w, http.StatusBadRequest, "artist_search_cap is 1 to 2000")
		return
	}
	if in.EarlyReleaseGraceHours < 0 || in.EarlyReleaseGraceHours > 720 {
		webserver.Fail(w, http.StatusBadRequest, "early_release_grace_hours is 0 to 720")
		return
	}
	if in.ImportMinVideoMB < 0 || in.ImportMinVideoMB > 10000 || in.ImportMinTrackKB < 0 || in.ImportMinTrackKB > 100000 {
		webserver.Fail(w, http.StatusBadRequest, "import_min_video_mb is 0 to 10000 and import_min_track_kb 0 to 100000")
		return
	}
	if in.EpisodeDefaultMinutes < 1 || in.EpisodeDefaultMinutes > 240 {
		webserver.Fail(w, http.StatusBadRequest, "episode_default_minutes is 1 to 240")
		return
	}
	if in.FeedCheckMinutes != 0 && (in.FeedCheckMinutes < 10 || in.FeedCheckMinutes > 120) {
		webserver.Fail(w, http.StatusBadRequest, "feed_check_minutes is 0 (off) or 10 to 120")
		return
	}
	if in.MissingSearchCount < 0 || in.MissingSearchCount > 500 {
		webserver.Fail(w, http.StatusBadRequest, "missing_search_count is 0 (off) to 500")
		return
	}
	err = h.Store.Write(r.Context(), func(tx *sql.Tx) error { return saveSearchSettings(tx, in, actorOf(r), time.Now()) })
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	// The change is logged once by logSettingsChanges as an audit Event (area
	// and who, never the values), so it is not written or sent live here.
	h.getSearchSettings(w, r)
}
