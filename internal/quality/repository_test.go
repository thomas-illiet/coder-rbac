package quality

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

var ignoredDirectories = map[string]struct{}{
	".git":         {},
	".cache":       {},
	"bin":          {},
	"dist":         {},
	"node_modules": {},
	"vendor":       {},
}

// repositoryRoot returns the absolute path to the repository under test.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("determine repository root: runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// repositoryFiles returns sorted repository files accepted by include.
func repositoryFiles(t *testing.T, include func(path string, entry fs.DirEntry) bool) []string {
	t.Helper()
	root := repositoryRoot(t)
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				if _, ignored := ignoredDirectories[entry.Name()]; ignored {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if include(path, entry) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	sort.Strings(paths)
	return paths
}
