// Package executor runs a pipeline: it resolves the dependency graph into
// waves, runs each wave concurrently across a bounded worker pool, skips steps
// whose inputs are unchanged, and retries transient failures.
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/CiaranFlanagan/bioflow/internal/cache"
	"github.com/CiaranFlanagan/bioflow/internal/dag"
	"github.com/CiaranFlanagan/bioflow/internal/metrics"
	"github.com/CiaranFlanagan/bioflow/internal/spec"
	"github.com/CiaranFlanagan/bioflow/internal/state"
)

// Runner executes a single step. The Docker implementation lives in
// docker.go; tests substitute a fake.
type Runner interface {
	Run(ctx context.Context, step spec.Step) error
}

// Executor coordinates a full pipeline run.
type Executor struct {
	Workers int
	Runner  Runner
	State   *state.Store
	Logger  *slog.Logger

	// BaseDelay is the first retry delay; each subsequent retry doubles it.
	BaseDelay time.Duration
}

// Run executes the pipeline. It returns as soon as a wave fails, since every
// later wave depends on it, but it lets the rest of the failing wave finish so
// that partial progress is still recorded.
func (e *Executor) Run(ctx context.Context, p *spec.Pipeline) error {
	waves, err := e.plan(p)
	if err != nil {
		return err
	}

	byID := make(map[string]spec.Step, len(p.Steps))
	for _, s := range p.Steps {
		byID[s.ID] = s
	}

	for i, wave := range waves {
		e.Logger.Info("starting wave", "wave", i+1, "of", len(waves), "steps", len(wave))
		if err := e.runWave(ctx, wave, byID); err != nil {
			return fmt.Errorf("wave %d: %w", i+1, err)
		}
	}
	return nil
}

// plan builds the dependency graph and resolves it into execution waves.
func (e *Executor) plan(p *spec.Pipeline) ([][]string, error) {
	g := dag.New()
	for _, s := range p.Steps {
		g.Add(s.ID, s.Needs...)
	}
	return g.Waves()
}

// runWave executes every step in a wave concurrently, bounded by Workers.
func (e *Executor) runWave(ctx context.Context, wave []string, byID map[string]spec.Step) error {
	sem := make(chan struct{}, e.Workers)
	var wg sync.WaitGroup

	var mu sync.Mutex
	var firstErr error

	for _, id := range wave {
		step := byID[id]

		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			if err := e.runStep(ctx, step); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return firstErr
}

// runStep executes one step, skipping it if its cache key is unchanged and
// retrying with exponential backoff on failure.
func (e *Executor) runStep(ctx context.Context, step spec.Step) error {
	key, err := e.cacheKey(step)
	if err != nil {
		return err
	}

	if e.State.Completed(step.ID, key) {
		e.Logger.Info("skipping step, inputs unchanged", "step", step.ID)
		metrics.StepsSkipped.Inc()
		return nil
	}

	start := time.Now()
	attempts := step.Retries + 1

	for attempt := 1; attempt <= attempts; attempt++ {
		err = e.Runner.Run(ctx, step)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt < attempts {
			delay := e.BaseDelay * (1 << (attempt - 1))
			e.Logger.Warn("step failed, retrying",
				"step", step.ID, "attempt", attempt, "of", attempts,
				"retry_in", delay, "error", err)
			metrics.StepRetries.WithLabelValues(step.ID).Inc()

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	elapsed := time.Since(start)
	metrics.StepDuration.WithLabelValues(step.ID).Observe(elapsed.Seconds())

	status := state.StatusDone
	if err != nil {
		status = state.StatusFailed
		metrics.StepsFailed.WithLabelValues(step.ID).Inc()
	} else {
		metrics.StepsCompleted.WithLabelValues(step.ID).Inc()
	}

	// Recorded even on failure, so a resumed run can tell the difference
	// between a step that never ran and one that ran and failed.
	if serr := e.State.Set(step.ID, state.Record{
		CacheKey:   key,
		Status:     status,
		FinishedAt: time.Now(),
		DurationMS: elapsed.Milliseconds(),
	}); serr != nil {
		return fmt.Errorf("record %s: %w", step.ID, serr)
	}

	if err != nil {
		return fmt.Errorf("step %q failed after %d attempts: %w", step.ID, attempts, err)
	}
	e.Logger.Info("step complete", "step", step.ID, "duration", elapsed)
	return nil
}

// cacheKey derives the step's content-addressed key.
//
// TODO: hash the step's declared input files once glob expansion is wired up.
// Until then the key covers image and command only, which correctly catches
// pipeline edits but not input data changes.
func (e *Executor) cacheKey(step spec.Step) (string, error) {
	var inputHashes []string
	_ = cache.HashFile // referenced once glob expansion lands
	return cache.Key(step.Image, step.Run, inputHashes), nil
}
