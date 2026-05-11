// Package index provides in-memory documentation indexing and search functionality
// using TF-IDF relevance ranking for fast and accurate search results.
package index

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Document represents a single documentation page with all its content and metadata.
type Document struct {
	ID          string    `json:"id"`           // Unique identifier (typically URL path)
	Title       string    `json:"title"`        // Document title
	URL         string    `json:"url"`          // Full URL to the documentation page
	Content     string    `json:"content"`      // Full text content
	Sections    []Section `json:"sections"`     // Subsections within the document
	LastUpdated time.Time `json:"last_updated"` // When the document was last fetched/updated
}

// Section represents a subsection within a document with its own heading and content.
type Section struct {
	Heading string `json:"heading"` // Section heading text
	Content string `json:"content"` // Section content
	Level   int    `json:"level"`   // Heading level (1-6 for h1-h6)
}

// DocumentStore provides thread-safe storage for documents with concurrent read access.
type DocumentStore struct {
	documents map[string]*Document // Map of document ID to document
	mu        sync.RWMutex         // Read-write mutex for thread safety
}

// NewDocumentStore creates a new empty document store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		documents: make(map[string]*Document),
	}
}

// AddDocument adds or updates a document in the store.
// If a document with the same ID already exists, it will be replaced.
func (ds *DocumentStore) AddDocument(doc *Document) error {
	if doc == nil {
		return fmt.Errorf("cannot add nil document")
	}
	if doc.ID == "" {
		return fmt.Errorf("document ID cannot be empty")
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.documents[doc.ID] = doc
	return nil
}

// GetDocument retrieves a document by its ID.
// Returns an error if the document is not found.
func (ds *DocumentStore) GetDocument(id string) (*Document, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	doc, exists := ds.documents[id]
	if !exists {
		return nil, fmt.Errorf("document not found: %s", id)
	}

	return doc, nil
}

// GetAllDocuments returns all documents in the store.
func (ds *DocumentStore) GetAllDocuments() []*Document {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	docs := make([]*Document, 0, len(ds.documents))
	for _, doc := range ds.documents {
		docs = append(docs, doc)
	}
	return docs
}

// Count returns the number of documents in the store.
func (ds *DocumentStore) Count() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return len(ds.documents)
}

// SearchIndex provides TF-IDF based search functionality for documents.
// It maintains term frequencies and document frequencies for relevance ranking.
type SearchIndex struct {
	// termFrequency maps document ID -> term -> frequency count
	termFrequency map[string]map[string]int

	// documentFrequency maps term -> number of documents containing the term
	documentFrequency map[string]int

	// totalDocuments is the total number of indexed documents
	totalDocuments int

	mu sync.RWMutex // Read-write mutex for thread safety
}

// NewSearchIndex creates a new empty search index.
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		termFrequency:     make(map[string]map[string]int),
		documentFrequency: make(map[string]int),
		totalDocuments:    0,
	}
}

// AddDocument indexes a document's content for searching.
// The content is tokenized, normalized to lowercase, and term frequencies are calculated.
func (si *SearchIndex) AddDocument(docID string, content string) error {
	if docID == "" {
		return fmt.Errorf("document ID cannot be empty")
	}

	si.mu.Lock()
	defer si.mu.Unlock()

	// Tokenize and normalize content
	tokens := tokenize(content)

	// Check if this is an update (document already exists)
	isUpdate := false
	if _, exists := si.termFrequency[docID]; exists {
		isUpdate = true
		// Remove old document frequencies
		for term := range si.termFrequency[docID] {
			si.documentFrequency[term]--
			if si.documentFrequency[term] == 0 {
				delete(si.documentFrequency, term)
			}
		}
	}

	// Calculate term frequencies for this document
	termFreq := make(map[string]int)
	for _, token := range tokens {
		termFreq[token]++
	}

	// Update document frequencies
	for term := range termFreq {
		if _, exists := si.documentFrequency[term]; !exists {
			si.documentFrequency[term] = 0
		}
		si.documentFrequency[term]++
	}

	// Store term frequencies for this document
	si.termFrequency[docID] = termFreq

	// Update total document count
	if !isUpdate {
		si.totalDocuments++
	}

	return nil
}

// GetTermFrequency returns the frequency of a term in a specific document.
func (si *SearchIndex) GetTermFrequency(docID string, term string) int {
	si.mu.RLock()
	defer si.mu.RUnlock()

	term = normalize(term)
	if termFreq, exists := si.termFrequency[docID]; exists {
		return termFreq[term]
	}
	return 0
}

// GetDocumentFrequency returns the number of documents containing a term.
func (si *SearchIndex) GetDocumentFrequency(term string) int {
	si.mu.RLock()
	defer si.mu.RUnlock()

	term = normalize(term)
	return si.documentFrequency[term]
}

// CalculateTFIDF calculates the TF-IDF score for a term in a document.
// TF-IDF = Term Frequency * Inverse Document Frequency
// IDF = log(total documents / documents containing term)
func (si *SearchIndex) CalculateTFIDF(docID string, term string) float64 {
	si.mu.RLock()
	defer si.mu.RUnlock()

	return si.calculateTFIDFUnsafe(docID, term)
}

// calculateTFIDFUnsafe calculates TF-IDF without locking (internal use only)
func (si *SearchIndex) calculateTFIDFUnsafe(docID string, term string) float64 {
	term = normalize(term)

	// Get term frequency in this document
	tf := 0
	if termFreq, exists := si.termFrequency[docID]; exists {
		tf = termFreq[term]
	}

	if tf == 0 {
		return 0.0
	}

	// Get document frequency for this term
	df := si.documentFrequency[term]
	if df == 0 {
		return 0.0
	}

	// Calculate IDF: log(N / df)
	// Using natural log and adding 1 to avoid division by zero
	idf := math.Log(float64(si.totalDocuments) / float64(df))

	// Calculate TF-IDF
	return float64(tf) * idf
}

// CalculateRelevance calculates the relevance score of a document for a query.
// The score is the sum of TF-IDF scores for all query terms.
func (si *SearchIndex) CalculateRelevance(query string, docID string) float64 {
	si.mu.RLock()
	defer si.mu.RUnlock()

	// Tokenize query
	queryTerms := tokenize(query)

	// Sum TF-IDF scores for all query terms
	score := 0.0
	for _, term := range queryTerms {
		tfidf := si.calculateTFIDFUnsafe(docID, term)
		score += tfidf
	}

	return score
}

// tokenize splits text into tokens (words), normalizes them, and removes punctuation.
func tokenize(text string) []string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Split on whitespace and punctuation
	tokens := []string{}
	var currentToken strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			currentToken.WriteRune(r)
		} else {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
		}
	}

	// Add last token if exists
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	return tokens
}

// normalize converts a term to its normalized form (lowercase).
func normalize(term string) string {
	return strings.ToLower(term)
}

// SearchResult represents a single search result with relevance information.
type SearchResult struct {
	DocumentID  string  // Document identifier
	Title       string  // Document title
	URL         string  // Document URL
	Summary     string  // Brief summary or excerpt
	Relevance   float64 // Relevance score (higher is more relevant)
	MatchedText string  // Text snippet containing matched terms
}

// DocumentationIndex combines document storage and search functionality.
// It provides a unified interface for indexing and searching documentation.
type DocumentationIndex struct {
	store       *DocumentStore
	searchIndex *SearchIndex
	aliases     map[string]string
	mu          sync.RWMutex
}

// NewDocumentationIndex creates a new documentation index.
func NewDocumentationIndex() *DocumentationIndex {
	return &DocumentationIndex{
		store:       NewDocumentStore(),
		searchIndex: NewSearchIndex(),
		aliases:     make(map[string]string),
	}
}

// Index adds or updates a document in the index.
// The document's content is tokenized and indexed for searching.
func (di *DocumentationIndex) Index(doc *Document) error {
	if doc == nil {
		return fmt.Errorf("cannot index nil document")
	}
	if doc.ID == "" {
		return fmt.Errorf("document ID cannot be empty")
	}

	di.mu.Lock()
	defer di.mu.Unlock()

	// Add document to store
	if err := di.store.AddDocument(doc); err != nil {
		return fmt.Errorf("failed to store document: %w", err)
	}

	// Build searchable content from title and content
	searchableContent := doc.Title + " " + doc.Content

	// Add sections to searchable content
	for _, section := range doc.Sections {
		searchableContent += " " + section.Heading + " " + section.Content
	}

	// Index the content for searching
	if err := di.searchIndex.AddDocument(doc.ID, searchableContent); err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}

	return nil
}

// Get retrieves a document by its ID.
func (di *DocumentationIndex) Get(id string) (*Document, error) {
	di.mu.RLock()
	defer di.mu.RUnlock()

	if doc, err := di.store.GetDocument(id); err == nil {
		return doc, nil
	}

	canonical, ok := di.aliases[id]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", id)
	}
	return di.store.GetDocument(canonical)
}

// SetAliases replaces the alias table used by Get.
func (di *DocumentationIndex) SetAliases(aliases map[string]string) {
	di.mu.Lock()
	defer di.mu.Unlock()

	next := make(map[string]string, len(aliases))
	for alias, canonical := range aliases {
		next[alias] = canonical
	}
	di.aliases = next
}

// Search performs a full-text search and returns ranked results.
// The query is tokenized and matched against indexed documents using TF-IDF scoring.
func (di *DocumentationIndex) Search(query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}

	di.mu.RLock()
	defer di.mu.RUnlock()

	// Get all documents
	docs := di.store.GetAllDocuments()

	// Calculate relevance scores for each document
	type scoredDoc struct {
		doc   *Document
		score float64
	}

	scoredDocs := make([]scoredDoc, 0, len(docs))
	queryTerms := tokenize(query)

	for _, doc := range docs {
		// Check if document contains any query terms
		hasMatch := false
		for _, term := range queryTerms {
			if di.searchIndex.GetTermFrequency(doc.ID, term) > 0 {
				hasMatch = true
				break
			}
		}

		if hasMatch {
			score := di.searchIndex.CalculateRelevance(query, doc.ID)
			scoredDocs = append(scoredDocs, scoredDoc{doc: doc, score: score})
		}
	}

	// Sort by relevance (descending)
	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	// Apply limit
	if limit > 0 && len(scoredDocs) > limit {
		scoredDocs = scoredDocs[:limit]
	}

	// Convert to SearchResult
	results := make([]SearchResult, 0, len(scoredDocs))
	for _, sd := range scoredDocs {
		summary := generateSummary(sd.doc.Content, query, 200)
		results = append(results, SearchResult{
			DocumentID:  sd.doc.ID,
			Title:       sd.doc.Title,
			URL:         sd.doc.URL,
			Summary:     summary,
			Relevance:   sd.score,
			MatchedText: summary,
		})
	}

	return results, nil
}

// Count returns the total number of indexed documents.
func (di *DocumentationIndex) Count() int {
	di.mu.RLock()
	defer di.mu.RUnlock()

	return di.store.Count()
}

// ExportDocuments returns all documents in the index for caching purposes.
// This exports the raw documents without the search index structure.
func (di *DocumentationIndex) ExportDocuments() []*Document {
	di.mu.RLock()
	defer di.mu.RUnlock()

	return di.store.GetAllDocuments()
}

// ImportDocuments re-indexes documents from cache by adding them to the index.
// This rebuilds the TF-IDF search index from the provided documents.
func (di *DocumentationIndex) ImportDocuments(docs []*Document) error {
	if docs == nil {
		return fmt.Errorf("documents cannot be nil")
	}

	di.mu.Lock()
	defer di.mu.Unlock()

	for _, doc := range docs {
		if doc == nil {
			continue
		}
		if doc.ID == "" {
			return fmt.Errorf("document ID cannot be empty")
		}

		// Add document to store
		if err := di.store.AddDocument(doc); err != nil {
			return fmt.Errorf("failed to store document %s: %w", doc.ID, err)
		}

		// Build searchable content from title and content
		searchableContent := doc.Title + " " + doc.Content

		// Add sections to searchable content
		for _, section := range doc.Sections {
			searchableContent += " " + section.Heading + " " + section.Content
		}

		// Index the content for searching
		if err := di.searchIndex.AddDocument(doc.ID, searchableContent); err != nil {
			return fmt.Errorf("failed to index document %s: %w", doc.ID, err)
		}
	}

	return nil
}

// generateSummary creates a brief summary of content, preferring text around query terms.
func generateSummary(content string, query string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}

	// Try to find query terms in content
	queryTerms := tokenize(query)
	lowerContent := strings.ToLower(content)

	// Find first occurrence of any query term
	bestPos := -1
	for _, term := range queryTerms {
		pos := strings.Index(lowerContent, term)
		if pos >= 0 && (bestPos < 0 || pos < bestPos) {
			bestPos = pos
		}
	}

	// If query term found, center summary around it
	if bestPos >= 0 {
		start := bestPos - maxLength/2
		if start < 0 {
			start = 0
		}
		end := start + maxLength
		if end > len(content) {
			end = len(content)
			start = end - maxLength
			if start < 0 {
				start = 0
			}
		}

		summary := content[start:end]

		// Trim to word boundaries
		if start > 0 {
			if idx := strings.Index(summary, " "); idx > 0 {
				summary = "..." + summary[idx+1:]
			}
		}
		if end < len(content) {
			if idx := strings.LastIndex(summary, " "); idx > 0 {
				summary = summary[:idx] + "..."
			}
		}

		return summary
	}

	// Otherwise, return first maxLength characters
	summary := content[:maxLength]
	if idx := strings.LastIndex(summary, " "); idx > 0 {
		summary = summary[:idx] + "..."
	}

	return summary
}
