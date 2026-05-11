package index

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Test for task 5.1: Document storage data structures
func TestDocumentCreation(t *testing.T) {
	doc := &Document{
		ID:          "test-doc",
		Title:       "Test Document",
		URL:         "https://docs.nats.io/test",
		Content:     "This is test content",
		Sections:    []Section{{Heading: "Introduction", Content: "Test intro", Level: 1}},
		LastUpdated: time.Now(),
	}

	if doc.ID != "test-doc" {
		t.Errorf("Expected ID 'test-doc', got '%s'", doc.ID)
	}
	if doc.Title != "Test Document" {
		t.Errorf("Expected Title 'Test Document', got '%s'", doc.Title)
	}
	if doc.URL != "https://docs.nats.io/test" {
		t.Errorf("Expected URL 'https://docs.nats.io/test', got '%s'", doc.URL)
	}
	if doc.Content != "This is test content" {
		t.Errorf("Expected Content 'This is test content', got '%s'", doc.Content)
	}
	if len(doc.Sections) != 1 {
		t.Errorf("Expected 1 section, got %d", len(doc.Sections))
	}
	if doc.LastUpdated.IsZero() {
		t.Error("Expected LastUpdated to be set, got zero time")
	}
}

func TestDocumentStoreCreation(t *testing.T) {
	store := NewDocumentStore()

	if store == nil {
		t.Fatal("Expected non-nil DocumentStore")
	}

	// Store should be empty initially
	if len(store.documents) != 0 {
		t.Errorf("Expected empty store, got %d documents", len(store.documents))
	}
}

func TestDocumentStoreAddDocument(t *testing.T) {
	store := NewDocumentStore()
	doc := &Document{
		ID:      "test-doc",
		Title:   "Test Document",
		URL:     "https://docs.nats.io/test",
		Content: "Test content",
	}

	err := store.AddDocument(doc)
	if err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	// Verify document was added
	if len(store.documents) != 1 {
		t.Errorf("Expected 1 document, got %d", len(store.documents))
	}
}

func TestDocumentStoreGetDocument(t *testing.T) {
	store := NewDocumentStore()
	doc := &Document{
		ID:      "test-doc",
		Title:   "Test Document",
		URL:     "https://docs.nats.io/test",
		Content: "Test content",
	}

	_ = store.AddDocument(doc)

	retrieved, err := store.GetDocument("test-doc")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if retrieved.ID != doc.ID {
		t.Errorf("Expected ID '%s', got '%s'", doc.ID, retrieved.ID)
	}
	if retrieved.Title != doc.Title {
		t.Errorf("Expected Title '%s', got '%s'", doc.Title, retrieved.Title)
	}
}

func TestDocumentStoreGetNonExistentDocument(t *testing.T) {
	store := NewDocumentStore()

	_, err := store.GetDocument("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent document, got nil")
	}
}

func TestDocumentStoreThreadSafety(t *testing.T) {
	store := NewDocumentStore()

	// Add a document
	doc := &Document{
		ID:      "test-doc",
		Title:   "Test Document",
		Content: "Test content",
	}
	_ = store.AddDocument(doc)

	// Test concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := store.GetDocument("test-doc")
			if err != nil {
				t.Errorf("Concurrent read failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test for task 5.2: Search index with TF-IDF
func TestSearchIndexCreation(t *testing.T) {
	index := NewSearchIndex()

	if index == nil {
		t.Fatal("Expected non-nil SearchIndex")
	}

	if index.totalDocuments != 0 {
		t.Errorf("Expected 0 documents, got %d", index.totalDocuments)
	}
}

func TestSearchIndexAddDocument(t *testing.T) {
	index := NewSearchIndex()

	err := index.AddDocument("doc1", "hello world test")
	if err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	if index.totalDocuments != 1 {
		t.Errorf("Expected 1 document, got %d", index.totalDocuments)
	}
}

func TestSearchIndexTermFrequency(t *testing.T) {
	index := NewSearchIndex()

	// Add document with repeated terms
	_ = index.AddDocument("doc1", "hello world hello test")

	// Check term frequency for "hello" (appears twice)
	tf := index.GetTermFrequency("doc1", "hello")
	if tf != 2 {
		t.Errorf("Expected term frequency 2 for 'hello', got %d", tf)
	}

	// Check term frequency for "world" (appears once)
	tf = index.GetTermFrequency("doc1", "world")
	if tf != 1 {
		t.Errorf("Expected term frequency 1 for 'world', got %d", tf)
	}
}

func TestSearchIndexDocumentFrequency(t *testing.T) {
	index := NewSearchIndex()

	// Add multiple documents
	if err := index.AddDocument("doc1", "hello world"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}
	if err := index.AddDocument("doc2", "hello test"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}
	if err := index.AddDocument("doc3", "world test"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	// "hello" appears in 2 documents
	df := index.GetDocumentFrequency("hello")
	if df != 2 {
		t.Errorf("Expected document frequency 2 for 'hello', got %d", df)
	}

	// "world" appears in 2 documents
	df = index.GetDocumentFrequency("world")
	if df != 2 {
		t.Errorf("Expected document frequency 2 for 'world', got %d", df)
	}

	// "test" appears in 2 documents
	df = index.GetDocumentFrequency("test")
	if df != 2 {
		t.Errorf("Expected document frequency 2 for 'test', got %d", df)
	}
}

func TestSearchIndexCalculateTFIDF(t *testing.T) {
	index := NewSearchIndex()

	// Add documents
	if err := index.AddDocument("doc1", "hello world hello"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}
	if err := index.AddDocument("doc2", "hello test"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}
	if err := index.AddDocument("doc3", "world test"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	// Calculate TF-IDF for "hello" in doc1
	// TF = 2 (appears twice), DF = 2 (appears in 2 docs), total docs = 3
	// IDF = log(3/2) ≈ 0.176
	// TF-IDF = 2 * 0.176 ≈ 0.352
	tfidf := index.CalculateTFIDF("doc1", "hello")
	if tfidf <= 0 {
		t.Errorf("Expected positive TF-IDF score, got %f", tfidf)
	}

	// Calculate TF-IDF for "world" in doc1
	// TF = 1, DF = 2, total docs = 3
	// IDF = log(3/2) ≈ 0.176
	// TF-IDF = 1 * 0.176 ≈ 0.176
	tfidf2 := index.CalculateTFIDF("doc1", "world")
	if tfidf2 <= 0 {
		t.Errorf("Expected positive TF-IDF score, got %f", tfidf2)
	}

	// "hello" should have higher score than "world" in doc1 (appears more frequently)
	if tfidf <= tfidf2 {
		t.Errorf("Expected 'hello' TF-IDF (%f) > 'world' TF-IDF (%f)", tfidf, tfidf2)
	}
}

func TestSearchIndexCalculateRelevance(t *testing.T) {
	index := NewSearchIndex()

	// Add documents
	if err := index.AddDocument("doc1", "nats messaging system"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}
	if err := index.AddDocument("doc2", "nats jetstream persistence"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}
	if err := index.AddDocument("doc3", "kafka messaging system"); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	// Calculate relevance for query "nats messaging"
	relevance := index.CalculateRelevance("nats messaging", "doc1")
	if relevance <= 0 {
		t.Errorf("Expected positive relevance score, got %f", relevance)
	}

	// doc1 should be more relevant than doc3 for "nats messaging"
	relevance1 := index.CalculateRelevance("nats messaging", "doc1")
	relevance3 := index.CalculateRelevance("nats messaging", "doc3")

	if relevance1 <= relevance3 {
		t.Errorf("Expected doc1 relevance (%f) > doc3 relevance (%f)", relevance1, relevance3)
	}
}

func TestSearchIndexTokenization(t *testing.T) {
	index := NewSearchIndex()

	// Test that tokenization handles punctuation and case
	if err := index.AddDocument("doc1", "Hello, World! This is a test."); err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	// Should be able to find lowercase versions
	tf := index.GetTermFrequency("doc1", "hello")
	if tf != 1 {
		t.Errorf("Expected term frequency 1 for 'hello', got %d", tf)
	}

	tf = index.GetTermFrequency("doc1", "world")
	if tf != 1 {
		t.Errorf("Expected term frequency 1 for 'world', got %d", tf)
	}
}

// Test for task 5.3: Indexing operations
func TestDocumentationIndexCreation(t *testing.T) {
	idx := NewDocumentationIndex()

	if idx == nil {
		t.Fatal("Expected non-nil DocumentationIndex")
	}

	if idx.Count() != 0 {
		t.Errorf("Expected 0 documents, got %d", idx.Count())
	}
}

func TestDocumentationIndexAddDocument(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "nats-concepts",
		Title:   "NATS Concepts",
		URL:     "https://docs.nats.io/nats-concepts",
		Content: "NATS is a messaging system for cloud native applications",
	}

	err := idx.Index(doc)
	if err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("Expected 1 document, got %d", idx.Count())
	}
}

func TestDocumentationIndexGetDocument(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "nats-concepts",
		Title:   "NATS Concepts",
		URL:     "https://docs.nats.io/nats-concepts",
		Content: "NATS is a messaging system",
	}

	_ = idx.Index(doc)

	retrieved, err := idx.Get("nats-concepts")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if retrieved.ID != doc.ID {
		t.Errorf("Expected ID '%s', got '%s'", doc.ID, retrieved.ID)
	}
	if retrieved.Title != doc.Title {
		t.Errorf("Expected Title '%s', got '%s'", doc.Title, retrieved.Title)
	}
}

func TestDocumentationIndexUpdateDocument(t *testing.T) {
	idx := NewDocumentationIndex()

	// Add initial document
	doc1 := &Document{
		ID:      "nats-concepts",
		Title:   "NATS Concepts",
		Content: "Initial content",
	}
	if err := idx.Index(doc1); err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	// Update with new content
	doc2 := &Document{
		ID:      "nats-concepts",
		Title:   "NATS Concepts Updated",
		Content: "Updated content with new information",
	}
	if err := idx.Index(doc2); err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	// Should still have only 1 document
	if idx.Count() != 1 {
		t.Errorf("Expected 1 document after update, got %d", idx.Count())
	}

	// Retrieved document should have updated content
	retrieved, _ := idx.Get("nats-concepts")
	if retrieved.Title != doc2.Title {
		t.Errorf("Expected updated title '%s', got '%s'", doc2.Title, retrieved.Title)
	}
}

func TestDocumentationIndexSearchAfterIndexing(t *testing.T) {
	idx := NewDocumentationIndex()

	// Index multiple documents
	docs := []*Document{
		{
			ID:      "nats-concepts",
			Title:   "NATS Concepts",
			Content: "NATS is a messaging system for cloud native applications",
		},
		{
			ID:      "jetstream",
			Title:   "JetStream",
			Content: "JetStream is the persistence layer for NATS",
		},
		{
			ID:      "security",
			Title:   "Security",
			Content: "NATS provides authentication and authorization",
		},
	}

	for _, doc := range docs {
		err := idx.Index(doc)
		if err != nil {
			t.Fatalf("Failed to index document %s: %v", doc.ID, err)
		}
	}

	// Search should work after indexing
	results, err := idx.Search("NATS messaging", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results, got none")
	}

	// First result should be most relevant (contains both "NATS" and "messaging")
	if len(results) > 0 && results[0].DocumentID != "nats-concepts" {
		t.Errorf("Expected 'nats-concepts' as top result, got '%s'", results[0].DocumentID)
	}
}

func TestDocumentationIndexEmptyQuery(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "test",
		Title:   "Test",
		Content: "Test content",
	}
	if err := idx.Index(doc); err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	results, err := idx.Search("", 10)
	if err == nil {
		t.Error("Expected error for empty query, got nil")
	}
	if len(results) != 0 {
		t.Errorf("Expected no results for empty query, got %d", len(results))
	}
}

func TestDocumentationIndexSearchLimit(t *testing.T) {
	idx := NewDocumentationIndex()

	// Index multiple documents
	for i := 0; i < 10; i++ {
		doc := &Document{
			ID:      fmt.Sprintf("doc%d", i),
			Title:   fmt.Sprintf("Document %d", i),
			Content: "NATS messaging system",
		}
		if err := idx.Index(doc); err != nil {
			t.Fatalf("Failed to index document: %v", err)
		}
	}

	// Search with limit of 5
	results, err := idx.Search("NATS", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) > 5 {
		t.Errorf("Expected at most 5 results, got %d", len(results))
	}
}

func TestDocumentationIndexCaseInsensitiveSearch(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "test",
		Title:   "Test Document",
		Content: "NATS Messaging System",
	}
	if err := idx.Index(doc); err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	// Search with different cases
	results1, _ := idx.Search("nats", 10)
	results2, _ := idx.Search("NATS", 10)
	results3, _ := idx.Search("NaTs", 10)

	// All should return the same document
	if len(results1) != len(results2) || len(results2) != len(results3) {
		t.Errorf("Case-insensitive search failed: got %d, %d, %d results",
			len(results1), len(results2), len(results3))
	}

	if len(results1) > 0 && results1[0].DocumentID != "test" {
		t.Errorf("Expected document 'test', got '%s'", results1[0].DocumentID)
	}
}

// Additional unit tests for task 5.5: Indexing operations edge cases

func TestDocumentationIndexNilDocument(t *testing.T) {
	idx := NewDocumentationIndex()

	err := idx.Index(nil)
	if err == nil {
		t.Error("Expected error when indexing nil document, got nil")
	}
}

func TestDocumentationIndexEmptyID(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "",
		Title:   "Test",
		Content: "Test content",
	}

	err := idx.Index(doc)
	if err == nil {
		t.Error("Expected error when indexing document with empty ID, got nil")
	}
}

func TestDocumentationIndexWithSections(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "test-doc",
		Title:   "Test Document",
		Content: "Main content",
		Sections: []Section{
			{Heading: "Introduction", Content: "Intro content", Level: 1},
			{Heading: "Details", Content: "Detail content", Level: 2},
		},
	}

	err := idx.Index(doc)
	if err != nil {
		t.Fatalf("Failed to index document with sections: %v", err)
	}

	// Search for term in section heading
	results, err := idx.Search("introduction", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find document by section heading")
	}

	// Search for term in section content
	results, err = idx.Search("detail", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find document by section content")
	}
}

func TestDocumentationIndexLargeContent(t *testing.T) {
	idx := NewDocumentationIndex()

	// Create a document with large content
	largeContent := strings.Repeat("NATS messaging system provides high performance. ", 1000)
	doc := &Document{
		ID:      "large-doc",
		Title:   "Large Document",
		Content: largeContent,
	}

	err := idx.Index(doc)
	if err != nil {
		t.Fatalf("Failed to index large document: %v", err)
	}

	// Search should still work
	results, err := idx.Search("NATS", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find large document")
	}

	// Summary should be truncated
	if len(results[0].Summary) > 250 {
		t.Errorf("Summary too long: %d characters", len(results[0].Summary))
	}
}

func TestDocumentationIndexSpecialCharacters(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "special-chars",
		Title:   "Special Characters: Test!",
		Content: "Content with special chars: @#$% and punctuation.",
	}

	err := idx.Index(doc)
	if err != nil {
		t.Fatalf("Failed to index document with special characters: %v", err)
	}

	// Search should work despite special characters
	results, err := idx.Search("special", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find document with special characters")
	}
}

func TestDocumentationIndexMultipleDocumentsRelevanceRanking(t *testing.T) {
	idx := NewDocumentationIndex()

	// Index multiple documents with varying relevance
	docs := []*Document{
		{
			ID:      "doc1",
			Title:   "NATS Overview",
			Content: "NATS is a messaging system. NATS provides high performance.",
		},
		{
			ID:      "doc2",
			Title:   "JetStream Guide",
			Content: "JetStream is part of NATS. It provides persistence.",
		},
		{
			ID:      "doc3",
			Title:   "Security",
			Content: "Security features for messaging systems.",
		},
	}

	for _, doc := range docs {
		if err := idx.Index(doc); err != nil {
			t.Fatalf("Failed to index document %s: %v", doc.ID, err)
		}
	}

	// Search for "NATS" - doc1 should rank higher (appears more frequently)
	results, err := idx.Search("NATS", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d", len(results))
	}

	// doc1 should be first (has "NATS" in title and multiple times in content)
	if results[0].DocumentID != "doc1" {
		t.Errorf("Expected doc1 to rank first, got %s", results[0].DocumentID)
	}

	// Relevance scores should be descending
	for i := 1; i < len(results); i++ {
		if results[i].Relevance > results[i-1].Relevance {
			t.Errorf("Results not sorted by relevance: result[%d]=%.2f > result[%d]=%.2f",
				i, results[i].Relevance, i-1, results[i-1].Relevance)
		}
	}
}

func TestDocumentationIndexEmptyContent(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "empty-content",
		Title:   "Empty Content Document",
		Content: "",
	}

	if err := idx.Index(doc); err != nil {
		t.Fatalf("Failed to index document with empty content: %v", err)
	}

	// Should still be able to find by title
	results, err := idx.Search("empty", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find document by title even with empty content")
	}
}

func TestDocumentationIndexNoMatchingDocuments(t *testing.T) {
	idx := NewDocumentationIndex()

	doc := &Document{
		ID:      "test-doc",
		Title:   "Test Document",
		Content: "Some content here",
	}

	if err := idx.Index(doc); err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	// Search for term that doesn't exist
	results, err := idx.Search("nonexistent", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected no results for non-matching query, got %d", len(results))
	}
}

func TestIndex_GetWithAlias(t *testing.T) {
	idx := NewDocumentationIndex()
	doc := &Document{ID: "new", Title: "New", Content: "content"}
	if err := idx.Index(doc); err != nil {
		t.Fatalf("Index returned error: %v", err)
	}
	idx.SetAliases(map[string]string{"old": "new"})

	got, err := idx.Get("old")
	if err != nil {
		t.Fatalf("Get alias returned error: %v", err)
	}
	if got.ID != "new" {
		t.Fatalf("expected canonical doc new, got %q", got.ID)
	}
}

func TestIndex_GetAliasMissingTarget(t *testing.T) {
	idx := NewDocumentationIndex()
	idx.SetAliases(map[string]string{"old": "missing"})

	_, err := idx.Get("old")
	if err == nil {
		t.Fatal("expected missing target error")
	}
}

func TestIndex_SetAliasesNilClears(t *testing.T) {
	idx := NewDocumentationIndex()
	doc := &Document{ID: "new", Title: "New", Content: "content"}
	if err := idx.Index(doc); err != nil {
		t.Fatalf("Index returned error: %v", err)
	}
	idx.SetAliases(map[string]string{"old": "new"})
	idx.SetAliases(nil)

	_, err := idx.Get("old")
	if err == nil {
		t.Fatal("expected alias to be cleared")
	}
}

func TestIndex_SetAliasesCopiesInput(t *testing.T) {
	idx := NewDocumentationIndex()
	doc := &Document{ID: "new", Title: "New", Content: "content"}
	if err := idx.Index(doc); err != nil {
		t.Fatalf("Index returned error: %v", err)
	}
	aliases := map[string]string{"old": "new"}
	idx.SetAliases(aliases)
	aliases["old"] = "missing"

	got, err := idx.Get("old")
	if err != nil {
		t.Fatalf("Get alias returned error: %v", err)
	}
	if got.ID != "new" {
		t.Fatalf("expected copied alias to still point to new, got %q", got.ID)
	}
}

func TestIndex_CanonicalGetUnchanged(t *testing.T) {
	idx := NewDocumentationIndex()
	doc := &Document{ID: "canonical", Title: "Canonical", Content: "content"}
	if err := idx.Index(doc); err != nil {
		t.Fatalf("Index returned error: %v", err)
	}

	got, err := idx.Get("canonical")
	if err != nil {
		t.Fatalf("Get canonical returned error: %v", err)
	}
	if got.ID != "canonical" {
		t.Fatalf("expected canonical doc, got %q", got.ID)
	}
}
