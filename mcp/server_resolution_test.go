package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

func TestMCPSymbolResolution(t *testing.T) {
	t.Parallel()

	t.Run("ResolveSymbolToNodeID", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:RunPipeline",
			Label:    graph.NodeFunction,
			Name:     "RunPipeline",
			FilePath: "test.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:HelperFunc",
			Label:    graph.NodeFunction,
			Name:     "HelperFunc",
			FilePath: "test.go",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Test exact match
		nodeID, err := resolveSymbolToNodeID(store, "RunPipeline")
		assert.NoError(t, err)
		assert.Equal(t, "function:test.go:RunPipeline", nodeID)

		// Test not found
		nodeID, err = resolveSymbolToNodeID(store, "NonExistent")
		assert.Error(t, err)
		assert.Empty(t, nodeID)
	})

	t.Run("HandleContextWithSymbolResolution", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph with callers and callees
		g := graph.NewKnowledgeGraph()
		
		// Add main function
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:main",
			Label:    graph.NodeFunction,
			Name:     "main",
			FilePath: "main.go",
		})
		
		// Add RunPipeline function
		g.AddNode(&graph.GraphNode{
			ID:       "function:pipeline.go:RunPipeline",
			Label:    graph.NodeFunction,
			Name:     "RunPipeline",
			FilePath: "pipeline.go",
		})
		
		// Add helper function
		g.AddNode(&graph.GraphNode{
			ID:       "function:helper.go:HelperFunc",
			Label:    graph.NodeFunction,
			Name:     "HelperFunc",
			FilePath: "helper.go",
		})

		// Add call relationships: main -> RunPipeline -> HelperFunc
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:pipeline.go:RunPipeline",
		})
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:pipeline.go:RunPipeline",
			Target: "function:helper.go:HelperFunc",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Test handleContext with symbol name (not node ID)
		result, err := handleContext(store, "RunPipeline")
		assert.NoError(t, err)
		assert.Contains(t, result, "Context for symbol: **RunPipeline**")
		assert.Contains(t, result, "## Callers (1)")
		assert.Contains(t, result, "main")
		assert.Contains(t, result, "## Callees (1)")
		assert.Contains(t, result, "HelperFunc")
	})

	t.Run("HandleImpactWithSymbolResolution", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph
		g := graph.NewKnowledgeGraph()
		
		// Add functions
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:main",
			Label:    graph.NodeFunction,
			Name:     "main",
			FilePath: "main.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:pipeline.go:RunPipeline",
			Label:    graph.NodeFunction,
			Name:     "RunPipeline",
			FilePath: "pipeline.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:cmd.go:AnalyzeCmd",
			Label:    graph.NodeFunction,
			Name:     "AnalyzeCmd",
			FilePath: "cmd.go",
		})

		// Add call relationships: AnalyzeCmd -> main -> RunPipeline
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:cmd.go:AnalyzeCmd",
			Target: "function:main.go:main",
		})
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:pipeline.go:RunPipeline",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Test handleImpact with symbol name (not node ID)
		result, err := handleImpact(store, "RunPipeline", 3)
		assert.NoError(t, err)
		assert.Contains(t, result, "Impact analysis for: **RunPipeline** (depth: 3)")
		assert.Contains(t, result, "## Affected Symbols")
		// Should find main and AnalyzeCmd as affected
		assert.Contains(t, result, "main")
		assert.Contains(t, result, "AnalyzeCmd")
	})

	t.Run("HandleContextNotFound", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		result, err := handleContext(store, "NonExistent")
		assert.NoError(t, err)
		assert.Contains(t, result, "Symbol 'NonExistent' not found in index")
	})

	t.Run("HandleImpactNotFound", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		result, err := handleImpact(store, "NonExistent", 3)
		assert.NoError(t, err)
		assert.Contains(t, result, "Symbol 'NonExistent' not found in index")
	})
}
