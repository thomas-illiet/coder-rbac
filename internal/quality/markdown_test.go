package quality

import (
	"bufio"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

// TestMarkdownLinksResolve verifies local Markdown files and heading anchors.
func TestMarkdownLinksResolve(t *testing.T) {
	paths := repositoryFiles(t, func(path string, _ fs.DirEntry) bool {
		return filepath.Ext(path) == ".md"
	})
	anchorsByPath := make(map[string]map[string]struct{}, len(paths))
	for _, path := range paths {
		anchorsByPath[path] = markdownAnchors(t, path)
	}

	for _, sourcePath := range paths {
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Errorf("read %s: %v", sourcePath, err)
			continue
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(string(content), -1) {
			target := strings.Trim(strings.TrimSpace(match[1]), "<>")
			if fields := strings.Fields(target); len(fields) > 0 {
				target = fields[0]
			}
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			parts := strings.SplitN(target, "#", 2)
			targetPath := sourcePath
			if parts[0] != "" {
				decoded, err := url.PathUnescape(parts[0])
				if err != nil {
					t.Errorf("%s: invalid local link %q: %v", sourcePath, target, err)
					continue
				}
				targetPath = filepath.Clean(filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(decoded)))
			}
			info, err := os.Stat(targetPath)
			if err != nil {
				t.Errorf("%s: local link %q does not resolve: %v", sourcePath, target, err)
				continue
			}
			if len(parts) != 2 || parts[1] == "" || info.IsDir() {
				continue
			}
			anchors, known := anchorsByPath[targetPath]
			if !known {
				t.Errorf("%s: anchor link %q targets a non-Markdown file", sourcePath, target)
				continue
			}
			anchor, err := url.PathUnescape(parts[1])
			if err != nil {
				t.Errorf("%s: invalid anchor in %q: %v", sourcePath, target, err)
				continue
			}
			if _, exists := anchors[anchor]; !exists {
				t.Errorf("%s: anchor %q does not exist in %s", sourcePath, anchor, targetPath)
			}
		}
	}
}

// markdownAnchors returns GitHub-style anchors declared by Markdown headings.
func markdownAnchors(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	anchors := make(map[string]struct{})
	counts := make(map[string]int)
	inFence := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if heading == "" {
			continue
		}
		base := githubHeadingAnchor(heading)
		anchor := base
		if count := counts[base]; count > 0 {
			anchor += "-" + strconv.Itoa(count)
		}
		counts[base]++
		anchors[anchor] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return anchors
}

// githubHeadingAnchor converts a heading to the anchor form used by GitHub Markdown.
func githubHeadingAnchor(heading string) string {
	heading = strings.ToLower(strings.ReplaceAll(heading, "`", ""))
	var builder strings.Builder
	lastWasSpace := false
	for _, character := range heading {
		switch {
		case unicode.IsLetter(character), unicode.IsDigit(character), character == '-', character == '_':
			builder.WriteRune(character)
			lastWasSpace = false
		case unicode.IsSpace(character):
			if builder.Len() > 0 && !lastWasSpace {
				builder.WriteByte('-')
				lastWasSpace = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}
