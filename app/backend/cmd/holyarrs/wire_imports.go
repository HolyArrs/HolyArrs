package main

import (
	"context"
	"fmt"
	"log"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// wireImports queues the import of a finished download: on its own when the
// download finishes, and when a person asks for it again, who is told when
// it could not be queued.
func wireImports(dm *downloads.Manager, q *jobs.Queue) {
	dm.QueueImport = func(ctx context.Context, downloadID int64) error {
		_, _, err := q.Enqueue(ctx, importer.KindImport, &domain.Subject{Type: "download", ID: downloadID}, nil, fmt.Sprintf("import:%d", downloadID), domain.System)
		return err
	}
	dm.OnFinished = func(ctx context.Context, downloadID int64) {
		if err := dm.QueueImport(ctx, downloadID); err != nil {
			log.Printf("queue %s: %v", importer.KindImport, err)
		}
	}
}
