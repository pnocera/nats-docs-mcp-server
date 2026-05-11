package parser

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Redirect maps a GitBook redirect source path to an archive-relative target.
type Redirect struct {
	From string
	To   string
}

type rawGitBookConfig struct {
	Redirects map[string]string `yaml:"redirects"`
}

// ParseGitBookRedirects parses the redirects block from .gitbook.yaml.
func ParseGitBookRedirects(content []byte) ([]Redirect, error) {
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, nil
	}

	var raw rawGitBookConfig
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	if len(raw.Redirects) == 0 {
		return nil, nil
	}

	redirects := make([]Redirect, 0, len(raw.Redirects))
	for from, to := range raw.Redirects {
		target, err := normalizeRedirectTo(to)
		if err != nil {
			return nil, err
		}
		redirects = append(redirects, Redirect{
			From: normalizeRedirectFrom(from),
			To:   target,
		})
	}
	sort.Slice(redirects, func(i, j int) bool {
		return redirects[i].From < redirects[j].From
	})
	return redirects, nil
}

func normalizeRedirectFrom(from string) string {
	from = strings.TrimSpace(unescapeGitBookPath(from))
	from = strings.Trim(from, "/")
	from = strings.TrimPrefix(from, "./")
	return from
}

func normalizeRedirectTo(to string) (string, error) {
	to = strings.TrimSpace(unescapeGitBookPath(to))
	to = strings.TrimPrefix(to, "./")
	to = strings.ReplaceAll(to, "\\", "/")
	if to == "" {
		return "", nil
	}
	if hasParentSegment(to) {
		return "", fmt.Errorf("redirect target contains path traversal: %s", to)
	}
	clean := path.Clean(to)
	if hasParentSegment(clean) {
		return "", fmt.Errorf("redirect target contains path traversal: %s", to)
	}
	return clean, nil
}

func hasParentSegment(p string) bool {
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
