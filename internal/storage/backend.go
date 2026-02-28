// Package storage provides the storage backend interface for Axon.
//
// It defines the StorageBackend protocol that all storage implementations
// must satisfy, along with common types used across backends.
package storage

import (
	"context"

	"github.com/Benny93/axon-go/internal/graph"
)

// SearchResult represents a search result from the storage backend.
type SearchResult struct {
	// NodeID is the ID of the matching node.
	NodeID string

	// Score is the relevance score (higher is better).
	Score float64

	// NodeName is the name of the node.
	NodeName string

	// FilePath is the file path of the node.
	FilePath string

	// Label is the node label.
	Label string

	// Snippet is a code snippet or content excerpt.
	Snippet string
}

// NodeEmbedding represents a vector embedding for a node.
type NodeEmbedding struct {
	// NodeID is the ID of the node.
	NodeID string

	// Embedding is the vector embedding (e.g., 384 dimensions).
	Embedding []float32
}

// HybridSearchResult represents a result from hybrid search.
type HybridSearchResult struct {
	// NodeID is the ID of the matching node.
	NodeID string

	// Score is the RRF fused score (higher is better).
	Score float64

	// NodeName is the name of the node.
	NodeName string

	// FilePath is the file path of the node.
	FilePath string

	// Label is the node label.
	Label string

	// Snippet is a code snippet or content excerpt.
	Snippet string
}

// StorageBackend defines the interface for storage implementations.
//
// Implementations must be thread-safe and support concurrent access.
type StorageBackend interface {
	// Lifecycle methods

	// Initialize opens or creates the storage backend at the given path.
	// If readOnly is true, the backend is opened in read-only mode.
	Initialize(path string, readOnly bool) error

	// Close releases all resources held by the backend.
	Close() error

	// Bulk operations

	// BulkLoad replaces the entire store with the contents of the graph.
	BulkLoad(ctx context.Context, g *graph.KnowledgeGraph) error

	// Node operations

	// AddNodes inserts nodes into the storage.
	AddNodes(ctx context.Context, nodes []*graph.GraphNode) error

	// RemoveNodesByFile deletes all nodes whose file path matches.
	// Returns the number of nodes removed.
	RemoveNodesByFile(ctx context.Context, filePath string) (int, error)

	// GetNode returns a single node by ID, or nil if not found.
	GetNode(ctx context.Context, nodeID string) (*graph.GraphNode, error)

	// GetNodesByLabel returns all nodes with the given label.
	GetNodesByLabel(ctx context.Context, label string) []*graph.GraphNode

	// Relationship operations

	// AddRelationships inserts relationships into the storage.
	AddRelationships(ctx context.Context, rels []*graph.GraphRelationship) error

	// Graph traversal

	// GetCallers returns nodes that CALL the given node.
	GetCallers(ctx context.Context, nodeID string) ([]*graph.GraphNode, error)

	// GetCallees returns nodes called by the given node.
	GetCallees(ctx context.Context, nodeID string) ([]*graph.GraphNode, error)

	// Traverse performs BFS traversal through CALLS edges.
	// Direction should be "callers" or "callees".
	Traverse(ctx context.Context, startID string, depth int, direction string) ([]*graph.GraphNode, error)

	// Search

	// FTSSearch performs full-text search using BM25.
	FTSSearch(ctx context.Context, query string, limit int) ([]SearchResult, error)

	// VectorSearch finds nodes closest to the given vector.
	VectorSearch(ctx context.Context, vector []float32, limit int) ([]SearchResult, error)

	// Maintenance

	// RebuildFTSIndexes drops and recreates all full-text search indexes.
	RebuildFTSIndexes(ctx context.Context) error

	// StoreEmbeddings persists node embeddings.
	StoreEmbeddings(ctx context.Context, embeddings []NodeEmbedding) error

	// GetDeadCode returns all nodes marked as dead code.
	GetDeadCode(ctx context.Context) ([]*graph.GraphNode, error)

	// HybridSearch combines FTS and vector search using RRF.
	HybridSearch(ctx context.Context, query string, queryVector []float32, limit int) ([]HybridSearchResult, error)
}
