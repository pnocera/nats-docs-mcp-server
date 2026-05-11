package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type testCase struct {
	name     string
	tool     string
	args     map[string]any
	contains []string
}

type testRunner struct {
	checks   int
	failures []string
}

func main() {
	serverPath := flag.String("server", "", "path to an existing nats-docs-mcp-server binary; if empty, build a temporary binary")
	archivePath := flag.String("archive", "assets/nats.docs-master.zip", "path to the NATS docs archive")
	cacheDir := flag.String("cache-dir", "", "cache directory for the test server; if empty, use a temporary directory")
	keepCache := flag.Bool("keep-cache", false, "keep the temporary cache directory after the run")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall test timeout")
	flag.Parse()

	if err := run(*serverPath, *archivePath, *cacheDir, *keepCache, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "real-world tests failed: %v\n", err)
		os.Exit(1)
	}
}

func run(serverPath, archivePath, cacheDir string, keepCache bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := os.Chdir(repoRoot); err != nil {
		return fmt.Errorf("chdir repo root: %w", err)
	}

	archiveAbs, err := filepath.Abs(archivePath)
	if err != nil {
		return fmt.Errorf("resolve archive path: %w", err)
	}
	if _, err := os.Stat(archiveAbs); err != nil {
		return fmt.Errorf("archive %q is not available: %w", archiveAbs, err)
	}

	cleanupServer := func() {}
	if serverPath == "" {
		serverPath, cleanupServer, err = buildServer(ctx, repoRoot)
		if err != nil {
			return err
		}
		defer cleanupServer()
	} else {
		serverPath, err = filepath.Abs(serverPath)
		if err != nil {
			return fmt.Errorf("resolve server path: %w", err)
		}
		if _, err := os.Stat(serverPath); err != nil {
			return fmt.Errorf("server binary %q is not available: %w", serverPath, err)
		}
	}

	cleanupCache := func() {}
	if cacheDir == "" {
		cacheDir, cleanupCache, err = tempDir("nats-docs-real-world-cache-")
		if err != nil {
			return err
		}
		if !keepCache {
			defer cleanupCache()
		}
	} else {
		cacheDir, err = filepath.Abs(cacheDir)
		if err != nil {
			return fmt.Errorf("resolve cache dir: %w", err)
		}
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return fmt.Errorf("create cache dir: %w", err)
		}
	}

	fmt.Printf("Real-world MCP tests\n")
	fmt.Printf("  repo:    %s\n", repoRoot)
	fmt.Printf("  server:  %s\n", serverPath)
	fmt.Printf("  archive: %s\n", archiveAbs)
	fmt.Printf("  cache:   %s\n\n", cacheDir)

	env := []string{
		"NATS_DOCS_LOG_LEVEL=error",
		"NATS_DOCS_TRANSPORT_TYPE=stdio",
		"NATS_DOCS_NATS_SOURCE_TYPE=archive",
		"NATS_DOCS_NATS_ARCHIVE_PATH=" + archiveAbs,
		"NATS_DOCS_CACHE_DIR=" + cacheDir,
		"NATS_DOCS_SYNADIA_ENABLED=false",
		"NATS_DOCS_GITHUB_ENABLED=false",
	}

	mcpClient, err := client.NewStdioMCPClient(serverPath, env, "--log-level", "error", "--transport", "stdio")
	if err != nil {
		return fmt.Errorf("start stdio MCP client: %w", err)
	}
	defer mcpClient.Close()

	stderrBuffer := captureStderr(mcpClient)
	runner := &testRunner{}

	if err := initialize(ctx, mcpClient); err != nil {
		printServerStderr(stderrBuffer)
		return err
	}
	runner.pass("initialize MCP session")

	if err := mcpClient.Ping(ctx); err != nil {
		printServerStderr(stderrBuffer)
		return fmt.Errorf("ping server: %w", err)
	}
	runner.pass("ping server")

	if err := runner.checkTools(ctx, mcpClient); err != nil {
		printServerStderr(stderrBuffer)
		return err
	}

	for _, tc := range searchCases() {
		runner.runToolCase(ctx, mcpClient, tc)
	}
	for _, tc := range retrieveCases() {
		runner.runToolCase(ctx, mcpClient, tc)
	}

	runner.checkToolError(ctx, mcpClient, testCase{
		name: "missing document returns tool error",
		tool: "retrieve_nats_doc",
		args: map[string]any{"doc_id": "does/not/exist"},
	})

	if len(runner.failures) > 0 {
		printServerStderr(stderrBuffer)
		return runner.err()
	}

	fmt.Printf("\nPASS real-world tests (%d checks)\n", runner.checks)
	if keepCache {
		fmt.Printf("Kept cache directory: %s\n", cacheDir)
	}
	return nil
}

func initialize(ctx context.Context, mcpClient *client.Client) error {
	request := mcp.InitializeRequest{}
	request.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = mcp.Implementation{
		Name:    "nats-docs-real-world-tests",
		Version: "dev",
	}
	request.Params.Capabilities = mcp.ClientCapabilities{}

	if _, err := mcpClient.Initialize(ctx, request); err != nil {
		return fmt.Errorf("initialize MCP session: %w", err)
	}
	return nil
}

func (r *testRunner) checkTools(ctx context.Context, mcpClient *client.Client) error {
	result, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	available := make(map[string]bool, len(result.Tools))
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		available[tool.Name] = true
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	for _, required := range []string{"search_nats_docs", "retrieve_nats_doc", "refresh_docs_cache"} {
		if !available[required] {
			r.fail("%s tool is registered", required)
			continue
		}
		r.pass("%s tool is registered", required)
	}
	fmt.Printf("  available tools: %s\n", strings.Join(names, ", "))
	return nil
}

func (r *testRunner) runToolCase(ctx context.Context, mcpClient *client.Client, tc testCase) {
	request := mcp.CallToolRequest{}
	request.Params.Name = tc.tool
	request.Params.Arguments = tc.args

	result, err := mcpClient.CallTool(ctx, request)
	if err != nil {
		r.fail("%s: call failed: %v", tc.name, err)
		return
	}
	if result.IsError {
		r.fail("%s: tool returned error: %s", tc.name, toolText(result))
		return
	}

	text := toolText(result)
	if strings.TrimSpace(text) == "" {
		r.fail("%s: tool returned empty text", tc.name)
		return
	}
	for _, expected := range tc.contains {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(expected)) {
			r.fail("%s: expected response to contain %q", tc.name, expected)
			return
		}
	}
	r.pass("%s", tc.name)
}

func (r *testRunner) checkToolError(ctx context.Context, mcpClient *client.Client, tc testCase) {
	request := mcp.CallToolRequest{}
	request.Params.Name = tc.tool
	request.Params.Arguments = tc.args

	result, err := mcpClient.CallTool(ctx, request)
	if err != nil {
		r.fail("%s: call failed: %v", tc.name, err)
		return
	}
	if !result.IsError {
		r.fail("%s: expected tool error, got success: %s", tc.name, toolText(result))
		return
	}
	r.pass("%s", tc.name)
}

func searchCases() []testCase {
	return []testCase{
		{
			name:     `search "jetstream streams"`,
			tool:     "search_nats_docs",
			args:     map[string]any{"query": "jetstream streams", "limit": 5},
			contains: []string{"Found", "https://docs.nats.io"},
		},
		{
			name:     `search "auth callout"`,
			tool:     "search_nats_docs",
			args:     map[string]any{"query": "auth callout", "limit": 5},
			contains: []string{"auth", "callout", "https://docs.nats.io"},
		},
		{
			name:     `search "leaf nodes"`,
			tool:     "search_nats_docs",
			args:     map[string]any{"query": "leaf nodes", "limit": 5},
			contains: []string{"leaf", "https://docs.nats.io"},
		},
		{
			name:     `search "subject mapping"`,
			tool:     "search_nats_docs",
			args:     map[string]any{"query": "subject mapping", "limit": 5},
			contains: []string{"subject", "https://docs.nats.io"},
		},
	}
}

func retrieveCases() []testCase {
	return []testCase{
		{
			name:     "retrieve root document",
			tool:     "retrieve_nats_doc",
			args:     map[string]any{"doc_id": "index"},
			contains: []string{"https://docs.nats.io/", "NATS"},
		},
		{
			name:     "retrieve canonical JetStream streams document",
			tool:     "retrieve_nats_doc",
			args:     map[string]any{"doc_id": "nats-concepts/jetstream/streams"},
			contains: []string{"Streams", "JetStream"},
		},
		{
			name:     "retrieve previous live-site release notes alias",
			tool:     "retrieve_nats_doc",
			args:     map[string]any{"doc_id": "release-notes/whats_new"},
			contains: []string{"What's New", "NATS"},
		},
		{
			name:     "retrieve previous live-site credentials alias",
			tool:     "retrieve_nats_doc",
			args:     map[string]any{"doc_id": "using-nats/developer/connecting/creds"},
			contains: []string{"Credentials", "https://docs.nats.io"},
		},
	}
}

func toolText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var out strings.Builder
	for _, content := range result.Content {
		if text, ok := mcp.AsTextContent(content); ok {
			if out.Len() > 0 {
				out.WriteString("\n")
			}
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

func (r *testRunner) pass(format string, args ...any) {
	r.checks++
	fmt.Printf("PASS %s\n", fmt.Sprintf(format, args...))
}

func (r *testRunner) fail(format string, args ...any) {
	r.checks++
	message := fmt.Sprintf(format, args...)
	r.failures = append(r.failures, message)
	fmt.Printf("FAIL %s\n", message)
}

func (r *testRunner) err() error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d checks failed", len(r.failures), r.checks)
	for _, failure := range r.failures {
		fmt.Fprintf(&b, "\n- %s", failure)
	}
	return errors.New(b.String())
}

func buildServer(ctx context.Context, repoRoot string) (string, func(), error) {
	dir, cleanup, err := tempDir("nats-docs-real-world-bin-")
	if err != nil {
		return "", nil, err
	}

	bin := filepath.Join(dir, "nats-docs-mcp-server")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/server")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("build server binary: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return bin, cleanup, nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root containing go.mod from %s", dir)
		}
		dir = parent
	}
}

func tempDir(pattern string) (string, func(), error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func captureStderr(mcpClient *client.Client) *safeBuffer {
	var stderr safeBuffer
	if r, ok := client.GetStderr(mcpClient); ok && r != nil {
		go func() {
			_, _ = io.Copy(&stderr, r)
		}()
	}
	return &stderr
}

func printServerStderr(stderr *safeBuffer) {
	if stderr == nil {
		return
	}
	text := strings.TrimSpace(stderr.String())
	if text == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\nServer stderr:\n%s\n", text)
}
