package cache

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
)

const (
	// cacheVersion is the current cache format version
	cacheVersion = "1.1"
	// cacheDirPermissions is the permissions for the cache directory
	cacheDirPermissions = 0755
	// cacheFilePermissions is the permissions for cache files
	cacheFilePermissions = 0644
)

// CachedDocuments represents cached documentation with metadata
type CachedDocuments struct {
	Version       string            `json:"version"`
	Source        string            `json:"source"`
	SourceURL     string            `json:"source_url"`
	Kind          string            `json:"kind,omitempty"`
	Origin        string            `json:"origin,omitempty"`
	Revision      string            `json:"revision,omitempty"`
	CachedAt      time.Time         `json:"cached_at"`
	DocumentCount int               `json:"document_count"`
	Documents     []*index.Document `json:"documents"`
	Aliases       map[string]string `json:"aliases,omitempty"`
}

// SaveParams bundles optional metadata for cache v1.1.
type SaveParams struct {
	SourceURL string
	Kind      string
	Origin    string
	Revision  string
	Documents []*index.Document
	Aliases   map[string]string
}

// Cache handles reading/writing documentation cache to disk
type Cache struct {
	baseDir string
	logger  *slog.Logger
}

// NewCache creates a new cache instance and ensures the cache directory exists
func NewCache(baseDir string, logger *slog.Logger) (*Cache, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("cache base directory cannot be empty")
	}

	c := &Cache{
		baseDir: baseDir,
		logger:  logger,
	}

	if err := c.ensureDir(); err != nil {
		return nil, fmt.Errorf("failed to ensure cache directory: %w", err)
	}

	return c, nil
}

// ensureDir creates the cache directory if it doesn't exist
func (c *Cache) ensureDir() error {
	return os.MkdirAll(c.baseDir, cacheDirPermissions)
}

// getCachePath returns the full path to a cache file for the given source
func (c *Cache) getCachePath(source string) string {
	return filepath.Join(c.baseDir, source+".json")
}

// Save persists documentation to cache with atomic writes
func (c *Cache) Save(source string, sourceURL string, docs []*index.Document) error {
	return c.SaveWithMetadata(source, SaveParams{
		SourceURL: sourceURL,
		Documents: docs,
	})
}

// SaveWithMetadata persists documentation to cache with source metadata.
func (c *Cache) SaveWithMetadata(source string, params SaveParams) error {
	if source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	// Create CachedDocuments struct
	cached := &CachedDocuments{
		Version:       cacheVersion,
		Source:        source,
		SourceURL:     params.SourceURL,
		Kind:          params.Kind,
		Origin:        params.Origin,
		Revision:      params.Revision,
		CachedAt:      time.Now(),
		DocumentCount: len(params.Documents),
		Documents:     params.Documents,
		Aliases:       copyAliases(params.Aliases),
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write atomically using temp file + rename pattern
	cachePath := c.getCachePath(source)
	tempPath := cachePath + ".tmp"

	// Write to temp file
	if err := os.WriteFile(tempPath, data, cacheFilePermissions); err != nil {
		return fmt.Errorf("failed to write temp cache file: %w", err)
	}

	// Sync to ensure data is written to disk
	tempFile, err := os.Open(tempPath)
	if err != nil {
		_ = os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to open temp cache file for sync: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to sync temp cache file: %w", err)
	}
	tempFile.Close()

	// Atomically rename temp file to actual cache file
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp cache file: %w", err)
	}

	c.logger.Debug("Cache saved", "source", source, "path", cachePath, "documents", len(params.Documents))
	return nil
}

// Load reads cached documentation from disk
func (c *Cache) Load(source string) (*CachedDocuments, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	cachePath := c.getCachePath(source)

	// Check if cache file exists
	if _, err := os.Stat(cachePath); err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to stat cache file: %w", err)
	}

	// Read cache file
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Unmarshal JSON
	var cached CachedDocuments
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache: %w", err)
	}

	// Validate structure
	if err := validateCachedDocuments(&cached); err != nil {
		return nil, fmt.Errorf("cache validation failed: %w", err)
	}

	c.logger.Debug("Cache loaded", "source", source, "documents", len(cached.Documents), "cached_at", cached.CachedAt)
	return &cached, nil
}

// IsValid checks if a cache exists and is still valid based on age
func (c *Cache) IsValid(source string, maxAge time.Duration) (bool, error) {
	if source == "" {
		return false, fmt.Errorf("source cannot be empty")
	}

	// Try to load cache
	cached, err := c.Load(source)
	if err != nil {
		if os.IsNotExist(err) {
			// Cache file doesn't exist - not an error, just invalid
			return false, nil
		}
		// Cache file exists but is corrupted - return error
		return false, err
	}

	// Check if cache has any documents
	if len(cached.Documents) == 0 {
		return false, nil
	}

	// Check age
	age := time.Since(cached.CachedAt)
	if age > maxAge {
		c.logger.Debug("Cache expired", "source", source, "age", age, "max_age", maxAge)
		return false, nil
	}

	c.logger.Debug("Cache valid", "source", source, "age", age, "max_age", maxAge)
	return true, nil
}

// Clear removes the cache file for a specific source
func (c *Cache) Clear(source string) error {
	if source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	cachePath := c.getCachePath(source)

	// Remove file (ignore if not found - idempotent)
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}

	c.logger.Debug("Cache cleared", "source", source)
	return nil
}

// ClearAll removes the entire cache directory
func (c *Cache) ClearAll() error {
	// Remove entire cache directory
	if err := os.RemoveAll(c.baseDir); err != nil {
		return fmt.Errorf("failed to remove cache directory: %w", err)
	}

	// Recreate empty directory
	if err := c.ensureDir(); err != nil {
		return fmt.Errorf("failed to recreate cache directory: %w", err)
	}

	c.logger.Debug("All caches cleared")
	return nil
}

// validateCachedDocuments validates the structure of a CachedDocuments
func validateCachedDocuments(cached *CachedDocuments) error {
	if cached == nil {
		return fmt.Errorf("cached documents is nil")
	}

	// Check version compatibility. v1.0 caches lack kind/origin/aliases but
	// still unmarshal into this struct and remain safe to import.
	if cached.Version != cacheVersion && cached.Version != "1.0" {
		return fmt.Errorf("cache version mismatch: got %s, expected %s", cached.Version, cacheVersion)
	}

	// Validate document count matches actual count
	if cached.DocumentCount != len(cached.Documents) {
		return fmt.Errorf("document count mismatch: metadata says %d, actual %d", cached.DocumentCount, len(cached.Documents))
	}

	// Ensure documents are not nil
	if cached.Documents == nil {
		return fmt.Errorf("documents list is nil")
	}

	// Check if cached time is not in the future (sanity check)
	if cached.CachedAt.After(time.Now()) {
		return fmt.Errorf("cached timestamp is in the future")
	}

	return nil
}

func copyAliases(aliases map[string]string) map[string]string {
	if len(aliases) == 0 {
		return nil
	}
	copied := make(map[string]string, len(aliases))
	for alias, canonical := range aliases {
		copied[alias] = canonical
	}
	return copied
}
