package server

import (
	"context"
	"log/slog"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/natsdocs"
)

type natsArchiveLoader struct {
	options natsdocs.ArchiveOptions
	logger  *slog.Logger
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
		options: natsdocs.ArchiveOptions{
			ArchivePath:    cfg.ArchivePath,
			DocsBaseURL:    cfg.DocsBaseURL,
			Revision:       cfg.Revision,
			IncludeOrphans: cfg.IncludeOrphans,
			IncludeLegacy:  cfg.IncludeLegacy,
		},
		logger: logger,
	}
}

func (l *natsArchiveLoader) Load(ctx context.Context) ([]*index.Document, SourceMetadata, error) {
	docs, meta, err := natsdocs.LoadArchive(ctx, l.options, l.logger)
	if err != nil {
		return nil, SourceMetadata{}, err
	}
	return docs, SourceMetadata{
		Source:         meta.Source,
		Kind:           meta.Kind,
		SourceURL:      meta.SourceURL,
		Origin:         meta.Origin,
		Revision:       meta.Revision,
		Aliases:        meta.Aliases,
		AliasesDropped: meta.AliasesDropped,
		DocumentIDs:    meta.DocumentIDs,
	}, nil
}

var _ DocumentationLoader = (*natsArchiveLoader)(nil)
