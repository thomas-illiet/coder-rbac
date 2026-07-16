package quality

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var frenchTextPattern = regexp.MustCompile(`(?i)([àâçéèêëîïôùûüÿœæ]|\b(avec|aucun|aucune|cependant|chaque|cette|dans|depuis|doit|doivent|également|être|français|lorsque|permet|pour|réseau|réinitialisation|sans|sécurité|toutes|utiliser|vérifier)\b)`)

var authoredTextExtensions = map[string]struct{}{
	".go":   {},
	".md":   {},
	".sh":   {},
	".tf":   {},
	".tpl":  {},
	".txt":  {},
	".yaml": {},
	".yml":  {},
}

// TestRepositoryAuthoredTextIsEnglish rejects high-signal residual French prose.
func TestRepositoryAuthoredTextIsEnglish(t *testing.T) {
	paths := repositoryFiles(t, func(path string, entry fs.DirEntry) bool {
		if filepath.Base(path) == "language_test.go" {
			return false
		}
		if _, accepted := authoredTextExtensions[strings.ToLower(filepath.Ext(path))]; accepted {
			return true
		}
		base := filepath.Base(path)
		return base == "Dockerfile" || base == "Makefile"
	})
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if !utf8.Valid(content) {
			t.Errorf("%s is not valid UTF-8", path)
			continue
		}
		if location := frenchTextPattern.FindIndex(content); location != nil {
			line := 1 + strings.Count(string(content[:location[0]]), "\n")
			match := string(content[location[0]:location[1]])
			t.Errorf("%s:%d: possible French prose %q", path, line, match)
		}
	}
}
