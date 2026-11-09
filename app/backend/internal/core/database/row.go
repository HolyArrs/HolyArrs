package database

import "time"

// System is the actor for changes HolyArrs makes on its own.
var System = Actor{Kind: "system"}

// Actor says who made a change: a user, a job, a linked app or the system.
type Actor struct {
	Kind string `json:"kind"` // user, job, link, system
	ID   int64  `json:"id,omitempty"`
}

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
