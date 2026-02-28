package storage

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v4"

	"github.com/Benny93/axon-go/internal/graph"
)

// Key prefixes for FTS
const (
	prefixFTSToken = "fts:t:" // fts:t:token:nodeID -> frequency
	prefixFTSMeta  = "fts:m:" // fts:m:nodeID -> serialized metadata
)

// FTSIndex is a simple inverted index for full-text search.
type FTSIndex struct {
	db *badger.DB
}

// NewFTSIndex creates a new FTS index using the given BadgerDB instance.
func NewFTSIndex(db *badger.DB) *FTSIndex {
	return &FTSIndex{db: db}
}

// tokenize splits text into searchable tokens.
// Handles camelCase, snake_case, dot notation, and other code patterns.
func tokenize(text string) []string {
	if text == "" {
		return []string{""}
	}

	tokens := make(map[string]bool)

	// Add full text as lowercase token
	tokens[strings.ToLower(text)] = true

	// Split on common separators (_, ., -, space)
	parts := regexp.MustCompile(`[_\.\-\s]+`).Split(text, -1)
	for _, part := range parts {
		if len(part) > 0 {
			tokens[strings.ToLower(part)] = true
		}
	}

	// Split camelCase: "UserService" -> "User", "Service"
	camelSplit := regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(text, "$1 $2")
	for _, part := range strings.Fields(camelSplit) {
		if len(part) > 0 {
			tokens[strings.ToLower(part)] = true
		}
	}

	// Split on number boundaries: "HTTP2" -> "HTTP", "2"
	numSplit := regexp.MustCompile(`([a-zA-Z])(\d)`).ReplaceAllString(text, "$1 $2")
	numSplit = regexp.MustCompile(`(\d)([a-zA-Z])`).ReplaceAllString(numSplit, "$1 $2")
	for _, part := range strings.Fields(numSplit) {
		if len(part) > 0 {
			tokens[strings.ToLower(part)] = true
		}
	}

	// Remove empty tokens
	result := make([]string, 0, len(tokens))
	for token := range tokens {
		if token != "" {
			result = append(result, token)
		}
	}

	return result
}

// IndexNode adds or updates a node in the FTS index.
func (f *FTSIndex) IndexNode(node *graph.GraphNode) error {
	if f.db == nil {
		return nil // Index not initialized
	}

	txn := f.db.NewTransaction(true)
	defer txn.Discard()

	// Delete old tokens for this node (for updates)
	f.deleteNodeTokens(txn, node.ID)

	// Tokenize searchable fields (name, signature, content)
	text := node.Name + " " + node.Signature + " " + node.Content
	tokens := tokenize(text)

	// Count token frequencies
	tokenFreq := make(map[string]int)
	for _, token := range tokens {
		tokenFreq[token]++
	}

	// Store token frequencies
	for token, freq := range tokenFreq {
		key := fmt.Sprintf("%s%s:%s", prefixFTSToken, token, node.ID)
		if err := txn.Set([]byte(key), []byte(strconv.Itoa(freq))); err != nil {
			return fmt.Errorf("setting token index: %w", err)
		}
	}

	// Store metadata for search results
	meta := map[string]any{
		"id":    node.ID,
		"name":  node.Name,
		"label": string(node.Label),
		"path":  node.FilePath,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	metaKey := fmt.Sprintf("%s%s", prefixFTSMeta, node.ID)
	if err := txn.Set([]byte(metaKey), metaJSON); err != nil {
		return fmt.Errorf("setting metadata: %w", err)
	}

	return txn.Commit()
}

// deleteNodeTokens removes all token indexes for a node.
func (f *FTSIndex) deleteNodeTokens(txn *badger.Txn, nodeID string) error {
	// Find all tokens for this node by scanning
	opts := badger.DefaultIteratorOptions
	prefix := []byte(fmt.Sprintf("%s", prefixFTSToken))
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()

	var keysToDelete [][]byte
	searchSuffix := ":" + nodeID

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		key := string(item.Key())
		if strings.HasSuffix(key, searchSuffix) {
			keysToDelete = append(keysToDelete, item.Key())
		}
	}

	// Delete all matching keys
	for _, key := range keysToDelete {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}

	return nil
}

// Search performs full-text search with simple TF scoring.
func (f *FTSIndex) Search(query string, limit int) ([]SearchResult, error) {
	if f.db == nil {
		return []SearchResult{}, nil
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return []SearchResult{}, nil
	}

	// Collect matching nodes with scores
	nodeScores := make(map[string]float64)

	txn := f.db.NewTransaction(false)
	defer txn.Discard()

	for _, token := range queryTokens {
		prefix := fmt.Sprintf("%s%s:", prefixFTSToken, token)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			// Extract nodeID from key: fts:t:token:nodeID
			nodeID := strings.TrimPrefix(key, prefix)

			var freq int
			_ = item.Value(func(val []byte) error {
				freq, _ = strconv.Atoi(string(val))
				return nil
			})

			// Simple TF scoring
			score := float64(freq)
			nodeScores[nodeID] += score
		}
		it.Close()
	}

	// Convert to results
	var results []SearchResult
	for nodeID, score := range nodeScores {
		if score <= 0 {
			continue
		}

		metaItem, err := txn.Get([]byte(fmt.Sprintf("%s%s", prefixFTSMeta, nodeID)))
		if err != nil {
			continue // Node metadata not found
		}

		var meta map[string]any
		_ = metaItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &meta)
		})

		results = append(results, SearchResult{
			NodeID:   nodeID,
			Score:    score,
			NodeName: getString(meta, "name"),
			FilePath: getString(meta, "path"),
			Label:    getString(meta, "label"),
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// getString safely extracts a string from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RemoveNode removes a node from the FTS index.
func (f *FTSIndex) RemoveNode(nodeID string) error {
	if f.db == nil {
		return nil
	}

	txn := f.db.NewTransaction(true)
	defer txn.Discard()

	// Delete token indexes
	if err := f.deleteNodeTokens(txn, nodeID); err != nil {
		return err
	}

	// Delete metadata
	metaKey := fmt.Sprintf("%s%s", prefixFTSMeta, nodeID)
	if err := txn.Delete([]byte(metaKey)); err != nil {
		return err
	}

	return txn.Commit()
}

// IndexSize returns the number of indexed tokens (for debugging/testing).
func (f *FTSIndex) IndexSize() (int, error) {
	if f.db == nil {
		return 0, nil
	}

	count := 0
	txn := f.db.NewTransaction(false)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixFTSToken)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		count++
	}

	return count, nil
}
