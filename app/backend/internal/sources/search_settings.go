package sources

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// saveSearchSettings writes the search settings and the two schedules they
// drive in one transaction.
func saveSearchSettings(tx *sql.Tx, in SearchSettings, by domain.Actor, now time.Time) error {
	feedOn, feedEvery := in.FeedCheckMinutes > 0, in.FeedCheckMinutes*60
	if !feedOn {
		feedEvery = 15 * 60 // off keeps the default interval for when it is turned on again
	}
	if err := jobs.SetScheduleEveryTx(tx, jobs.KindFeedCheck, feedEvery, feedOn, by, now); err != nil {
		return err
	}
	if err := jobs.SetScheduleEnabledTx(tx, jobs.KindSearchMissing, in.MissingSearchCount > 0, by, now); err != nil {
		return err
	}
	for k, v := range map[string]string{SettingMissingCount: strconv.Itoa(max(in.MissingSearchCount, 0)),
		SettingMissingBetter: strconv.FormatBool(in.MissingSearchBetter), "try_next_downloader": strconv.FormatBool(in.TryNextDownloader),
		SettingMissingShows: strconv.FormatBool(in.MissingSearchShows), SettingPackUpgrade: in.SeasonPackUpgrade,
		SettingPackPrefer: in.SeasonPackPrefer, SettingMultiSeason: strconv.FormatBool(in.MultiSeasonPacks),
		SettingMissingMusic: strconv.FormatBool(in.MissingSearchMusic), SettingDiscography: strconv.FormatBool(in.MusicDiscographyPacks),
		SettingTrackTolerance: strconv.Itoa(in.AlbumTrackTolerance), SettingExtraFiles: strconv.Itoa(in.AlbumExtraFiles),
		SettingArtistSearchCap: strconv.Itoa(in.ArtistSearchCap),
		SettingRefuseEarly:     strconv.FormatBool(in.RefuseEarlyReleases), SettingEarlyGrace: strconv.Itoa(in.EarlyReleaseGraceHours),
		SettingRefuseTiny: strconv.FormatBool(in.RefuseTinyReleases), "import_min_video_mb": strconv.Itoa(in.ImportMinVideoMB),
		"import_min_track_kb": strconv.Itoa(in.ImportMinTrackKB), SettingImportFloors: strconv.FormatBool(in.ImportSizeFloors),
		SettingHoldUnknown: strconv.FormatBool(in.HoldUnknownReleases), SettingEpisodeMinutes: strconv.Itoa(in.EpisodeDefaultMinutes)} {
		if k == SettingMissingCount && in.MissingSearchCount == 0 {
			continue // off keeps the last count for when it is turned on again
		}
		if err := database.PutSetting(tx, searchSettingsGroup, k, v, by, now); err != nil {
			return err
		}
	}
	return nil
}
