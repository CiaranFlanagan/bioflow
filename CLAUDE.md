# CLAUDE.md

## What this is

A DAG-based job execution engine for containerized pipelines, aimed at bioinformatics workloads. Go, Docker, Prometheus. Written to be an infrastructure project: the biology is only the workload that makes the scheduling and caching problems real.

## Build and test

Go 1.26 is installed and on the default PATH. A Docker daemon is required for the CLI but **not** for the test suite — tests substitute a fake `Runner`.

```bash
make test    # unit tests
make race    # with the race detector
make cover   # coverage report
make lint    # go vet + gofmt
make build   # -> bin/bioflow
```

Run `make race` before considering executor work done. The worker pool is where subtle concurrency bugs live, and the race detector is in the normal loop for that reason, not just CI.

## Package layout

| Package | Responsibility |
|---|---|
| `internal/spec` | YAML → typed `Pipeline`, plus validation |
| `internal/dag` | dependency graph → ordered waves of concurrently-runnable steps |
| `internal/cache` | content-addressed cache keys, streaming file hashes |
| `internal/state` | crash-safe checkpoint store |
| `internal/executor` | worker pool, retries, cache skip; `Runner` interface + Docker impl |
| `internal/metrics` | Prometheus collectors |
| `cmd/bioflow` | CLI |

## Decisions already made — don't undo these

- **`dag.Waves()` returns groups, not a flat topological order.** Grouping is what expresses parallelism; cycle detection falls out of the same pass. Don't "simplify" it to a linear sort.
- **Cache key fields are length-prefixed.** Without it `("ab","c")` and `("a","bc")` collide.
- **Checkpoints write via temp file + rename.** A crash mid-write must not truncate state.
- **Failures are recorded, not just successes.** A resumed run needs to distinguish "never ran" from "ran and failed."
- **Docker via CLI, not the SDK.** Steps run for minutes; process overhead is irrelevant and the CLI is easier to reason about.
- **`Runner` is an interface on purpose.** It's the seam for the Kubernetes and remote executors. Keep it narrow.

## Known gaps

- **`executor.cacheKey()` ignores input data.** It hashes image and command only, so editing a pipeline invalidates the cache but *changing input files does not*. This is a correctness bug, not a missing feature. Fixing it means wiring up glob expansion for `spec.Pipeline.Inputs` and hashing matched files with `cache.HashFile`. **Highest-priority work in the repo.**
- `spec.Pipeline.Inputs` is parsed and then unused — no per-sample fan-out yet.
- No Kubernetes executor.
- No benchmark on real genomic data. Only an `alpine` smoke test, whose numbers are **not** citable as performance results.

## Conventions

- Wrap errors with `fmt.Errorf("context: %w", err)`; the CLI prints the chain.
- Table-driven tests keyed by scenario name.
- Comments explain *why*, not what. Match the existing density.
- Keep `README.md`'s "Status" section honest as things land.

## Related

Listed in the PROJECTS section of `~/resume`. If you change what this repo does, check whether the resume bullet is still accurate — see that repo's CLAUDE.md for the layout constraints that make edits there non-trivial.
