package library

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Release gate choices for a movie.
const (
	GateAnnounced = "announced"
	GateInCinemas = "in_cinemas"
	GateReleased  = "released"
)

// ValidGate reports whether g is a Release gate choice.
func ValidGate(g string) bool { return g == GateAnnounced || g == GateInCinemas || g == GateReleased }

// cinemaRun is how long after the cinema date a movie counts as released
// when no digital or physical date is known.
const cinemaRun = 90 * 24 * time.Hour

// Dates are a movie's release dates; nil when unknown.
type Dates struct {
	Cinema, Digital, Physical *time.Time
}

// DatesOf reads a Title's release dates.
func DatesOf(t mediafiles.Title) Dates {
	return Dates{Cinema: t.CinemaDate, Digital: t.DigitalDate, Physical: t.PhysicalDate}
}

// released is the date a movie counts as released: the earlier of the
// digital and physical dates, else the cinema date plus 90 days. driver
// names which one; it is "none" when no date is known.
func (d Dates) released() (driver string, at *time.Time) {
	switch {
	case d.Digital != nil && (d.Physical == nil || !d.Physical.Before(*d.Digital)):
		return "digital", d.Digital
	case d.Physical != nil:
		return "physical", d.Physical
	case d.Cinema != nil:
		t := d.Cinema.Add(cinemaRun)
		return "cinema_plus_90", &t
	}
	return "none", nil
}

// Gate works out when searching may start for a movie with Release gate
// gate and delayDays extra days. driver names the date that decides it:
// "announced" means any time (no delay is added), "none" means never until
// dates are known. A movie gated on cinemas without a cinema date falls back
// to the released rule.
func Gate(gate string, delayDays int, d Dates) (driver string, opensAt *time.Time) {
	if gate == GateAnnounced {
		return "announced", nil
	}
	var at *time.Time
	if gate == GateInCinemas && d.Cinema != nil {
		driver, at = "cinema", d.Cinema
	} else {
		driver, at = d.released()
	}
	if at == nil {
		return driver, nil
	}
	t := at.AddDate(0, 0, delayDays)
	return driver, &t
}

// GateOpen reports whether a Title's gate is open at now.
func GateOpen(driver string, opensAt *time.Time, now time.Time) bool {
	if driver == "announced" {
		return true
	}
	return opensAt != nil && !now.Before(*opensAt)
}

// Movie stages, worked out from the dates (never stored apart from them).
const (
	StageAnnounced = "announced"
	StageInCinemas = "in_cinemas"
	StageReleased  = "released"
)

// Stage says where a movie is at now: released once the released date has
// passed, in cinemas once the cinema date has passed, else announced.
func Stage(d Dates, now time.Time) string {
	if _, at := d.released(); at != nil && !now.Before(*at) {
		return StageReleased
	}
	if d.Cinema != nil && !now.Before(*d.Cinema) {
		return StageInCinemas
	}
	return StageAnnounced
}

// titleStatus maps a stage to the Title status column.
func titleStatus(stage string) string {
	if stage == StageReleased {
		return "released"
	}
	return "announced"
}
