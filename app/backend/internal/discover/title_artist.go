package discover

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// ArtistInfo is what an artist's page needs besides the shared Title fields.
type ArtistInfo struct {
	MBID            string     `json:"mbid"`
	Type            string     `json:"type,omitempty"`
	Disambiguation  string     `json:"disambiguation,omitempty"`
	Status          string     `json:"status"` // running, ended
	AlbumChoice     string     `json:"album_choice,omitempty"`
	NewAlbums       string     `json:"new_albums"`
	AlbumTypes      []string   `json:"album_types"`
	SecondaryTypes  []string   `json:"secondary_types"`
	ReleaseStatuses []string   `json:"release_statuses"`
	Albums          int        `json:"albums"`   // every album row
	Included        int        `json:"included"` // albums the kinds include
	Tracks          int        `json:"tracks"`
	InLibrary       int        `json:"in_library"`
	Missing         int        `json:"missing"`
	NotOutYet       int        `json:"not_out_yet"`
	BetterWanted    int        `json:"better_wanted"`
	Downloading     int        `json:"downloading"`
	Failed          int        `json:"failed"`
	NextRelease     *time.Time `json:"next_release,omitempty"`
	// Bio is the start of the artist's kept Wikipedia summary (Library list
	// only, read from what is already stored; empty when none is kept).
	Bio string `json:"bio,omitempty"`
}

func (h *ViewHandlers) artistInfo(ctx context.Context, t mediafiles.Title) (*ArtistInfo, error) {
	n, err := library.Totals(ctx, h.Store.Read(), t.ID, time.Now())
	if err != nil {
		return nil, err
	}
	return &ArtistInfo{MBID: t.SourceIDs["musicbrainz"], Type: strOr(t.ArtistType), Disambiguation: strOr(t.Disambiguation), Status: t.Status,
		AlbumChoice: strOr(t.AlbumChoice), NewAlbums: library.NewAlbumsOf(t.NewPartsPolicy), AlbumTypes: music.ListOr(t.MusicTypes),
		SecondaryTypes: music.ListOr(t.MusicSecondary), ReleaseStatuses: music.ListOr(t.MusicStatuses), Albums: n.Albums, Included: n.Included,
		Tracks: n.Tracks, InLibrary: n.InLibrary, Missing: n.Missing, NotOutYet: n.NotOutYet, BetterWanted: n.BetterWanted,
		Downloading: n.Downloading, Failed: n.Failed, NextRelease: n.NextRelease}, nil
}
