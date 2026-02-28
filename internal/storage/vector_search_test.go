package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestVectorSearch(t *testing.T) {
	t.Parallel()

	t.Run("CosineSimilarity", func(t *testing.T) {
		t.Parallel()

		// Identical vectors
		v1 := []float32{1.0, 0.0, 0.0}
		v2 := []float32{1.0, 0.0, 0.0}
		sim := cosineSimilarity(v1, v2)
		assert.InDelta(t, 1.0, sim, 0.001)

		// Orthogonal vectors
		v3 := []float32{0.0, 1.0, 0.0}
		sim = cosineSimilarity(v1, v3)
		assert.InDelta(t, 0.0, sim, 0.001)

		// Opposite vectors
		v4 := []float32{-1.0, 0.0, 0.0}
		sim = cosineSimilarity(v1, v4)
		assert.InDelta(t, -1.0, sim, 0.001)

		// Different magnitudes but same direction
		v5 := []float32{2.0, 0.0, 0.0}
		sim = cosineSimilarity(v1, v5)
		assert.InDelta(t, 1.0, sim, 0.001)
	})

	t.Run("VectorSearch", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph with embeddings
		g := graph.NewKnowledgeGraph()
		
		// Add nodes
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:FuncA",
			Label:    graph.NodeFunction,
			Name:     "FuncA",
			FilePath: "test.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:FuncB",
			Label:    graph.NodeFunction,
			Name:     "FuncB",
			FilePath: "test.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:FuncC",
			Label:    graph.NodeFunction,
			Name:     "FuncC",
			FilePath: "test.go",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Store embeddings (3-dimensional for testing)
		embeddings := []NodeEmbedding{
			{NodeID: "function:test.go:FuncA", Embedding: []float32{1.0, 0.0, 0.0}},
			{NodeID: "function:test.go:FuncB", Embedding: []float32{0.0, 1.0, 0.0}},
			{NodeID: "function:test.go:FuncC", Embedding: []float32{0.0, 0.0, 1.0}},
		}

		err = store.StoreEmbeddings(t.Context(), embeddings)
		require.NoError(t, err)

		// Search for vector similar to FuncA
		queryVector := []float32{0.9, 0.1, 0.0}
		results, err := store.VectorSearch(t.Context(), queryVector, 2)
		require.NoError(t, err)

		assert.Len(t, results, 2)
		// First result should be FuncA (most similar)
		assert.Equal(t, "function:test.go:FuncA", results[0].NodeID)
		assert.Greater(t, results[0].Score, results[1].Score)
	})

	t.Run("VectorSearchEmpty", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		results, err := store.VectorSearch(t.Context(), []float32{1.0, 0.0, 0.0}, 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("VectorSearchLimit", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create graph
		g := graph.NewKnowledgeGraph()
		for i := 0; i < 10; i++ {
			g.AddNode(&graph.GraphNode{
				ID:       "function:test.go:Func" + string(rune('A'+i)),
				Label:    graph.NodeFunction,
				Name:     "Func" + string(rune('A'+i)),
				FilePath: "test.go",
			})
		}

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Store embeddings
		embeddings := make([]NodeEmbedding, 10)
		for i := 0; i < 10; i++ {
			embeddings[i] = NodeEmbedding{
				NodeID:    "function:test.go:Func" + string(rune('A'+i)),
				Embedding: []float32{float32(i), 0.0, 0.0},
			}
		}

		err = store.StoreEmbeddings(t.Context(), embeddings)
		require.NoError(t, err)

		// Search with limit
		results, err := store.VectorSearch(t.Context(), []float32{5.0, 0.0, 0.0}, 3)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("VectorSearchResultFields", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:TestFunc",
			Label:     graph.NodeFunction,
			Name:      "TestFunc",
			FilePath:  "test.go",
			Content:   "func TestFunc() {}",
			Signature: "TestFunc()",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		err = store.StoreEmbeddings(t.Context(), []NodeEmbedding{
			{NodeID: "function:test.go:TestFunc", Embedding: []float32{1.0, 0.0, 0.0}},
		})
		require.NoError(t, err)

		results, err := store.VectorSearch(t.Context(), []float32{1.0, 0.0, 0.0}, 1)
		require.NoError(t, err)
		require.Len(t, results, 1)

		// Verify result fields
		assert.Equal(t, "function:test.go:TestFunc", results[0].NodeID)
		assert.Equal(t, "TestFunc", results[0].NodeName)
		assert.Equal(t, "test.go", results[0].FilePath)
		assert.Equal(t, "function", results[0].Label)
		assert.Greater(t, results[0].Score, float64(0))
	})
}

func TestStoreEmbeddings(t *testing.T) {
	t.Parallel()

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create nodes first
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{ID: "node1", Label: graph.NodeFunction, Name: "Func1", FilePath: "test.go"})
		g.AddNode(&graph.GraphNode{ID: "node2", Label: graph.NodeFunction, Name: "Func2", FilePath: "test.go"})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		embeddings := []NodeEmbedding{
			{NodeID: "node1", Embedding: []float32{1.0, 2.0, 3.0}},
			{NodeID: "node2", Embedding: []float32{4.0, 5.0, 6.0}},
		}

		err = store.StoreEmbeddings(t.Context(), embeddings)
		require.NoError(t, err)

		// Verify embeddings were stored by searching
		results, err := store.VectorSearch(t.Context(), []float32{1.0, 2.0, 3.0}, 10)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("StoreEmptyEmbeddings", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		err = store.StoreEmbeddings(t.Context(), []NodeEmbedding{})
		assert.NoError(t, err)
	})

	t.Run("UpsertEmbeddings", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create node first
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{ID: "node1", Label: graph.NodeFunction, Name: "Func1", FilePath: "test.go"})
		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Store initial embedding
		err = store.StoreEmbeddings(t.Context(), []NodeEmbedding{
			{NodeID: "node1", Embedding: []float32{1.0, 0.0, 0.0}},
		})
		require.NoError(t, err)

		// Update embedding
		err = store.StoreEmbeddings(t.Context(), []NodeEmbedding{
			{NodeID: "node1", Embedding: []float32{0.0, 1.0, 0.0}},
		})
		require.NoError(t, err)

		// Search should return updated embedding
		results, err := store.VectorSearch(t.Context(), []float32{0.0, 1.0, 0.0}, 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "node1", results[0].NodeID)
	})
}
