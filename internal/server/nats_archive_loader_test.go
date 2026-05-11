package server

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/j4ng5y/nats-docs-mcp-server/internal/index"
)

func TestArchiveLoader_LoadsCanonicalPages(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(docs) != 4 {
		t.Fatalf("expected 4 canonical docs, got %d: %v", len(docs), docIDs(docs))
	}
	for _, doc := range docs {
		if !strings.HasPrefix(doc.URL, "https://docs.nats.io") {
			t.Fatalf("expected docsBaseURL URL, got %q", doc.URL)
		}
	}
}

func TestArchiveLoader_RootReadmeIsIndex(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	doc := findDoc(t, docs, "index")
	if doc.URL != "https://docs.nats.io/" {
		t.Fatalf("expected root URL, got %q", doc.URL)
	}
}

func TestArchiveLoader_NestedReadme(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	doc := findDoc(t, docs, "nested/b")
	if doc.URL != "https://docs.nats.io/nested/b" {
		t.Fatalf("expected nested README URL, got %q", doc.URL)
	}
}

func TestArchiveLoader_SkipsBookIgnored(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, true, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	for _, skipped := range []string{"docs/generated", "_examples/sample", "building_the_book"} {
		if hasDoc(docs, skipped) {
			t.Fatalf("expected %s to be skipped; docs=%v", skipped, docIDs(docs))
		}
	}
}

func TestArchiveLoader_SkipsZhCN(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, true, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if hasDoc(docs, "zh-cn/placeholder") {
		t.Fatalf("expected zh-cn page to be skipped; docs=%v", docIDs(docs))
	}
}

func TestArchiveLoader_SkipsEmpty(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if hasDoc(docs, "empty") {
		t.Fatalf("expected empty.md to be skipped; docs=%v", docIDs(docs))
	}
}

func TestArchiveLoader_SkipsOrphansByDefault(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if hasDoc(docs, "orphan") {
		t.Fatalf("expected orphan to be skipped by default; docs=%v", docIDs(docs))
	}
}

func TestArchiveLoader_IncludesOrphans(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, true, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	findDoc(t, docs, "orphan")
}

func TestArchiveLoader_OrphansSkipSummary(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, true, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if hasDoc(docs, "SUMMARY") {
		t.Fatalf("expected SUMMARY.md to be skipped; docs=%v", docIDs(docs))
	}
}

func TestArchiveLoader_CanonicalRedirectAlias(t *testing.T) {
	_, meta, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := meta.Aliases["overview-old"]; got != "overview" {
		t.Fatalf("expected overview-old alias to overview, got %q", got)
	}
}

func TestArchiveLoader_LegacyRedirectDroppedByDefault(t *testing.T) {
	_, meta, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if _, ok := meta.Aliases["nats-tools/nas"]; ok {
		t.Fatal("expected legacy alias to be dropped by default")
	}
	if meta.AliasesDropped == 0 {
		t.Fatal("expected at least one dropped alias")
	}
}

func TestArchiveLoader_LegacyRedirectResolvesWhenIncluded(t *testing.T) {
	docs, meta, err := loadFixtureArchive(t, false, true)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	findDoc(t, docs, "legacy/nas")
	if got := meta.Aliases["nats-tools/nas"]; got != "legacy/nas" {
		t.Fatalf("expected legacy alias to resolve, got %q", got)
	}
}

func TestArchiveCompatibilityAliases(t *testing.T) {
	indexedIDs := map[string]struct{}{
		"overview":                   {},
		"release_notes/whats_new":    {},
		"release_notes/whats_new_20": {},
		"nats-concepts/core-nats/publish-subscribe/pubsub":                         {},
		"nats-concepts/core-nats/publish-subscribe/pubsub_walkthrough":             {},
		"nats-concepts/jetstream/object-store/obj_store":                           {},
		"nats-concepts/jetstream/object-store/obj_walkthrough":                     {},
		"reference/nats-protocol/nats-protocol/nats-client-dev":                    {},
		"using-nats/developing-with-nats/connecting/security/creds":                {},
		"using-nats/developing-with-nats/reconnect/max":                            {},
		"using-nats/developing-with-nats/js/streams":                               {},
		"using-nats/jetstream/nats_api_reference":                                  {},
		"running-a-nats-service/configuration/jetstream-config/configuration_mgmt": {},
		"running-a-nats-service/configuration/securing_nats/jwt/resolver":          {},
		"running-a-nats-service/running/nats_docker/jetstream_docker":              {},
	}
	aliases := make(map[string]string)
	addArchiveCompatibilityAliases(aliases, indexedIDs)

	expected := map[string]string{
		"nats-concepts/overview":                                                      "overview",
		"release-notes/whats_new":                                                     "release_notes/whats_new",
		"release-notes/whats_new/whats_new_20":                                        "release_notes/whats_new_20",
		"nats-concepts/core-nats/pubsub":                                              "nats-concepts/core-nats/publish-subscribe/pubsub",
		"nats-concepts/core-nats/pubsub/pubsub_walkthrough":                           "nats-concepts/core-nats/publish-subscribe/pubsub_walkthrough",
		"nats-concepts/jetstream/obj_store":                                           "nats-concepts/jetstream/object-store/obj_store",
		"nats-concepts/jetstream/obj_store/obj_walkthrough":                           "nats-concepts/jetstream/object-store/obj_walkthrough",
		"reference/reference-protocols/nats-protocol/nats-client-dev":                 "reference/nats-protocol/nats-protocol/nats-client-dev",
		"using-nats/developer/connecting/creds":                                       "using-nats/developing-with-nats/connecting/security/creds",
		"using-nats/developer/connecting/reconnect/max":                               "using-nats/developing-with-nats/reconnect/max",
		"using-nats/developer/develop_jetstream/streams":                              "using-nats/developing-with-nats/js/streams",
		"reference/reference-protocols/nats_api_reference":                            "using-nats/jetstream/nats_api_reference",
		"running-a-nats-service/configuration/resource_management/configuration_mgmt": "running-a-nats-service/configuration/jetstream-config/configuration_mgmt",
		"running-a-nats-service/configuration/securing_nats/auth_intro/jwt/resolver":  "running-a-nats-service/configuration/securing_nats/jwt/resolver",
		"running-a-nats-service/nats_docker/jetstream_docker":                         "running-a-nats-service/running/nats_docker/jetstream_docker",
	}
	for alias, canonical := range expected {
		if got := aliases[alias]; got != canonical {
			t.Fatalf("expected alias %q to resolve to %q, got %q", alias, canonical, got)
		}
	}
}

func TestArchiveLoader_PreprocessApplied(t *testing.T) {
	docs, _, err := loadFixtureArchive(t, false, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	doc := findDoc(t, docs, "overview")
	if strings.Contains(doc.Content, "{% hint") {
		t.Fatalf("expected GitBook hint directive to be removed, got %q", doc.Content)
	}
	if !strings.Contains(doc.Content, "Info:") {
		t.Fatalf("expected hint label in content, got %q", doc.Content)
	}
}

func TestArchiveLoader_MissingSummary(t *testing.T) {
	archivePath := buildArchiveFromEntries(t, map[string]string{"README.md": "# Welcome\n"})
	loader := newNATSArchiveLoader(natsArchiveConfig{ArchivePath: archivePath, DocsBaseURL: "https://docs.nats.io"}, testLogger())
	_, _, err := loader.Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SUMMARY.md") {
		t.Fatalf("expected missing SUMMARY.md error, got %v", err)
	}
}

func TestArchiveLoader_MissingArchive(t *testing.T) {
	loader := newNATSArchiveLoader(natsArchiveConfig{ArchivePath: "/does/not/exist.zip", DocsBaseURL: "https://docs.nats.io"}, testLogger())
	_, _, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected missing archive error")
	}
}

func loadFixtureArchive(t *testing.T, includeOrphans, includeLegacy bool) ([]*index.Document, SourceMetadata, error) {
	t.Helper()
	loader := newNATSArchiveLoader(natsArchiveConfig{
		ArchivePath:    buildFixtureArchive(t),
		DocsBaseURL:    "https://docs.nats.io",
		IncludeOrphans: includeOrphans,
		IncludeLegacy:  includeLegacy,
	}, testLogger())
	return loader.Load(context.Background())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func docIDs(docs []*index.Document) []string {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	return ids
}

func hasDoc(docs []*index.Document, id string) bool {
	for _, doc := range docs {
		if doc.ID == id {
			return true
		}
	}
	return false
}

func findDoc(t *testing.T, docs []*index.Document, id string) *index.Document {
	t.Helper()
	for _, doc := range docs {
		if doc.ID == id {
			return doc
		}
	}
	t.Fatalf("missing doc %q in %v", id, docIDs(docs))
	return nil
}
