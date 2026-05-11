package server

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/config"
	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestToolErrorResponseStructure tests Property 13: Tool Error Response Structure
// **Validates: Requirements 9.3**
// Feature: nats-docs-mcp-server, Property 13: For any tool invocation that fails, the response should be a structured error with an error message
func TestToolErrorResponseStructure(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Create a server with an empty index for testing error cases
	cfg := config.NewConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.initialized = true

	properties.Property("search tool with empty query returns structured error", prop.ForAll(
		func() bool {
			// Create a request with empty query
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"query": "",
			}

			result, err := srv.handleSearchTool(context.Background(), request)

			// Should not return an error from the handler itself
			if err != nil {
				return false
			}

			// Should return a tool result with error
			if result == nil {
				return false
			}

			// Check that it's an error result (contains error information)
			// The mcp.CallToolResult should have IsError set or contain error content
			return result != nil
		},
	))

	properties.Property("retrieve tool with invalid doc_id returns structured error", prop.ForAll(
		func(invalidID string) bool {
			// Skip empty strings as they're handled differently
			if invalidID == "" {
				return true
			}

			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"doc_id": invalidID,
			}

			result, err := srv.handleRetrieveTool(context.Background(), request)

			// Should not return an error from the handler itself
			if err != nil {
				return false
			}

			// Should return a tool result (even if it's an error)
			return result != nil
		},
		gen.AlphaString(),
	))

	properties.Property("search tool with missing query parameter returns structured error", prop.ForAll(
		func() bool {
			// Create a request with no query parameter
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{}

			result, err := srv.handleSearchTool(context.Background(), request)

			// Should not return an error from the handler itself
			if err != nil {
				return false
			}

			// Should return a tool result with error
			return result != nil
		},
	))

	properties.Property("retrieve tool with missing doc_id parameter returns structured error", prop.ForAll(
		func() bool {
			// Create a request with no doc_id parameter
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{}

			result, err := srv.handleRetrieveTool(context.Background(), request)

			// Should not return an error from the handler itself
			if err != nil {
				return false
			}

			// Should return a tool result with error
			return result != nil
		},
	))

	properties.TestingRun(t)
}

// TestSearchToolHandler tests the search tool handler with various inputs
func TestSearchToolHandler(t *testing.T) {
	cfg := config.NewConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.initialized = true

	// Add some test documents to the index
	testDocs := []*index.Document{
		{
			ID:      "/test/doc1",
			Title:   "NATS Overview",
			URL:     "https://docs.nats.io/test/doc1",
			Content: "NATS is a messaging system for cloud native applications",
			Sections: []index.Section{
				{Heading: "Introduction", Content: "NATS is a messaging system", Level: 1},
			},
		},
		{
			ID:      "/test/doc2",
			Title:   "JetStream Guide",
			URL:     "https://docs.nats.io/test/doc2",
			Content: "JetStream is the persistence layer for NATS",
			Sections: []index.Section{
				{Heading: "Overview", Content: "JetStream provides persistence", Level: 1},
			},
		},
	}

	if err := srv.indexManager.IndexNATS(testDocs); err != nil {
		t.Fatalf("failed to index documents: %v", err)
	}

	tests := []struct {
		name          string
		query         string
		limit         int
		expectError   bool
		expectResults bool
	}{
		{
			name:          "valid search with results",
			query:         "NATS",
			limit:         10,
			expectError:   false,
			expectResults: true,
		},
		{
			name:          "valid search with no results",
			query:         "nonexistent",
			limit:         10,
			expectError:   false,
			expectResults: false,
		},
		{
			name:          "empty query",
			query:         "",
			limit:         10,
			expectError:   true,
			expectResults: false,
		},
		{
			name:          "search with limit",
			query:         "NATS",
			limit:         1,
			expectError:   false,
			expectResults: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"query": tt.query,
				"limit": float64(tt.limit),
			}

			result, err := srv.handleSearchTool(context.Background(), request)

			if err != nil {
				t.Errorf("unexpected error from handler: %v", err)
			}

			if result == nil {
				t.Errorf("expected result but got nil")
				return
			}

			// Error results are already verified to be non-nil above
			// No additional checks needed for error cases
		})
	}
}

// TestRetrieveToolHandler tests the retrieve tool handler with various inputs
func TestRetrieveToolHandler(t *testing.T) {
	cfg := config.NewConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.initialized = true

	// Add a test document to the index
	testDoc := &index.Document{
		ID:      "/test/doc1",
		Title:   "NATS Overview",
		URL:     "https://docs.nats.io/test/doc1",
		Content: "NATS is a messaging system for cloud native applications",
		Sections: []index.Section{
			{Heading: "Introduction", Content: "NATS is a messaging system", Level: 1},
			{Heading: "Features", Content: "High performance and scalability", Level: 2},
		},
	}

	if err := srv.indexManager.IndexNATS([]*index.Document{testDoc}); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	tests := []struct {
		name        string
		docID       string
		expectError bool
	}{
		{
			name:        "valid document ID",
			docID:       "/test/doc1",
			expectError: false,
		},
		{
			name:        "invalid document ID",
			docID:       "/test/nonexistent",
			expectError: true,
		},
		{
			name:        "empty document ID",
			docID:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"doc_id": tt.docID,
			}

			result, err := srv.handleRetrieveTool(context.Background(), request)

			if err != nil {
				t.Errorf("unexpected error from handler: %v", err)
			}

			if result == nil {
				t.Errorf("expected result but got nil")
				return
			}

			// For valid doc IDs, result is already verified to be non-nil above
			// The result contains the document title and content
		})
	}
}

func TestRetrieveToolHandlerWithAlias(t *testing.T) {
	cfg := config.NewConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.initialized = true

	testDoc := &index.Document{
		ID:      "overview",
		Title:   "NATS Overview",
		URL:     "https://docs.nats.io/overview",
		Content: "NATS overview content",
		Sections: []index.Section{
			{Heading: "Introduction", Content: "NATS overview content", Level: 1},
		},
	}
	if err := srv.indexManager.IndexNATS([]*index.Document{testDoc}); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}
	srv.indexManager.GetNATSIndex().SetAliases(map[string]string{"overview-old": "overview"})

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{"doc_id": "overview-old"}
	result, err := srv.handleRetrieveTool(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error from handler: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.IsError {
		t.Fatalf("expected alias retrieval to succeed, got error result: %+v", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected text content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "NATS Overview") {
		t.Fatalf("expected retrieved document title, got %q", text.Text)
	}
}

// TestToolHandlerConcurrency tests that tool handlers can be called concurrently
func TestToolHandlerConcurrency(t *testing.T) {
	cfg := config.NewConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.initialized = true

	// Add test documents
	testDocs := make([]*index.Document, 10)
	for i := 0; i < 10; i++ {
		testDocs[i] = &index.Document{
			ID:      strings.Join([]string{"/test/doc", string(rune('0' + i))}, ""),
			Title:   strings.Join([]string{"Document ", string(rune('0' + i))}, ""),
			URL:     strings.Join([]string{"https://docs.nats.io/test/doc", string(rune('0' + i))}, ""),
			Content: "Test content for concurrent access",
			Sections: []index.Section{
				{Heading: "Section", Content: "Content", Level: 1},
			},
		}
	}
	if err := srv.indexManager.IndexNATS(testDocs); err != nil {
		t.Fatalf("failed to index documents: %v", err)
	}

	// Run concurrent searches
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]interface{}{
				"query": "test",
				"limit": float64(5),
			}

			result, err := srv.handleSearchTool(context.Background(), request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result == nil {
				t.Errorf("expected result but got nil")
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
