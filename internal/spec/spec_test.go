package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSpec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidPipeline(t *testing.T) {
	path := writeSpec(t, `
name: test
steps:
  - id: a
    image: alpine
    run: echo a
  - id: b
    image: alpine
    run: echo b
    needs: [a]
    retries: 2
`)

	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if p.Name != "test" {
		t.Errorf("Name = %q, want %q", p.Name, "test")
	}
	if len(p.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(p.Steps))
	}
	if got := p.Steps[1].Needs; len(got) != 1 || got[0] != "a" {
		t.Errorf("Steps[1].Needs = %v, want [a]", got)
	}
	if p.Steps[1].Retries != 2 {
		t.Errorf("Steps[1].Retries = %d, want 2", p.Steps[1].Retries)
	}
}

// A step may depend on one defined further down the file.
func TestLoadForwardReference(t *testing.T) {
	path := writeSpec(t, `
name: test
steps:
  - id: a
    image: alpine
    run: echo a
    needs: [b]
  - id: b
    image: alpine
    run: echo b
`)
	if _, err := Load(path); err != nil {
		t.Errorf("Load() error = %v, want nil for forward reference", err)
	}
}

func TestValidateRejects(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"no name": {`
steps:
  - id: a
    image: alpine
    run: echo a
`, "no name"},

		"no steps": {`
name: test
`, "no steps"},

		"duplicate id": {`
name: test
steps:
  - id: a
    image: alpine
    run: echo a
  - id: a
    image: alpine
    run: echo again
`, "duplicate"},

		"missing image": {`
name: test
steps:
  - id: a
    run: echo a
`, "no image"},

		"missing run": {`
name: test
steps:
  - id: a
    image: alpine
`, "no run"},

		"unknown dependency": {`
name: test
steps:
  - id: a
    image: alpine
    run: echo a
    needs: [ghost]
`, "unknown step"},

		"self dependency": {`
name: test
steps:
  - id: a
    image: alpine
    run: echo a
    needs: [a]
`, "needs itself"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeSpec(t, tc.body))
			if err == nil {
				t.Fatalf("Load() error = nil, want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("Load() error = nil for missing file")
	}
}
