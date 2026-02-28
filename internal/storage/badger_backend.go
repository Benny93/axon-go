// Package storage provides the storage backend for Axon.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/dgraph-io/badger/v4"

	"github.com/Benny93/axon-go/internal/graph"
)

// Key prefixes for different data types
const (
	prefixNode       = "n:"       // node data
	prefixRel        = "r:"       // relationship data
	prefixIncoming   = "i:in:"    // incoming relationships
	prefixOutgoing   = "i:out:"   // outgoing relationships
	prefixEmbedding  = "e:"       // embedding data
)

// BadgerBackend is a BadgerDB-backed storage implementation.
type BadgerBackend struct {
	db                *badger.DB
	initialized       bool
	mu                sync.RWMutex
	nodeCount         int
	relationshipCount int
	ftsIndex          map[string][]string // token -> []nodeID
}

// NewBadgerBackend creates a new BadgerDB backend.
func NewBadgerBackend() *BadgerBackend {
	return &BadgerBackend{}
}

// Initialize opens or creates the BadgerDB database at the given path.
func (b *BadgerBackend) Initialize(path string, readOnly bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	opts := badger.DefaultOptions(path).
		WithNumCompactors(2).
		WithNumMemtables(5).
		WithLoggingLevel(badger.ERROR) // Suppress INFO/WARNING logs

	if readOnly {
		opts = opts.WithReadOnly(true)
	}

	var err error
	b.db, err = badger.Open(opts)
	if err != nil {
		return fmt.Errorf("opening badger DB: %w", err)
	}

	b.initialized = true

	// Rebuild FTS index from database
	b.rebuildFTSIndexFromDB()

	return nil
}

// rebuildFTSIndexFromDB rebuilds the FTS index from the database.
func (b *BadgerBackend) rebuildFTSIndexFromDB() {
	b.ftsIndex = make(map[string][]string)
	b.nodeCount = 0
	b.relationshipCount = 0

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixNode)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var node graph.GraphNode
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &node)
		}); err != nil {
			continue
		}
		b.nodeCount++
		b.indexNodeForFTS(&node)
	}

	// Count relationships
	opts.Prefix = []byte(prefixRel)
	it = txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		b.relationshipCount++
	}
}

// Close releases all resources held by the backend.
func (b *BadgerBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.db == nil {
		return nil
	}

	err := b.db.Close()
	b.db = nil
	b.initialized = false
	return err
}

// BulkLoad replaces the entire store with the contents of the graph.
func (b *BadgerBackend) BulkLoad(ctx context.Context, g *graph.KnowledgeGraph) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	wb := b.db.NewWriteBatch()
	defer wb.Cancel()

	// Reset counts
	b.nodeCount = 0
	b.relationshipCount = 0
	b.ftsIndex = make(map[string][]string)

	// Add nodes
	for node := range g.IterNodes() {
		data, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("marshaling node: %w", err)
		}
		if err := wb.Set(b.nodeKey(node.ID), data); err != nil {
			return fmt.Errorf("setting node: %w", err)
		}
		b.nodeCount++

		// Build FTS index for this node
		b.indexNodeForFTS(node)
	}

	// Add relationships
	for rel := range g.IterRelationships() {
		data, err := json.Marshal(rel)
		if err != nil {
			return fmt.Errorf("marshaling relationship: %w", err)
		}
		if err := wb.Set(b.relKey(rel.ID), data); err != nil {
			return fmt.Errorf("setting relationship: %w", err)
		}
		b.relationshipCount++

		// Index for adjacency lists
		if err := b.indexRelationshipWB(wb, rel); err != nil {
			return err
		}
	}

	if err := wb.Flush(); err != nil {
		return err
	}

	return nil
}

// indexNodeForFTS adds a node to the FTS index.
func (b *BadgerBackend) indexNodeForFTS(node *graph.GraphNode) {
	// Index by name
	tokens := tokenizeForFTS(node.Name)
	for _, token := range tokens {
		b.ftsIndex[token] = append(b.ftsIndex[token], node.ID)
	}

	// Index by content (first 500 chars)
	if len(node.Content) > 0 {
		content := node.Content
		if len(content) > 500 {
			content = content[:500]
		}
		tokens = tokenizeForFTS(content)
		for _, token := range tokens {
			b.ftsIndex[token] = append(b.ftsIndex[token], node.ID)
		}
	}

	// Index by signature
	if len(node.Signature) > 0 {
		tokens = tokenizeForFTS(node.Signature)
		for _, token := range tokens {
			b.ftsIndex[token] = append(b.ftsIndex[token], node.ID)
		}
	}
}

// tokenizeForFTS splits text into searchable tokens.
func tokenizeForFTS(text string) []string {
	text = strings.ToLower(text)
	// Split on non-alphanumeric characters
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	// Filter out very short tokens
	result := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len(t) >= 2 {
			result = append(result, t)
		}
	}
	return result
}

// AddNodes inserts nodes into the storage.
func (b *BadgerBackend) AddNodes(ctx context.Context, nodes []*graph.GraphNode) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	txn := b.db.NewTransaction(true)
	defer txn.Discard()

	for _, node := range nodes {
		data, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("marshaling node: %w", err)
		}
		if err := txn.Set(b.nodeKey(node.ID), data); err != nil {
			return fmt.Errorf("setting node: %w", err)
		}
	}

	return txn.Commit()
}

// RemoveNodesByFile deletes all nodes whose file path matches.
func (b *BadgerBackend) RemoveNodesByFile(ctx context.Context, filePath string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := 0
	txn := b.db.NewTransaction(true)
	defer txn.Discard()

	// Find all nodes with this file path
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixNode)
	it := txn.NewIterator(opts)

	var keysToDelete [][]byte
	var relIDsToDelete [][]byte

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var node graph.GraphNode
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &node)
		}); err != nil {
			it.Close()
			return 0, fmt.Errorf("unmarshaling node: %w", err)
		}

		if node.FilePath == filePath {
			keysToDelete = append(keysToDelete, item.Key())
			count++

			// Also mark relationships for deletion
			// We'll find them by scanning relationship indexes
			inPrefix := []byte(fmt.Sprintf("%s%s:%s", prefixIncoming, node.ID, graph.RelCalls))
			b.collectRelationshipIDs(txn, inPrefix, &relIDsToDelete)

			outPrefix := []byte(fmt.Sprintf("%s%s:%s", prefixOutgoing, node.ID, graph.RelCalls))
			b.collectRelationshipIDs(txn, outPrefix, &relIDsToDelete)
		}
	}
	it.Close()

	// Delete nodes
	for _, key := range keysToDelete {
		if err := txn.Delete(key); err != nil {
			return count, fmt.Errorf("deleting node: %w", err)
		}
	}

	// Delete relationships
	for _, relKey := range relIDsToDelete {
		if err := txn.Delete(relKey); err != nil {
			return count, fmt.Errorf("deleting relationship: %w", err)
		}
	}

	return count, txn.Commit()
}

// collectRelationshipIDs collects relationship IDs from an index prefix.
func (b *BadgerBackend) collectRelationshipIDs(txn *badger.Txn, prefix []byte, ids *[][]byte) {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		_ = item.Value(func(val []byte) error {
			// val contains relationship ID
			relKey := b.relKey(string(val))
			*ids = append(*ids, relKey)
			return nil
		})
	}
}

// GetNode returns a single node by ID, or nil if not found.
func (b *BadgerBackend) GetNode(ctx context.Context, nodeID string) (*graph.GraphNode, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(b.nodeKey(nodeID))
	if err == badger.ErrKeyNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting node: %w", err)
	}

	var node graph.GraphNode
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &node)
	}); err != nil {
		return nil, fmt.Errorf("unmarshaling node: %w", err)
	}

	return &node, nil
}

// GetNodesByLabel returns all nodes with the given label.
func (b *BadgerBackend) GetNodesByLabel(ctx context.Context, label string) []*graph.GraphNode {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var nodes []*graph.GraphNode

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixNode)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var node graph.GraphNode
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &node)
		}); err != nil {
			continue
		}

		if string(node.Label) == label {
			nodes = append(nodes, &node)
		}
	}

	return nodes
}

// AddRelationships inserts relationships into the storage.
func (b *BadgerBackend) AddRelationships(ctx context.Context, rels []*graph.GraphRelationship) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	txn := b.db.NewTransaction(true)
	defer txn.Discard()

	for _, rel := range rels {
		data, err := json.Marshal(rel)
		if err != nil {
			return fmt.Errorf("marshaling relationship: %w", err)
		}
		if err := txn.Set(b.relKey(rel.ID), data); err != nil {
			return fmt.Errorf("setting relationship: %w", err)
		}

		// Index for adjacency lists
		if err := b.indexRelationship(txn, rel); err != nil {
			return err
		}
	}

	return txn.Commit()
}

// indexRelationship creates adjacency list indexes for a relationship.
func (b *BadgerBackend) indexRelationship(txn *badger.Txn, rel *graph.GraphRelationship) error {
	// Outgoing: source -> rel_type -> rel.ID (unique key per relationship)
	outKey := fmt.Sprintf("%s%s:%s:%s", prefixOutgoing, rel.Source, rel.Type, rel.ID)
	if err := txn.Set([]byte(outKey), []byte(rel.ID)); err != nil {
		return fmt.Errorf("setting outgoing index: %w", err)
	}

	// Incoming: target -> rel_type -> rel.ID (unique key per relationship)
	inKey := fmt.Sprintf("%s%s:%s:%s", prefixIncoming, rel.Target, rel.Type, rel.ID)
	if err := txn.Set([]byte(inKey), []byte(rel.ID)); err != nil {
		return fmt.Errorf("setting incoming index: %w", err)
	}

	return nil
}

// indexRelationshipWB creates adjacency list indexes for a relationship in a write batch.
func (b *BadgerBackend) indexRelationshipWB(wb *badger.WriteBatch, rel *graph.GraphRelationship) error {
	outKey := fmt.Sprintf("%s%s:%s:%s", prefixOutgoing, rel.Source, rel.Type, rel.ID)
	if err := wb.Set([]byte(outKey), []byte(rel.ID)); err != nil {
		return fmt.Errorf("setting outgoing index: %w", err)
	}

	inKey := fmt.Sprintf("%s%s:%s:%s", prefixIncoming, rel.Target, rel.Type, rel.ID)
	if err := wb.Set([]byte(inKey), []byte(rel.ID)); err != nil {
		return fmt.Errorf("setting incoming index: %w", err)
	}

	return nil
}

// GetCallers returns nodes that CALL the given node.
func (b *BadgerBackend) GetCallers(ctx context.Context, nodeID string) ([]*graph.GraphNode, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var callers []*graph.GraphNode

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	// Get incoming CALLS relationships
	prefix := fmt.Sprintf("%s%s:%s", prefixIncoming, nodeID, graph.RelCalls)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefix)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var relID string
		if err := item.Value(func(val []byte) error {
			relID = string(val)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("reading rel ID: %w", err)
		}

		// Get relationship
		relItem, err := txn.Get(b.relKey(relID))
		if err != nil {
			continue // Skip if relationship not found
		}

		var rel graph.GraphRelationship
		if err := relItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &rel)
		}); err != nil {
			continue
		}

		// Get source node (caller)
		callerItem, err := txn.Get(b.nodeKey(rel.Source))
		if err != nil {
			continue // Skip if caller not found
		}

		var caller graph.GraphNode
		if err := callerItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &caller)
		}); err != nil {
			continue
		}

		callers = append(callers, &caller)
	}

	return callers, nil
}

// GetCallees returns nodes called by the given node.
func (b *BadgerBackend) GetCallees(ctx context.Context, nodeID string) ([]*graph.GraphNode, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var callees []*graph.GraphNode

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	// Get outgoing CALLS relationships
	prefix := fmt.Sprintf("%s%s:%s", prefixOutgoing, nodeID, graph.RelCalls)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefix)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var relID string
		if err := item.Value(func(val []byte) error {
			relID = string(val)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("reading rel ID: %w", err)
		}

		// Get relationship
		relItem, err := txn.Get(b.relKey(relID))
		if err != nil {
			continue
		}

		var rel graph.GraphRelationship
		if err := relItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &rel)
		}); err != nil {
			continue
		}

		// Get target node (callee)
		calleeItem, err := txn.Get(b.nodeKey(rel.Target))
		if err != nil {
			continue
		}

		var callee graph.GraphNode
		if err := calleeItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &callee)
		}); err != nil {
			continue
		}

		callees = append(callees, &callee)
	}

	return callees, nil
}

// Traverse performs BFS traversal through CALLS edges.
func (b *BadgerBackend) Traverse(ctx context.Context, startID string, depth int, direction string) ([]*graph.GraphNode, error) {
	if depth > 10 {
		depth = 10 // Safety limit
	}

	visited := make(map[string]bool)
	var result []*graph.GraphNode

	type traversalItem struct {
		nodeID string
		depth  int
	}

	queue := []traversalItem{{nodeID: startID, depth: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true

		if current.nodeID != startID {
			node, err := b.getNode(current.nodeID)
			if err != nil {
				continue
			}
			if node != nil {
				result = append(result, node)
			}
		}

		if current.depth < depth {
			var neighbors []*graph.GraphNode
			var err error

			if direction == "callers" {
				neighbors, err = b.GetCallers(ctx, current.nodeID)
			} else {
				neighbors, err = b.GetCallees(ctx, current.nodeID)
			}

			if err != nil {
				continue
			}

			for _, neighbor := range neighbors {
				if !visited[neighbor.ID] {
					queue = append(queue, traversalItem{
						nodeID: neighbor.ID,
						depth:  current.depth + 1,
					})
				}
			}
		}
	}

	return result, nil
}

// getNode is a helper that gets a node without locking (caller must hold lock).
func (b *BadgerBackend) getNode(nodeID string) (*graph.GraphNode, error) {
	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(b.nodeKey(nodeID))
	if err == badger.ErrKeyNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var node graph.GraphNode
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &node)
	}); err != nil {
		return nil, err
	}

	return &node, nil
}

// FTSSearch performs full-text search using the in-memory FTS index.
func (b *BadgerBackend) FTSSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.ftsIndex == nil {
		return []SearchResult{}, nil
	}

	// Tokenize query
	queryTokens := tokenizeForFTS(query)
	if len(queryTokens) == 0 {
		return []SearchResult{}, nil
	}

	// Find matching node IDs
	nodeIDSet := make(map[string]int) // nodeID -> score (token matches)
	for _, token := range queryTokens {
		if nodeIDs, ok := b.ftsIndex[token]; ok {
			for _, nodeID := range nodeIDs {
				nodeIDSet[nodeID]++
			}
		}
	}

	if len(nodeIDSet) == 0 {
		return []SearchResult{}, nil
	}

	// Fetch matching nodes and build results
	results := make([]SearchResult, 0, len(nodeIDSet))
	for nodeID, score := range nodeIDSet {
		node, err := b.getNode(nodeID)
		if err != nil || node == nil {
			continue
		}

		snippet := node.Content
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}

		results = append(results, SearchResult{
			NodeID:   nodeID,
			Score:    float64(score),
			NodeName: node.Name,
			FilePath: node.FilePath,
			Label:    string(node.Label),
			Snippet:  snippet,
		})

		if len(results) >= limit {
			break
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// VectorSearch finds nodes closest to the given vector using cosine similarity.
func (b *BadgerBackend) VectorSearch(ctx context.Context, vector []float32, limit int) ([]SearchResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	// Scan all embeddings and compute cosine similarity
	type scoredNode struct {
		nodeID string
		score  float64
	}
	var scoredNodes []scoredNode

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixEmbedding)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var embedding []float32
		if err := item.Value(func(val []byte) error {
			// Deserialize embedding (stored as JSON array)
			return json.Unmarshal(val, &embedding)
		}); err != nil {
			continue
		}

		// Extract node ID from key
		key := string(item.Key())
		nodeID := strings.TrimPrefix(key, prefixEmbedding)

		// Compute cosine similarity
		sim := cosineSimilarity(vector, embedding)
		if sim > 0 {
			scoredNodes = append(scoredNodes, scoredNode{
				nodeID: nodeID,
				score:  float64(sim),
			})
		}
	}

	// Sort by score descending
	sort.Slice(scoredNodes, func(i, j int) bool {
		return scoredNodes[i].score > scoredNodes[j].score
	})

	// Limit results
	if len(scoredNodes) > limit {
		scoredNodes = scoredNodes[:limit]
	}

	// Fetch node details and build results
	results := make([]SearchResult, 0, len(scoredNodes))
	for _, sn := range scoredNodes {
		node, err := b.getNode(sn.nodeID)
		if err != nil || node == nil {
			continue
		}

		snippet := node.Content
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}

		results = append(results, SearchResult{
			NodeID:   node.ID,
			Score:    sn.score,
			NodeName: node.Name,
			FilePath: node.FilePath,
			Label:    string(node.Label),
			Snippet:  snippet,
		})
	}

	return results, nil
}

// StoreEmbeddings persists node embeddings.
func (b *BadgerBackend) StoreEmbeddings(ctx context.Context, embeddings []NodeEmbedding) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	txn := b.db.NewTransaction(true)
	defer txn.Discard()

	for _, emb := range embeddings {
		data, err := json.Marshal(emb.Embedding)
		if err != nil {
			return fmt.Errorf("marshaling embedding: %w", err)
		}

		key := []byte(prefixEmbedding + emb.NodeID)
		if err := txn.Set(key, data); err != nil {
			return fmt.Errorf("setting embedding: %w", err)
		}
	}

	return txn.Commit()
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// GetDeadCode returns all nodes marked as dead code.
func (b *BadgerBackend) GetDeadCode(ctx context.Context) ([]*graph.GraphNode, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var deadNodes []*graph.GraphNode

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixNode)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var node graph.GraphNode
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &node)
		}); err != nil {
			continue
		}

		if node.IsDead {
			deadNodes = append(deadNodes, &node)
		}
	}

	return deadNodes, nil
}

// RebuildFTSIndexes drops and recreates all full-text search indexes.
func (b *BadgerBackend) RebuildFTSIndexes(ctx context.Context) error {
	// Placeholder - Bleve integration will handle this
	return nil
}

// nodeKey returns the BadgerDB key for a node.
func (b *BadgerBackend) nodeKey(nodeID string) []byte {
	return []byte(prefixNode + nodeID)
}

// relKey returns the BadgerDB key for a relationship.
func (b *BadgerBackend) relKey(relID string) []byte {
	return []byte(prefixRel + relID)
}

// MCP adapter methods

// NodeCount returns the node count.
func (b *BadgerBackend) NodeCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nodeCount
}

// RelationshipCount returns the relationship count.
func (b *BadgerBackend) RelationshipCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.relationshipCount
}

// MCPNodeCount returns the node count for MCP.
func (b *BadgerBackend) MCPNodeCount() int {
	return b.NodeCount()
}

// MCPRelationshipCount returns the relationship count for MCP.
func (b *BadgerBackend) MCPRelationshipCount() int {
	return b.RelationshipCount()
}

// HybridSearch combines FTS and vector search using RRF.
func (b *BadgerBackend) HybridSearch(ctx context.Context, query string, queryVector []float32, limit int) ([]HybridSearchResult, error) {
	return HybridSearch(ctx, b, query, queryVector, limit, 60)
}
