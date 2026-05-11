package server

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/fetcher"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/parser"
)

type natsArchiveLoader struct {
	archivePath    string
	docsBaseURL    string
	includeOrphans bool
	includeLegacy  bool
	limits         fetcher.ArchiveLimits
	logger         *slog.Logger
}

type natsArchiveConfig struct {
	ArchivePath    string
	DocsBaseURL    string
	IncludeOrphans bool
	IncludeLegacy  bool
}

func newNATSArchiveLoader(cfg natsArchiveConfig, logger *slog.Logger) *natsArchiveLoader {
	return &natsArchiveLoader{
		archivePath:    cfg.ArchivePath,
		docsBaseURL:    cfg.DocsBaseURL,
		includeOrphans: cfg.IncludeOrphans,
		includeLegacy:  cfg.IncludeLegacy,
		limits:         fetcher.DefaultArchiveLimits(),
		logger:         logger,
	}
}

func (l *natsArchiveLoader) Load(ctx context.Context) ([]*index.Document, SourceMetadata, error) {
	arch, err := fetcher.OpenLocalArchive(ctx, l.archivePath, l.limits)
	if err != nil {
		return nil, SourceMetadata{}, err
	}

	summaryEntry, ok := arch.Get("SUMMARY.md")
	if !ok {
		return nil, SourceMetadata{}, fmt.Errorf("archive is missing SUMMARY.md")
	}

	var ignore *parser.BookIgnore
	if entry, ok := arch.Get(".bookignore"); ok {
		ignore = parser.ParseBookIgnore(entry.Content)
	} else {
		ignore = parser.ParseBookIgnore(nil)
	}

	summaryLinks, err := parser.ParseSummary(summaryEntry.Content)
	if err != nil {
		return nil, SourceMetadata{}, fmt.Errorf("failed to parse SUMMARY.md: %w", err)
	}

	var redirects []parser.Redirect
	if entry, ok := arch.Get(".gitbook.yaml"); ok {
		redirects, err = parser.ParseGitBookRedirects(entry.Content)
		if err != nil {
			return nil, SourceMetadata{}, fmt.Errorf("failed to parse .gitbook.yaml: %w", err)
		}
	}

	candidates := orderedPathSet{}
	candidates.Add("README.md")
	for _, link := range summaryLinks {
		candidates.Add(link.Path)
	}
	if l.includeLegacy {
		for _, redirect := range redirects {
			if strings.HasPrefix(redirect.To, "legacy/") && strings.HasSuffix(redirect.To, ".md") {
				candidates.Add(redirect.To)
			}
		}
	}
	if l.includeOrphans {
		for _, p := range arch.List(func(p string) bool { return strings.HasSuffix(p, ".md") }) {
			if isArchiveMetadataMarkdown(p) {
				continue
			}
			candidates.Add(p)
		}
	}

	docs := make([]*index.Document, 0, len(candidates.items))
	indexedIDs := make(map[string]struct{})
	documentIDs := make([]string, 0, len(candidates.items))
	now := time.Now()

	for _, p := range candidates.items {
		if shouldSkipArchivePath(p, ignore, l.includeLegacy) {
			continue
		}

		entry, ok := arch.Get(p)
		if !ok {
			if l.logger != nil {
				l.logger.Warn("Archive candidate missing", "path", p)
			}
			continue
		}
		if strings.TrimSpace(string(entry.Content)) == "" {
			continue
		}

		id, err := docIDFromPath(p)
		if err != nil {
			if l.logger != nil {
				l.logger.Warn("Skipping archive path with invalid ID", "path", p, "error", err)
			}
			continue
		}
		urlPath, err := urlFromPath(p)
		if err != nil {
			if l.logger != nil {
				l.logger.Warn("Skipping archive path with invalid URL", "path", p, "error", err)
			}
			continue
		}

		parsed, err := parser.ParseMarkdown(parser.PreprocessGitBook(entry.Content), p)
		if err != nil {
			if l.logger != nil {
				l.logger.Warn("Failed to parse archive Markdown", "path", p, "error", err)
			}
			continue
		}

		doc := &index.Document{
			ID:          id,
			Title:       parsed.Title,
			URL:         joinURL(l.docsBaseURL, urlPath),
			Content:     extractContent(parsed),
			Sections:    convertSections(parsed.Sections),
			LastUpdated: now,
		}
		docs = append(docs, doc)
		indexedIDs[id] = struct{}{}
		documentIDs = append(documentIDs, id)
	}

	aliases, aliasesDropped := buildArchiveAliases(redirects, indexedIDs, l.logger)
	sort.Strings(documentIDs)

	return docs, SourceMetadata{
		Source:         "nats",
		Kind:           "github_archive",
		SourceURL:      l.docsBaseURL,
		Origin:         l.archivePath,
		Revision:       "master",
		Aliases:        aliases,
		AliasesDropped: aliasesDropped,
		DocumentIDs:    documentIDs,
	}, nil
}

func shouldSkipArchivePath(p string, ignore *parser.BookIgnore, includeLegacy bool) bool {
	if isArchiveMetadataMarkdown(p) {
		return true
	}
	if ignore != nil && ignore.Match(p) {
		return true
	}
	if strings.HasPrefix(p, "zh-cn/") {
		return true
	}
	if !includeLegacy && strings.HasPrefix(p, "legacy/") {
		return true
	}
	return false
}

func isArchiveMetadataMarkdown(p string) bool {
	return p == "SUMMARY.md"
}

func buildArchiveAliases(redirects []parser.Redirect, indexedIDs map[string]struct{}, logger *slog.Logger) (map[string]string, int) {
	aliases := make(map[string]string)
	var dropped int
	for _, redirect := range redirects {
		from := normalizeRedirectAlias(redirect.From)
		to, err := docIDFromPath(redirect.To)
		if err != nil {
			dropped++
			if logger != nil {
				logger.Info("Dropping redirect alias with invalid target", "from", from, "to", redirect.To, "error", err)
			}
			continue
		}
		if _, ok := indexedIDs[to]; !ok {
			dropped++
			if logger != nil {
				category := "aliases_dropped_missing"
				if strings.HasPrefix(redirect.To, "legacy/") {
					category = "aliases_dropped_legacy"
				}
				logger.Info("Dropping redirect alias", "category", category, "from", from, "to", redirect.To)
			}
			continue
		}
		aliases[from] = to
	}
	return aliases, dropped
}

func normalizeRedirectAlias(p string) string {
	p = unescapeArchivePath(p)
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	p = strings.TrimPrefix(p, "./")
	return p
}

func urlFromPath(p string) (string, error) {
	id, err := docIDFromPath(p)
	if err != nil {
		return "", err
	}
	if id == "index" {
		return "/", nil
	}
	return "/" + id, nil
}

func docIDFromPath(p string) (string, error) {
	normalized, err := normalizeArchiveDocPath(p)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(normalized, ".md") {
		return "", fmt.Errorf("archive path must end in .md: %s", p)
	}

	withoutExt := strings.TrimSuffix(normalized, ".md")
	if withoutExt == "README" {
		return "index", nil
	}
	if strings.HasSuffix(withoutExt, "/README") {
		return strings.TrimSuffix(withoutExt, "/README"), nil
	}
	return withoutExt, nil
}

func normalizeArchiveDocPath(p string) (string, error) {
	p = strings.TrimSpace(unescapeArchivePath(p))
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return "", fmt.Errorf("archive path cannot be empty")
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("archive path cannot be absolute: %s", p)
	}
	if hasParentPathSegment(p) {
		return "", fmt.Errorf("archive path contains traversal: %s", p)
	}
	clean := path.Clean(p)
	if clean != p {
		return "", fmt.Errorf("archive path is not clean: %s", p)
	}
	return clean, nil
}

func hasParentPathSegment(p string) bool {
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func unescapeArchivePath(p string) string {
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

func joinURL(base, urlPath string) string {
	base = strings.TrimRight(base, "/")
	if urlPath == "/" {
		return base + "/"
	}
	return base + urlPath
}

type orderedPathSet struct {
	seen  map[string]struct{}
	items []string
}

func (s *orderedPathSet) Add(p string) {
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	p = strings.TrimPrefix(unescapeArchivePath(p), "./")
	if _, ok := s.seen[p]; ok {
		return
	}
	s.seen[p] = struct{}{}
	s.items = append(s.items, p)
}

var _ DocumentationLoader = (*natsArchiveLoader)(nil)
