package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/cache"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/config"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
)

func TestServer_NATSArchiveSelected(t *testing.T) {
	cfg := config.NewConfig()
	cfg.NATSSourceType = "archive"
	cfg.NATSArchivePath = buildFixtureArchive(t)
	cfg.CacheDir = t.TempDir()

	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if _, ok := srv.natsLoader.(*natsArchiveLoader); !ok {
		t.Fatalf("expected natsArchiveLoader, got %T", srv.natsLoader)
	}
}

func TestServer_NATSArchiveSelectedByDefault(t *testing.T) {
	cfg := config.NewConfig()
	cfg.CacheDir = t.TempDir()

	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if _, ok := srv.natsLoader.(*natsArchiveLoader); !ok {
		t.Fatalf("expected natsArchiveLoader, got %T", srv.natsLoader)
	}
}

func TestServer_NATSSiteSelectedWhenConfigured(t *testing.T) {
	cfg := config.NewConfig()
	cfg.NATSSourceType = "site"
	cfg.CacheDir = t.TempDir()

	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if _, ok := srv.natsLoader.(*natsSiteLoader); !ok {
		t.Fatalf("expected natsSiteLoader, got %T", srv.natsLoader)
	}
}

func TestInitialize_ArchiveSource_IndexesFixture(t *testing.T) {
	srv := newArchiveServer(t, t.TempDir(), false)
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if got := srv.indexManager.GetNATSIndex().Count(); got != 4 {
		t.Fatalf("expected 4 docs, got %d", got)
	}
}

func TestInitialize_ArchiveSource_RetrieveRootByID(t *testing.T) {
	srv := newArchiveServer(t, t.TempDir(), false)
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	doc, err := srv.indexManager.GetNATSIndex().Get("index")
	if err != nil {
		t.Fatalf("Get index returned error: %v", err)
	}
	if doc.Title != "Welcome" {
		t.Fatalf("expected Welcome title, got %q", doc.Title)
	}
}

func TestInitialize_ArchiveSource_RetrieveNestedReadme(t *testing.T) {
	srv := newArchiveServer(t, t.TempDir(), false)
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	doc, err := srv.indexManager.GetNATSIndex().Get("nested/b")
	if err != nil {
		t.Fatalf("Get nested/b returned error: %v", err)
	}
	if doc.Title != "Nested B" {
		t.Fatalf("expected Nested B title, got %q", doc.Title)
	}
}

func TestInitialize_ArchiveSource_RetrieveAlias(t *testing.T) {
	srv := newArchiveServer(t, t.TempDir(), false)
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	doc, err := srv.indexManager.GetNATSIndex().Get("overview-old")
	if err != nil {
		t.Fatalf("Get overview-old returned error: %v", err)
	}
	if doc.ID != "overview" {
		t.Fatalf("expected overview via alias, got %q", doc.ID)
	}
}

func TestInitialize_ArchiveSource_WarmCacheRestoresAliases(t *testing.T) {
	cacheDir := t.TempDir()
	first := newArchiveServer(t, cacheDir, false)
	if err := first.Initialize(context.Background()); err != nil {
		t.Fatalf("first Initialize returned error: %v", err)
	}

	second := newArchiveServer(t, cacheDir, false)
	if err := second.Initialize(context.Background()); err != nil {
		t.Fatalf("second Initialize returned error: %v", err)
	}
	doc, err := second.indexManager.GetNATSIndex().Get("overview-old")
	if err != nil {
		t.Fatalf("warm cache alias lookup returned error: %v", err)
	}
	if doc.ID != "overview" {
		t.Fatalf("expected overview via warm-cache alias, got %q", doc.ID)
	}
}

func TestInitialize_ArchiveSource_KindMismatchInvalidatesCache(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, "nats-archive", "site_html", []*index.Document{{ID: "stale", Title: "Stale"}})

	srv := newArchiveServer(t, cacheDir, false)
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if _, err := srv.indexManager.GetNATSIndex().Get("stale"); err == nil {
		t.Fatal("expected stale wrong-kind cache document not to be imported")
	}
	if got := srv.indexManager.GetNATSIndex().Count(); got != 4 {
		t.Fatalf("expected archive reload count 4, got %d", got)
	}
}

func TestInitialize_ModeSwitch_SiteToArchive(t *testing.T) {
	cacheDir := t.TempDir()

	siteCfg := config.NewConfig()
	siteCfg.NATSSourceType = "site"
	siteCfg.CacheDir = cacheDir
	siteSrv, err := NewServer(siteCfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer site returned error: %v", err)
	}
	siteSrv.natsLoader = stubDocumentationLoader{
		docs: []*index.Document{{ID: "site-only", Title: "Site", URL: "https://docs.nats.io/site", LastUpdated: time.Now()}},
		meta: SourceMetadata{Source: "nats", Kind: "site_html", SourceURL: siteCfg.DocsBaseURL, Origin: siteCfg.DocsBaseURL},
	}
	if err := siteSrv.Initialize(context.Background()); err != nil {
		t.Fatalf("site Initialize returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "nats-site.json")); err != nil {
		t.Fatalf("expected nats-site cache: %v", err)
	}

	archiveSrv := newArchiveServer(t, cacheDir, false)
	if err := archiveSrv.Initialize(context.Background()); err != nil {
		t.Fatalf("archive Initialize returned error: %v", err)
	}
	if _, err := archiveSrv.indexManager.GetNATSIndex().Get("site-only"); err == nil {
		t.Fatal("expected site cache document not to be imported by archive mode")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "nats-archive.json")); err != nil {
		t.Fatalf("expected nats-archive cache: %v", err)
	}
}

func TestInitialize_ModeSwitch_ArchiveToSite(t *testing.T) {
	cacheDir := t.TempDir()
	archiveSrv := newArchiveServer(t, cacheDir, false)
	if err := archiveSrv.Initialize(context.Background()); err != nil {
		t.Fatalf("archive Initialize returned error: %v", err)
	}

	siteCfg := config.NewConfig()
	siteCfg.NATSSourceType = "site"
	siteCfg.CacheDir = cacheDir
	siteSrv, err := NewServer(siteCfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer site returned error: %v", err)
	}
	siteSrv.natsLoader = stubDocumentationLoader{
		docs: []*index.Document{{ID: "site-only", Title: "Site", URL: "https://docs.nats.io/site", LastUpdated: time.Now()}},
		meta: SourceMetadata{Source: "nats", Kind: "site_html", SourceURL: siteCfg.DocsBaseURL, Origin: siteCfg.DocsBaseURL},
	}
	if err := siteSrv.Initialize(context.Background()); err != nil {
		t.Fatalf("site Initialize returned error: %v", err)
	}
	if _, err := siteSrv.indexManager.GetNATSIndex().Get("index"); err == nil {
		t.Fatal("expected archive cache document not to be imported by site mode")
	}
	if _, err := siteSrv.indexManager.GetNATSIndex().Get("site-only"); err != nil {
		t.Fatalf("expected site-only doc: %v", err)
	}
}

func TestInitialize_ArchiveSource_OfflineNoNetwork(t *testing.T) {
	srv := newArchiveServer(t, t.TempDir(), false)
	srv.config.DocsBaseURL = "http://invalid.example.invalid"
	srv.natsLoader = newNATSArchiveLoader(natsArchiveConfig{
		ArchivePath: buildFixtureArchive(t),
		DocsBaseURL: srv.config.DocsBaseURL,
	}, testLogger())
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
}

func TestInitialize_DefaultArchiveAssetSmoke(t *testing.T) {
	archivePath := filepath.Join("..", "..", "assets", "nats.docs-master.zip")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("default archive asset is required for smoke test: %v", err)
	}

	cfg := config.NewConfig()
	cfg.CacheDir = t.TempDir()
	cfg.NATSArchivePath = archivePath
	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if got := srv.NATSIndex().Count(); got < 100 {
		t.Fatalf("expected default archive to index more than 100 documents, got %d", got)
	}
	results, err := srv.NATSIndex().Search("jetstream", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty jetstream search results")
	}
	if _, err := srv.NATSIndex().Get("index"); err != nil {
		t.Fatalf("expected root document to be retrievable: %v", err)
	}
}

func TestInitialize_RefreshCacheClearsOtherMode(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, "nats-site", "site_html", []*index.Document{{ID: "site-only", Title: "Site"}})
	writeCache(t, cacheDir, "nats-archive", "github_archive", []*index.Document{{ID: "stale", Title: "Stale"}})

	srv := newArchiveServer(t, cacheDir, true)
	if err := srv.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "nats-site.json")); !os.IsNotExist(err) {
		t.Fatalf("expected stale site cache removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "nats-archive.json")); err != nil {
		t.Fatalf("expected refreshed archive cache: %v", err)
	}
}

func newArchiveServer(t *testing.T, cacheDir string, refresh bool) *Server {
	t.Helper()
	cfg := config.NewConfig()
	cfg.CacheDir = cacheDir
	cfg.NATSSourceType = "archive"
	cfg.NATSArchivePath = buildFixtureArchive(t)
	cfg.RefreshCache = refresh

	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	return srv
}

func writeCache(t *testing.T, cacheDir, source, kind string, docs []*index.Document) {
	t.Helper()
	c, err := cache.NewCache(cacheDir, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("NewCache returned error: %v", err)
	}
	if err := c.SaveWithMetadata(source, cache.SaveParams{
		SourceURL: "https://docs.nats.io",
		Kind:      kind,
		Origin:    "test",
		Revision:  "test",
		Documents: docs,
	}); err != nil {
		t.Fatalf("SaveWithMetadata returned error: %v", err)
	}
}

type stubDocumentationLoader struct {
	docs []*index.Document
	meta SourceMetadata
	err  error
}

func (l stubDocumentationLoader) Load(context.Context) ([]*index.Document, SourceMetadata, error) {
	return l.docs, l.meta, l.err
}
