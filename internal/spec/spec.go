// Package spec parses and validates pipeline definitions.
package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Pipeline is a complete pipeline definition, as written in YAML.
type Pipeline struct {
	Name   string `yaml:"name"`
	Inputs string `yaml:"inputs"`
	Steps  []Step `yaml:"steps"`
}

// Step is one unit of work: a single command run inside a single container.
// Needs lists the IDs of steps that must complete before this one starts.
type Step struct {
	ID      string   `yaml:"id"`
	Image   string   `yaml:"image"`
	Run     string   `yaml:"run"`
	Needs   []string `yaml:"needs"`
	Retries int      `yaml:"retries"`
}

// Load reads and validates a pipeline definition from disk.
func Load(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline: %w", err)
	}

	var p Pipeline
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse pipeline: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks that the pipeline is well-formed. It does not check that
// images exist or that commands are runnable — those fail at execution time.
func (p *Pipeline) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("pipeline has no name")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("pipeline %q has no steps", p.Name)
	}

	seen := make(map[string]bool, len(p.Steps))
	for i, s := range p.Steps {
		switch {
		case s.ID == "":
			return fmt.Errorf("step %d has no id", i)
		case seen[s.ID]:
			return fmt.Errorf("duplicate step id %q", s.ID)
		case s.Image == "":
			return fmt.Errorf("step %q has no image", s.ID)
		case s.Run == "":
			return fmt.Errorf("step %q has no run command", s.ID)
		case s.Retries < 0:
			return fmt.Errorf("step %q has negative retries", s.ID)
		}
		seen[s.ID] = true
	}

	// Dependencies are resolved after every ID is known, so that a step may
	// declare a dependency on a step defined later in the file.
	for _, s := range p.Steps {
		for _, dep := range s.Needs {
			if !seen[dep] {
				return fmt.Errorf("step %q needs unknown step %q", s.ID, dep)
			}
			if dep == s.ID {
				return fmt.Errorf("step %q needs itself", s.ID)
			}
		}
	}
	return nil
}
