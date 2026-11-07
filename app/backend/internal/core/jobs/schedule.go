package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

type scheduleNoop struct {
	fingerprint string
	next        time.Time
}

type scheduleProbeResult struct {
	schedule Schedule
	needed   bool
	prepared any
	err      error
}

type scheduleProbeWork struct {
	schedule Schedule
	probe    ScheduleProbe
}

// UpdateSchedule changes a schedule and, when turning it off, cancels its
// queued scheduled work in the same write. Jobs explicitly started by hand
// are left alone. Re-enabling plans one future pass from now.
func (q *Queue) UpdateSchedule(ctx context.Context, kind string, every *int64, enabled *bool, by database.Actor, now time.Time) error {
	var cancelled []Job
	err := q.s.Write(ctx, func(tx *sql.Tx) error {
		scs, err := database.Select[Schedule](ctx, tx, "schedules", "kind = ?", kind)
		if err != nil {
			return err
		}
		if len(scs) == 0 {
			return sql.ErrNoRows
		}
		sc := scs[0]
		if every != nil {
			sc.EverySeconds = *every
			next := now.Add(time.Duration(*every) * time.Second)
			sc.NextRun = &next
		}
		disabling := enabled != nil && !*enabled
		turningOn := enabled != nil && !sc.Enabled && *enabled
		if enabled != nil {
			sc.Enabled = *enabled
		}
		if turningOn {
			next := now.Add(time.Duration(sc.EverySeconds) * time.Second)
			sc.NextRun = &next
		}
		if err := database.Update(tx, "schedules", &sc, by); err != nil {
			return err
		}
		if !disabling {
			return nil
		}
		queued, err := database.Select[Job](ctx, tx, "jobs", "kind = ? AND state = 'queued'", kind)
		if err != nil {
			return err
		}
		ts := database.FormatTime(now)
		for _, j := range queued {
			scheduled, _ := j.Params["scheduled"].(bool)
			byHand, _ := j.Params["by_hand"].(bool)
			if !scheduled || byHand {
				continue
			}
			if _, err := tx.Exec("UPDATE jobs SET state = 'cancelled', ended_at = ?, updated_at = ? WHERE id = ? AND state = 'queued'", ts, ts, j.ID); err != nil {
				return err
			}
			j.State, j.EndedAt, j.UpdatedAt = "cancelled", &now, now
			cancelled = append(cancelled, j)
		}
		return nil
	})
	if err != nil {
		return err
	}
	q.clearScheduleNoop(kind)
	for _, j := range cancelled {
		q.clearPrepared(j.ID)
		q.notify(j)
	}
	q.poke()
	return nil
}

// EffectiveScheduleNext returns the next visible pass for a schedule. A
// process-local no-work result overlays the persisted due time without
// changing the schedule's durable history.
func (q *Queue) EffectiveScheduleNext(sc Schedule) *time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()
	if mark, ok := q.scheduleNoop[sc.Kind]; ok && mark.fingerprint == scheduleFingerprint(sc) {
		next := mark.next
		return &next
	}
	if sc.NextRun == nil {
		return nil
	}
	next := *sc.NextRun
	return &next
}

// RunSchedules queues a Job for every enabled schedule that is due, and
// moves its clock: last_run is the start, next_run is start plus every. The
// Job gets the previous start as "last_start". It reads first and writes
// only when something is due. A kind whose part is off is left due. It returns the kinds it queued.
func (q *Queue) RunSchedules(ctx context.Context, now time.Time) ([]string, error) {
	due, err := database.Select[Schedule](ctx, q.s.Read(), "schedules",
		"enabled = 1 AND (next_run IS NULL OR next_run <= ?) ORDER BY id", database.FormatTime(now))
	if err != nil || len(due) == 0 {
		return nil, err
	}
	var kinds []string
	g := q.activeGate()
	var work []scheduleProbeWork
	for _, sc := range due {
		if g != nil && g.KindOff(sc.Kind) {
			continue // its part is off: queued once the part is on again
		}
		probe, skip := q.scheduleProbe(sc, now)
		if skip {
			continue
		}
		if probe != nil {
			work = append(work, scheduleProbeWork{schedule: sc, probe: probe})
			continue
		}
		accepted, err := q.enqueueSchedule(ctx, sc, now, nil)
		if err != nil {
			return kinds, err
		}
		if accepted {
			kinds = append(kinds, sc.Kind)
		}
	}
	results := make(chan scheduleProbeResult, len(work))
	ready := work[:0]
	for _, item := range work {
		live, err := q.scheduledJobLive(ctx, item.schedule.Kind)
		if err != nil {
			return kinds, err
		}
		if !live {
			ready = append(ready, item)
		}
	}
	started := 0
	for _, item := range ready {
		probe, ok := q.beginScheduleProbe(item.schedule, now)
		if !ok {
			continue
		}
		started++
		go func(item scheduleProbeWork, probe ScheduleProbe) {
			needed, prepared, err := safeScheduleProbe(ctx, probe, now)
			results <- scheduleProbeResult{schedule: item.schedule, needed: needed, prepared: prepared, err: err}
		}(item, probe)
	}
	remaining := started
	for remaining > 0 {
		var result scheduleProbeResult
		select {
		case result = <-results:
			remaining--
			q.endScheduleProbe(result.schedule.Kind)
		case <-ctx.Done():
			go q.releaseScheduleProbes(results, remaining)
			return kinds, ctx.Err()
		}
		current, valid, err := q.currentSchedule(ctx, result.schedule, now)
		if err != nil {
			go q.releaseScheduleProbes(results, remaining)
			return kinds, err
		}
		if !valid {
			continue
		}
		if result.err == nil && !result.needed {
			q.markScheduleNoop(current, now)
			continue
		}
		q.clearScheduleNoop(result.schedule.Kind)
		prepared := result.prepared
		if result.err != nil {
			prepared = nil
		}
		accepted, err := q.enqueueSchedule(ctx, current, now, prepared)
		if err != nil {
			go q.releaseScheduleProbes(results, remaining)
			return kinds, err
		}
		if accepted {
			kinds = append(kinds, result.schedule.Kind)
		}
	}
	return kinds, nil
}

func scheduleFingerprint(sc Schedule) string {
	next := ""
	if sc.NextRun != nil {
		next = sc.NextRun.UTC().Format(time.RFC3339Nano)
	}
	return sc.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(sc.EverySeconds, 10) + "|" + next
}

func (q *Queue) scheduleProbe(sc Schedule, now time.Time) (ScheduleProbe, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	probe := q.probe[sc.Kind]
	if probe == nil {
		return nil, false
	}
	mark, ok := q.scheduleNoop[sc.Kind]
	return probe, q.scheduleProbeInFlight[sc.Kind] || ok && mark.fingerprint == scheduleFingerprint(sc) && now.Before(mark.next)
}

func (q *Queue) beginScheduleProbe(sc Schedule, now time.Time) (ScheduleProbe, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	probe := q.probe[sc.Kind]
	mark, marked := q.scheduleNoop[sc.Kind]
	if probe == nil || q.scheduleProbeInFlight[sc.Kind] || marked && mark.fingerprint == scheduleFingerprint(sc) && now.Before(mark.next) {
		return nil, false
	}
	q.scheduleProbeInFlight[sc.Kind] = true
	return probe, true
}

func (q *Queue) endScheduleProbe(kind string) {
	q.mu.Lock()
	delete(q.scheduleProbeInFlight, kind)
	q.mu.Unlock()
}

func (q *Queue) markScheduleNoop(sc Schedule, now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.scheduleNoop[sc.Kind] = scheduleNoop{
		fingerprint: scheduleFingerprint(sc),
		next:        now.Add(time.Duration(sc.EverySeconds) * time.Second),
	}
}

func (q *Queue) clearScheduleNoop(kind string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.scheduleNoop, kind)
}

func scheduleDueUnchanged(sc, before Schedule, now time.Time) bool {
	return sc.Enabled && scheduleFingerprint(sc) == scheduleFingerprint(before) && (sc.NextRun == nil || !sc.NextRun.After(now))
}

func (q *Queue) schedulePartOff(kind string) bool {
	g := q.activeGate()
	return g != nil && g.KindOff(kind)
}

func (q *Queue) scheduledJobLive(ctx context.Context, kind string) (bool, error) {
	var live int
	err := q.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE dedupe_key = ? AND state IN ('queued', 'running')", "schedule:"+kind).Scan(&live)
	return live > 0, err
}

func (q *Queue) currentSchedule(ctx context.Context, before Schedule, now time.Time) (Schedule, bool, error) {
	var current Schedule
	if err := database.Get(ctx, q.s.Read(), "schedules", before.ID, &current); err != nil {
		return current, false, err
	}
	return current, scheduleDueUnchanged(current, before, now) && !q.schedulePartOff(current.Kind), nil
}

func (q *Queue) enqueueSchedule(ctx context.Context, before Schedule, now time.Time, prepared any) (accepted bool, err error) {
	var live int
	dedupeKey := "schedule:" + before.Kind
	if err := q.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE dedupe_key = ? AND state IN ('queued', 'running')", dedupeKey).Scan(&live); err != nil {
		return false, err
	}
	if live > 0 {
		return false, nil
	}
	var id int64
	var created bool
	var job *Job
	err = q.s.Write(ctx, func(tx *sql.Tx) error {
		var current Schedule
		if err := database.Get(ctx, tx, "schedules", before.ID, &current); err != nil {
			return err
		}
		if !scheduleDueUnchanged(current, before, now) || q.schedulePartOff(current.Kind) {
			return nil
		}
		params := map[string]any{"scheduled": true}
		if current.LastRun != nil {
			params["last_start"] = current.LastRun.UTC().Format(time.RFC3339Nano)
		}
		job = &Job{Kind: current.Kind, Params: params, Priority: "normal", State: "queued",
			MaxAttempts: q.opt.MaxAttempts, RunAfter: time.Now().UTC(), AskedBy: database.System}
		dedupeKey := "schedule:" + current.Kind
		job.DedupeKey = &dedupeKey
		scanErr := tx.QueryRow("SELECT id FROM jobs WHERE dedupe_key = ? AND state IN ('queued', 'running')", dedupeKey).Scan(&id)
		switch {
		case scanErr == nil:
			return nil
		case errors.Is(scanErr, sql.ErrNoRows):
			id, err = database.Insert(tx, "jobs", job, database.System)
			if err != nil {
				return err
			}
			created = true
			accepted = true
			if prepared != nil {
				q.mu.Lock()
				q.prepared[id] = preparedJob{kind: current.Kind, value: prepared,
					expires: now.Add(time.Duration(current.EverySeconds) * time.Second)}
				q.mu.Unlock()
			}
		default:
			return scanErr
		}
		ts := database.FormatTime(now)
		_, err = tx.Exec("UPDATE schedules SET last_run = ?, next_run = ?, updated_at = ? WHERE id = ?",
			ts, database.FormatTime(now.Add(time.Duration(current.EverySeconds)*time.Second)), ts, current.ID)
		return err
	})
	if err != nil {
		q.clearPrepared(id)
		return false, err
	}
	if created {
		q.notify(*job)
		q.poke()
	}
	return accepted, nil
}

func safeScheduleProbe(ctx context.Context, probe ScheduleProbe, now time.Time) (needed bool, prepared any, err error) {
	defer func() {
		if p := recover(); p != nil {
			needed, prepared, err = true, nil, fmt.Errorf("schedule probe panicked (%T)", p)
		}
	}()
	return probe(ctx, now)
}

func (q *Queue) releaseScheduleProbes(results <-chan scheduleProbeResult, remaining int) {
	for range remaining {
		result := <-results
		q.endScheduleProbe(result.schedule.Kind)
	}
}
