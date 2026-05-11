// Package server provides the MCP server core implementation, handling protocol
// communication, tool registration, and request routing.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/cache"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/classifier"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/config"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/fetcher"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/parser"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/search"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"
)

// normalizePath removes leading and trailing slashes from a path.
// This ensures consistent document ID lookups regardless of slash usage.
// Examples: "/nats-concepts/jetstream" -> "nats-concepts/jetstream"
//
//	"nats-concepts/jetstream/" -> "nats-concepts/jetstream"
//	"/" -> ""
func normalizePath(p string) string {
	p = strings.TrimLeft(p, "/")
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "index"
	}
	return p
}

// Server represents the MCP server instance with all its dependencies.
// It coordinates the MCP protocol handling, documentation indexing, and tool execution.
// Supports dual documentation sources (NATS and Synadia) with classification-based routing.
type Server struct {
	config       *config.Config
	indexManager *index.Manager        // Dual-index manager for NATS and Synadia
	orchestrator *search.Orchestrator  // Search orchestrator for multi-source search
	classifier   classifier.Classifier // Query classifier for routing
	logger       *slog.Logger
	mcpServer    *server.MCPServer
	multiFetcher *fetcher.MultiSourceFetcher // Fetcher for both NATS and Synadia
	natsLoader   DocumentationLoader
	transport    TransportStarter
	cache        *cache.Cache // Cache for persisting documentation
	initialized  bool
}

// NewServer creates a new MCP server instance with the provided configuration and logger.
// The server is not started until Start() is called.
//
// Parameters:
//   - cfg: Configuration for the server
//   - logger: Structured logger for logging
//
// Returns a configured Server instance ready to be started.
// Returns an error if transport creation fails.
func NewServer(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Validate transport configuration
	if err := cfg.ValidateTransport(); err != nil {
		return nil, fmt.Errorf("invalid transport configuration: %w", err)
	}

	// Create MCP server instance
	mcpServer := server.NewMCPServer(
		"nats-docs-mcp-server",
		"1.0.0",
	)

	// Create index manager for multiple indices (NATS, Synadia, and GitHub)
	indexManager := index.NewManager()

	// Create classifier with configured keywords
	queryClassifier := classifier.NewKeywordClassifier(
		cfg.SynadiaKeywords,
		cfg.NATSKeywords,
		cfg.GitHubKeywords,
	)

	// Create search orchestrator
	searchOrchestrator := search.NewOrchestrator(
		indexManager.GetNATSIndex(),
		indexManager.GetSynadiaIndex(),
		indexManager.GetGitHubIndex(),
		queryClassifier,
	)

	// Create zerolog logger for fetcher (use os.Stderr for structured logging)
	zerologLogger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

	// Create multi-source fetcher for both NATS and Synadia
	natsConfig := fetcher.FetchConfig{
		BaseURL:       cfg.DocsBaseURL,
		MaxRetries:    5,
		FetchTimeout:  time.Duration(cfg.FetchTimeout) * time.Second,
		MaxConcurrent: cfg.MaxConcurrent,
	}

	syadiaConfig := fetcher.FetchConfig{
		BaseURL:       cfg.SynadiaBaseURL,
		MaxRetries:    5,
		FetchTimeout:  time.Duration(cfg.SynadiaFetchTimeout) * time.Second,
		MaxConcurrent: cfg.MaxConcurrent,
	}

	// Create GitHub fetcher config (always, for cache refresh support)
	// The GitHubEnabled flag controls initialization on startup, but we always prepare
	// the config so cache refresh can pull GitHub sources regardless of enable flags.
	// A token is optional - unauthenticated requests work but have rate limits.
	repos := make([]fetcher.GitHubRepo, len(cfg.GitHubRepositories))
	for i, repoStr := range cfg.GitHubRepositories {
		parts := strings.Split(repoStr, "/")
		if len(parts) == 2 {
			repos[i] = fetcher.GitHubRepo{
				Owner:     parts[0],
				Name:      parts[1],
				Branch:    cfg.GitHubBranch,
				ShortName: parts[1],
			}
		}
	}
	githubConfig := fetcher.GitHubFetchConfig{
		Token:         cfg.GitHubToken,
		Repositories:  repos,
		MaxRetries:    5,
		FetchTimeout:  time.Duration(cfg.GitHubFetchTimeout) * time.Second,
		MaxConcurrent: cfg.MaxConcurrent,
	}

	multiFetcher := fetcher.NewMultiSourceFetcher(natsConfig, syadiaConfig, githubConfig, zerologLogger)

	var natsLoader DocumentationLoader
	switch cfg.NATSSourceType {
	case "archive":
		natsLoader = newNATSArchiveLoader(natsArchiveConfig{
			ArchivePath:    cfg.NATSArchivePath,
			DocsBaseURL:    cfg.DocsBaseURL,
			Revision:       cfg.NATSArchiveBranch,
			IncludeOrphans: cfg.NATSArchiveIncludeOrphans,
			IncludeLegacy:  cfg.NATSArchiveIncludeLegacy,
		}, logger)
	default:
		natsLoader = &natsSiteLoader{
			fetcher:     multiFetcher,
			docsBaseURL: cfg.DocsBaseURL,
			logger:      logger,
		}
	}

	// Create transport based on configuration
	transport, err := NewTransport(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	// Create cache instance (best-effort, gracefully handle failures)
	cacheDir := cfg.GetCacheDir()
	cacheInstance, err := cache.NewCache(cacheDir, logger)
	if err != nil {
		logger.Warn("Failed to create cache, continuing without caching", "error", err)
		cacheInstance = nil
	}

	return &Server{
		config:       cfg,
		indexManager: indexManager,
		orchestrator: searchOrchestrator,
		classifier:   queryClassifier,
		logger:       logger,
		mcpServer:    mcpServer,
		multiFetcher: multiFetcher,
		natsLoader:   natsLoader,
		transport:    transport,
		cache:        cacheInstance,
		initialized:  false,
	}, nil
}

// Initialize performs server initialization including documentation fetching and indexing.
// This should be called before Start() to ensure the server is ready to handle requests.
// Uses cached documentation if available and valid, otherwise fetches from network.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns an error if initialization fails.
func (s *Server) Initialize(ctx context.Context) error {
	if s.initialized {
		return fmt.Errorf("server already initialized")
	}

	s.logger.Info("Starting server initialization")

	// Initialize NATS documentation
	if err := s.initializeNATS(ctx); err != nil {
		return err
	}

	// Initialize Synadia documentation (if enabled)
	if s.config.SynadiaEnabled {
		if err := s.initializeSynadia(ctx); err != nil {
			s.logger.Warn("Failed to initialize Synadia docs", "error", err)
			// Continue without Synadia (graceful degradation)
		}
	}

	// Initialize GitHub documentation (if enabled)
	if s.config.GitHubEnabled {
		if err := s.initializeGitHub(ctx); err != nil {
			s.logger.Warn("Failed to initialize GitHub docs", "error", err)
			// Continue without GitHub (graceful degradation)
		}
	}

	// Report index statistics
	stats := s.indexManager.Stats()
	s.logger.Info("Documentation indexing complete",
		"nats_docs", stats.NATSDocCount,
		"syncp_docs", stats.SynadiaDocCount,
		"total_docs", stats.TotalDocCount)

	s.initialized = true
	return nil
}

// initializeNATS initializes NATS documentation, using cache if available and valid.
func (s *Server) initializeNATS(ctx context.Context) error {
	source := natsCacheSourceForType(s.config.NATSSourceType)
	expectedKind := kindForSourceType(s.config.NATSSourceType)

	if s.config.RefreshCache && s.cache != nil {
		if err := s.cache.Clear(otherNATSCacheSource(s.config.NATSSourceType)); err != nil {
			s.logger.Warn("Failed to clear stale NATS cache", "error", err)
		}
	}

	// Check if we should use cache
	if !s.config.RefreshCache && s.cache != nil {
		maxAge := time.Duration(s.config.CacheMaxAge) * 24 * time.Hour
		valid, err := s.cache.IsValid(source, maxAge)

		if err != nil {
			s.logger.Warn("Cache validation failed, will reload",
				"source", source, "error", err)
		} else if valid {
			// Load from cache
			s.logger.Info("Loading NATS docs from cache", "source", source)
			cached, err := s.cache.Load(source)
			switch {
			case err != nil:
				s.logger.Warn("Cache load failed, will reload", "source", source, "error", err)
			case len(cached.Documents) == 0:
				s.logger.Info("Empty cache, will reload", "source", source)
			case cached.Kind != "" && cached.Kind != expectedKind:
				s.logger.Info("Cache kind mismatch, will reload",
					"source", source, "cached_kind", cached.Kind, "expected_kind", expectedKind)
			default:
				if err := s.indexManager.GetNATSIndex().ImportDocuments(cached.Documents); err != nil {
					s.logger.Warn("Failed to import cached docs, will reload", "error", err)
				} else {
					s.indexManager.GetNATSIndex().SetAliases(cached.Aliases)
					s.logger.Info("Loaded NATS docs from cache",
						"source", source,
						"count", len(cached.Documents),
						"aliases", len(cached.Aliases),
						"cached_at", cached.CachedAt)
					return nil
				}
			}
		}
	}

	s.logger.Info("Loading NATS documentation",
		"source_type", s.config.NATSSourceType)

	docs, meta, err := s.natsLoader.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load NATS documentation: %w", err)
	}

	if len(docs) == 0 {
		return fmt.Errorf("NATS loader returned zero documents")
	}

	if err := s.indexManager.IndexNATS(docs); err != nil {
		return fmt.Errorf("failed to index NATS documentation: %w", err)
	}
	s.indexManager.GetNATSIndex().SetAliases(meta.Aliases)

	s.logger.Info("NATS documentation indexed",
		"count", len(docs),
		"kind", meta.Kind,
		"origin", meta.Origin,
		"revision", meta.Revision,
		"aliases", len(meta.Aliases),
		"aliases_dropped", meta.AliasesDropped)

	// Save to cache (best-effort, log errors but don't fail)
	if s.cache != nil {
		if err := s.cache.SaveWithMetadata(source, cache.SaveParams{
			SourceURL: meta.SourceURL,
			Kind:      meta.Kind,
			Origin:    meta.Origin,
			Revision:  meta.Revision,
			Documents: docs,
			Aliases:   meta.Aliases,
		}); err != nil {
			s.logger.Warn("Failed to save cache", "source", source, "error", err)
		} else {
			s.logger.Info("Saved NATS docs to cache", "source", source, "count", len(docs))
		}
	}

	return nil
}

func natsCacheSourceForType(sourceType string) string {
	if sourceType == "archive" {
		return "nats-archive"
	}
	return "nats-site"
}

func otherNATSCacheSource(sourceType string) string {
	if sourceType == "archive" {
		return "nats-site"
	}
	return "nats-archive"
}

func kindForSourceType(sourceType string) string {
	if sourceType == "archive" {
		return "github_archive"
	}
	return "site_html"
}

// initializeSynadia initializes Synadia documentation, using cache if available and valid.
func (s *Server) initializeSynadia(ctx context.Context) error {
	source := "syncp"

	// Check if we should use cache
	if !s.config.RefreshCache && s.cache != nil {
		maxAge := time.Duration(s.config.CacheMaxAge) * 24 * time.Hour
		valid, err := s.cache.IsValid(source, maxAge)

		if err != nil {
			s.logger.Warn("Cache validation failed, will fetch from network",
				"source", source, "error", err)
		} else if valid {
			// Load from cache
			s.logger.Info("Loading Synadia docs from cache", "source", source)
			cached, err := s.cache.Load(source)
			if err == nil && len(cached.Documents) > 0 {
				// Import documents into index
				if err := s.indexManager.GetSynadiaIndex().ImportDocuments(cached.Documents); err == nil {
					s.logger.Info("Loaded Synadia docs from cache",
						"count", len(cached.Documents),
						"cached_at", cached.CachedAt)
					return nil
				}
				s.logger.Warn("Failed to import cached docs, will fetch", "error", err)
			}
		}
	}

	// Cache miss or refresh requested - fetch from network
	s.logger.Info("Fetching Synadia documentation from network",
		"base_url", s.config.SynadiaBaseURL)

	syadiaPages, err := s.multiFetcher.FetchSynadia(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch Synadia documentation: %w", err)
	}

	s.logger.Info("Fetched Synadia documentation pages", "count", len(syadiaPages))

	// Parse and index documents
	syadiaIndexDocs := make([]*index.Document, 0)
	for _, page := range syadiaPages {
		doc, err := parser.ParseHTML(strings.NewReader(string(page.Content)))
		if err != nil {
			s.logger.Warn("Failed to parse Synadia page", "path", page.Path, "error", err)
			continue
		}

		indexDoc := &index.Document{
			ID:          normalizePath(page.Path),
			Title:       doc.Title,
			URL:         s.config.SynadiaBaseURL + page.Path,
			Content:     extractContent(doc),
			Sections:    convertSections(doc.Sections),
			LastUpdated: time.Now(),
		}
		syadiaIndexDocs = append(syadiaIndexDocs, indexDoc)
	}

	if len(syadiaIndexDocs) == 0 {
		return fmt.Errorf("failed to parse any Synadia documentation pages")
	}

	if err := s.indexManager.IndexSynadia(syadiaIndexDocs); err != nil {
		return fmt.Errorf("failed to index Synadia documentation: %w", err)
	}

	s.logger.Info("Synadia documentation indexed", "count", len(syadiaIndexDocs))

	// Save to cache (best-effort, log errors but don't fail)
	if s.cache != nil {
		if err := s.cache.Save(source, s.config.SynadiaBaseURL, syadiaIndexDocs); err != nil {
			s.logger.Warn("Failed to save cache", "source", source, "error", err)
		} else {
			s.logger.Info("Saved Synadia docs to cache", "count", len(syadiaIndexDocs))
		}
	}

	return nil
}

// initializeGitHub initializes GitHub documentation, using cache if available and valid.
func (s *Server) initializeGitHub(ctx context.Context) error {
	source := "github"

	// Check if we should use cache
	if !s.config.RefreshCache && s.cache != nil {
		maxAge := time.Duration(s.config.CacheMaxAge) * 24 * time.Hour
		valid, err := s.cache.IsValid(source, maxAge)

		if err != nil {
			s.logger.Warn("Cache validation failed, will fetch from network",
				"source", source, "error", err)
		} else if valid {
			// Load from cache
			s.logger.Info("Loading GitHub docs from cache", "source", source)
			cached, err := s.cache.Load(source)
			if err == nil && len(cached.Documents) > 0 {
				// Import documents into index
				if err := s.indexManager.GetGitHubIndex().ImportDocuments(cached.Documents); err == nil {
					s.logger.Info("Loaded GitHub docs from cache",
						"count", len(cached.Documents),
						"cached_at", cached.CachedAt)
					return nil
				}
				s.logger.Warn("Failed to import cached docs, will fetch", "error", err)
			}
		}
	}

	// Cache miss or refresh requested - fetch from network
	s.logger.Info("Fetching GitHub documentation from network")

	githubFiles, err := s.multiFetcher.FetchGitHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch GitHub documentation: %w", err)
	}

	s.logger.Info("Fetched GitHub documentation files", "count", len(githubFiles))

	// Parse and index documents
	githubIndexDocs := make([]*index.Document, 0)
	for _, file := range githubFiles {
		doc, err := parser.ParseMarkdown(file.Content, file.Path)
		if err != nil {
			s.logger.Warn("Failed to parse GitHub markdown file", "path", file.Path, "error", err)
			continue
		}

		// Build GitHub URL
		parts := strings.Split(file.Repo, "/")
		var owner, name string
		if len(parts) == 2 {
			owner = parts[0]
			name = parts[1]
		} else {
			// Try to find the repo in the configured repositories
			for _, repoStr := range s.config.GitHubRepositories {
				repoParts := strings.Split(repoStr, "/")
				if len(repoParts) == 2 && repoParts[1] == file.Repo {
					owner = repoParts[0]
					name = repoParts[1]
					break
				}
			}
		}

		githubURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s",
			owner, name, s.config.GitHubBranch, file.Path)

		indexDoc := &index.Document{
			ID:          file.Repo + "/" + file.Path,
			Title:       doc.Title,
			URL:         githubURL,
			Content:     extractContent(doc),
			Sections:    convertSections(doc.Sections),
			LastUpdated: time.Now(),
		}
		githubIndexDocs = append(githubIndexDocs, indexDoc)
	}

	if len(githubIndexDocs) == 0 {
		return fmt.Errorf("failed to parse any GitHub documentation files")
	}

	if err := s.indexManager.IndexGitHub(githubIndexDocs); err != nil {
		return fmt.Errorf("failed to index GitHub documentation: %w", err)
	}

	s.logger.Info("GitHub documentation indexed", "count", len(githubIndexDocs))

	// Save to cache (best-effort, log errors but don't fail)
	if s.cache != nil {
		if err := s.cache.Save(source, "github", githubIndexDocs); err != nil {
			s.logger.Warn("Failed to save cache", "source", source, "error", err)
		} else {
			s.logger.Info("Saved GitHub docs to cache", "count", len(githubIndexDocs))
		}
	}

	return nil
}

// RefreshCache performs a cache refresh by clearing the cache and re-fetching documentation.
// Unlike Initialize, this always attempts to refresh all available sources regardless of enable flags,
// since the user is explicitly requesting a cache refresh.
func (s *Server) RefreshCache(ctx context.Context) (int, error) {
	if s.cache == nil {
		return 0, fmt.Errorf("cache is not configured")
	}

	s.logger.Info("Manual cache refresh requested - pulling all documentation")

	// Clear existing caches
	if err := s.cache.ClearAll(); err != nil {
		return 0, fmt.Errorf("failed to clear cache: %w", err)
	}

	// Reset indices
	s.indexManager.Reset()

	// Re-initialize NATS (always)
	if err := s.initializeNATS(ctx); err != nil {
		return 0, fmt.Errorf("failed to refresh NATS cache: %w", err)
	}

	docsRefreshed := s.indexManager.GetNATSIndex().Count()

	// Re-initialize Synadia (always attempt, regardless of enable flag)
	// This ensures all documentation is available when explicitly refreshing
	if err := s.initializeSynadia(ctx); err != nil {
		s.logger.Warn("Failed to refresh Synadia cache during refresh operation", "error", err)
		// Don't fail the entire refresh, continue with other sources
	} else {
		docsRefreshed += s.indexManager.GetSynadiaIndex().Count()
	}

	// Re-initialize GitHub (always attempt, regardless of enable flag)
	// This ensures all documentation is available when explicitly refreshing
	if err := s.initializeGitHub(ctx); err != nil {
		s.logger.Warn("Failed to refresh GitHub cache during refresh operation", "error", err)
		// Don't fail the entire refresh, continue with other sources
	} else {
		docsRefreshed += s.indexManager.GetGitHubIndex().Count()
	}

	s.logger.Info("Cache refresh complete", "docs_refreshed", docsRefreshed)
	return docsRefreshed, nil
}

// RegisterTools registers all MCP tools with the server.
// This should be called after Initialize() and before Start().
//
// Returns an error if tool registration fails.
func (s *Server) RegisterTools() error {
	if !s.initialized {
		return fmt.Errorf("server not initialized, call Initialize() first")
	}

	s.logger.Info("Registering MCP tools")

	// Register search_nats_docs tool
	searchTool := mcp.NewTool(
		"search_nats_docs",
		mcp.WithDescription("Search NATS documentation by keywords or topics. Returns relevant documentation sections with summaries."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query (keywords or topic)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 10)"),
		),
	)

	s.mcpServer.AddTool(searchTool, s.handleSearchTool)

	// Register retrieve_nats_doc tool
	retrieveTool := mcp.NewTool(
		"retrieve_nats_doc",
		mcp.WithDescription("Retrieve complete content of a specific NATS documentation page by ID or URL path."),
		mcp.WithString("doc_id",
			mcp.Required(),
			mcp.Description("Document ID or URL path (e.g., 'nats-concepts/overview')"),
		),
	)

	s.mcpServer.AddTool(retrieveTool, s.handleRetrieveTool)

	// Register refresh_docs_cache tool
	refreshTool := mcp.NewTool(
		"refresh_docs_cache",
		mcp.WithDescription("Refresh the documentation cache by fetching latest docs from source URLs. Use this to update stale documentation."),
	)

	s.mcpServer.AddTool(refreshTool, s.handleRefreshCacheTool)

	s.logger.Info("MCP tools registered successfully")
	return nil
}

// Start starts the MCP server and begins listening for client connections.
// This is a blocking call that runs until the context is cancelled or an error occurs.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns an error if the server fails to start or encounters an error during operation.
func (s *Server) Start(ctx context.Context) error {
	if !s.initialized {
		return fmt.Errorf("server not initialized, call Initialize() first")
	}

	s.logger.Info("Starting MCP server", "transport", s.transport.Type())
	if addr := s.config.GetTransportAddress(); addr != "" {
		s.logger.Info("Transport address", "address", addr)
	}

	// Start the transport with the MCP server
	if err := s.transport.Start(ctx, s.mcpServer); err != nil {
		s.logger.Error("MCP server error", "error", err, "transport", s.transport.Type())
		return fmt.Errorf("MCP server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server and cleans up resources.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns an error if shutdown fails.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server", "transport", s.transport.Type())

	// Shutdown the transport
	if err := s.transport.Shutdown(ctx); err != nil {
		s.logger.Error("Error during transport shutdown", "error", err, "transport", s.transport.Type())
		return fmt.Errorf("transport shutdown error: %w", err)
	}

	s.logger.Info("Server shutdown complete", "transport", s.transport.Type())
	return nil
}

// NATSIndex returns the NATS documentation index for developer tooling. It is
// not part of the MCP server surface.
func (s *Server) NATSIndex() *index.DocumentationIndex {
	return s.indexManager.GetNATSIndex()
}

// extractContent extracts all text content from a parsed document
func extractContent(doc *parser.Document) string {
	var content strings.Builder
	for _, section := range doc.Sections {
		content.WriteString(section.Content)
		content.WriteString("\n")
	}
	return content.String()
}

// convertSections converts parser.Section to index.Section
func convertSections(sections []parser.Section) []index.Section {
	result := make([]index.Section, len(sections))
	for i, s := range sections {
		result[i] = index.Section{
			Heading: s.Heading,
			Content: s.Content,
			Level:   s.Level,
		}
	}
	return result
}

// handleSearchTool handles the search_nats_docs tool invocation
// Uses the Search Orchestrator to route queries to appropriate documentation source(s)
func (s *Server) handleSearchTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract query parameter (required)
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required and must be a non-empty string"), nil
	}

	// Extract limit parameter (optional, default to 10)
	limit := request.GetInt("limit", 10)

	// Perform multi-source search using orchestrator
	results, err := s.orchestrator.Search(query, limit)
	if err != nil {
		s.logger.Error("Search failed", "query", query, "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	// Format results with source information
	var content strings.Builder
	content.WriteString(fmt.Sprintf("Found %d results for query: %s\n\n", len(results), query))

	for i, result := range results {
		content.WriteString(fmt.Sprintf("%d. %s [%s]\n", i+1, result.Title, result.Source))
		content.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		content.WriteString(fmt.Sprintf("   Relevance: %.2f\n", result.Score))
		content.WriteString(fmt.Sprintf("   Summary: %s\n\n", result.Snippet))
	}

	s.logger.Info("Search completed", "query", query, "results", len(results))

	return mcp.NewToolResultText(content.String()), nil
}

// handleRetrieveTool handles the retrieve_nats_doc tool invocation
// Retrieves documents from both NATS and Synadia indices
func (s *Server) handleRetrieveTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract doc_id parameter (required)
	docID, err := request.RequireString("doc_id")
	if err != nil {
		return mcp.NewToolResultError("doc_id parameter is required and must be a non-empty string"), nil
	}

	// Normalize the document ID to handle leading/trailing slashes
	normalizedID := normalizePath(docID)

	// Try to retrieve from NATS index first
	doc, err := s.indexManager.GetNATSIndex().Get(normalizedID)
	if err != nil {
		// Try Synadia index if NATS fails
		doc, err = s.indexManager.GetSynadiaIndex().Get(normalizedID)
		if err != nil {
			// Try GitHub index if Synadia fails
			doc, err = s.indexManager.GetGitHubIndex().Get(normalizedID)
			if err != nil {
				s.logger.Warn("Document not found in any index", "doc_id", docID, "normalized_id", normalizedID)
				return mcp.NewToolResultError(fmt.Sprintf("document not found: %s", docID)), nil
			}
		}
	}

	// Format document content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("# %s\n\n", doc.Title))
	content.WriteString(fmt.Sprintf("URL: %s\n\n", doc.URL))

	// Add sections
	for _, section := range doc.Sections {
		content.WriteString(fmt.Sprintf("%s %s\n\n", strings.Repeat("#", section.Level+1), section.Heading))
		content.WriteString(fmt.Sprintf("%s\n\n", section.Content))
	}

	s.logger.Info("Document retrieved", "doc_id", docID, "title", doc.Title)

	return mcp.NewToolResultText(content.String()), nil
}

// handleRefreshCacheTool handles requests to refresh the documentation cache.
func (s *Server) handleRefreshCacheTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Perform cache refresh
	docsRefreshed, err := s.RefreshCache(ctx)
	if err != nil {
		s.logger.Error("Cache refresh failed", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("cache refresh failed: %v", err)), nil
	}

	message := fmt.Sprintf("Successfully refreshed documentation cache\n")
	message += fmt.Sprintf("Documents refreshed: %d\n", docsRefreshed)
	if s.cache != nil {
		message += fmt.Sprintf("Cache location: %s\n", s.config.GetCacheDir())
	}

	return mcp.NewToolResultText(message), nil
}
