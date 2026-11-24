package pluginstest

import (
	"context"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
	"sync"
)

// FakeRunner records starts instead of running programs, for tests.
type FakeRunner struct {
	mu     sync.Mutex
	Starts []plugins.Spec
	Err    error
	// OneShot, when set, runs for each one-shot start; its error is what
	// that run's Wait returns (such as an exit code).
	OneShot func(plugins.Spec) error
	last    *fakeProcess
}

// Exit ends the last started program on its own with err, as a crash would.
func (f *FakeRunner) Exit(err error) {
	f.mu.Lock()
	p := f.last
	f.mu.Unlock()
	if p != nil {
		p.err = err
		p.Stop()
	}
}

// Start records spec and returns a process that runs until stopped or ctx
// ends; a start with arguments is a one-shot run that ends at once.
func (f *FakeRunner) Start(ctx context.Context, spec plugins.Spec) (plugins.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	f.Starts = append(f.Starts, spec)
	p := &fakeProcess{done: make(chan struct{})}
	if len(spec.Args) > 0 { // one-shot runs such as --sign-out end at once
		if f.OneShot != nil {
			p.err = f.OneShot(spec)
		}
		p.Stop()
	} else {
		f.last = p
	}
	go func() {
		select {
		case <-ctx.Done():
			p.Stop()
		case <-p.done:
		}
	}()
	return p, nil
}

type fakeProcess struct {
	once sync.Once
	done chan struct{}
	err  error
}

func (p *fakeProcess) Stop() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *fakeProcess) Wait() error {
	<-p.done
	return p.err
}
