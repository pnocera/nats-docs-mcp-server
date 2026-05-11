package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultConfig verifies that NewConfig returns a Config with all default values set
func TestDefaultConfig(t *testing.T) {
	cfg := NewConfig()

	// Test server settings
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default LogLevel to be 'info', got '%s'", cfg.LogLevel)
	}

	// Test documentation settings
	if cfg.DocsBaseURL != "https://docs.nats.io" {
		t.Errorf("Expected default DocsBaseURL to be 'https://docs.nats.io', got '%s'", cfg.DocsBaseURL)
	}

	if cfg.FetchTimeout != 30 {
		t.Errorf("Expected default FetchTimeout to be 30, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 5 {
		t.Errorf("Expected default MaxConcurrent to be 5, got %d", cfg.MaxConcurrent)
	}

	if cfg.CacheDir != "" {
		t.Errorf("Expected default CacheDir to be empty, got '%s'", cfg.CacheDir)
	}

	if cfg.NATSSourceType != "site" {
		t.Errorf("Expected default NATSSourceType to be 'site', got '%s'", cfg.NATSSourceType)
	}
	if cfg.NATSArchivePath != "" {
		t.Errorf("Expected default NATSArchivePath to be empty, got '%s'", cfg.NATSArchivePath)
	}
	if cfg.NATSArchiveURL != "" {
		t.Errorf("Expected default NATSArchiveURL to be empty, got '%s'", cfg.NATSArchiveURL)
	}
	if cfg.NATSArchiveBranch != "master" {
		t.Errorf("Expected default NATSArchiveBranch to be 'master', got '%s'", cfg.NATSArchiveBranch)
	}
	if cfg.NATSArchiveIncludeOrphans {
		t.Error("Expected default NATSArchiveIncludeOrphans to be false")
	}
	if cfg.NATSArchiveIncludeLegacy {
		t.Error("Expected default NATSArchiveIncludeLegacy to be false")
	}

	// Test search settings
	if cfg.MaxSearchResults != 50 {
		t.Errorf("Expected default MaxSearchResults to be 50, got %d", cfg.MaxSearchResults)
	}

	// Test transport settings
	if cfg.TransportType != "stdio" {
		t.Errorf("Expected default TransportType to be 'stdio', got '%s'", cfg.TransportType)
	}

	if cfg.Host != "localhost" {
		t.Errorf("Expected default Host to be 'localhost', got '%s'", cfg.Host)
	}

	if cfg.Port != 0 {
		t.Errorf("Expected default Port to be 0, got %d", cfg.Port)
	}
}

// TestConfigStructFields verifies that Config struct has all required fields
func TestConfigStructFields(t *testing.T) {
	cfg := Config{
		LogLevel:         "debug",
		DocsBaseURL:      "https://example.com",
		FetchTimeout:     60,
		MaxConcurrent:    10,
		CacheDir:         "/tmp/cache",
		MaxSearchResults: 100,
	}

	// Verify all fields can be set
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel to be 'debug', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://example.com" {
		t.Errorf("Expected DocsBaseURL to be 'https://example.com', got '%s'", cfg.DocsBaseURL)
	}

	if cfg.FetchTimeout != 60 {
		t.Errorf("Expected FetchTimeout to be 60, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 10 {
		t.Errorf("Expected MaxConcurrent to be 10, got %d", cfg.MaxConcurrent)
	}

	if cfg.CacheDir != "/tmp/cache" {
		t.Errorf("Expected CacheDir to be '/tmp/cache', got '%s'", cfg.CacheDir)
	}

	if cfg.MaxSearchResults != 100 {
		t.Errorf("Expected MaxSearchResults to be 100, got %d", cfg.MaxSearchResults)
	}
}

// TestConfigZeroValues verifies that Config can be created with zero values
func TestConfigZeroValues(t *testing.T) {
	cfg := Config{}

	// Zero values should be valid (empty strings, zero ints)
	if cfg.LogLevel != "" {
		t.Errorf("Expected zero value LogLevel to be empty, got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "" {
		t.Errorf("Expected zero value DocsBaseURL to be empty, got '%s'", cfg.DocsBaseURL)
	}

	if cfg.FetchTimeout != 0 {
		t.Errorf("Expected zero value FetchTimeout to be 0, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 0 {
		t.Errorf("Expected zero value MaxConcurrent to be 0, got %d", cfg.MaxConcurrent)
	}

	if cfg.CacheDir != "" {
		t.Errorf("Expected zero value CacheDir to be empty, got '%s'", cfg.CacheDir)
	}

	if cfg.MaxSearchResults != 0 {
		t.Errorf("Expected zero value MaxSearchResults to be 0, got %d", cfg.MaxSearchResults)
	}

	// Transport fields should also have zero values
	if cfg.TransportType != "" {
		t.Errorf("Expected zero value TransportType to be empty, got '%s'", cfg.TransportType)
	}

	if cfg.Host != "" {
		t.Errorf("Expected zero value Host to be empty, got '%s'", cfg.Host)
	}

	if cfg.Port != 0 {
		t.Errorf("Expected zero value Port to be 0, got %d", cfg.Port)
	}
}

// TestNewConfigReturnsPointer verifies that NewConfig returns a pointer
func TestNewConfigReturnsPointer(t *testing.T) {
	cfg := NewConfig()

	if cfg == nil {
		t.Fatal("Expected NewConfig to return non-nil pointer")
	}

	// Verify we can modify the returned config
	cfg.LogLevel = "debug"
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected to be able to modify returned config")
	}
}

// TestMultipleNewConfigCalls verifies that each call returns a new instance
func TestMultipleNewConfigCalls(t *testing.T) {
	cfg1 := NewConfig()
	cfg2 := NewConfig()

	// Modify first config
	cfg1.LogLevel = "debug"

	// Second config should still have default value
	if cfg2.LogLevel != "info" {
		t.Errorf("Expected cfg2.LogLevel to be 'info', got '%s'", cfg2.LogLevel)
	}

	// Verify they are different instances
	if cfg1 == cfg2 {
		t.Error("Expected NewConfig to return different instances")
	}
}

// TestLoadFromEnvironmentVariables verifies that configuration can be loaded from environment variables
func TestLoadFromEnvironmentVariables(t *testing.T) {
	// Set environment variables
	_ = os.Setenv("LOG_LEVEL", "debug")
	_ = os.Setenv("DOCS_BASE_URL", "https://test.example.com")
	_ = os.Setenv("FETCH_TIMEOUT", "60")
	_ = os.Setenv("MAX_CONCURRENT", "10")
	_ = os.Setenv("CACHE_DIR", "/tmp/test-cache")
	_ = os.Setenv("MAX_SEARCH_RESULTS", "100")
	_ = os.Setenv("NATS_SOURCE_TYPE", "archive")
	_ = os.Setenv("NATS_ARCHIVE_PATH", "assets/nats.docs-master.zip")
	_ = os.Setenv("NATS_ARCHIVE_BRANCH", "main")
	_ = os.Setenv("NATS_ARCHIVE_INCLUDE_ORPHANS", "true")
	_ = os.Setenv("NATS_ARCHIVE_INCLUDE_LEGACY", "1")
	defer func() {
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("DOCS_BASE_URL")
		_ = os.Unsetenv("FETCH_TIMEOUT")
		_ = os.Unsetenv("MAX_CONCURRENT")
		_ = os.Unsetenv("CACHE_DIR")
		_ = os.Unsetenv("MAX_SEARCH_RESULTS")
		_ = os.Unsetenv("NATS_SOURCE_TYPE")
		_ = os.Unsetenv("NATS_ARCHIVE_PATH")
		_ = os.Unsetenv("NATS_ARCHIVE_BRANCH")
		_ = os.Unsetenv("NATS_ARCHIVE_INCLUDE_ORPHANS")
		_ = os.Unsetenv("NATS_ARCHIVE_INCLUDE_LEGACY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected Load to succeed, got error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel to be 'debug', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://test.example.com" {
		t.Errorf("Expected DocsBaseURL to be 'https://test.example.com', got '%s'", cfg.DocsBaseURL)
	}

	if cfg.FetchTimeout != 60 {
		t.Errorf("Expected FetchTimeout to be 60, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 10 {
		t.Errorf("Expected MaxConcurrent to be 10, got %d", cfg.MaxConcurrent)
	}

	if cfg.CacheDir != "/tmp/test-cache" {
		t.Errorf("Expected CacheDir to be '/tmp/test-cache', got '%s'", cfg.CacheDir)
	}

	if cfg.MaxSearchResults != 100 {
		t.Errorf("Expected MaxSearchResults to be 100, got %d", cfg.MaxSearchResults)
	}
	if cfg.NATSSourceType != "archive" || cfg.NATSArchivePath != "assets/nats.docs-master.zip" {
		t.Errorf("Expected archive source config from env, got source=%q path=%q", cfg.NATSSourceType, cfg.NATSArchivePath)
	}
	if cfg.NATSArchiveBranch != "main" {
		t.Errorf("Expected NATSArchiveBranch to be main, got %q", cfg.NATSArchiveBranch)
	}
	if !cfg.NATSArchiveIncludeOrphans || !cfg.NATSArchiveIncludeLegacy {
		t.Error("Expected archive include flags from env to be true")
	}
}

// TestLoadFromConfigFile verifies that configuration can be loaded from a YAML file
func TestLoadFromConfigFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
log_level: warn
docs_base_url: https://config.example.com
fetch_timeout: 45
max_concurrent: 8
cache_dir: /tmp/config-cache
max_search_results: 75
nats:
  source_type: archive
  archive_path: assets/nats.docs-master.zip
  archive_branch: main
  archive_include_orphans: true
  archive_include_legacy: true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Expected LoadFromFile to succeed, got error: %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Errorf("Expected LogLevel to be 'warn', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://config.example.com" {
		t.Errorf("Expected DocsBaseURL to be 'https://config.example.com', got '%s'", cfg.DocsBaseURL)
	}

	if cfg.FetchTimeout != 45 {
		t.Errorf("Expected FetchTimeout to be 45, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 8 {
		t.Errorf("Expected MaxConcurrent to be 8, got %d", cfg.MaxConcurrent)
	}

	if cfg.CacheDir != "/tmp/config-cache" {
		t.Errorf("Expected CacheDir to be '/tmp/config-cache', got '%s'", cfg.CacheDir)
	}

	if cfg.MaxSearchResults != 75 {
		t.Errorf("Expected MaxSearchResults to be 75, got %d", cfg.MaxSearchResults)
	}
	if cfg.NATSSourceType != "archive" {
		t.Errorf("Expected NATSSourceType to be archive, got %q", cfg.NATSSourceType)
	}
	if cfg.NATSArchivePath != "assets/nats.docs-master.zip" {
		t.Errorf("Expected NATSArchivePath from config file, got %q", cfg.NATSArchivePath)
	}
	if cfg.NATSArchiveBranch != "main" {
		t.Errorf("Expected NATSArchiveBranch to be main, got %q", cfg.NATSArchiveBranch)
	}
	if !cfg.NATSArchiveIncludeOrphans || !cfg.NATSArchiveIncludeLegacy {
		t.Error("Expected archive include flags from config file to be true")
	}
}

// TestLoadWithDefaults verifies that Load returns defaults when no config is provided
func TestLoadWithDefaults(t *testing.T) {
	// Clear any environment variables that might interfere
	_ = os.Unsetenv("LOG_LEVEL")
	_ = os.Unsetenv("DOCS_BASE_URL")
	_ = os.Unsetenv("FETCH_TIMEOUT")
	_ = os.Unsetenv("MAX_CONCURRENT")
	_ = os.Unsetenv("CACHE_DIR")
	_ = os.Unsetenv("MAX_SEARCH_RESULTS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected Load to succeed, got error: %v", err)
	}

	// Should have default values
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default LogLevel to be 'info', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://docs.nats.io" {
		t.Errorf("Expected default DocsBaseURL to be 'https://docs.nats.io', got '%s'", cfg.DocsBaseURL)
	}

	if cfg.FetchTimeout != 30 {
		t.Errorf("Expected default FetchTimeout to be 30, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 5 {
		t.Errorf("Expected default MaxConcurrent to be 5, got %d", cfg.MaxConcurrent)
	}

	if cfg.CacheDir != "" {
		t.Errorf("Expected default CacheDir to be empty, got '%s'", cfg.CacheDir)
	}

	if cfg.MaxSearchResults != 50 {
		t.Errorf("Expected default MaxSearchResults to be 50, got %d", cfg.MaxSearchResults)
	}
}

// TestConfigPrecedence verifies that config file takes precedence over environment variables
func TestConfigPrecedence(t *testing.T) {
	// Set environment variables
	_ = os.Setenv("LOG_LEVEL", "debug")
	_ = os.Setenv("DOCS_BASE_URL", "https://env.example.com")
	defer func() {
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("DOCS_BASE_URL")
	}()

	// Create a config file with different values
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
log_level: error
docs_base_url: https://file.example.com
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Expected LoadFromFile to succeed, got error: %v", err)
	}

	// Config file should take precedence
	if cfg.LogLevel != "error" {
		t.Errorf("Expected LogLevel from config file to be 'error', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://file.example.com" {
		t.Errorf("Expected DocsBaseURL from config file to be 'https://file.example.com', got '%s'", cfg.DocsBaseURL)
	}
}

// TestLoadWithFlags verifies that command-line flags have highest precedence
func TestLoadWithFlags(t *testing.T) {
	// Set environment variables
	_ = os.Setenv("LOG_LEVEL", "debug")
	_ = os.Setenv("DOCS_BASE_URL", "https://env.example.com")
	defer func() {
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("DOCS_BASE_URL")
	}()

	// Create a config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
log_level: warn
docs_base_url: https://file.example.com
fetch_timeout: 45
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Provide flags (should have highest precedence)
	flags := map[string]interface{}{
		"log_level":      "error",
		"docs_base_url":  "https://flag.example.com",
		"max_concurrent": 20,
	}

	cfg, err := LoadWithFlags(configFile, flags)
	if err != nil {
		t.Fatalf("Expected LoadWithFlags to succeed, got error: %v", err)
	}

	// Flags should override everything
	if cfg.LogLevel != "error" {
		t.Errorf("Expected LogLevel from flags to be 'error', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://flag.example.com" {
		t.Errorf("Expected DocsBaseURL from flags to be 'https://flag.example.com', got '%s'", cfg.DocsBaseURL)
	}

	if cfg.MaxConcurrent != 20 {
		t.Errorf("Expected MaxConcurrent from flags to be 20, got %d", cfg.MaxConcurrent)
	}

	// Config file should override env for values not in flags
	if cfg.FetchTimeout != 45 {
		t.Errorf("Expected FetchTimeout from config file to be 45, got %d", cfg.FetchTimeout)
	}
}

// TestLoadWithFlagsNoConfigFile verifies LoadWithFlags works without a config file
func TestLoadWithFlagsNoConfigFile(t *testing.T) {
	// Clear environment variables
	_ = os.Unsetenv("LOG_LEVEL")
	_ = os.Unsetenv("DOCS_BASE_URL")

	flags := map[string]interface{}{
		"log_level":     "debug",
		"docs_base_url": "https://flag.example.com",
	}

	cfg, err := LoadWithFlags("", flags)
	if err != nil {
		t.Fatalf("Expected LoadWithFlags to succeed without config file, got error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel from flags to be 'debug', got '%s'", cfg.LogLevel)
	}

	if cfg.DocsBaseURL != "https://flag.example.com" {
		t.Errorf("Expected DocsBaseURL from flags to be 'https://flag.example.com', got '%s'", cfg.DocsBaseURL)
	}

	// Should have defaults for other values
	if cfg.FetchTimeout != 30 {
		t.Errorf("Expected default FetchTimeout to be 30, got %d", cfg.FetchTimeout)
	}
}

// TestLoadFromFileInvalidPath verifies error handling for invalid config file path
func TestLoadFromFileInvalidPath(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected LoadFromFile to return error for invalid path")
	}
}

// TestLoadFromFileEmptyValues verifies that empty values in config file are handled
func TestLoadFromFileEmptyValues(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
cache_dir: ""
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Expected LoadFromFile to succeed, got error: %v", err)
	}

	// Empty string values should be set
	if cfg.CacheDir != "" {
		t.Errorf("Expected CacheDir to be empty string, got '%s'", cfg.CacheDir)
	}

	// Other values should have defaults
	if cfg.DocsBaseURL != "https://docs.nats.io" {
		t.Errorf("Expected default DocsBaseURL, got '%s'", cfg.DocsBaseURL)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("Expected default LogLevel, got '%s'", cfg.LogLevel)
	}
}

// TestLoadWithNilFlags verifies that nil flags don't cause issues
func TestLoadWithNilFlags(t *testing.T) {
	flags := map[string]interface{}{
		"log_level":     nil,
		"docs_base_url": "https://flag.example.com",
	}

	cfg, err := LoadWithFlags("", flags)
	if err != nil {
		t.Fatalf("Expected LoadWithFlags to succeed with nil flag values, got error: %v", err)
	}

	// Nil flag should not override default
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default LogLevel to be 'info', got '%s'", cfg.LogLevel)
	}

	// Non-nil flag should be set
	if cfg.DocsBaseURL != "https://flag.example.com" {
		t.Errorf("Expected DocsBaseURL from flags to be 'https://flag.example.com', got '%s'", cfg.DocsBaseURL)
	}
}

// TestLoadFromEnvWithInvalidIntegers verifies that invalid integer env vars are ignored
func TestLoadFromEnvWithInvalidIntegers(t *testing.T) {
	_ = os.Setenv("FETCH_TIMEOUT", "not-a-number")
	_ = os.Setenv("MAX_CONCURRENT", "invalid")
	defer func() {
		_ = os.Unsetenv("FETCH_TIMEOUT")
		_ = os.Unsetenv("MAX_CONCURRENT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected Load to succeed, got error: %v", err)
	}

	// Should fall back to defaults for invalid integers
	if cfg.FetchTimeout != 30 {
		t.Errorf("Expected default FetchTimeout for invalid env var, got %d", cfg.FetchTimeout)
	}

	if cfg.MaxConcurrent != 5 {
		t.Errorf("Expected default MaxConcurrent for invalid env var, got %d", cfg.MaxConcurrent)
	}
}

// TestValidateInvalidLogLevel verifies that invalid log levels are rejected
func TestValidateInvalidLogLevel(t *testing.T) {
	cfg := NewConfig()
	cfg.LogLevel = "invalid"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for invalid log level")
	}

	if err != nil && !strings.Contains(err.Error(), "invalid log level: invalid") {
		t.Errorf("Expected error message to contain 'invalid log level: invalid', got: %v", err)
	}
}

// TestValidateValidLogLevels verifies that valid log levels are accepted
func TestValidateValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		cfg := NewConfig()
		cfg.LogLevel = level

		err := cfg.Validate()
		if err != nil {
			t.Errorf("Expected Validate to accept log level '%s', got error: %v", level, err)
		}
	}
}

// TestValidateNegativeTimeout verifies that negative timeouts are rejected
func TestValidateNegativeTimeout(t *testing.T) {
	cfg := NewConfig()
	cfg.FetchTimeout = -1

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for negative timeout")
	}

	if err != nil && !strings.Contains(err.Error(), "fetch_timeout must be positive") {
		t.Errorf("Expected error message to contain 'fetch_timeout must be positive', got: %v", err)
	}
}

// TestValidateZeroTimeout verifies that zero timeout is rejected
func TestValidateZeroTimeout(t *testing.T) {
	cfg := NewConfig()
	cfg.FetchTimeout = 0

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for zero timeout")
	}

	if err != nil && !strings.Contains(err.Error(), "fetch_timeout must be positive") {
		t.Errorf("Expected error message to contain 'fetch_timeout must be positive', got: %v", err)
	}
}

// TestValidateNegativeMaxConcurrent verifies that negative max concurrent is rejected
func TestValidateNegativeMaxConcurrent(t *testing.T) {
	cfg := NewConfig()
	cfg.MaxConcurrent = -1

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for negative max concurrent")
	}

	if err != nil && !strings.Contains(err.Error(), "max_concurrent must be positive") {
		t.Errorf("Expected error message to contain 'max_concurrent must be positive', got: %v", err)
	}
}

// TestValidateZeroMaxConcurrent verifies that zero max concurrent is rejected
func TestValidateZeroMaxConcurrent(t *testing.T) {
	cfg := NewConfig()
	cfg.MaxConcurrent = 0

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for zero max concurrent")
	}

	if err != nil && !strings.Contains(err.Error(), "max_concurrent must be positive") {
		t.Errorf("Expected error message to contain 'max_concurrent must be positive', got: %v", err)
	}
}

// TestValidateNegativeMaxSearchResults verifies that negative max search results is rejected
func TestValidateNegativeMaxSearchResults(t *testing.T) {
	cfg := NewConfig()
	cfg.MaxSearchResults = -1

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for negative max search results")
	}

	if err != nil && !strings.Contains(err.Error(), "max_search_results must be positive") {
		t.Errorf("Expected error message to contain 'max_search_results must be positive', got: %v", err)
	}
}

// TestValidateZeroMaxSearchResults verifies that zero max search results is rejected
func TestValidateZeroMaxSearchResults(t *testing.T) {
	cfg := NewConfig()
	cfg.MaxSearchResults = 0

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for zero max search results")
	}

	if err != nil && !strings.Contains(err.Error(), "max_search_results must be positive") {
		t.Errorf("Expected error message to contain 'max_search_results must be positive', got: %v", err)
	}
}

// TestValidateInvalidURL verifies that invalid URLs are rejected
func TestValidateInvalidURL(t *testing.T) {
	invalidURLs := []string{
		"not-a-url",
		"ftp://invalid-scheme.com",
		"://missing-scheme.com",
		"http://",
		"",
	}

	for _, url := range invalidURLs {
		cfg := NewConfig()
		cfg.DocsBaseURL = url

		err := cfg.Validate()
		if err == nil {
			t.Errorf("Expected Validate to return error for invalid URL: %s", url)
		}
	}
}

// TestValidateValidURLs verifies that valid URLs are accepted
func TestValidateValidURLs(t *testing.T) {
	validURLs := []string{
		"https://docs.nats.io",
		"http://localhost:8080",
		"https://example.com/path",
		"http://192.168.1.1",
	}

	for _, url := range validURLs {
		cfg := NewConfig()
		cfg.DocsBaseURL = url

		err := cfg.Validate()
		if err != nil {
			t.Errorf("Expected Validate to accept valid URL '%s', got error: %v", url, err)
		}
	}
}

// TestValidateValidConfig verifies that a valid config passes validation
func TestValidateValidConfig(t *testing.T) {
	cfg := NewConfig()

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected default config to be valid, got error: %v", err)
	}
}

// TestValidateMultipleErrors verifies that multiple validation errors are reported
func TestValidateMultipleErrors(t *testing.T) {
	cfg := NewConfig()
	cfg.LogLevel = "invalid"
	cfg.FetchTimeout = -1
	cfg.MaxConcurrent = 0
	cfg.DocsBaseURL = "not-a-url"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected Validate to return error for multiple invalid values")
	}

	// Error message should mention multiple issues
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Expected non-empty error message")
	}
}

// TestLoadValidatesConfiguration verifies that Load validates the configuration
func TestLoadValidatesConfiguration(t *testing.T) {
	_ = os.Setenv("LOG_LEVEL", "invalid")
	defer func() { _ = os.Unsetenv("LOG_LEVEL") }()

	_, err := Load()
	if err == nil {
		t.Error("Expected Load to return validation error for invalid log level")
	}
}

func TestNewConfig_NATSSourceDefaults(t *testing.T) {
	cfg := NewConfig()
	if cfg.NATSSourceType != "site" {
		t.Fatalf("expected site default, got %q", cfg.NATSSourceType)
	}
	if cfg.NATSArchivePath != "" || cfg.NATSArchiveURL != "" {
		t.Fatalf("expected empty archive path/url, got path=%q url=%q", cfg.NATSArchivePath, cfg.NATSArchiveURL)
	}
	if cfg.NATSArchiveBranch != "master" {
		t.Fatalf("expected branch master, got %q", cfg.NATSArchiveBranch)
	}
	if cfg.NATSArchiveIncludeOrphans || cfg.NATSArchiveIncludeLegacy {
		t.Fatal("expected archive include flags false by default")
	}
}

func TestValidate_InvalidSourceType(t *testing.T) {
	cfg := NewConfig()
	cfg.NATSSourceType = "foo"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid nats.source_type") {
		t.Fatalf("expected invalid source type error, got %v", err)
	}
}

func TestValidate_ArchiveSourceMissingPath(t *testing.T) {
	cfg := NewConfig()
	cfg.NATSSourceType = "archive"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "archive_path") {
		t.Fatalf("expected missing archive path error, got %v", err)
	}
}

func TestValidate_ArchiveSourceRejectsURL(t *testing.T) {
	cfg := NewConfig()
	cfg.NATSSourceType = "archive"
	cfg.NATSArchivePath = "assets/nats.docs-master.zip"
	cfg.NATSArchiveURL = "https://example.com/archive.zip"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved archive URL error, got %v", err)
	}
}

func TestValidate_ArchiveSourcePathOnly(t *testing.T) {
	cfg := NewConfig()
	cfg.NATSSourceType = "archive"
	cfg.NATSArchivePath = "assets/nats.docs-master.zip"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected archive path-only config to validate, got %v", err)
	}
}

func TestValidate_SiteSourceIgnoresArchiveFields(t *testing.T) {
	cfg := NewConfig()
	cfg.NATSSourceType = "site"
	cfg.NATSArchiveURL = "https://example.com/archive.zip"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected site config to ignore archive URL, got %v", err)
	}
}

func TestLoadWithFlags_NATSArchiveFlags(t *testing.T) {
	flags := map[string]interface{}{
		"nats_source_type":             "archive",
		"nats_archive_path":            "assets/nats.docs-master.zip",
		"nats_archive_branch":          "main",
		"nats_archive_include_orphans": true,
		"nats_archive_include_legacy":  true,
	}
	cfg, err := LoadWithFlags("", flags)
	if err != nil {
		t.Fatalf("LoadWithFlags returned error: %v", err)
	}
	if cfg.NATSSourceType != "archive" || cfg.NATSArchivePath != "assets/nats.docs-master.zip" {
		t.Fatalf("unexpected archive flag config: %+v", cfg)
	}
	if cfg.NATSArchiveBranch != "main" || !cfg.NATSArchiveIncludeOrphans || !cfg.NATSArchiveIncludeLegacy {
		t.Fatalf("archive flag details not loaded: %+v", cfg)
	}
}

// TestLoadFromFileValidatesConfiguration verifies that LoadFromFile validates the configuration
func TestLoadFromFileValidatesConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
log_level: invalid
fetch_timeout: -1
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	_, err := LoadFromFile(configFile)
	if err == nil {
		t.Error("Expected LoadFromFile to return validation error")
	}
}

// TestLoadWithFlagsValidatesConfiguration verifies that LoadWithFlags validates the configuration
func TestLoadWithFlagsValidatesConfiguration(t *testing.T) {
	flags := map[string]interface{}{
		"log_level":      "invalid",
		"fetch_timeout":  -1,
		"max_concurrent": 0,
	}

	_, err := LoadWithFlags("", flags)
	if err == nil {
		t.Error("Expected LoadWithFlags to return validation error")
	}
}

// TestTransportFieldsExist verifies that Config struct has transport fields
func TestTransportFieldsExist(t *testing.T) {
	cfg := Config{
		LogLevel:         "info",
		DocsBaseURL:      "https://docs.nats.io",
		FetchTimeout:     30,
		MaxConcurrent:    5,
		CacheDir:         "",
		MaxSearchResults: 50,
		TransportType:    "stdio",
		Host:             "localhost",
		Port:             8080,
	}

	// Verify transport fields can be set
	if cfg.TransportType != "stdio" {
		t.Errorf("Expected TransportType to be 'stdio', got '%s'", cfg.TransportType)
	}

	if cfg.Host != "localhost" {
		t.Errorf("Expected Host to be 'localhost', got '%s'", cfg.Host)
	}

	if cfg.Port != 8080 {
		t.Errorf("Expected Port to be 8080, got %d", cfg.Port)
	}
}

// TestDefaultTransportValues verifies that NewConfig returns correct transport defaults
func TestDefaultTransportValues(t *testing.T) {
	cfg := NewConfig()

	// Test transport defaults
	if cfg.TransportType != "stdio" {
		t.Errorf("Expected default TransportType to be 'stdio', got '%s'", cfg.TransportType)
	}

	if cfg.Host != "localhost" {
		t.Errorf("Expected default Host to be 'localhost', got '%s'", cfg.Host)
	}

	if cfg.Port != 0 {
		t.Errorf("Expected default Port to be 0, got %d", cfg.Port)
	}
}
