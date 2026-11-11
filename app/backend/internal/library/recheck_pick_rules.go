package library

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// KindRecheckPickRules re-checks the good enough state of every file whose
// Version uses a Pick rules set, after the set changed.
const KindRecheckPickRules = "recheck_pick_rules"

// QueueRecheckPickRules queues the re-check for a saved Pick rules set. Each
// revision gets its own Job, so a save during a running re-check is judged
// again with the newest rules.
func (s *Service) QueueRecheckPickRules(ctx context.Context, p mediafiles.PickRules, by domain.Actor) error {
	if s == nil || s.Queue == nil {
		return nil
	}
	key := KindRecheckPickRules + ":" + strconv.FormatInt(p.ID, 10) + ":" + strconv.Itoa(p.Revision)
	_, _, err := s.Queue.Enqueue(ctx, KindRecheckPickRules, &domain.Subject{Type: "pick_rules", ID: p.ID}, nil, key, by)
	return err
}

func (s *Service) handleRecheckPickRules(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil {
		return errors.New("recheck_pick_rules needs a pick_rules subject")
	}
	n, err := RecheckPickRules(ctx, s.Store, r.Job.Subject.ID, JobActor(r))
	r.SetResult(map[string]any{"slot_changes": n})
	return err
}

// RecheckPickRules works out the good enough state of the files of every
// Version that uses the Pick rules set, in small writes so a large Library
// stays responsive. It returns how many Slots changed.
func RecheckPickRules(ctx context.Context, st *database.Store, pickRulesID int64, by domain.Actor) (int, error) {
	ids, err := mediafiles.VersionIDsWithPickRules(ctx, st.Read(), pickRulesID)
	if err != nil {
		return 0, err
	}
	changed := 0
	for start := 0; start < len(ids); start += 50 {
		batch := ids[start:min(start+50, len(ids))]
		err := st.Write(ctx, func(tx *sql.Tx) error {
			for _, id := range batch {
				var v mediafiles.Version
				if err := database.Get(ctx, tx, "versions", id, &v); err != nil || v.PickRulesID != pickRulesID {
					continue
				}
				var t mediafiles.Title
				if err := database.Get(ctx, tx, "titles", v.TitleID, &t); err != nil {
					continue
				}
				ch, err := goodEnoughTx(ctx, tx, v, t.KeepUpToDate && v.KeepUpToDate, by)
				if err != nil {
					return err
				}
				changed += len(ch)
			}
			return nil
		})
		if err != nil {
			return changed, err
		}
	}
	return changed, nil
}
