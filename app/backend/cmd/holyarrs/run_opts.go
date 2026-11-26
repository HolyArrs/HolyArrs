package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// Command holyarrs runs the HolyArrs server.

// runOpts are test hooks for serve.
type runOpts struct {
	// wrap changes each supervised loop before it starts (tests inject crashes).
	wrap func(jobs.Loop) jobs.Loop
	// reg replaces the Registry (tests set short restart delays).
	reg *jobs.Registry
	// ready is told the Registry and the address once the server listens.
	ready func(reg *jobs.Registry, addr string)
	// kinds is told every Job kind with a Handler.
	kinds func([]string)
	// parts is told the parts Control once it is made.
	parts func(*parts.Control)
	// store is told the main store once it is open (tests emit Events).
	store func(*database.Store)
}

func run(c config) error {
	startup.UseMemoryMode()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := serve(ctx, c, runOpts{})
	if err == nil && updateRun != nil {
		updateRun.CleanStop()
	}
	return err
}
