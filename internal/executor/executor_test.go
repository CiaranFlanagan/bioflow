package executor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CiaranFlanagan/bioflow/internal/spec"
	"github.com/CiaranFlanagan/bioflow/internal/state"
)

// fakeRunner records what ran, and can be told to fail a step a set number of
// times before succeeding — enough to exercise retry and cache behaviour
// without needing Docker.
type fakeRunner struct {
	mu       sync.Mutex
	calls    []string
	failFor  map[string]int
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	delay    time.Duration
}

func (f *fakeRunner) Run(ctx context.Context, step spec.Step) error {
	n := f.inFlight.Add(1)
	for {
		max := f.maxSeen.Load()
		if n <= max || f.maxSeen.CompareAndSwap(max, n) {
			break
		}
	}
	defer f.inFlight.Add(-1)

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	f.mu.Lock()
	f.calls = append(f.calls, step.ID)
	remaining := f.failFor[step.ID]
	if remaining > 0 {
		f.failFor[step.ID] = remaining - 1
	}
	f.mu.Unlock()

	if remaining > 0 {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeRunner) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0
	for _, c := range f.calls {
		if c == id {
			n++
		}
	}
	return n
}

func newExecutor(t *testing.T, r Runner, workers int) (*Executor, *state.Store) {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &Executor{
		Workers:   workers,
		Runner:    r,
		State:     store,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		BaseDelay: time.Millisecond,
	}, store
}

func pipeline(steps ...spec.Step) *spec.Pipeline {
	return &spec.Pipeline{Name: "test", Steps: steps}
}

func TestRunExecutesEveryStep(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{}}
	e, _ := newExecutor(t, r, 4)

	p := pipeline(
		spec.Step{ID: "a", Image: "alpine", Run: "echo a"},
		spec.Step{ID: "b", Image: "alpine", Run: "echo b", Needs: []string{"a"}},
	)

	if err := e.Run(context.Background(), p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, id := range []string{"a", "b"} {
		if r.callCount(id) != 1 {
			t.Errorf("step %q ran %d times, want 1", id, r.callCount(id))
		}
	}
}

func TestRunSkipsCompletedSteps(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{}}
	e, _ := newExecutor(t, r, 4)

	p := pipeline(spec.Step{ID: "a", Image: "alpine", Run: "echo a"})

	if err := e.Run(context.Background(), p); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := e.Run(context.Background(), p); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if got := r.callCount("a"); got != 1 {
		t.Errorf("step ran %d times across two runs, want 1 (second should hit cache)", got)
	}
}

func TestRunRerunsWhenCommandChanges(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{}}
	e, _ := newExecutor(t, r, 4)

	if err := e.Run(context.Background(), pipeline(
		spec.Step{ID: "a", Image: "alpine", Run: "echo one"},
	)); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), pipeline(
		spec.Step{ID: "a", Image: "alpine", Run: "echo two"},
	)); err != nil {
		t.Fatal(err)
	}

	if got := r.callCount("a"); got != 2 {
		t.Errorf("step ran %d times, want 2 (command changed, cache must miss)", got)
	}
}

func TestRunRetriesThenSucceeds(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{"flaky": 2}}
	e, _ := newExecutor(t, r, 1)

	p := pipeline(spec.Step{ID: "flaky", Image: "alpine", Run: "flake", Retries: 2})

	if err := e.Run(context.Background(), p); err != nil {
		t.Fatalf("Run() error = %v, want nil after retries", err)
	}
	if got := r.callCount("flaky"); got != 3 {
		t.Errorf("step attempted %d times, want 3", got)
	}
}

func TestRunFailsAfterExhaustingRetries(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{"doomed": 99}}
	e, _ := newExecutor(t, r, 1)

	p := pipeline(spec.Step{ID: "doomed", Image: "alpine", Run: "fail", Retries: 1})

	if err := e.Run(context.Background(), p); err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if got := r.callCount("doomed"); got != 2 {
		t.Errorf("step attempted %d times, want 2", got)
	}
}

func TestRunSkipsDownstreamOfFailure(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{"a": 99}}
	e, _ := newExecutor(t, r, 1)

	p := pipeline(
		spec.Step{ID: "a", Image: "alpine", Run: "fail"},
		spec.Step{ID: "b", Image: "alpine", Run: "echo b", Needs: []string{"a"}},
	)

	if err := e.Run(context.Background(), p); err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if got := r.callCount("b"); got != 0 {
		t.Errorf("downstream step ran %d times, want 0", got)
	}
}

func TestRunRespectsWorkerLimit(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{}, delay: 20 * time.Millisecond}
	e, _ := newExecutor(t, r, 2)

	// Six independent steps land in a single wave; at most two may run at once.
	var steps []spec.Step
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		steps = append(steps, spec.Step{ID: id, Image: "alpine", Run: "echo " + id})
	}

	if err := e.Run(context.Background(), pipeline(steps...)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := r.maxSeen.Load(); got > 2 {
		t.Errorf("peak concurrency = %d, want <= 2", got)
	}
}

func TestRunHonoursCancellation(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{}, delay: time.Hour}
	e, _ := newExecutor(t, r, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	p := pipeline(spec.Step{ID: "slow", Image: "alpine", Run: "sleep"})

	done := make(chan struct{})
	go func() {
		_ = e.Run(ctx, p)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRunRejectsCycle(t *testing.T) {
	r := &fakeRunner{failFor: map[string]int{}}
	e, _ := newExecutor(t, r, 1)

	p := pipeline(
		spec.Step{ID: "a", Image: "alpine", Run: "echo a", Needs: []string{"b"}},
		spec.Step{ID: "b", Image: "alpine", Run: "echo b", Needs: []string{"a"}},
	)

	if err := e.Run(context.Background(), p); err == nil {
		t.Fatal("Run() error = nil, want cycle error")
	}
}
