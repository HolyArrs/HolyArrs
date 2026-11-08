package mediafiles

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTrackPathDefaults(t *testing.T) {
	s := NamingDefaults()
	f := TrackFacts{Artist: "Lab Lanterns", Album: "Small Hours", Year: 2025, Disc: 1, Discs: 1, Track: 3, Title: "Tide", Ext: ".FLAC"}
	if got := TrackPath(s, "Lab Lanterns", f); got != "Lab Lanterns/Small Hours (2025)/03 - Tide.flac" {
		t.Fatalf("single disc: %q", got)
	}
	f.Disc, f.Discs = 2, 2
	if got := TrackPath(s, "Lab Lanterns", f); got != "Lab Lanterns/Small Hours (2025)/2-03 - Tide.flac" {
		t.Fatalf("multi-disc: %q", got)
	}
	s.DiscFolders = true
	if got := TrackPath(s, "Lab Lanterns", f); got != "Lab Lanterns/Small Hours (2025)/Disc 2/03 - Tide.flac" {
		t.Fatalf("disc folders: %q", got)
	}
	f = TrackFacts{Artist: "A", Album: "B", Track: 7, Ext: ".mp3"}
	if got := TrackPath(NamingDefaults(), "A", f); got != "A/B/07.mp3" {
		t.Fatalf("no year, no title: %q", got)
	}
}

func TestMusicPatterns(t *testing.T) {
	s := NamingDefaults()
	s.ArtistFolderPattern = "{artist_sort}"
	s.AlbumFolderPattern = "{year} - {album} [{edition}] ({album_type})"
	s.TrackPattern = "{track} - {track_artist} - {track_title} [{format}]"
	f := TrackFacts{Artist: "The Lab Band", Album: "Old Days", AlbumType: "ep", Edition: "deluxe", Year: 2010, Disc: 1, Discs: 1, Track: 1, Title: "Song 1", Ext: ".flac"}
	if got := ArtistFolder(s, f); got != "Lab Band, The" {
		t.Errorf("artist sort: %q", got)
	}
	if got := AlbumFolder(s, f); got != "2010 - Old Days [deluxe] (EP)" {
		t.Errorf("album: %q", got)
	}
	if got := TrackFile(s, f); got != "01 - The Lab Band - Song 1 [FLAC].flac" {
		t.Errorf("track: %q", got)
	}
	f.TrackArtist, f.Edition = "Guest", ""
	if got := TrackFile(s, f); got != "01 - Guest - Song 1 [FLAC].flac" {
		t.Errorf("various artists: %q", got)
	}
	if got := AlbumFolder(s, f); got != "2010 - Old Days (EP)" {
		t.Errorf("empty edition: %q", got)
	}
	if theLast("Them") != "Them" || theLast("The ") != "The " {
		t.Error("only a leading The word moves")
	}
}

func TestMusicColonAndIllegal(t *testing.T) {
	s := NamingDefaults()
	f := TrackFacts{Artist: "AC/DC", Album: "Live: At \"Home\"?", Year: 1992, Track: 2, Title: "What*s <Next>|", Ext: ".mp3", Discs: 1, Disc: 1}
	got := TrackPath(s, ArtistFolder(s, f), f)
	if got != "AC-DC/Live - At 'Home' (1992)/02 - What-s Next.mp3" {
		t.Fatalf("got %q", got)
	}
	s.Colon, s.Illegal = ColonRemove, IllegalRemove
	if got := AlbumFolder(s, f); got != "Live At Home (1992)" {
		t.Fatalf("remove: %q", got)
	}
	if got := AlbumFolder(NamingDefaults(), TrackFacts{Album: "..", Year: 2001}); got != "Unknown album (2001)" {
		t.Fatalf("empty album: %q", got)
	}
	if got := AlbumFolder(NamingDefaults(), TrackFacts{Album: "CON"}); got != "CON_" {
		t.Fatalf("reserved: %q", got)
	}
}

func TestMusicLongNamesKeepYearAndNumber(t *testing.T) {
	s := NamingDefaults()
	long := strings.Repeat("é", 300)
	f := TrackFacts{Artist: long, Album: long, Year: 2024, Track: 12, Title: long, Ext: ".flac", Disc: 1, Discs: 1}
	album := AlbumFolder(s, f)
	if len(album) > MusicMaxComponent || !strings.HasSuffix(album, " (2024)") || !utf8.ValidString(album) {
		t.Fatalf("album %d bytes: %q", len(album), album[len(album)-20:])
	}
	file := TrackFile(s, f)
	if len(file) > MusicMaxComponent || !strings.HasPrefix(file, "12 - ") || !strings.HasSuffix(file, ".flac") || !utf8.ValidString(file) {
		t.Fatalf("file %d bytes", len(file))
	}
	if a := ArtistFolder(s, f); len(a) > MusicMaxComponent || !utf8.ValidString(a) {
		t.Fatalf("artist %d bytes", len(a))
	}
}

func TestMusicValidate(t *testing.T) {
	for _, c := range []struct {
		set  func(*NamingSettings)
		want string
	}{
		{func(s *NamingSettings) { s.ArtistFolderPattern = "{album}" }, "must hold {artist} or {artist_sort}"},
		{func(s *NamingSettings) { s.AlbumFolderPattern = "{year}" }, "must hold {album}"},
		{func(s *NamingSettings) { s.TrackPattern = "{track_title}" }, "must hold {track}"},
		{func(s *NamingSettings) { s.MultiDiscTrackPattern = "{track} - {track_title}" }, "must hold {disc}"},
		{func(s *NamingSettings) { s.MultiDiscTrackPattern = "{disc} - {track_title}" }, "must hold {track}"},
		{func(s *NamingSettings) { s.DiscFolderPattern = "CD" }, "must hold {disc}"},
		{func(s *NamingSettings) { s.TrackPattern = "{track} {quality}" }, "cannot use {quality}"},
		{func(s *NamingSettings) { s.AlbumFolderPattern = "{album}/{year}" }, "folder separator"},
		{func(s *NamingSettings) { s.FilePattern = "{title} {artist}" }, "cannot use music words"},
		{func(s *NamingSettings) { s.EpisodePattern = "{title} {episode} {track}" }, "cannot use music words"},
	} {
		s := NamingDefaults()
		c.set(&s)
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, got %v", c.want, err)
		}
	}
	if err := NamingDefaults().Validate(); err != nil {
		t.Fatal(err)
	}
	s := NamingSettings{FilePattern: DefaultFilePattern, FolderPattern: DefaultFolderPattern, Colon: ColonSmart, Illegal: IllegalReplace}
	if err := s.Validate(); err != nil {
		t.Fatalf("empty music patterns take their default: %v", err)
	}
}
