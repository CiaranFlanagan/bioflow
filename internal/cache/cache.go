// Package cache computes content-addressed keys for pipeline steps.
//
// A step's key is derived from everything that could change its output: the
// image it runs, the command it runs, and the contents of its inputs. If the
// key matches a previous run, the step's output is still valid and the step
// can be skipped.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
)

// Key returns the cache key for a step. Input hashes are sorted first so that
// the key does not depend on the order inputs happen to be discovered in.
func Key(image, command string, inputHashes []string) string {
	sorted := make([]string, len(inputHashes))
	copy(sorted, inputHashes)
	sort.Strings(sorted)

	h := sha256.New()
	writeField(h, image)
	writeField(h, command)
	for _, ih := range sorted {
		writeField(h, ih)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeField length-prefixes each field so that concatenation is unambiguous:
// without this, ("ab", "c") and ("a", "bc") would hash identically.
func writeField(h io.Writer, s string) {
	fmt.Fprintf(h, "%d:%s", len(s), s)
}

// HashFile returns the SHA-256 of a file's contents. It streams rather than
// reading the whole file, since pipeline inputs are routinely gigabytes.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
