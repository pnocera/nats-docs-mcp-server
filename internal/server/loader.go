package server

import (
	"context"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
)

// DocumentationLoader produces ready-to-index documents from one source.
type DocumentationLoader interface {
	Load(ctx context.Context) ([]*index.Document, SourceMetadata, error)
}

// SourceMetadata describes the source that produced a document set.
type SourceMetadata struct {
	Source         string
	Kind           string
	SourceURL      string
	Origin         string
	Revision       string
	Aliases        map[string]string
	AliasesDropped int
	DocumentIDs    []string
}
