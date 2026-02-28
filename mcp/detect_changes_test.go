package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

func TestHandleDetectChanges(t *testing.T) {
	t.Parallel()

	t.Run("DetectsChangedSymbols", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph with changed file
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:changed.go:FuncA",
			Label:     graph.NodeFunction,
			Name:      "FuncA",
			FilePath:  "changed.go",
			Content:   "func FuncA() {}",
			Signature: "FuncA()",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:changed.go:FuncB",
			Label:     graph.NodeFunction,
			Name:      "FuncB",
			FilePath:  "changed.go",
			Content:   "func FuncB() {}",
			Signature: "FuncB()",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:unchanged.go:FuncC",
			Label:     graph.NodeFunction,
			Name:      "FuncC",
			FilePath:  "unchanged.go",
			Content:   "func FuncC() {}",
			Signature: "FuncC()",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Detect changes in changed.go
		result, err := handleDetectChanges(store, []string{"changed.go"})
		require.NoError(t, err)

		assert.Contains(t, result, "changed.go")
		assert.Contains(t, result, "FuncA")
		assert.Contains(t, result, "FuncB")
		assert.NotContains(t, result, "FuncC")
	})

	t.Run("HandlesNoChanges", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		result, err := handleDetectChanges(store, []string{})
		require.NoError(t, err)

		assert.Contains(t, result, "No changed files provided")
	})

	t.Run("HandlesNonExistentFile", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		result, err := handleDetectChanges(store, []string{"nonexistent.go"})
		require.NoError(t, err)

		assert.Contains(t, result, "No symbols found")
	})

	t.Run("ShowsImpactAnalysis", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph with call relationships
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:changed.go:ChangedFunc",
			Label:     graph.NodeFunction,
			Name:      "ChangedFunc",
			FilePath:  "changed.go",
			Content:   "func ChangedFunc() {}",
			Signature: "ChangedFunc()",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:caller.go:CallerFunc",
			Label:     graph.NodeFunction,
			Name:      "CallerFunc",
			FilePath:  "caller.go",
			Content:   "func CallerFunc() {}",
			Signature: "CallerFunc()",
		})

		// CallerFunc calls ChangedFunc
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:caller.go:CallerFunc",
			Target: "function:changed.go:ChangedFunc",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		result, err := handleDetectChanges(store, []string{"changed.go"})
		require.NoError(t, err)

		// Should show impact (CallerFunc is affected)
		assert.Contains(t, result, "Impact Analysis")
		assert.Contains(t, result, "CallerFunc")
	})

	t.Run("MultipleFiles", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph with multiple changed files
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:file1.go:Func1",
			Label:     graph.NodeFunction,
			Name:      "Func1",
			FilePath:  "file1.go",
			Content:   "func Func1() {}",
			Signature: "Func1()",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:file2.go:Func2",
			Label:     graph.NodeFunction,
			Name:      "Func2",
			FilePath:  "file2.go",
			Content:   "func Func2() {}",
			Signature: "Func2()",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		result, err := handleDetectChanges(store, []string{"file1.go", "file2.go"})
		require.NoError(t, err)

		assert.Contains(t, result, "file1.go")
		assert.Contains(t, result, "Func1")
		assert.Contains(t, result, "file2.go")
		assert.Contains(t, result, "Func2")
	})
}

func TestGetSymbolsInFiles(t *testing.T) {
	t.Parallel()

	t.Run("GetsSymbolsFromFiles", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:FuncA",
			Label:     graph.NodeFunction,
			Name:      "FuncA",
			FilePath:  "test.go",
			Content:   "func FuncA() {}",
			Signature: "FuncA()",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "class:test.go:MyClass",
			Label:     graph.NodeClass,
			Name:      "MyClass",
			FilePath:  "test.go",
			Content:   "type MyClass struct {}",
			Signature: "type MyClass struct",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		symbols := getSymbolsInFiles(store, []string{"test.go"})

		assert.Len(t, symbols, 2)
	})

	t.Run("HandlesEmptyFileList", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		symbols := getSymbolsInFiles(store, []string{})
		assert.Empty(t, symbols)
	})
}

func TestGetAffectedSymbols(t *testing.T) {
	t.Parallel()

	t.Run("GetsAffectedSymbols", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph with call relationships
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:changed.go:ChangedFunc",
			Label:     graph.NodeFunction,
			Name:      "ChangedFunc",
			FilePath:  "changed.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:caller.go:CallerFunc",
			Label:     graph.NodeFunction,
			Name:      "CallerFunc",
			FilePath:  "caller.go",
		})

		// CallerFunc calls ChangedFunc
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:caller.go:CallerFunc",
			Target: "function:changed.go:ChangedFunc",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		changedSymbols := []*graph.GraphNode{
			{ID: "function:changed.go:ChangedFunc", Name: "ChangedFunc", FilePath: "changed.go"},
		}

		affected := getAffectedSymbols(t.Context(), store, changedSymbols)

		// CallerFunc should be affected
		assert.NotEmpty(t, affected)
		// Check if any affected symbol has the name "CallerFunc"
		found := false
		for _, sym := range affected {
			if sym.Name == "CallerFunc" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find CallerFunc as affected")
	})

	t.Run("HandlesNoAffectedSymbols", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create isolated node
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:isolated.go:IsolatedFunc",
			Label:     graph.NodeFunction,
			Name:      "IsolatedFunc",
			FilePath:  "isolated.go",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		changedSymbols := []*graph.GraphNode{
			{ID: "function:isolated.go:IsolatedFunc", Name: "IsolatedFunc", FilePath: "isolated.go"},
		}

		affected := getAffectedSymbols(t.Context(), store, changedSymbols)

		// No other symbols should be affected
		assert.Empty(t, affected)
	})
}
