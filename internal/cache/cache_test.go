package cache

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
)

func TestNewCacheSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c, err := NewCache(tmpDir, logger)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	if c == nil {
		t.Fatal("Cache is nil")
	}

	if c.baseDir != tmpDir {
		t.Errorf("baseDir mismatch: got %s, want %s", c.baseDir, tmpDir)
	}

	// Verify directory exists
	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("Cache directory does not exist: %v", err)
	}
}

func TestNewCacheEmptyBaseDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_, err := NewCache("", logger)
	if err == nil {
		t.Fatal("Expected error for empty baseDir")
	}
}

func TestNewCacheCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := tmpDir + "/my/cache/dir"
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c, err := NewCache(cacheDir, logger)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	if c == nil {
		t.Fatal("Cache is nil")
	}

	// Verify nested directory was created
	if _, err := os.Stat(cacheDir); err != nil {
		t.Errorf("Cache directory was not created: %v", err)
	}
}

func TestCacheSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Create test documents
	docs := []*index.Document{
		{
			ID:      "test1",
			Title:   "Test Document 1",
			URL:     "https://example.com/test1",
			Content: "This is test content",
			Sections: []index.Section{
				{
					Heading: "Introduction",
					Content: "Introduction content",
					Level:   1,
				},
			},
			LastUpdated: time.Now(),
		},
	}

	// Save cache
	err := c.Save("test-source", "https://example.com", docs)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load cache
	cached, err := c.Load("test-source")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded data
	if cached.Source != "test-source" {
		t.Errorf("Source mismatch: got %s, want test-source", cached.Source)
	}

	if cached.SourceURL != "https://example.com" {
		t.Errorf("SourceURL mismatch: got %s, want https://example.com", cached.SourceURL)
	}

	if cached.DocumentCount != 1 {
		t.Errorf("DocumentCount mismatch: got %d, want 1", cached.DocumentCount)
	}

	if len(cached.Documents) != 1 {
		t.Errorf("Documents length mismatch: got %d, want 1", len(cached.Documents))
	}

	if cached.Documents[0].ID != "test1" {
		t.Errorf("Document ID mismatch: got %s, want test1", cached.Documents[0].ID)
	}
}

func TestCacheSaveEmptyDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Save empty documents
	err := c.Save("test-source", "https://example.com", []*index.Document{})
	if err != nil {
		t.Fatalf("Save with empty docs failed: %v", err)
	}

	// Load and verify
	cached, err := c.Load("test-source")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cached.DocumentCount != 0 {
		t.Errorf("DocumentCount mismatch: got %d, want 0", cached.DocumentCount)
	}
}

func TestCacheSaveEmptySource(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Try to save with empty source
	err := c.Save("", "https://example.com", []*index.Document{})
	if err == nil {
		t.Fatal("Expected error for empty source")
	}
}

func TestCacheSaveWithMetadataRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	docs := []*index.Document{{ID: "overview", Title: "Overview", URL: "https://docs.nats.io/overview", Content: "content"}}
	aliases := map[string]string{"overview-old": "overview"}
	err := c.SaveWithMetadata("nats-archive", SaveParams{
		SourceURL: "https://docs.nats.io",
		Kind:      "github_archive",
		Origin:    "assets/nats.docs-master.zip",
		Revision:  "master",
		Documents: docs,
		Aliases:   aliases,
	})
	if err != nil {
		t.Fatalf("SaveWithMetadata failed: %v", err)
	}

	aliases["overview-old"] = "mutated"
	cached, err := c.Load("nats-archive")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cached.Version != cacheVersion {
		t.Fatalf("expected cache version %s, got %s", cacheVersion, cached.Version)
	}
	if cached.Kind != "github_archive" || cached.Origin != "assets/nats.docs-master.zip" || cached.Revision != "master" {
		t.Fatalf("metadata did not round-trip: %+v", cached)
	}
	if got := cached.Aliases["overview-old"]; got != "overview" {
		t.Fatalf("expected copied alias overview, got %q", got)
	}
}

func TestCacheLoadV10Compatibility(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	cached := &CachedDocuments{
		Version:       "1.0",
		Source:        "nats",
		SourceURL:     "https://docs.nats.io",
		CachedAt:      time.Now(),
		DocumentCount: 1,
		Documents:     []*index.Document{{ID: "index", Title: "Welcome"}},
	}
	data, err := marshalForTesting(cached)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(c.getCachePath("nats"), data, 0644); err != nil {
		t.Fatalf("write v1.0 cache failed: %v", err)
	}

	loaded, err := c.Load("nats")
	if err != nil {
		t.Fatalf("expected v1.0 cache to load, got %v", err)
	}
	if loaded.Kind != "" || len(loaded.Aliases) != 0 {
		t.Fatalf("expected missing v1.1 metadata to be zero-valued, got %+v", loaded)
	}
}

func TestCacheLoadMissing(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Try to load non-existent cache
	_, err := c.Load("non-existent")
	if err != os.ErrNotExist {
		t.Errorf("Expected os.ErrNotExist, got %v", err)
	}
}

func TestCacheLoadEmptySource(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Try to load with empty source
	_, err := c.Load("")
	if err == nil {
		t.Fatal("Expected error for empty source")
	}
}

func TestCacheLoadCorruptedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Write corrupted JSON
	cacheFile := c.getCachePath("corrupted")
	corruptedData := []byte("{invalid json}")
	if err := os.WriteFile(cacheFile, corruptedData, 0644); err != nil {
		t.Fatalf("Failed to write corrupted cache file: %v", err)
	}

	// Try to load corrupted cache
	_, err := c.Load("corrupted")
	if err == nil {
		t.Fatal("Expected error when loading corrupted cache")
	}
}

func TestCacheIsValidFreshCache(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Create and save fresh cache
	docs := []*index.Document{
		{ID: "test", Title: "Test", URL: "https://example.com/test", Content: "test"},
	}
	c.Save("test", "https://example.com", docs)

	// Check if valid with 24 hour max age
	maxAge := 24 * time.Hour
	valid, err := c.IsValid("test", maxAge)
	if err != nil {
		t.Fatalf("IsValid failed: %v", err)
	}

	if !valid {
		t.Errorf("Fresh cache should be valid")
	}
}

func TestCacheIsValidExpiredCache(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Create and save cache
	docs := []*index.Document{
		{ID: "test", Title: "Test", URL: "https://example.com/test", Content: "test"},
	}
	c.Save("test", "https://example.com", docs)

	// Modify cache file to have old timestamp
	cached, _ := c.Load("test")
	cached.CachedAt = time.Now().Add(-48 * time.Hour) // 2 days ago
	data, _ := marshalForTesting(cached)
	cacheFile := c.getCachePath("test")
	os.WriteFile(cacheFile, data, 0644)

	// Check if valid with 24 hour max age (should be expired)
	maxAge := 24 * time.Hour
	valid, err := c.IsValid("test", maxAge)
	if err != nil {
		t.Fatalf("IsValid failed: %v", err)
	}

	if valid {
		t.Errorf("Expired cache should not be valid")
	}
}

func TestCacheIsValidMissing(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Check non-existent cache
	maxAge := 24 * time.Hour
	valid, err := c.IsValid("non-existent", maxAge)
	if err != nil {
		t.Fatalf("IsValid should not error for missing cache: %v", err)
	}

	if valid {
		t.Errorf("Missing cache should not be valid")
	}
}

func TestCacheIsValidEmptySource(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Check with empty source
	maxAge := 24 * time.Hour
	_, err := c.IsValid("", maxAge)
	if err == nil {
		t.Fatal("Expected error for empty source")
	}
}

func TestCacheIsValidEmptyDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Save cache with empty documents
	c.Save("empty", "https://example.com", []*index.Document{})

	// Check if valid
	maxAge := 24 * time.Hour
	valid, err := c.IsValid("empty", maxAge)
	if err != nil {
		t.Fatalf("IsValid failed: %v", err)
	}

	if valid {
		t.Errorf("Cache with empty documents should not be valid")
	}
}

func TestCacheClear(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Create and save cache
	docs := []*index.Document{
		{ID: "test", Title: "Test", URL: "https://example.com/test", Content: "test"},
	}
	c.Save("test", "https://example.com", docs)

	// Verify cache exists
	cacheFile := c.getCachePath("test")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatal("Cache file should exist before clear")
	}

	// Clear cache
	err := c.Clear("test")
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify cache is gone
	if _, err := os.Stat(cacheFile); err == nil {
		t.Fatal("Cache file should not exist after clear")
	}
}

func TestCacheClearNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Clear non-existent cache (should be idempotent)
	err := c.Clear("non-existent")
	if err != nil {
		t.Fatalf("Clear should be idempotent: %v", err)
	}
}

func TestCacheClearEmptySource(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Try to clear with empty source
	err := c.Clear("")
	if err == nil {
		t.Fatal("Expected error for empty source")
	}
}

func TestCacheClearAll(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Create multiple caches
	docs := []*index.Document{
		{ID: "test", Title: "Test", URL: "https://example.com/test", Content: "test"},
	}
	c.Save("nats", "https://example.com", docs)
	c.Save("Synadia", "https://Synadia.example.com", docs)

	// Verify caches exist
	if _, err := os.Stat(c.getCachePath("nats")); err != nil {
		t.Fatal("NATS cache should exist")
	}
	if _, err := os.Stat(c.getCachePath("Synadia")); err != nil {
		t.Fatal("Synadia cache should exist")
	}

	// Clear all
	err := c.ClearAll()
	if err != nil {
		t.Fatalf("ClearAll failed: %v", err)
	}

	// Verify caches are gone
	if _, err := os.Stat(c.getCachePath("nats")); err == nil {
		t.Fatal("NATS cache should not exist after ClearAll")
	}
	if _, err := os.Stat(c.getCachePath("Synadia")); err == nil {
		t.Fatal("Synadia cache should not exist after ClearAll")
	}

	// Verify directory still exists and is empty
	if _, err := os.Stat(c.baseDir); err != nil {
		t.Fatal("Cache directory should still exist after ClearAll")
	}
}

func TestCacheAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, _ := NewCache(tmpDir, logger)

	// Save initial cache
	docs := []*index.Document{
		{ID: "v1", Title: "Version 1", URL: "https://example.com/v1", Content: "v1"},
	}
	c.Save("test", "https://example.com", docs)

	// Read initial cache
	cached1, _ := c.Load("test")
	if cached1.Documents[0].ID != "v1" {
		t.Fatal("Initial cache should have v1")
	}

	// Save updated cache (atomic write should not leave partial state)
	docs = []*index.Document{
		{ID: "v2", Title: "Version 2", URL: "https://example.com/v2", Content: "v2"},
	}
	c.Save("test", "https://example.com", docs)

	// Load and verify update
	cached2, _ := c.Load("test")
	if cached2.Documents[0].ID != "v2" {
		t.Fatal("Updated cache should have v2")
	}

	// Verify temp file is cleaned up
	tempFile := c.getCachePath("test") + ".tmp"
	if _, err := os.Stat(tempFile); err == nil {
		t.Fatal("Temp file should be cleaned up after atomic write")
	}
}

// marshalForTesting is a helper to marshal CachedDocuments for testing
func marshalForTesting(cached *CachedDocuments) ([]byte, error) {
	return json.MarshalIndent(cached, "", "  ")
}
