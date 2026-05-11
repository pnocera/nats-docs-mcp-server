package parser

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// SummaryLink is one Markdown page linked from SUMMARY.md.
type SummaryLink struct {
	Title string
	Path  string
}

// ParseSummary extracts Markdown links from a GitBook SUMMARY.md.
func ParseSummary(content []byte) ([]SummaryLink, error) {
	md := goldmark.New()
	reader := text.NewReader(content)
	doc := md.Parser().Parse(reader)

	links := make([]SummaryLink, 0)
	seen := make(map[string]struct{})

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}

		dest := normalizeSummaryPath(string(link.Destination))
		if isExternalSummaryDestination(dest) {
			return ast.WalkContinue, nil
		}
		if !strings.HasSuffix(dest, ".md") {
			return ast.WalkContinue, nil
		}
		if _, ok := seen[dest]; ok {
			return ast.WalkContinue, nil
		}
		seen[dest] = struct{}{}
		links = append(links, SummaryLink{
			Title: extractTextFromNode(link, content),
			Path:  dest,
		})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	return links, nil
}

func isExternalSummaryDestination(dest string) bool {
	return strings.Contains(dest, "://") || strings.HasPrefix(dest, "//")
}

func normalizeSummaryPath(p string) string {
	p = strings.TrimSpace(p)
	p = unescapeGitBookPath(p)
	p = strings.TrimPrefix(p, "./")
	return p
}

func unescapeGitBookPath(p string) string {
	var out strings.Builder
	out.Grow(len(p))
	escaped := false
	for _, r := range p {
		if escaped {
			out.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		out.WriteRune(r)
	}
	if escaped {
		out.WriteRune('\\')
	}
	return out.String()
}
