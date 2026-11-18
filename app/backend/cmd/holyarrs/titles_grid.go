package main

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// gridTitle is one entry of GET /api/v1/titles?grid=1: the title as the
// full list gives it, without what only a title's own page reads (why a
// movie is not searched yet, its cast, every field of each Version and file, the
// numbers of every season beyond what the Library page draws). The outer
// fields hide the full ones of the same name when the entry is written.
type gridTitle struct {
	discover.TitleView
	GateReason    string        `json:"gate_reason,omitempty"`
	GateOpen      bool          `json:"gate_open,omitempty"`
	GateDelayDays int           `json:"gate_delay_days,omitempty"`
	GateDriver    string        `json:"gate_driver,omitempty"`
	ReleaseGate   string        `json:"release_gate,omitempty"`
	WhyAdded      string        `json:"why_added,omitempty"`
	Stage         string        `json:"stage,omitempty"`
	NoDownloader  string        `json:"no_downloader,omitempty"`
	GateOpensAt   *int          `json:"gate_opens_at,omitempty"`
	Tags          []int         `json:"tags,omitempty"`
	People        []int         `json:"people,omitempty"`
	Versions      []gridVersion `json:"versions"`
	Slots         []gridSlot    `json:"slots"`
	Show          *gridShow     `json:"show,omitempty"`
}

type gridVersion struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Main  bool   `json:"main"`
}

type gridSlot struct {
	ID         int64                   `json:"id"`
	State      string                  `json:"state"`
	StateLabel string                  `json:"state_label"`
	Reason     string                  `json:"reason,omitempty"`
	Download   *downloads.DownloadView `json:"download,omitempty"`
	File       *gridFile               `json:"file,omitempty"`
}

// gridFile keeps the fields of a file the Library page reads: its state and
// its quality (the resolution shown beside "In library").
type gridFile struct {
	ID      int64
	State   string
	Quality mediafiles.Quality
}

type gridShow struct {
	shows.Info
	Seasons []gridSeason `json:"seasons"`
}

type gridSeason struct {
	Number       int  `json:"number"`
	KeepUpToDate bool `json:"keep_up_to_date"`
	Aired        int  `json:"aired"`
	Dropped      bool `json:"dropped,omitempty"`
}

// gridOf cuts a built title view down to its grid entry.
func gridOf(v discover.TitleView) gridTitle {
	g := gridTitle{TitleView: v, Versions: make([]gridVersion, 0, len(v.Versions)), Slots: make([]gridSlot, 0, len(v.Slots))}
	for _, x := range v.Versions {
		g.Versions = append(g.Versions, gridVersion{ID: x.ID, Label: x.Label, Main: x.Main})
	}
	for _, s := range v.Slots {
		o := gridSlot{ID: s.ID, State: s.State, StateLabel: s.StateLabel, Reason: s.Reason, Download: s.Download}
		if s.File != nil {
			o.File = &gridFile{ID: s.File.ID, State: s.File.State, Quality: s.File.Quality}
		}
		g.Slots = append(g.Slots, o)
	}
	if v.Show != nil {
		gs := &gridShow{Info: *v.Show, Seasons: make([]gridSeason, 0, len(v.Show.Seasons))}
		for _, s := range v.Show.Seasons {
			gs.Seasons = append(gs.Seasons, gridSeason{Number: s.Number, KeepUpToDate: s.KeepUpToDate, Aired: s.Aired, Dropped: s.Dropped})
		}
		g.Show = gs
	}
	return g
}
