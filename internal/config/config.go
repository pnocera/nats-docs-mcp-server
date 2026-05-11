// Package config provides configuration management for the NATS Documentation MCP Server.
// It supports loading configuration from multiple sources: command-line flags, config files,
// and environment variables, with proper precedence handling.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/classifier"
	"github.com/spf13/viper"
)

// Config holds all configuration settings for the NATS Documentation MCP Server.
// It includes server settings, documentation fetching settings, and search settings.
type Config struct {
	// Server settings
	LogLevel string // Log level: debug, info, warn, error (default: info)

	// Documentation settings
	DocsBaseURL   string // Base URL for NATS documentation (default: https://docs.nats.io)
	FetchTimeout  int    // Timeout for fetching documentation in seconds (default: 30)
	MaxConcurrent int    // Maximum concurrent fetches (default: 5)
	CacheDir      string // Directory for caching fetched documentation (default: ~/.cache/nats-mcp)
	CacheMaxAge   int    // Maximum age of cache in days before auto-refresh (default: 7)
	RefreshCache  bool   // Force refresh cache on startup (default: false)

	// NATS documentation source settings
	NATSSourceType            string // Source type: site or archive (default: archive)
	NATSArchivePath           string // Local path to nats.docs zip when source type is archive
	NATSArchiveURL            string // Reserved for future remote archive download
	NATSArchiveBranch         string // Branch/revision label for archive metadata (default: master)
	NATSArchiveIncludeOrphans bool   // Include Markdown files not linked from SUMMARY.md
	NATSArchiveIncludeLegacy  bool   // Include legacy/ Markdown targets

	// Search settings
	MaxSearchResults int // Maximum number of search results to return (default: 50)

	// Transport settings
	TransportType string // Transport type: stdio, sse, streamablehttp (default: stdio)
	Host          string // Host to bind for network transports (default: localhost)
	Port          int    // Port to bind for network transports (default: 0)

	// Synadia documentation settings
	SynadiaEnabled      bool   // Enable Synadia documentation support (default: false)
	SynadiaBaseURL      string // Base URL for Synadia documentation (default: https://docs.synadia.com)
	SynadiaFetchTimeout int    // Timeout for fetching Synadia documentation in seconds (default: 30)

	// GitHub documentation settings
	GitHubEnabled      bool     // Enable GitHub documentation support (default: false)
	GitHubToken        string   // GitHub Personal Access Token for authentication
	GitHubRepositories []string // GitHub repositories to index (default: nats-io/nats-server, nats-io/nats.docs, nats-io/nats)
	GitHubBranch       string   // Default branch to fetch from (default: main)
	GitHubFetchTimeout int      // Timeout for fetching GitHub documentation in seconds (default: 30)

	// Classification keywords
	SynadiaKeywords []string // Keywords that classify queries as Synadia-specific
	NATSKeywords    []string // Keywords that classify queries as NATS-specific
	GitHubKeywords  []string // Keywords that classify queries as GitHub-specific
}

// NewConfig creates a new Config with default values for all optional parameters.
// This ensures that the server can run with sensible defaults without requiring
// explicit configuration.
func NewConfig() *Config {
	return &Config{
		// Server defaults
		LogLevel: "info",

		// Documentation defaults
		DocsBaseURL:   "https://docs.nats.io",
		FetchTimeout:  30,
		MaxConcurrent: 5,
		CacheDir:      "",
		CacheMaxAge:   7,
		RefreshCache:  false,

		// NATS source defaults
		NATSSourceType:            "archive",
		NATSArchivePath:           "assets/nats.docs-master.zip",
		NATSArchiveURL:            "",
		NATSArchiveBranch:         "master",
		NATSArchiveIncludeOrphans: false,
		NATSArchiveIncludeLegacy:  false,

		// Search defaults
		MaxSearchResults: 50,

		// Transport defaults
		TransportType: "stdio",
		Host:          "localhost",
		Port:          0,

		// Synadia defaults
		SynadiaEnabled:      false, // Disabled by default for backward compatibility
		SynadiaBaseURL:      "https://docs.synadia.com",
		SynadiaFetchTimeout: 30,

		// GitHub defaults
		GitHubEnabled: false, // Disabled by default
		GitHubToken:   "",
		GitHubRepositories: []string{
			"nats-io/nats-server",
			"nats-io/nats.docs",
			"nats-io/nats",
		},
		GitHubBranch:       "main",
		GitHubFetchTimeout: 30,

		// Classification keyword defaults
		SynadiaKeywords: classifier.DefaultSyadiaKeywords(),
		NATSKeywords:    classifier.DefaultNATSKeywords(),
		GitHubKeywords:  classifier.DefaultGitHubKeywords(),
	}
}

// Load loads configuration from environment variables with defaults.
// Environment variables should be prefixed with the application name.
// Returns a Config with values from environment variables or defaults.
func Load() (*Config, error) {
	// Start with defaults
	cfg := NewConfig()

	// Load environment variables (override defaults)
	loadFromEnv(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFromFile loads configuration from a YAML file, with environment variables
// as fallback, and defaults as final fallback.
// The precedence order is: config file > environment variables > defaults.
func LoadFromFile(configPath string) (*Config, error) {
	// Start with defaults
	cfg := NewConfig()

	// Load environment variables (override defaults)
	loadFromEnv(cfg)

	// Load config file (override env vars and defaults)
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Override with config file values (only if they exist in the file)
	if v.IsSet("log_level") {
		cfg.LogLevel = v.GetString("log_level")
	}
	if v.IsSet("docs_base_url") {
		cfg.DocsBaseURL = v.GetString("docs_base_url")
	}
	if v.IsSet("fetch_timeout") {
		cfg.FetchTimeout = v.GetInt("fetch_timeout")
	}
	if v.IsSet("max_concurrent") {
		cfg.MaxConcurrent = v.GetInt("max_concurrent")
	}
	if v.IsSet("cache_dir") {
		cfg.CacheDir = v.GetString("cache_dir")
	}
	if v.IsSet("max_search_results") {
		cfg.MaxSearchResults = v.GetInt("max_search_results")
	}
	loadNATSArchiveFromViper(cfg, v)
	// Transport settings
	if v.IsSet("transport_type") {
		cfg.TransportType = v.GetString("transport_type")
	}
	if v.IsSet("host") {
		cfg.Host = v.GetString("host")
	}
	if v.IsSet("port") {
		cfg.Port = v.GetInt("port")
	}

	// Synadia settings
	if v.IsSet("synadia.enabled") {
		cfg.SynadiaEnabled = v.GetBool("synadia.enabled")
	}
	if v.IsSet("synadia.base_url") {
		cfg.SynadiaBaseURL = v.GetString("synadia.base_url")
	}
	if v.IsSet("synadia.fetch_timeout") {
		cfg.SynadiaFetchTimeout = v.GetInt("synadia.fetch_timeout")
	}
	if v.IsSet("classification.synadia_keywords") {
		cfg.SynadiaKeywords = v.GetStringSlice("classification.synadia_keywords")
	}
	if v.IsSet("classification.nats_keywords") {
		cfg.NATSKeywords = v.GetStringSlice("classification.nats_keywords")
	}
	if v.IsSet("classification.github_keywords") {
		cfg.GitHubKeywords = v.GetStringSlice("classification.github_keywords")
	}

	// GitHub settings
	if v.IsSet("github.enabled") {
		cfg.GitHubEnabled = v.GetBool("github.enabled")
	}
	if v.IsSet("github.token") {
		cfg.GitHubToken = v.GetString("github.token")
	}
	if v.IsSet("github.repositories") {
		cfg.GitHubRepositories = v.GetStringSlice("github.repositories")
	}
	if v.IsSet("github.default_branch") {
		cfg.GitHubBranch = v.GetString("github.default_branch")
	}
	if v.IsSet("github.fetch_timeout") {
		cfg.GitHubFetchTimeout = v.GetInt("github.fetch_timeout")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadWithFlags loads configuration from command-line flags, config file,
// environment variables, and defaults.
// The precedence order is: flags > config file > environment variables > defaults.
func LoadWithFlags(configPath string, flags map[string]interface{}) (*Config, error) {
	// Start with defaults
	cfg := NewConfig()

	// Load environment variables (override defaults)
	loadFromEnv(cfg)

	// Load config file if provided (override env vars)
	if configPath != "" {
		v := viper.New()
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		// Override with config file values
		if v.IsSet("log_level") {
			cfg.LogLevel = v.GetString("log_level")
		}
		if v.IsSet("docs_base_url") {
			cfg.DocsBaseURL = v.GetString("docs_base_url")
		}
		if v.IsSet("fetch_timeout") {
			cfg.FetchTimeout = v.GetInt("fetch_timeout")
		}
		if v.IsSet("max_concurrent") {
			cfg.MaxConcurrent = v.GetInt("max_concurrent")
		}
		if v.IsSet("cache_dir") {
			cfg.CacheDir = v.GetString("cache_dir")
		}
		if v.IsSet("max_search_results") {
			cfg.MaxSearchResults = v.GetInt("max_search_results")
		}
		loadNATSArchiveFromViper(cfg, v)
		// Transport settings
		if v.IsSet("transport_type") {
			cfg.TransportType = v.GetString("transport_type")
		}
		if v.IsSet("host") {
			cfg.Host = v.GetString("host")
		}
		if v.IsSet("port") {
			cfg.Port = v.GetInt("port")
		}
		// Synadia settings
		if v.IsSet("synadia.enabled") {
			cfg.SynadiaEnabled = v.GetBool("synadia.enabled")
		}
		if v.IsSet("synadia.base_url") {
			cfg.SynadiaBaseURL = v.GetString("synadia.base_url")
		}
		if v.IsSet("synadia.fetch_timeout") {
			cfg.SynadiaFetchTimeout = v.GetInt("synadia.fetch_timeout")
		}
		if v.IsSet("classification.synadia_keywords") {
			cfg.SynadiaKeywords = v.GetStringSlice("classification.synadia_keywords")
		}
		if v.IsSet("classification.nats_keywords") {
			cfg.NATSKeywords = v.GetStringSlice("classification.nats_keywords")
		}
	}

	// Override with flags (highest precedence)
	if val, ok := flags["log_level"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.LogLevel = strVal
		}
	}
	if val, ok := flags["docs_base_url"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.DocsBaseURL = strVal
		}
	}
	if val, ok := flags["fetch_timeout"]; ok && val != nil {
		if intVal, ok := val.(int); ok {
			cfg.FetchTimeout = intVal
		}
	}
	if val, ok := flags["max_concurrent"]; ok && val != nil {
		if intVal, ok := val.(int); ok {
			cfg.MaxConcurrent = intVal
		}
	}
	if val, ok := flags["cache_dir"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.CacheDir = strVal
		}
	}
	if val, ok := flags["max_search_results"]; ok && val != nil {
		if intVal, ok := val.(int); ok {
			cfg.MaxSearchResults = intVal
		}
	}
	if val, ok := flags["nats_source_type"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.NATSSourceType = strVal
		}
	}
	if val, ok := flags["nats_archive_path"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.NATSArchivePath = strVal
		}
	}
	if val, ok := flags["nats_archive_url"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.NATSArchiveURL = strVal
		}
	}
	if val, ok := flags["nats_archive_branch"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.NATSArchiveBranch = strVal
		}
	}
	if val, ok := flags["nats_archive_include_orphans"]; ok && val != nil {
		if boolVal, ok := val.(bool); ok {
			cfg.NATSArchiveIncludeOrphans = boolVal
		}
	}
	if val, ok := flags["nats_archive_include_legacy"]; ok && val != nil {
		if boolVal, ok := val.(bool); ok {
			cfg.NATSArchiveIncludeLegacy = boolVal
		}
	}
	// Transport settings
	if val, ok := flags["transport_type"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.TransportType = strVal
		}
	}
	if val, ok := flags["host"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.Host = strVal
		}
	}
	if val, ok := flags["port"]; ok && val != nil {
		if intVal, ok := val.(int); ok {
			cfg.Port = intVal
		}
	}
	// Synadia settings
	if val, ok := flags["synadia_enabled"]; ok && val != nil {
		if boolVal, ok := val.(bool); ok {
			cfg.SynadiaEnabled = boolVal
		}
	}
	if val, ok := flags["synadia_base_url"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			cfg.SynadiaBaseURL = strVal
		}
	}
	if val, ok := flags["synadia_fetch_timeout"]; ok && val != nil {
		if intVal, ok := val.(int); ok {
			cfg.SynadiaFetchTimeout = intVal
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadFromEnv loads configuration from environment variables into the provided Config
// loadFromEnv loads configuration from environment variables with NATS_DOCS_ prefix.
// This implements 12-factor app principles (III. Store config in environment).
// Supports both NATS_DOCS_ prefixed and bare names for backward compatibility.
func loadFromEnv(cfg *Config) {
	// Helper function to get env var with prefix, falling back to bare name
	getEnv := func(name string) string {
		// Try with NATS_DOCS_ prefix first (12-factor standard)
		if val := os.Getenv("NATS_DOCS_" + name); val != "" {
			return val
		}
		// Fall back to bare name for backward compatibility
		return os.Getenv(name)
	}

	// Server settings
	if val := getEnv("LOG_LEVEL"); val != "" {
		cfg.LogLevel = val
	}

	// Documentation settings
	if val := getEnv("DOCS_BASE_URL"); val != "" {
		cfg.DocsBaseURL = val
	}
	if val := getEnv("FETCH_TIMEOUT"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.FetchTimeout = intVal
		}
	}
	if val := getEnv("MAX_CONCURRENT"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.MaxConcurrent = intVal
		}
	}
	if val := getEnv("CACHE_DIR"); val != "" {
		cfg.CacheDir = val
	}
	if val := getEnv("CACHE_MAX_AGE_DAYS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.CacheMaxAge = intVal
		}
	}
	if val := getEnv("MAX_SEARCH_RESULTS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.MaxSearchResults = intVal
		}
	}
	if val := getEnv("NATS_SOURCE_TYPE"); val != "" {
		cfg.NATSSourceType = val
	}
	if val := getEnv("NATS_ARCHIVE_PATH"); val != "" {
		cfg.NATSArchivePath = val
	}
	if val := getEnv("NATS_ARCHIVE_URL"); val != "" {
		cfg.NATSArchiveURL = val
	}
	if val := getEnv("NATS_ARCHIVE_BRANCH"); val != "" {
		cfg.NATSArchiveBranch = val
	}
	if val := getEnv("NATS_ARCHIVE_INCLUDE_ORPHANS"); val != "" {
		cfg.NATSArchiveIncludeOrphans = parseBoolEnv(val)
	}
	if val := getEnv("NATS_ARCHIVE_INCLUDE_LEGACY"); val != "" {
		cfg.NATSArchiveIncludeLegacy = parseBoolEnv(val)
	}

	// Transport settings
	if val := getEnv("TRANSPORT_TYPE"); val != "" {
		cfg.TransportType = val
	}
	if val := getEnv("HOST"); val != "" {
		cfg.Host = val
	}
	if val := getEnv("PORT"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.Port = intVal
		}
	}

	// Synadia settings
	if val := getEnv("SYNADIA_ENABLED"); val != "" {
		cfg.SynadiaEnabled = val == "true" || val == "1" || val == "yes"
	}
	if val := getEnv("SYNADIA_BASE_URL"); val != "" {
		cfg.SynadiaBaseURL = val
	}
	if val := getEnv("SYNADIA_FETCH_TIMEOUT"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.SynadiaFetchTimeout = intVal
		}
	}

	// GitHub settings
	if val := getEnv("GITHUB_ENABLED"); val != "" {
		cfg.GitHubEnabled = val == "true" || val == "1" || val == "yes"
	}
	if val := getEnv("GITHUB_TOKEN"); val != "" {
		cfg.GitHubToken = val
	}
	if val := getEnv("GITHUB_REPOSITORIES"); val != "" {
		cfg.GitHubRepositories = strings.Split(val, ",")
		// Trim whitespace from each repo
		for i := range cfg.GitHubRepositories {
			cfg.GitHubRepositories[i] = strings.TrimSpace(cfg.GitHubRepositories[i])
		}
	}
	if val := getEnv("GITHUB_BRANCH"); val != "" {
		cfg.GitHubBranch = val
	}
	if val := getEnv("GITHUB_FETCH_TIMEOUT"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			cfg.GitHubFetchTimeout = intVal
		}
	}

	// Classification keywords - comma-separated lists
	if val := getEnv("SYNADIA_KEYWORDS"); val != "" {
		cfg.SynadiaKeywords = strings.Split(val, ",")
		// Trim whitespace from each keyword
		for i := range cfg.SynadiaKeywords {
			cfg.SynadiaKeywords[i] = strings.TrimSpace(cfg.SynadiaKeywords[i])
		}
	}
	if val := getEnv("NATS_KEYWORDS"); val != "" {
		cfg.NATSKeywords = strings.Split(val, ",")
		// Trim whitespace from each keyword
		for i := range cfg.NATSKeywords {
			cfg.NATSKeywords[i] = strings.TrimSpace(cfg.NATSKeywords[i])
		}
	}
	if val := getEnv("GITHUB_KEYWORDS"); val != "" {
		cfg.GitHubKeywords = strings.Split(val, ",")
		// Trim whitespace from each keyword
		for i := range cfg.GitHubKeywords {
			cfg.GitHubKeywords[i] = strings.TrimSpace(cfg.GitHubKeywords[i])
		}
	}
}

// NormalizeEnvKey converts environment variable names to viper keys
// Example: LOG_LEVEL -> log_level
func NormalizeEnvKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(key, "_", "_"))
}

// GetTransportAddress returns the network address for network transports.
// For STDIO transport, it returns an empty string.
// For SSE and StreamableHTTP transports, it returns "host:port".
func (c *Config) GetTransportAddress() string {
	// STDIO transport doesn't use network address
	if c.TransportType == "stdio" {
		return ""
	}

	// Network transports return "host:port"
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// GetTransportType returns the configured transport type.
// This method is part of the transportConfig interface used by NewTransport.
func (c *Config) GetTransportType() string {
	return c.TransportType
}

// GetPort returns the configured port for network transports.
// This method is part of the transportConfig interface used by NewTransport.
func (c *Config) GetPort() int {
	return c.Port
}

// ValidateTransport validates transport-specific configuration settings.
// It checks that the transport type is valid and that network transports
// have the required port and host configuration.
func (c *Config) ValidateTransport() error {
	var errors []string

	// Validate transport type
	validTransportTypes := map[string]bool{
		"stdio":          true,
		"sse":            true,
		"streamablehttp": true,
	}
	if !validTransportTypes[c.TransportType] {
		errors = append(errors, fmt.Sprintf("invalid transport type: %s (must be one of: stdio, sse, streamablehttp)", c.TransportType))
	}

	// Validate network transport requirements (sse and streamablehttp)
	if c.TransportType == "sse" || c.TransportType == "streamablehttp" {
		// Validate port is in valid range (1-65535)
		if c.Port < 1 || c.Port > 65535 {
			errors = append(errors, fmt.Sprintf("port must be between 1 and 65535 for network transports, got: %d", c.Port))
		}

		// Validate host is non-empty
		if c.Host == "" {
			errors = append(errors, "host cannot be empty for network transports")
		}
	}

	// If there are validation errors, return them all
	if len(errors) > 0 {
		return fmt.Errorf("transport validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Validate validates all configuration values and returns descriptive errors
// for any invalid settings. This should be called after loading configuration
// to ensure the server doesn't start with invalid configuration.
func (c *Config) Validate() error {
	var errors []string

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		errors = append(errors, fmt.Sprintf("invalid log level: %s (must be one of: debug, info, warn, error)", c.LogLevel))
	}

	// Validate fetch timeout (must be positive)
	if c.FetchTimeout <= 0 {
		errors = append(errors, fmt.Sprintf("fetch_timeout must be positive, got: %d", c.FetchTimeout))
	}

	// Validate max concurrent (must be positive)
	if c.MaxConcurrent <= 0 {
		errors = append(errors, fmt.Sprintf("max_concurrent must be positive, got: %d", c.MaxConcurrent))
	}

	// Validate max search results (must be positive)
	if c.MaxSearchResults <= 0 {
		errors = append(errors, fmt.Sprintf("max_search_results must be positive, got: %d", c.MaxSearchResults))
	}

	switch c.NATSSourceType {
	case "site":
	case "archive":
		if c.NATSArchivePath == "" {
			errors = append(errors, "nats.archive_path cannot be empty when nats.source_type is archive")
		}
		if c.NATSArchiveURL != "" {
			errors = append(errors, "nats_archive_url is reserved and not supported in this release; remove the field or set nats_source_type=site")
		}
	default:
		errors = append(errors, fmt.Sprintf("invalid nats.source_type: %s (must be one of: site, archive)", c.NATSSourceType))
	}

	// Validate docs base URL
	if c.DocsBaseURL == "" {
		errors = append(errors, "docs_base_url cannot be empty")
	} else {
		// Check if URL has valid scheme (http or https)
		if !strings.HasPrefix(c.DocsBaseURL, "http://") && !strings.HasPrefix(c.DocsBaseURL, "https://") {
			errors = append(errors, fmt.Sprintf("docs_base_url must start with http:// or https://, got: %s", c.DocsBaseURL))
		}
		// Basic URL validation - check for scheme and host
		if strings.HasPrefix(c.DocsBaseURL, "http://") && len(c.DocsBaseURL) <= 7 {
			errors = append(errors, fmt.Sprintf("docs_base_url is incomplete: %s", c.DocsBaseURL))
		}
		if strings.HasPrefix(c.DocsBaseURL, "https://") && len(c.DocsBaseURL) <= 8 {
			errors = append(errors, fmt.Sprintf("docs_base_url is incomplete: %s", c.DocsBaseURL))
		}
	}

	// Validate synadia configuration (only if enabled)
	if c.SynadiaEnabled {
		// Validate synadia base URL
		if c.SynadiaBaseURL == "" {
			errors = append(errors, "synadia.base_url cannot be empty when synadia is enabled")
		} else {
			// Check if URL has valid scheme (http or https)
			if !strings.HasPrefix(c.SynadiaBaseURL, "http://") && !strings.HasPrefix(c.SynadiaBaseURL, "https://") {
				errors = append(errors, fmt.Sprintf("synadia.base_url must start with http:// or https://, got: %s", c.SynadiaBaseURL))
			}
			// Basic URL validation - check for scheme and host
			if strings.HasPrefix(c.SynadiaBaseURL, "http://") && len(c.SynadiaBaseURL) <= 7 {
				errors = append(errors, fmt.Sprintf("synadia.base_url is incomplete: %s", c.SynadiaBaseURL))
			}
			if strings.HasPrefix(c.SynadiaBaseURL, "https://") && len(c.SynadiaBaseURL) <= 8 {
				errors = append(errors, fmt.Sprintf("synadia.base_url is incomplete: %s", c.SynadiaBaseURL))
			}
		}

		// Validate synadia fetch timeout
		if c.SynadiaFetchTimeout <= 0 {
			errors = append(errors, fmt.Sprintf("synadia.fetch_timeout must be positive, got: %d", c.SynadiaFetchTimeout))
		}

		// Validate keyword lists are not empty
		if len(c.SynadiaKeywords) == 0 {
			errors = append(errors, "classification.synadia_keywords cannot be empty when synadia is enabled")
		}
		if len(c.NATSKeywords) == 0 {
			errors = append(errors, "classification.nats_keywords cannot be empty when synadia is enabled")
		}
	}

	// Validate GitHub configuration (only if enabled)
	if c.GitHubEnabled {
		// Validate GitHub token
		if c.GitHubToken == "" {
			errors = append(errors, "github.token cannot be empty when GitHub is enabled")
		}

		// Validate GitHub repositories
		if len(c.GitHubRepositories) == 0 {
			errors = append(errors, "github.repositories cannot be empty when GitHub is enabled")
		}
		for _, repo := range c.GitHubRepositories {
			if !strings.Contains(repo, "/") {
				errors = append(errors, fmt.Sprintf("github.repositories must be in format 'owner/repo', got: %s", repo))
			}
		}

		// Validate GitHub fetch timeout
		if c.GitHubFetchTimeout <= 0 {
			errors = append(errors, fmt.Sprintf("github.fetch_timeout must be positive, got: %d", c.GitHubFetchTimeout))
		}

		// Validate GitHub branch
		if c.GitHubBranch == "" {
			errors = append(errors, "github.default_branch cannot be empty when GitHub is enabled")
		}

		// Validate keyword list is not empty
		if len(c.GitHubKeywords) == 0 {
			errors = append(errors, "classification.github_keywords cannot be empty when GitHub is enabled")
		}
	}

	// If there are validation errors, return them all
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

func loadNATSArchiveFromViper(cfg *Config, v *viper.Viper) {
	if v.IsSet("nats.source_type") {
		cfg.NATSSourceType = v.GetString("nats.source_type")
	}
	if v.IsSet("nats.archive_path") {
		cfg.NATSArchivePath = v.GetString("nats.archive_path")
	}
	if v.IsSet("nats.archive_url") {
		cfg.NATSArchiveURL = v.GetString("nats.archive_url")
	}
	if v.IsSet("nats.archive_branch") {
		cfg.NATSArchiveBranch = v.GetString("nats.archive_branch")
	}
	if v.IsSet("nats.archive_include_orphans") {
		cfg.NATSArchiveIncludeOrphans = v.GetBool("nats.archive_include_orphans")
	}
	if v.IsSet("nats.archive_include_legacy") {
		cfg.NATSArchiveIncludeLegacy = v.GetBool("nats.archive_include_legacy")
	}
}

func parseBoolEnv(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// GetCacheDir returns the cache directory, using default if not configured.
// It expands ~ to the user's home directory and returns a sensible default
// if the user's home directory cannot be determined.
func (c *Config) GetCacheDir() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}
	// Default: ~/.cache/nats-mcp/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/nats-mcp-cache"
	}
	return homeDir + "/.cache/nats-mcp"
}
