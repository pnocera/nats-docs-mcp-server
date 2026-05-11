package parser

import (
	"path"
	"strings"
)

// BookIgnore matches the simple subset of .bookignore patterns used by GitBook.
type BookIgnore struct {
	dirPrefixes []string
	exactNames  []string
}

// ParseBookIgnore parses simple .bookignore content.
func ParseBookIgnore(content []byte) *BookIgnore {
	ignore := &BookIgnore{}
	for _, line := range strings.Split(string(content), "\n") {
		pattern := strings.TrimSpace(line)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}

		pattern = strings.TrimPrefix(strings.ReplaceAll(pattern, "\\", "/"), "./")
		if strings.HasSuffix(pattern, "/") {
			ignore.dirPrefixes = append(ignore.dirPrefixes, pattern)
			continue
		}
		ignore.exactNames = append(ignore.exactNames, pattern)
	}
	return ignore
}

// Match returns true if relPath is ignored.
func (b *BookIgnore) Match(relPath string) bool {
	if b == nil {
		return false
	}

	relPath = strings.TrimPrefix(path.Clean(strings.ReplaceAll(relPath, "\\", "/")), "./")
	for _, prefix := range b.dirPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}

	base := path.Base(relPath)
	for _, name := range b.exactNames {
		if base == name {
			return true
		}
	}
	return false
}
