package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/config"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	docserver "github.com/j4ng5y/nats-docs-mcp-server/internal/server"
	"github.com/rs/zerolog"
)

var canonicalPages = []string{
	"/",
	"/overview",
	"/nats-concepts/jetstream/streams",
	"/running-a-nats-service/configuration/securing_nats/auth_callout",
	"/using-nats/jetstream/nats_api_reference",
	"/nats-concepts/subjects",
	"/using-nats/developing-with-nats/connecting",
	"/release_notes/whats_new",
}

var searchQueries = []string{
	"jetstream streams",
	"auth callout",
	"leaf nodes",
	"nats cli",
	"subject mapping",
}

func main() {
	archivePath := flag.String("archive", "assets/nats.docs-master.zip", "path to nats.docs zip archive")
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.Disabled)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	site, err := loadIndex(ctx, "site", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load site index: %v\n", err)
		os.Exit(1)
	}
	defer site.cleanup()

	archive, err := loadIndex(ctx, "archive", *archivePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load archive index: %v\n", err)
		os.Exit(1)
	}
	defer archive.cleanup()

	printSetReport(site, archive)
	printSpotChecks(site, archive)
	printSearchParity(site, archive)
}

type loadedIndex struct {
	name    string
	cache   string
	index   *index.DocumentationIndex
	docs    map[string]*index.Document
	cleanup func()
}

func loadIndex(ctx context.Context, sourceType, archivePath string) (loadedIndex, error) {
	cacheDir, err := os.MkdirTemp("", "nats-docs-parity-"+sourceType+"-")
	if err != nil {
		return loadedIndex{}, err
	}

	cfg := config.NewConfig()
	cfg.NATSSourceType = sourceType
	cfg.CacheDir = cacheDir
	cfg.RefreshCache = true
	cfg.MaxConcurrent = 3
	cfg.FetchTimeout = 60
	if sourceType == "archive" {
		cfg.NATSArchivePath = archivePath
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	srv, err := docserver.NewServer(cfg, logger)
	if err != nil {
		os.RemoveAll(cacheDir)
		return loadedIndex{}, err
	}
	if err := srv.Initialize(ctx); err != nil {
		os.RemoveAll(cacheDir)
		return loadedIndex{}, err
	}

	idx := srv.NATSIndex()
	docs := make(map[string]*index.Document)
	for _, doc := range idx.ExportDocuments() {
		docs[doc.ID] = doc
	}

	return loadedIndex{
		name:  sourceType,
		cache: cacheDir,
		index: idx,
		docs:  docs,
		cleanup: func() {
			_ = os.RemoveAll(cacheDir)
		},
	}, nil
}

func printSetReport(site, archive loadedIndex) {
	siteIDs := sortedIDs(site.docs)
	archiveIDs := sortedIDs(archive.docs)
	siteOnly, archiveOnly, common := diffIDs(siteIDs, archiveIDs)
	resolved, unresolved := archiveCoverageForSiteIDs(siteIDs, archive.index)
	coverage := float64(resolved) / float64(len(siteIDs)) * 100

	fmt.Printf("site:    %d documents\n", len(siteIDs))
	fmt.Printf("archive: %d documents\n", len(archiveIDs))
	fmt.Printf("direct common IDs: %d\n", common)
	fmt.Printf("site IDs resolved by archive IDs or aliases: %d/%d (%.1f%%)\n", resolved, len(siteIDs), coverage)
	fmt.Printf("unresolved site IDs after aliases: %s (%d)\n", formatIDs(unresolved, 20), len(unresolved))
	fmt.Printf("direct site-only IDs:    %s (%d)\n", formatIDs(siteOnly, 20), len(siteOnly))
	fmt.Printf("direct archive-only IDs: %s (%d)\n\n", formatIDs(archiveOnly, 20), len(archiveOnly))
}

func printSpotChecks(site, archive loadedIndex) {
	fmt.Println("Spot checks:")
	var archiveNonEmpty int
	for _, page := range canonicalPages {
		id := idFromURLPath(page)
		siteDoc := site.docs[id]
		archiveDoc, archiveErr := archive.index.Get(id)
		if archiveErr != nil {
			archiveDoc = nil
		}
		if archiveDoc != nil && strings.TrimSpace(archiveDoc.Content) != "" {
			archiveNonEmpty++
		}
		switch {
		case siteDoc == nil && archiveDoc == nil:
			fmt.Printf("- %s (%s): missing in both\n", page, id)
		case siteDoc == nil:
			fmt.Printf("- %s (%s): missing in site, archive=%s\n", page, id, archiveDoc.URL)
		case archiveDoc == nil:
			fmt.Printf("- %s (%s): missing in archive, site=%s\n", page, id, siteDoc.URL)
		default:
			ratio := contentLengthRatio(len(siteDoc.Content), len(archiveDoc.Content))
			fmt.Printf("- %s (%s): site=%s archive=%s length_ratio=%.2f\n",
				page, id, siteDoc.URL, archiveDoc.URL, ratio)
			fmt.Printf("  site:    %.500s\n", compact(siteDoc.Content))
			fmt.Printf("  archive: %.500s\n", compact(archiveDoc.Content))
		}
	}
	fmt.Printf("Archive spot checks with non-empty content: %d/%d\n", archiveNonEmpty, len(canonicalPages))
	fmt.Println()
}

func printSearchParity(site, archive loadedIndex) {
	fmt.Println("Search parity:")
	for _, query := range searchQueries {
		siteResults, siteErr := site.index.Search(query, 10)
		archiveResults, archiveErr := archive.index.Search(query, 10)
		if siteErr != nil || archiveErr != nil {
			fmt.Printf("- %q: site_err=%v archive_err=%v\n", query, siteErr, archiveErr)
			continue
		}
		urlOverlap := topURLOverlap(siteResults, archiveResults, 3)
		resolvedOverlap := topResolvedIDOverlap(siteResults, archiveResults, archive.index, 3)
		fmt.Printf("- %q: top3_url_overlap=%d/3 top3_resolved_id_overlap=%d/3\n", query, urlOverlap, resolvedOverlap)
		fmt.Printf("  site:    %s\n", formatSearchResults(siteResults, 3))
		fmt.Printf("  archive: %s\n", formatSearchResults(archiveResults, 3))
	}
}

func sortedIDs(docs map[string]*index.Document) []string {
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func archiveCoverageForSiteIDs(siteIDs []string, archive *index.DocumentationIndex) (int, []string) {
	var unresolved []string
	for _, id := range siteIDs {
		if _, err := archive.Get(id); err != nil {
			unresolved = append(unresolved, id)
			continue
		}
	}
	return len(siteIDs) - len(unresolved), unresolved
}

func formatIDs(ids []string, limit int) string {
	if len(ids) == 0 {
		return "[]"
	}
	if len(ids) <= limit {
		return fmt.Sprintf("%v", ids)
	}
	shown := append([]string(nil), ids[:limit]...)
	return fmt.Sprintf("%v +%d more", shown, len(ids)-limit)
}

func diffIDs(a, b []string) (aOnly, bOnly []string, common int) {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			common++
			i++
			j++
		case a[i] < b[j]:
			aOnly = append(aOnly, a[i])
			i++
		default:
			bOnly = append(bOnly, b[j])
			j++
		}
	}
	aOnly = append(aOnly, a[i:]...)
	bOnly = append(bOnly, b[j:]...)
	return aOnly, bOnly, common
}

func idFromURLPath(p string) string {
	clean := strings.Trim(path.Clean("/"+p), "/")
	if clean == "" || clean == "." {
		return "index"
	}
	return clean
}

func contentLengthRatio(siteLen, archiveLen int) float64 {
	if siteLen == 0 && archiveLen == 0 {
		return 1
	}
	if siteLen == 0 || archiveLen == 0 {
		return 0
	}
	if siteLen > archiveLen {
		return float64(archiveLen) / float64(siteLen)
	}
	return float64(siteLen) / float64(archiveLen)
}

func compact(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func topURLOverlap(a, b []index.SearchResult, n int) int {
	urls := make(map[string]struct{})
	for i := 0; i < len(a) && i < n; i++ {
		urls[a[i].URL] = struct{}{}
	}
	var overlap int
	for i := 0; i < len(b) && i < n; i++ {
		if _, ok := urls[b[i].URL]; ok {
			overlap++
		}
	}
	return overlap
}

func topResolvedIDOverlap(siteResults, archiveResults []index.SearchResult, archive *index.DocumentationIndex, n int) int {
	archiveIDs := make(map[string]struct{})
	for i := 0; i < len(archiveResults) && i < n; i++ {
		archiveIDs[archiveResults[i].DocumentID] = struct{}{}
	}

	var overlap int
	for i := 0; i < len(siteResults) && i < n; i++ {
		doc, err := archive.Get(siteResults[i].DocumentID)
		if err != nil {
			continue
		}
		if _, ok := archiveIDs[doc.ID]; ok {
			overlap++
		}
	}
	return overlap
}

func formatSearchResults(results []index.SearchResult, n int) string {
	parts := make([]string, 0, n)
	for i := 0; i < len(results) && i < n; i++ {
		parts = append(parts, fmt.Sprintf("%s <%s>", results[i].DocumentID, results[i].URL))
	}
	return strings.Join(parts, "; ")
}
