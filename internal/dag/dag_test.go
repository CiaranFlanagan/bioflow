package dag

import (
	"reflect"
	"strings"
	"testing"
)

func TestWavesLinearChain(t *testing.T) {
	g := New()
	g.Add("a")
	g.Add("b", "a")
	g.Add("c", "b")

	waves, err := g.Waves()
	if err != nil {
		t.Fatalf("Waves() error = %v", err)
	}
	want := [][]string{{"a"}, {"b"}, {"c"}}
	if !reflect.DeepEqual(waves, want) {
		t.Errorf("Waves() = %v, want %v", waves, want)
	}
}

func TestWavesGroupsIndependentSteps(t *testing.T) {
	// qc and trim both depend only on the raw inputs, so they belong in the
	// same wave — this is the parallelism the executor relies on.
	g := New()
	g.Add("qc")
	g.Add("trim")
	g.Add("align", "trim")
	g.Add("call", "align")

	waves, err := g.Waves()
	if err != nil {
		t.Fatalf("Waves() error = %v", err)
	}
	want := [][]string{{"qc", "trim"}, {"align"}, {"call"}}
	if !reflect.DeepEqual(waves, want) {
		t.Errorf("Waves() = %v, want %v", waves, want)
	}
}

func TestWavesDetectsCycle(t *testing.T) {
	g := New()
	g.Add("a", "c")
	g.Add("b", "a")
	g.Add("c", "b")

	_, err := g.Waves()
	if err == nil {
		t.Fatal("Waves() error = nil, want cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("Waves() error = %q, want it to mention a cycle", err)
	}
}

func TestWavesRejectsUnknownDependency(t *testing.T) {
	g := New()
	g.Add("a", "ghost")

	if _, err := g.Waves(); err == nil {
		t.Fatal("Waves() error = nil, want unknown-node error")
	}
}

func TestWavesEmptyGraph(t *testing.T) {
	waves, err := New().Waves()
	if err != nil {
		t.Fatalf("Waves() error = %v", err)
	}
	if len(waves) != 0 {
		t.Errorf("Waves() = %v, want empty", waves)
	}
}
