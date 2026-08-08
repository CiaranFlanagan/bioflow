package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/CiaranFlanagan/bioflow/internal/spec"
)

// DockerRunner runs each step inside a container.
//
// It shells out to the docker CLI rather than using the Docker SDK. That is a
// deliberate tradeoff: the CLI is simpler to reason about and matches what a
// user would run by hand, at the cost of process overhead per step. Steps here
// run for minutes, so the overhead is irrelevant.
type DockerRunner struct {
	// WorkDir is mounted into the container as /work and used as the cwd.
	WorkDir string
}

// Run executes one step's command in a container and returns an error
// containing the tail of stderr if it exits non-zero.
func (d *DockerRunner) Run(ctx context.Context, step spec.Step) error {
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/work", d.WorkDir),
		"-w", "/work",
		step.Image,
		"sh", "-c", step.Run,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker run %s: %w: %s",
			step.Image, err, lastLines(stderr.String(), 10))
	}
	return nil
}

// lastLines trims container output down to the part that usually explains the
// failure. Bioinformatics tools are extremely verbose on stderr.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
