package plugins

import (
	"context"
	"sync"
)

// FakeRunner records starts instead of running programs, for tests in this
// and other packages.
type FakeRunner struct {
	mu     sync.Mutex
	Starts []Spec
	Err    error
}

// Start records spec and returns a process that runs until stopped or ctx ends.
func (f *FakeRunner) Start(ctx context.Context, spec Spec) (Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	f.Starts = append(f.Starts, spec)
	p := &fakeProcess{done: make(chan struct{})}
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
}

func (p *fakeProcess) Stop() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *fakeProcess) Wait() error {
	<-p.done
	return nil
}
