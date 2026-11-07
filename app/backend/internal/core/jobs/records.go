package jobs

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// JobError says why a job failed.
type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Job is background work stored in the database.
type Job struct {
	database.Meta
	Kind         string            `db:"kind"`
	Subject      *database.Subject `db:"subject,json"`
	Params       map[string]any    `db:"params,json"`
	Priority     string            `db:"priority"` // now, normal, idle
	State        string            `db:"state"`    // queued, running, done, failed, cancelled
	Progress     float64           `db:"progress"`
	ProgressNote *string           `db:"progress_note"`
	Attempt      int               `db:"attempt"`
	MaxAttempts  int               `db:"max_attempts"`
	RunAfter     time.Time         `db:"run_after"`
	StartedAt    *time.Time        `db:"started_at"`
	EndedAt      *time.Time        `db:"ended_at"`
	Result       map[string]any    `db:"result,json"`
	Error        *JobError         `db:"error,json"`
	AskedBy      database.Actor    `db:"asked_by,json"`
	DedupeKey    *string           `db:"dedupe_key"`
}

// Schedule runs a job kind on a timer.
type Schedule struct {
	database.Meta
	Kind         string     `db:"kind"`
	EverySeconds int64      `db:"every_seconds"`
	LastRun      *time.Time `db:"last_run"`
	NextRun      *time.Time `db:"next_run"`
	Enabled      bool       `db:"enabled"`
}
