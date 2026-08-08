// Package dag builds a dependency graph over pipeline steps and resolves it
// into waves of steps that may run concurrently.
package dag

import (
	"fmt"
	"sort"
)

// Graph is a directed acyclic graph of step IDs.
type Graph struct {
	deps map[string][]string
}

// New returns an empty graph.
func New() *Graph {
	return &Graph{deps: make(map[string][]string)}
}

// Add registers a node and the nodes it depends on. Calling Add twice for the
// same id replaces the previous dependency list.
func (g *Graph) Add(id string, deps ...string) {
	cp := make([]string, len(deps))
	copy(cp, deps)
	g.deps[id] = cp
}

// Waves resolves the graph into ordered groups. Every node in a wave has all
// of its dependencies satisfied by earlier waves, so a wave can be executed
// entirely in parallel.
//
// This is Kahn's algorithm, peeling off all zero-indegree nodes at once rather
// than one at a time. Nodes left over when no zero-indegree node remains are
// exactly the nodes involved in (or downstream of) a cycle.
func (g *Graph) Waves() ([][]string, error) {
	indegree := make(map[string]int, len(g.deps))
	dependents := make(map[string][]string, len(g.deps))

	for id, deps := range g.deps {
		if _, ok := indegree[id]; !ok {
			indegree[id] = 0
		}
		for _, d := range deps {
			if _, ok := g.deps[d]; !ok {
				return nil, fmt.Errorf("node %q depends on unknown node %q", id, d)
			}
			indegree[id]++
			dependents[d] = append(dependents[d], id)
		}
	}

	var waves [][]string
	remaining := len(indegree)

	for remaining > 0 {
		var wave []string
		for id, deg := range indegree {
			if deg == 0 {
				wave = append(wave, id)
			}
		}
		if len(wave) == 0 {
			return nil, fmt.Errorf("cycle detected among steps: %v", sortedKeys(indegree))
		}

		// Sorted so that wave order is deterministic across runs, which keeps
		// logs and test assertions stable.
		sort.Strings(wave)
		waves = append(waves, wave)

		for _, id := range wave {
			delete(indegree, id)
			remaining--
			for _, dep := range dependents[id] {
				indegree[dep]--
			}
		}
	}
	return waves, nil
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
