package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/j4ng5y/nats-docs-mcp-server/assets"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/config"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/natsdocs"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "source"
)

type cliOptions struct {
	configPath     string
	archivePath    string
	docsBaseURL    string
	revision       string
	includeOrphans bool
	includeLegacy  bool
	jsonOutput     bool
	limit          int
}

type cliApp struct {
	opts cliOptions
}

func main() {
	app := &cliApp{}

	rootCmd := &cobra.Command{
		Use:   "nats-docs",
		Short: "Standalone NATS documentation CLI",
		Long: `Standalone NATS documentation CLI.

The CLI is MCP-independent and embeds a NATS documentation archive by default,
so it can be deployed as a single executable. Use --archive to query a different
nats.docs GitHub zip file.`,
	}
	rootCmd.PersistentFlags().StringVarP(&app.opts.configPath, "config", "c", "", "optional config file for docs_base_url and archive settings")
	rootCmd.PersistentFlags().StringVar(&app.opts.archivePath, "archive", "", "optional nats.docs zip archive path; defaults to the embedded archive")
	rootCmd.PersistentFlags().StringVar(&app.opts.docsBaseURL, "docs-base-url", "", "public docs base URL for result links")
	rootCmd.PersistentFlags().StringVar(&app.opts.revision, "revision", "", "archive revision label for metadata")
	rootCmd.PersistentFlags().BoolVar(&app.opts.includeOrphans, "include-orphans", false, "include Markdown files not linked from SUMMARY.md")
	rootCmd.PersistentFlags().BoolVar(&app.opts.includeLegacy, "include-legacy", false, "include legacy Markdown targets")

	rootCmd.AddCommand(app.searchCommand(), app.getCommand(), app.listCommand(), app.doctorCommand(), versionCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func (a *cliApp) searchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search NATS documentation",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, _, err := a.loadIndex(cmd.Context())
			if err != nil {
				return err
			}
			query := strings.Join(args, " ")
			results, err := idx.Search(query, a.opts.limit)
			if err != nil {
				return err
			}
			if a.opts.jsonOutput {
				return writeJSON(results)
			}
			writeSearchResults(query, results)
			return nil
		},
	}
	cmd.Flags().IntVar(&a.opts.limit, "limit", 10, "maximum number of results")
	cmd.Flags().BoolVar(&a.opts.jsonOutput, "json", false, "write JSON output")
	return cmd
}

func (a *cliApp) getCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <doc-id>",
		Aliases: []string{"retrieve"},
		Short:   "Retrieve a NATS documentation page by ID",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, _, err := a.loadIndex(cmd.Context())
			if err != nil {
				return err
			}
			doc, err := idx.Get(normalizeDocID(args[0]))
			if err != nil {
				return err
			}
			if a.opts.jsonOutput {
				return writeJSON(doc)
			}
			writeDocument(doc)
			return nil
		},
	}
	cmd.Flags().BoolVar(&a.opts.jsonOutput, "json", false, "write JSON output")
	return cmd
}

func (a *cliApp) listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List indexed NATS documentation pages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, _, err := a.loadIndex(cmd.Context())
			if err != nil {
				return err
			}
			docs := idx.ExportDocuments()
			sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
			if a.opts.jsonOutput {
				return writeJSON(docs)
			}
			for _, doc := range docs {
				fmt.Printf("%s\t%s\t%s\n", doc.ID, doc.Title, doc.URL)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&a.opts.jsonOutput, "json", false, "write JSON output")
	return cmd
}

func (a *cliApp) doctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the embedded or configured NATS docs archive",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, meta, err := a.loadIndex(cmd.Context())
			if err != nil {
				return err
			}
			results, searchErr := idx.Search("jetstream", 5)
			_, retrieveErr := idx.Get("index")
			report := map[string]any{
				"documents":         idx.Count(),
				"aliases":           len(meta.Aliases),
				"aliases_dropped":   meta.AliasesDropped,
				"origin":            meta.Origin,
				"revision":          meta.Revision,
				"docs_base_url":     meta.SourceURL,
				"jetstream_results": len(results),
				"index_retrievable": retrieveErr == nil,
			}
			if searchErr != nil {
				report["search_error"] = searchErr.Error()
			}
			if retrieveErr != nil {
				report["retrieve_error"] = retrieveErr.Error()
			}
			if a.opts.jsonOutput {
				return writeJSON(report)
			}
			fmt.Println("NATS docs CLI health")
			fmt.Printf("  Origin: %s\n", meta.Origin)
			fmt.Printf("  Revision: %s\n", meta.Revision)
			fmt.Printf("  Docs URL: %s\n", meta.SourceURL)
			fmt.Printf("  Documents: %d\n", idx.Count())
			fmt.Printf("  Aliases: %d (%d dropped)\n", len(meta.Aliases), meta.AliasesDropped)
			if searchErr != nil {
				fmt.Printf("  Search \"jetstream\": ERROR: %v\n", searchErr)
			} else {
				fmt.Printf("  Search \"jetstream\": %d results\n", len(results))
			}
			if retrieveErr != nil {
				fmt.Printf("  Retrieve \"index\": ERROR: %v\n", retrieveErr)
			} else {
				fmt.Println("  Retrieve \"index\": OK")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&a.opts.jsonOutput, "json", false, "write JSON output")
	return cmd
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("nats-docs CLI\n")
			fmt.Printf("Version: %s\n", version)
			fmt.Printf("Commit:  %s\n", commit)
			fmt.Printf("Built:   %s\n", date)
			fmt.Printf("BuiltBy: %s\n", builtBy)
		},
	}
}

func (a *cliApp) loadIndex(ctx context.Context) (*index.DocumentationIndex, natsdocs.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, natsdocs.Metadata{}, err
	}

	opts := natsdocs.ArchiveOptions{
		DocsBaseURL:    cfg.DocsBaseURL,
		Revision:       cfg.NATSArchiveBranch,
		IncludeOrphans: cfg.NATSArchiveIncludeOrphans,
		IncludeLegacy:  cfg.NATSArchiveIncludeLegacy,
	}
	if a.opts.docsBaseURL != "" {
		opts.DocsBaseURL = a.opts.docsBaseURL
	}
	if a.opts.revision != "" {
		opts.Revision = a.opts.revision
	}
	if a.opts.includeOrphans {
		opts.IncludeOrphans = true
	}
	if a.opts.includeLegacy {
		opts.IncludeLegacy = true
	}

	switch archivePath := a.archivePath(cfg); archivePath {
	case "":
		opts.ArchiveBytes = assets.NATSDocsMasterZip
	default:
		opts.ArchivePath = archivePath
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return natsdocs.LoadArchiveIndex(ctx, opts, logger)
}

func (a *cliApp) loadConfig() (*config.Config, error) {
	var cfg *config.Config
	var err error
	if a.opts.configPath != "" {
		cfg, err = config.LoadFromFile(a.opts.configPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (a *cliApp) archivePath(cfg *config.Config) string {
	if a.opts.archivePath != "" {
		return a.opts.archivePath
	}
	if envPath := os.Getenv("NATS_DOCS_NATS_ARCHIVE_PATH"); envPath != "" {
		return envPath
	}
	if envPath := os.Getenv("NATS_ARCHIVE_PATH"); envPath != "" {
		return envPath
	}
	if a.opts.configPath != "" && cfg.NATSArchivePath != "" {
		return cfg.NATSArchivePath
	}
	return ""
}

func writeSearchResults(query string, results []index.SearchResult) {
	if len(results) == 0 {
		fmt.Printf("No results for query: %s\n", query)
		return
	}
	fmt.Printf("Found %d results for query: %s\n\n", len(results), query)
	for i, result := range results {
		fmt.Printf("%d. %s\n", i+1, result.Title)
		fmt.Printf("   ID: %s\n", result.DocumentID)
		fmt.Printf("   URL: %s\n", result.URL)
		fmt.Printf("   Relevance: %.2f\n", result.Relevance)
		if strings.TrimSpace(result.Summary) != "" {
			fmt.Printf("   Summary: %s\n", result.Summary)
		}
		fmt.Println()
	}
}

func writeDocument(doc *index.Document) {
	fmt.Printf("# %s\n\n", doc.Title)
	fmt.Printf("ID: %s\n", doc.ID)
	fmt.Printf("URL: %s\n\n", doc.URL)
	for _, section := range doc.Sections {
		level := section.Level + 1
		if level < 2 {
			level = 2
		}
		fmt.Printf("%s %s\n\n", strings.Repeat("#", level), section.Heading)
		fmt.Printf("%s\n\n", section.Content)
	}
}

func normalizeDocID(docID string) string {
	docID = strings.TrimSpace(docID)
	if u, err := url.Parse(docID); err == nil && u.Scheme != "" && u.Host != "" {
		docID = u.Path
	}
	docID = strings.Trim(docID, "/")
	if docID == "" {
		return "index"
	}
	return docID
}

func writeJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
