# bioflow

A job execution engine for containerized pipelines, built for bioinformatics workloads.

## The problem

Going from raw sequencing reads to called variants means chaining six to ten separate tools, each from a different lab, each with conflicting dependencies, each taking hours over gigabytes of data. People run this as a bash script, and it breaks:

- The chain takes 12 hours. Step 5 fails at hour 9, and you restart from zero.
- Independent work runs serially — 50 samples that could process in parallel go one at a time.
- Every tool needs a different libc or Python, so they fight unless containerized.
- Change one parameter and you re-run everything, including the 8 hours that didn't change.
- No visibility into what's running.

## What it does

Describe the pipeline; the engine works out the rest.

```yaml
name: variant-calling

steps:
  - id: qc
    image: biocontainers/fastqc:v0.11.9_cv8
    run: fastqc samples/*.fastq.gz -o out/qc

  - id: trim
    image: biocontainers/fastp:v0.20.1_cv1
    run: fastp -i samples/sample.fastq.gz -o out/trimmed.fastq.gz
    retries: 2

  - id: align
    image: biocontainers/bwa:v0.7.17_cv1
    needs: [trim]
    run: bwa mem ref/genome.fa out/trimmed.fastq.gz > out/aligned.sam

  - id: call
    image: biocontainers/bcftools:v1.9-1-deb_cv1
    needs: [align]
    run: bcftools mpileup -f ref/genome.fa out/aligned.sam | bcftools call -mv -o out/variants.vcf
```

```bash
bioflow -workers 8 -metrics :9090 pipeline.yaml
```

- **Parallelism.** `qc` and `trim` are independent, so they land in the same wave and run concurrently across a bounded worker pool.
- **Caching.** Each step's key is a hash of its image, command, and inputs. Change only the variant-calling parameters and the first three steps are skipped.
- **Crash recovery.** Results are checkpointed as they complete. A resumed run skips everything already done.
- **Retries.** Per-step retry counts with exponential backoff, for tools that fail transiently.
- **Metrics.** Prometheus endpoint exposing step duration histograms, completions, failures, retries, and cache hits.

## Install

```bash
go build -o bin/bioflow ./cmd/bioflow
```

Requires Go 1.24+ and a running Docker daemon.

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-workers` | `4` | maximum steps running concurrently |
| `-state` | `.bioflow/state.json` | resume checkpoint path |
| `-workdir` | `.` | directory mounted into each container as `/work` |
| `-metrics` | off | address to serve Prometheus metrics on |
| `-v` | off | debug logging |

## Design notes

**Waves, not a flat topological order.** Kahn's algorithm peels off all zero-indegree nodes at once rather than one at a time, so the output directly expresses what can run in parallel. Cycle detection falls out of the same pass.

**Cache keys are length-prefixed.** Without it, `("ab", "c")` and `("a", "bc")` hash identically.

**Checkpoints are written via temp-file-and-rename.** A crash mid-write can't leave a truncated state file.

**Failures are recorded, not just successes.** A resumed run can distinguish a step that never ran from one that ran and failed.

**Docker via CLI, not the SDK.** Simpler to reason about and matches what a user would run by hand. Steps run for minutes, so process overhead is irrelevant.

**Input hashing is stubbed.** Cache keys currently cover image and command, which catches pipeline edits but not input data changes. Glob expansion is the next piece.

## Development

```bash
make test    # unit tests
make race    # with the race detector — the executor is the risky part
make cover   # coverage report
make lint    # vet + gofmt
```

## Status

Early. Working: spec parsing and validation, DAG resolution into parallel waves, bounded worker pool, content-addressed caching, checkpoint and resume, retries with backoff, context cancellation, Docker execution, Prometheus metrics.

Not yet: input glob expansion (so cache keys don't yet cover data), per-sample fan-out, a Kubernetes executor backend, live progress rendering.

## License

MIT
