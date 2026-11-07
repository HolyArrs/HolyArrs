package database

import "time"

// Actor says who made a change: a user, a job, a linked app or the system.
type Actor struct {
	Kind string `json:"kind"` // user, job, link, system
	ID   int64  `json:"id,omitempty"`
}

// System is the actor for changes HolyArrs makes on its own.
var System = Actor{Kind: "system"}

// Meta is on every record.
type Meta struct {
	ID        int64     `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	CreatedBy Actor     `db:"created_by,json"`
	UpdatedAt time.Time `db:"updated_at"`
	UpdatedBy Actor     `db:"updated_by,json"`
}

// Subject is what a job is about.
type Subject struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// Event is one line of history. Events live in the events database.
type Event struct {
	Meta
	At         time.Time      `db:"at"`
	Actor      Actor          `db:"actor,json"`
	Kind       string         `db:"kind"`
	TitleID    *int64         `db:"title_id"`
	SlotID     *int64         `db:"slot_id"`
	DownloadID *int64         `db:"download_id"`
	JobID      *int64         `db:"job_id"`
	Summary    string         `db:"summary"`
	Data       map[string]any `db:"data,json"`
	GroupKey   string         `db:"group_key"`
}
