// Package storage provides the storage backend for Axon.
package storage

import (
	"context"
	"sync"

	"github.com/Benny93/axon-go/internal/graph"
)

// MemoryBackend is an in-memory implementation of StorageBackend for testing.
type MemoryBackend struct {
	mu         sync.RWMutex
	nodes      map[string]*graph.GraphNode
	embeddings map[string][]float32
	indexed    bool
	ftsIndexed bool
}

// NewMemoryBackend creates a new in-memory storage backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		nodes:      make(map[string]*graph.GraphNode),
		embeddings: make(map[string][]float32),
	}
}

// Initialize implements StorageBackend.
func (m *MemoryBackend) Initialize(path string, readOnly bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexed = true
	return nil
}

// Close implements StorageBackend.
func (m *MemoryBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = nil
	m.embeddings = nil
	return nil
}

// GetDeadCode implements StorageBackend.
func (m *MemoryBackend) GetDeadCode(ctx context.Context) ([]*graph.GraphNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var deadNodes []*graph.GraphNode
	for _, node := range m.nodes {
		if node.IsDead {
			deadNodes = append(deadNodes, node)
		}
	}
	return deadNodes, nil
}

// BulkLoad implements StorageBackend.
func (m *MemoryBackend) BulkLoad(ctx context.Context, g *graph.KnowledgeGraph) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for node := range g.IterNodes() {
		m.nodes[node.ID] = node
	}
	m.indexed = true
	return nil
}

// AddNodes implements StorageBackend.
func (m *MemoryBackend) AddNodes(ctx context.Context, nodes []*graph.GraphNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, node := range nodes {
		m.nodes[node.ID] = node
	}
	return nil
}

// RemoveNodesByFile implements StorageBackend.
func (m *MemoryBackend) RemoveNodesByFile(ctx context.Context, filePath string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, node := range m.nodes {
		if node.FilePath == filePath {
			delete(m.nodes, id)
			count++
		}
	}
	return count, nil
}

// GetNode implements StorageBackend.
func (m *MemoryBackend) GetNode(ctx context.Context, nodeID string) (*graph.GraphNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodes[nodeID], nil
}

// GetNodesByLabel implements StorageBackend.
func (m *MemoryBackend) GetNodesByLabel(ctx context.Context, label string) []*graph.GraphNode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var nodes []*graph.GraphNode
	for _, node := range m.nodes {
		if string(node.Label) == label {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// AddRelationships implements StorageBackend.
func (m *MemoryBackend) AddRelationships(ctx context.Context, rels []*graph.GraphRelationship) error {
	// In-memory backend doesn't store relationships for simplicity
	return nil
}

// GetCallers implements StorageBackend.
func (m *MemoryBackend) GetCallers(ctx context.Context, nodeID string) ([]*graph.GraphNode, error) {
	return nil, nil
}

// GetCallees implements StorageBackend.
func (m *MemoryBackend) GetCallees(ctx context.Context, nodeID string) ([]*graph.GraphNode, error) {
	return nil, nil
}

// Traverse implements StorageBackend.
func (m *MemoryBackend) Traverse(ctx context.Context, startID string, depth int, direction string) ([]*graph.GraphNode, error) {
	return nil, nil
}

// FTSSearch implements StorageBackend.
func (m *MemoryBackend) FTSSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []SearchResult
	for _, node := range m.nodes {
		if len(results) >= limit {
			break
		}
		results = append(results, SearchResult{
			NodeID:   node.ID,
			NodeName: node.Name,
			FilePath: node.FilePath,
			Label:    string(node.Label),
			Score:    1.0,
		})
	}
	return results, nil
}

// VectorSearch implements StorageBackend.
func (m *MemoryBackend) VectorSearch(ctx context.Context, vector []float32, limit int) ([]SearchResult, error) {
	return nil, nil
}

// RebuildFTSIndexes implements StorageBackend.
func (m *MemoryBackend) RebuildFTSIndexes(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ftsIndexed = true
	return nil
}

// StoreEmbeddings implements StorageBackend.
func (m *MemoryBackend) StoreEmbeddings(ctx context.Context, embeddings []NodeEmbedding) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, emb := range embeddings {
		m.embeddings[emb.NodeID] = emb.Embedding
	}
	return nil
}

// IsIndexed returns true if the backend has been initialized.
func (m *MemoryBackend) IsIndexed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.indexed
}

// IsFTSIndexed returns true if FTS indexes have been built.
func (m *MemoryBackend) IsFTSIndexed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ftsIndexed
}

// GetEmbedding returns the embedding for a node.
func (m *MemoryBackend) GetEmbedding(nodeID string) []float32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.embeddings[nodeID]
}

// NodeCount returns the number of stored nodes.
func (m *MemoryBackend) NodeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}

// HybridSearch combines FTS and vector search using RRF.
func (m *MemoryBackend) HybridSearch(ctx context.Context, query string, queryVector []float32, limit int) ([]HybridSearchResult, error) {
	return HybridSearch(ctx, m, query, queryVector, limit, 60)
}
