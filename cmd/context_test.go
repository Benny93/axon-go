package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

func TestContextCmd_Run(t *testing.T) {
	t.Run("NoSymbolProvided", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ContextCmd{}

		err := cmd.Run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "symbol name required")
	})

	t.Run("NoIndexFound", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ContextCmd{
			Symbol: "Foo",
		}

		err := cmd.Run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no index found")
	})

	t.Run("SymbolNotFound", func(t *testing.T) {
		tmpDir := setupTestIndex(t)
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ContextCmd{
			Symbol: "NonExistent",
		}

		err := cmd.Run()
		assert.NoError(t, err)
		// Should print "not found" message
	})

	t.Run("SymbolFound", func(t *testing.T) {
		tmpDir := setupTestIndex(t)
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ContextCmd{
			Symbol: "Foo",
		}

		err := cmd.Run()
		assert.NoError(t, err)
	})
}

func TestContextLookup(t *testing.T) {
	t.Run("GetSymbolContext", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(filepath.Join(tmpDir, "badger"), false)
		require.NoError(t, err)
		defer store.Close()

		// Create test graph
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:Foo",
			Label:    graph.NodeFunction,
			Name:     "Foo",
			FilePath: "main.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:Bar",
			Label:    graph.NodeFunction,
			Name:     "Bar",
			FilePath: "main.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:Baz",
			Label:    graph.NodeFunction,
			Name:     "Baz",
			FilePath: "main.go",
		})

		// Foo calls Bar and Baz
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:Foo",
			Target: "function:main.go:Bar",
		})
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:main.go:Foo",
			Target: "function:main.go:Baz",
		})

		// Bar calls Baz
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:3",
			Type:   graph.RelCalls,
			Source: "function:main.go:Bar",
			Target: "function:main.go:Baz",
		})

		err = store.BulkLoad(context.Background(), g)
		require.NoError(t, err)

		ctx := context.Background()

		// Get callers of Baz
		callers, err := store.GetCallers(ctx, "function:main.go:Baz")
		assert.NoError(t, err)
		assert.Len(t, callers, 2) // Foo and Bar

		// Get callees of Foo
		callees, err := store.GetCallees(ctx, "function:main.go:Foo")
		assert.NoError(t, err)
		assert.Len(t, callees, 2) // Bar and Baz
	})
}

func setupTestIndex(t *testing.T) string {
	tmpDir := t.TempDir()

	// Create .axon directory with BadgerDB
	axonDir := filepath.Join(tmpDir, ".axon")
	dbPath := filepath.Join(axonDir, "badger")
	err := os.MkdirAll(dbPath, 0o755)
	require.NoError(t, err)

	// Create and initialize store
	store := storage.NewBadgerBackend()
	err = store.Initialize(dbPath, false)
	require.NoError(t, err)

	// Create test graph
	g := graph.NewKnowledgeGraph()
	g.AddNode(&graph.GraphNode{
		ID:       "function:main.go:Foo",
		Label:    graph.NodeFunction,
		Name:     "Foo",
		FilePath: "main.go",
	})

	err = store.BulkLoad(context.Background(), g)
	require.NoError(t, err)
	store.Close()

	return tmpDir
}
