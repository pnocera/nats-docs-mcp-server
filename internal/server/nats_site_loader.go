package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/fetcher"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/parser"
)

type natsSiteLoader struct {
	fetcher     *fetcher.MultiSourceFetcher
	docsBaseURL string
	logger      *slog.Logger
}

func (l *natsSiteLoader) Load(ctx context.Context) ([]*index.Document, SourceMetadata, error) {
	if l.fetcher == nil {
		return nil, SourceMetadata{}, fmt.Errorf("NATS site loader fetcher cannot be nil")
	}

	natsPages, err := l.fetcher.FetchNATS(ctx)
	if err != nil {
		return nil, SourceMetadata{}, fmt.Errorf("failed to fetch NATS documentation: %w", err)
	}

	docs := make([]*index.Document, 0, len(natsPages))
	now := time.Now()
	for _, page := range natsPages {
		doc, err := parser.ParseHTML(strings.NewReader(string(page.Content)))
		if err != nil {
			if l.logger != nil {
				l.logger.Warn("Failed to parse NATS page", "path", page.Path, "error", err)
			}
			continue
		}

		docs = append(docs, &index.Document{
			ID:          normalizePath(page.Path),
			Title:       doc.Title,
			URL:         strings.TrimRight(l.docsBaseURL, "/") + page.Path,
			Content:     extractContent(doc),
			Sections:    convertSections(doc.Sections),
			LastUpdated: now,
		})
	}

	return docs, SourceMetadata{
		Source:    "nats",
		Kind:      "site_html",
		SourceURL: l.docsBaseURL,
		Origin:    l.docsBaseURL,
	}, nil
}

var _ DocumentationLoader = (*natsSiteLoader)(nil)
