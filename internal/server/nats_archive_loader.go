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
	// revision labels SourceMetadata; callers currently supply NATSArchiveBranch.
	revision string
	limits   fetcher.ArchiveLimits
	logger   *slog.Logger
}

type natsArchiveConfig struct {
	ArchivePath    string
	DocsBaseURL    string
	Revision       string
	IncludeOrphans bool
	IncludeLegacy  bool
}

func newNATSArchiveLoader(cfg natsArchiveConfig, logger *slog.Logger) *natsArchiveLoader {
	return &natsArchiveLoader{
		archivePath:    cfg.ArchivePath,
		docsBaseURL:    cfg.DocsBaseURL,
		includeOrphans: cfg.IncludeOrphans,
		includeLegacy:  cfg.IncludeLegacy,
		revision:       archiveRevisionOrDefault(cfg.Revision),
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
	addArchiveCompatibilityAliases(aliases, indexedIDs)
	sort.Strings(documentIDs)

	return docs, SourceMetadata{
		Source:         "nats",
		Kind:           "github_archive",
		SourceURL:      l.docsBaseURL,
		Origin:         l.archivePath,
		Revision:       l.revision,
		Aliases:        aliases,
		AliasesDropped: aliasesDropped,
		DocumentIDs:    documentIDs,
	}, nil
}

func archiveRevisionOrDefault(revision string) string {
	if revision == "" {
		return "master"
	}
	return revision
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

func addArchiveCompatibilityAliases(aliases map[string]string, indexedIDs map[string]struct{}) {
	for id := range indexedIDs {
		for _, alias := range archiveCompatibilityAliases(id) {
			if alias == "" || alias == id {
				continue
			}
			if _, indexed := indexedIDs[alias]; indexed {
				continue
			}
			if _, exists := aliases[alias]; exists {
				continue
			}
			aliases[alias] = id
		}
	}
}

func archiveCompatibilityAliases(id string) []string {
	aliases := make([]string, 0, 4)
	add := func(alias string) {
		aliases = append(aliases, strings.Trim(alias, "/"))
	}
	addReplacePrefix := func(oldPrefix, newPrefix string) {
		if strings.HasPrefix(id, oldPrefix) {
			add(newPrefix + strings.TrimPrefix(id, oldPrefix))
		}
	}

	switch id {
	case "overview":
		add("nats-concepts/overview")
	case "nats-concepts/adaptive_edge_deployment":
		add("nats-concepts/service_infrastructure/adaptive_edge_deployment")
	case "nats-concepts/core-nats/publish-subscribe/pubsub_walkthrough":
		add("nats-concepts/core-nats/pubsub/pubsub_walkthrough")
	case "nats-concepts/core-nats/queue-groups/queues_walkthrough":
		add("nats-concepts/core-nats/queue/queues_walkthrough")
	case "nats-concepts/core-nats/request-reply/reqreply_walkthrough":
		add("nats-concepts/core-nats/reqreply/reqreply_walkthrough")
	case "nats-concepts/jetstream/example_configuration":
		add("nats-concepts/jetstream/consumers/example_configuration")
	case "nats-concepts/jetstream/object-store/obj_walkthrough":
		add("nats-concepts/jetstream/obj_store/obj_walkthrough")
	case "nats-concepts/jetstream/source_and_mirror_example":
		add("nats-concepts/jetstream/source_and_mirror/source_and_mirror_example")
	case "reference-protocols":
		add("reference/reference-protocols")
	case "using-nats/jetstream/nats_api_reference":
		add("reference/reference-protocols/nats_api_reference")
	case "using-nats/developing-with-nats/developer":
		add("using-nats/developer")
	case "using-nats/developing-with-nats/anatomy":
		add("using-nats/developer/anatomy")
	case "using-nats/developing-with-nats/services":
		add("using-nats/developer/services")
	case "using-nats/jetstream/develop_jetstream":
		add("using-nats/developer/develop_jetstream")
	case "using-nats/jetstream/model_deep_dive":
		add("using-nats/developer/develop_jetstream/model_deep_dive")
	case "running-a-nats-service/installation":
		add("running-a-nats-service/introduction/installation")
	case "running-a-nats-service/running":
		add("running-a-nats-service/introduction/running")
	case "running-a-nats-service/running/nats_docker":
		add("running-a-nats-service/nats_docker")
	case "running-a-nats-service/nats-on-kubernetes/nats-kubernetes":
		add("running-a-nats-service/nats-kubernetes")
	case "running-a-nats-service/nats_admin/jwt":
		add("running-a-nats-service/nats_admin/security/jwt")
	case "running-a-nats-service/configuration/jetstream-config/resource_management":
		add("running-a-nats-service/configuration/resource_management")
	case "running-a-nats-service/configuration/clustering/cluster_tls":
		add("running-a-nats-service/configuration/securing_nats/auth_intro/tls_mutual_auth/cluster_tls")
	case "running-a-nats-service/configuration/ocsp":
		add("running-a-nats-service/configuration/securing_nats/ocsp")
	}

	addReplacePrefix("release_notes/", "release-notes/")
	addReplacePrefix("release_notes/", "release-notes/whats_new/")
	addReplacePrefix("nats-concepts/core-nats/publish-subscribe/", "nats-concepts/core-nats/")
	addReplacePrefix("nats-concepts/core-nats/queue-groups/", "nats-concepts/core-nats/")
	addReplacePrefix("nats-concepts/core-nats/request-reply/", "nats-concepts/core-nats/")
	addReplacePrefix("nats-concepts/jetstream/object-store/", "nats-concepts/jetstream/")
	addReplacePrefix("reference/nats-protocol/", "reference/reference-protocols/")
	addReplacePrefix("using-nats/developing-with-nats/connecting/security/", "using-nats/developer/connecting/")
	addReplacePrefix("using-nats/developing-with-nats/connecting", "using-nats/developer/connecting")
	addReplacePrefix("using-nats/developing-with-nats/reconnect", "using-nats/developer/connecting/reconnect")
	addReplacePrefix("using-nats/developing-with-nats/events", "using-nats/developer/connecting/events")
	addReplacePrefix("using-nats/developing-with-nats/js/", "using-nats/developer/develop_jetstream/")
	addReplacePrefix("using-nats/developing-with-nats/receiving", "using-nats/developer/receiving")
	addReplacePrefix("using-nats/developing-with-nats/sending", "using-nats/developer/sending")
	addReplacePrefix("using-nats/developing-with-nats/tutorials", "using-nats/developer/tutorials")
	addReplacePrefix("running-a-nats-service/running/nats_docker/", "running-a-nats-service/nats_docker/")
	addReplacePrefix("running-a-nats-service/running/", "running-a-nats-service/introduction/")
	addReplacePrefix("running-a-nats-service/configuration/jetstream-config/configuration_mgmt", "running-a-nats-service/configuration/resource_management/configuration_mgmt")
	addReplacePrefix("running-a-nats-service/configuration/securing_nats/jwt", "running-a-nats-service/configuration/securing_nats/auth_intro/jwt")

	return aliases
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
