package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyIsStable(t *testing.T) {
	a := Key("bwa:0.7.17", "bwa mem ref.fa in.fq", []string{"h1", "h2"})
	b := Key("bwa:0.7.17", "bwa mem ref.fa in.fq", []string{"h1", "h2"})
	if a != b {
		t.Errorf("Key() is not stable: %s != %s", a, b)
	}
}

func TestKeyIgnoresInputOrder(t *testing.T) {
	a := Key("img", "cmd", []string{"h1", "h2", "h3"})
	b := Key("img", "cmd", []string{"h3", "h1", "h2"})
	if a != b {
		t.Errorf("Key() depends on input order: %s != %s", a, b)
	}
}

func TestKeyChangesWithEachComponent(t *testing.T) {
	base := Key("img", "cmd", []string{"h1"})

	cases := map[string]string{
		"image":   Key("other", "cmd", []string{"h1"}),
		"command": Key("img", "other", []string{"h1"}),
		"inputs":  Key("img", "cmd", []string{"h2"}),
	}
	for name, got := range cases {
		if got == base {
			t.Errorf("Key() unchanged when %s changed", name)
		}
	}
}

// Without length-prefixing, these two would collide.
func TestKeyIsUnambiguous(t *testing.T) {
	a := Key("ab", "c", nil)
	b := Key("a", "bc", nil)
	if a == b {
		t.Error("Key() collides on shifted field boundaries")
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reads.fastq")
	if err := os.WriteFile(path, []byte("@read1\nACGT\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile() error = %v", err)
	}

	if err := os.WriteFile(path, []byte("@read1\nTGCA\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile() error = %v", err)
	}

	if h1 == h2 {
		t.Error("HashFile() unchanged after contents changed")
	}
}

func TestHashFileMissing(t *testing.T) {
	if _, err := HashFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("HashFile() error = nil for missing file")
	}
}
