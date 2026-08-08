// Command bioflow runs containerized pipelines described in YAML.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CiaranFlanagan/bioflow/internal/executor"
	"github.com/CiaranFlanagan/bioflow/internal/metrics"
	"github.com/CiaranFlanagan/bioflow/internal/spec"
	"github.com/CiaranFlanagan/bioflow/internal/state"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "bioflow: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		workers     = flag.Int("workers", 4, "maximum steps to run concurrently")
		stateFile   = flag.String("state", ".bioflow/state.json", "path to the resume checkpoint")
		workDir     = flag.String("workdir", ".", "directory mounted into each container as /work")
		metricsAddr = flag.String("metrics", "", "expose Prometheus metrics on this address (e.g. :9090)")
		verbose     = flag.Bool("v", false, "log at debug level")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: bioflow [flags] <pipeline.yaml>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		return fmt.Errorf("expected exactly one pipeline file")
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	pipeline, err := spec.Load(flag.Arg(0))
	if err != nil {
		return err
	}

	store, err := state.Open(*stateFile)
	if err != nil {
		return err
	}
	if done := store.CompletedCount(); done > 0 {
		logger.Info("resuming from checkpoint", "steps_already_complete", done)
	}

	if *metricsAddr != "" {
		srv := metrics.Serve(*metricsAddr)
		defer srv.Close()
		logger.Info("metrics available", "addr", *metricsAddr+"/metrics")
	}

	// A first interrupt cancels the context so running steps can stop cleanly
	// and the checkpoint stays consistent; a second one kills the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	exec := &executor.Executor{
		Workers:   *workers,
		Runner:    &executor.DockerRunner{WorkDir: *workDir},
		State:     store,
		Logger:    logger,
		BaseDelay: time.Second,
	}

	logger.Info("running pipeline", "name", pipeline.Name, "steps", len(pipeline.Steps), "workers", *workers)
	return exec.Run(ctx, pipeline)
}
