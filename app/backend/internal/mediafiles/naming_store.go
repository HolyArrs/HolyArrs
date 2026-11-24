package mediafiles

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Naming turns a movie's facts into folder and file names through

const group = "where_files_go"

// ReadNaming reads the naming settings; unset or broken ones keep their default.
func ReadNaming(ctx context.Context, db database.Lister) (NamingSettings, error) {
	s := NamingDefaults()
	seen := map[string]bool{}
	err := database.EachSetting(ctx, db, group, func(k, v string) {
		seen[k] = true
		switch k {
		case "file_pattern":
			s.FilePattern = v
		case "folder_pattern":
			s.FolderPattern = v
		case "group_by_genre":
			s.GroupByGenre = v == "true"
		case "group_by_letter":
			s.GroupByLetter = v == "true"
		case "show_group_by_letter":
			s.ShowGroupByLetter = v == "true"
		case "colon":
			s.Colon = v
		case "illegal":
			s.Illegal = v
		case "rename_on_import":
			s.RenameOnImport = v != "false"
		case "show_colon":
			s.ShowColon = v
		case "show_illegal":
			s.ShowIllegal = v
		case "show_rename_on_import":
			s.ShowRenameOnImport = v != "false"
		case "music_colon":
			s.MusicColon = v
		case "music_illegal":
			s.MusicIllegal = v
		case "music_rename_on_import":
			s.MusicRenameOnImport = v != "false"
		case "adopt_scan_limit":
			if n, err := strconv.Atoi(v); err == nil {
				s.AdoptScanLimit = n
			}
		case "music_adopt_scan_limit":
			if n, err := strconv.Atoi(v); err == nil {
				s.MusicAdoptScanLimit = n
			}
		case "show_folder_pattern":
			s.ShowFolderPattern = v
		case "season_folders":
			s.SeasonFolders = v != "false"
		case "season_folder_pattern":
			s.SeasonFolderPattern = v
		case "specials_folder":
			s.SpecialsFolder = v
		case "episode_pattern":
			s.EpisodePattern = v
		case "daily_pattern":
			s.DailyPattern = v
		case "anime_pattern":
			s.AnimePattern = v
		case "multi_episode_style":
			s.MultiEpisodeStyle = v
		case "air_date_style":
			s.AirDateStyle = v
		case "absolute_digits":
			if n, err := strconv.Atoi(v); err == nil {
				s.AbsoluteDigits = n
			}
		case "artist_folder_pattern":
			s.ArtistFolderPattern = v
		case "album_folder_pattern":
			s.AlbumFolderPattern = v
		case "track_pattern":
			s.TrackPattern = v
		case "multi_disc_track_pattern":
			s.MultiDiscTrackPattern = v
		case "disc_folders":
			s.DiscFolders = v == "true"
		case "disc_folder_pattern":
			s.DiscFolderPattern = v
		case "music_read_tags":
			s.MusicReadTags = v != "false"
		case "title_language":
			s.TitleLanguage = v
		}
	})
	if err != nil {
		return s, err
	}
	// Saved before each part had its own: a part with none yet
	// takes the one shared value, so nothing changes on upgrade.
	for _, f := range []struct {
		key  string
		to   *string
		from string
	}{{"show_colon", &s.ShowColon, s.Colon}, {"show_illegal", &s.ShowIllegal, s.Illegal},
		{"music_colon", &s.MusicColon, s.Colon}, {"music_illegal", &s.MusicIllegal, s.Illegal}} {
		if !seen[f.key] {
			*f.to = f.from
		}
	}
	if !seen["show_rename_on_import"] {
		s.ShowRenameOnImport = s.RenameOnImport
	}
	if !seen["music_rename_on_import"] {
		s.MusicRenameOnImport = s.RenameOnImport
	}
	s.fillShows()
	if s.Validate() != nil {
		d := NamingDefaults()
		d.RenameOnImport = s.RenameOnImport
		d.ShowRenameOnImport, d.MusicRenameOnImport = s.ShowRenameOnImport, s.MusicRenameOnImport
		d.SeasonFolders = s.SeasonFolders
		d.DiscFolders = s.DiscFolders
		d.MusicReadTags = s.MusicReadTags
		if metadata.IsTextLanguage(s.TitleLanguage) {
			d.TitleLanguage = s.TitleLanguage
		}
		return d, nil
	}
	if s.AdoptScanLimit == 0 {
		s.AdoptScanLimit = DefaultAdoptScanLimit
	}
	if s.MusicAdoptScanLimit == 0 {
		s.MusicAdoptScanLimit = DefaultMusicAdoptScanLimit
	}
	return s, nil
}

// PutNamingSettings stores valid naming settings. then, when given, runs in the same
// transaction (the move tool marks its change there).
func PutNamingSettings(ctx context.Context, st *database.Store, s NamingSettings, by database.Actor, then ...func(tx *sql.Tx) error) error {
	s.fillShows()
	if err := s.Validate(); err != nil {
		return err
	}
	if s.AdoptScanLimit == 0 {
		s.AdoptScanLimit = DefaultAdoptScanLimit
	}
	if s.MusicAdoptScanLimit == 0 {
		s.MusicAdoptScanLimit = DefaultMusicAdoptScanLimit
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		now := time.Now()
		for k, v := range map[string]string{"file_pattern": s.FilePattern, "folder_pattern": s.FolderPattern, "colon": s.Colon,
			"illegal": s.Illegal, "rename_on_import": fmt.Sprint(s.RenameOnImport), "group_by_genre": fmt.Sprint(s.GroupByGenre),
			"group_by_letter": fmt.Sprint(s.GroupByLetter), "show_group_by_letter": fmt.Sprint(s.ShowGroupByLetter),
			"show_colon": s.ShowColon, "show_illegal": s.ShowIllegal, "show_rename_on_import": fmt.Sprint(s.ShowRenameOnImport),
			"music_colon": s.MusicColon, "music_illegal": s.MusicIllegal, "music_rename_on_import": fmt.Sprint(s.MusicRenameOnImport),
			"adopt_scan_limit": strconv.Itoa(s.AdoptScanLimit), "music_adopt_scan_limit": strconv.Itoa(s.MusicAdoptScanLimit), "show_folder_pattern": s.ShowFolderPattern, "season_folders": fmt.Sprint(s.SeasonFolders),
			"season_folder_pattern": s.SeasonFolderPattern, "specials_folder": s.SpecialsFolder, "episode_pattern": s.EpisodePattern,
			"daily_pattern": s.DailyPattern, "anime_pattern": s.AnimePattern, "multi_episode_style": s.MultiEpisodeStyle,
			"air_date_style": s.AirDateStyle, "absolute_digits": strconv.Itoa(s.AbsoluteDigits),
			"artist_folder_pattern": s.ArtistFolderPattern, "album_folder_pattern": s.AlbumFolderPattern, "track_pattern": s.TrackPattern,
			"multi_disc_track_pattern": s.MultiDiscTrackPattern, "disc_folders": fmt.Sprint(s.DiscFolders), "disc_folder_pattern": s.DiscFolderPattern,
			"music_read_tags": fmt.Sprint(s.MusicReadTags), "title_language": s.TitleLanguage} {
			if err := database.PutSetting(tx, group, k, v, by, now); err != nil {
				return err
			}
		}
		for _, f := range then {
			if err := f(tx); err != nil {
				return err
			}
		}
		return nil
	})
}
