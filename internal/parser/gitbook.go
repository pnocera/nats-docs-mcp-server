package parser

import (
	"bytes"
	"html"
	"regexp"
	"strings"
)

var gitBookDirectiveRewrites = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)\{\%\s*hint\s+style=["']?info["']?\s*\%\}`), "Info:\n"},
	{regexp.MustCompile(`(?i)\{\%\s*hint\s+style=["']?warning["']?\s*\%\}`), "Warning:\n"},
	{regexp.MustCompile(`(?i)\{\%\s*hint\s+style=["']?danger["']?\s*\%\}`), "Warning:\n"},
	{regexp.MustCompile(`(?i)\{\%\s*hint\s+style=["']?success["']?\s*\%\}`), "Note:\n"},
	{regexp.MustCompile(`(?i)\{\%\s*hint\s*\%\}`), "Note:\n"},
	{regexp.MustCompile(`(?i)\{\%\s*endhint\s*\%\}`), "\n"},
	{regexp.MustCompile(`(?i)\{\%\s*tabs\s*\%\}`), ""},
	{regexp.MustCompile(`(?i)\{\%\s*endtabs\s*\%\}`), ""},
	{regexp.MustCompile(`(?i)\{\%\s*tab\s+title=["']([^"']+)["']\s*\%\}`), "\n### $1\n"},
	{regexp.MustCompile(`(?i)\{\%\s*endtab\s*\%\}`), "\n"},
	{regexp.MustCompile(`(?i)\{\%\s*embed\s+url=["']([^"']+)["']\s*\%\}`), "Embedded: $1"},
	{regexp.MustCompile(`(?i)\{\%\s*endembed\s*\%\}`), ""},
	{regexp.MustCompile(`(?i)\{\%\s*code(?:\s+title=["'][^"']+["'])?\s*\%\}`), ""},
	{regexp.MustCompile(`(?i)\{\%\s*endcode\s*\%\}`), ""},
	{regexp.MustCompile(`(?is)<figure[^>]*>`), ""},
	{regexp.MustCompile(`(?is)</figure>`), ""},
	{regexp.MustCompile(`(?is)<img[^>]*\salt=["']([^"']*)["'][^>]*>`), "$1\n"},
	{regexp.MustCompile(`(?is)<figcaption[^>]*>(.*?)</figcaption>`), "$1"},
	{regexp.MustCompile(`(?i)\{\%.*?\%\}`), ""},
}

// PreprocessGitBook rewrites GitBook-specific directives into plain Markdown.
// Fenced code blocks are passed through byte-for-byte.
func PreprocessGitBook(src []byte) []byte {
	var out bytes.Buffer
	inFence := false
	var fenceMarker string

	for _, line := range splitKeepNewlines(src) {
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence {
			if marker, ok := fenceOpener(trimmed); ok {
				inFence = true
				fenceMarker = marker
				out.WriteString(line)
				continue
			}
			out.WriteString(applyDirectiveRewrites(line))
			continue
		}

		out.WriteString(line)
		if isFenceCloser(trimmed, fenceMarker) {
			inFence = false
			fenceMarker = ""
		}
	}

	return out.Bytes()
}

func splitKeepNewlines(src []byte) []string {
	if len(src) == 0 {
		return nil
	}

	parts := bytes.SplitAfter(src, []byte("\n"))
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		lines = append(lines, string(part))
	}
	return lines
}

func fenceOpener(trimmed string) (string, bool) {
	if marker := repeatedFenceMarker(trimmed, '`'); len(marker) >= 3 {
		return marker, true
	}
	if marker := repeatedFenceMarker(trimmed, '~'); len(marker) >= 3 {
		return marker, true
	}
	return "", false
}

func isFenceCloser(trimmed, marker string) bool {
	if marker == "" || !strings.HasPrefix(trimmed, marker) {
		return false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, marker))
	if rest == "" {
		return true
	}

	markerChar := marker[0]
	for i := 0; i < len(rest); i++ {
		if rest[i] != markerChar {
			return false
		}
	}
	return true
}

func repeatedFenceMarker(s string, marker byte) string {
	count := 0
	for count < len(s) && s[count] == marker {
		count++
	}
	if count == 0 {
		return ""
	}
	return s[:count]
}

func applyDirectiveRewrites(line string) string {
	for _, rewrite := range gitBookDirectiveRewrites {
		line = rewrite.re.ReplaceAllString(line, rewrite.repl)
	}
	return html.UnescapeString(line)
}
