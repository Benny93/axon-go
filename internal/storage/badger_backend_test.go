package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func setupTestBadgerBackend(t *testing.T) (*BadgerBackend, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger")

	backend := NewBadgerBackend()
	err := backend.Initialize(dbPath, false)
	require.NoError(t, err)

	cleanup := func() {
		backend.Close()
	}

	return backend, cleanup
}

func TestBadgerBackend_Initialize(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "badger")

		backend := NewBadgerBackend()
		err := backend.Initialize(dbPath, false)

		assert.NoError(t, err)
		assert.NotNil(t, backend.db)
		assert.True(t, backend.initialized)

		backend.Close()
	})

	t.Run("ReadOnly", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "badger")

		// First create the DB
		backend1 := NewBadgerBackend()
		err := backend1.Initialize(dbPath, false)
		require.NoError(t, err)
		backend1.Close()

		// Open in read-only mode
		backend2 := NewBadgerBackend()
		err = backend2.Initialize(dbPath, true)

		assert.NoError(t, err)
		assert.True(t, backend2.initialized)

		backend2.Close()
	})

	t.Run("InvalidPath", func(t *testing.T) {
		backend := NewBadgerBackend()
		err := backend.Initialize("/nonexistent/path/that/does/not/exist", false)

		assert.Error(t, err)
	})
}

func TestBadgerBackend_AddNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	t.Run("AddSingle", func(t *testing.T) {
		node := &graph.GraphNode{
			ID:        "function:test.py:foo",
			Label:     graph.NodeFunction,
			Name:      "foo",
			FilePath:  "test.py",
			StartLine: 10,
			EndLine:   20,
			Language:  "python",
		}

		err := backend.AddNodes(ctx, []*graph.GraphNode{node})
		assert.NoError(t, err)

		// Verify node was stored
		retrieved, err := backend.GetNode(ctx, node.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, node.Name, retrieved.Name)
		assert.Equal(t, node.FilePath, retrieved.FilePath)
	})

	t.Run("AddMultiple", func(t *testing.T) {
		nodes := []*graph.GraphNode{
			{ID: "function:a.py:foo", Label: graph.NodeFunction, Name: "foo", FilePath: "a.py"},
			{ID: "class:b.py:Bar", Label: graph.NodeClass, Name: "Bar", FilePath: "b.py"},
			{ID: "function:c.py:baz", Label: graph.NodeFunction, Name: "baz", FilePath: "c.py"},
		}

		err := backend.AddNodes(ctx, nodes)
		assert.NoError(t, err)

		// Verify all nodes were stored
		for _, node := range nodes {
			retrieved, err := backend.GetNode(ctx, node.ID)
			assert.NoError(t, err)
			assert.NotNil(t, retrieved)
			assert.Equal(t, node.Name, retrieved.Name)
		}
	})

	t.Run("UpdateExisting", func(t *testing.T) {
		original := &graph.GraphNode{
			ID:        "function:test.py:foo",
			Label:     graph.NodeFunction,
			Name:      "foo",
			FilePath:  "test.py",
			StartLine: 10,
		}

		err := backend.AddNodes(ctx, []*graph.GraphNode{original})
		require.NoError(t, err)

		updated := &graph.GraphNode{
			ID:        "function:test.py:foo",
			Label:     graph.NodeFunction,
			Name:      "foo",
			FilePath:  "test.py",
			StartLine: 50, // Changed
		}

		err = backend.AddNodes(ctx, []*graph.GraphNode{updated})
		assert.NoError(t, err)

		retrieved, err := backend.GetNode(ctx, original.ID)
		assert.NoError(t, err)
		assert.Equal(t, 50, retrieved.StartLine)
	})
}

func TestBadgerBackend_GetNode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	t.Run("Exists", func(t *testing.T) {
		node := &graph.GraphNode{
			ID:       "function:test.py:foo",
			Label:    graph.NodeFunction,
			Name:     "foo",
			FilePath: "test.py",
		}

		err := backend.AddNodes(ctx, []*graph.GraphNode{node})
		require.NoError(t, err)

		retrieved, err := backend.GetNode(ctx, node.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, node.Name, retrieved.Name)
	})

	t.Run("NotExists", func(t *testing.T) {
		retrieved, err := backend.GetNode(ctx, "nonexistent")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestBadgerBackend_RemoveNodesByFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	// Add nodes from multiple files
	nodes := []*graph.GraphNode{
		{ID: "function:test.py:foo", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"},
		{ID: "class:test.py:Foo", Label: graph.NodeClass, Name: "Foo", FilePath: "test.py"},
		{ID: "function:other.py:bar", Label: graph.NodeFunction, Name: "bar", FilePath: "other.py"},
	}

	err := backend.AddNodes(ctx, nodes)
	require.NoError(t, err)

	t.Run("RemoveSingleFile", func(t *testing.T) {
		count, err := backend.RemoveNodesByFile(ctx, "test.py")
		assert.NoError(t, err)
		assert.Equal(t, 2, count)

		// Verify nodes from test.py are removed
		foo, err := backend.GetNode(ctx, "function:test.py:foo")
		assert.NoError(t, err)
		assert.Nil(t, foo)

		// Verify nodes from other.py still exist
		bar, err := backend.GetNode(ctx, "function:other.py:bar")
		assert.NoError(t, err)
		assert.NotNil(t, bar)
	})

	t.Run("RemoveNonExistentFile", func(t *testing.T) {
		count, err := backend.RemoveNodesByFile(ctx, "nonexistent.py")
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestBadgerBackend_AddRelationships(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	// Add nodes first
	nodes := []*graph.GraphNode{
		{ID: "function:a.py:foo", Label: graph.NodeFunction, Name: "foo", FilePath: "a.py"},
		{ID: "function:b.py:bar", Label: graph.NodeFunction, Name: "bar", FilePath: "b.py"},
	}
	err := backend.AddNodes(ctx, nodes)
	require.NoError(t, err)

	t.Run("AddSingle", func(t *testing.T) {
		rel := &graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:a.py:foo",
			Target: "function:b.py:bar",
			Properties: map[string]any{"confidence": 0.9},
		}

		err := backend.AddRelationships(ctx, []*graph.GraphRelationship{rel})
		assert.NoError(t, err)

		// Verify relationship was stored
		callers, err := backend.GetCallers(ctx, "function:b.py:bar")
		assert.NoError(t, err)
		assert.Len(t, callers, 1)
		assert.Equal(t, "foo", callers[0].Name)
	})

	t.Run("AddMultiple", func(t *testing.T) {
		rels := []*graph.GraphRelationship{
			{ID: "calls:2", Type: graph.RelCalls, Source: "function:a.py:foo", Target: "function:b.py:bar"},
			{ID: "calls:3", Type: graph.RelCalls, Source: "function:b.py:bar", Target: "function:a.py:foo"},
		}

		err := backend.AddRelationships(ctx, rels)
		assert.NoError(t, err)

		// Verify both directions
		// Note: calls:1 was added in AddSingle, so bar has 2 callers (foo via calls:1 and calls:2)
		callersOfBar, _ := backend.GetCallers(ctx, "function:b.py:bar")
		assert.GreaterOrEqual(t, len(callersOfBar), 1)

		callersOfFoo, _ := backend.GetCallers(ctx, "function:a.py:foo")
		assert.Len(t, callersOfFoo, 1)
	})
}

func TestBadgerBackend_GetCallers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	// Setup: foo -> bar -> baz
	nodes := []*graph.GraphNode{
		{ID: "function:a.py:foo", Label: graph.NodeFunction, Name: "foo", FilePath: "a.py"},
		{ID: "function:b.py:bar", Label: graph.NodeFunction, Name: "bar", FilePath: "b.py"},
		{ID: "function:c.py:baz", Label: graph.NodeFunction, Name: "baz", FilePath: "c.py"},
	}
	err := backend.AddNodes(ctx, nodes)
	require.NoError(t, err)

	rels := []*graph.GraphRelationship{
		{ID: "calls:1", Type: graph.RelCalls, Source: "function:a.py:foo", Target: "function:b.py:bar"},
		{ID: "calls:2", Type: graph.RelCalls, Source: "function:b.py:bar", Target: "function:c.py:baz"},
	}
	err = backend.AddRelationships(ctx, rels)
	require.NoError(t, err)

	t.Run("DirectCallers", func(t *testing.T) {
		callers, err := backend.GetCallers(ctx, "function:b.py:bar")
		assert.NoError(t, err)
		assert.Len(t, callers, 1)
		assert.Equal(t, "foo", callers[0].Name)
	})

	t.Run("NoCallers", func(t *testing.T) {
		callers, err := backend.GetCallers(ctx, "function:a.py:foo")
		assert.NoError(t, err)
		assert.Empty(t, callers)
	})

	t.Run("MultipleCallers", func(t *testing.T) {
		// Add another caller
		qux := &graph.GraphNode{
			ID: "function:d.py:qux", Label: graph.NodeFunction, Name: "qux", FilePath: "d.py",
		}
		err := backend.AddNodes(ctx, []*graph.GraphNode{qux})
		require.NoError(t, err)

		rel := &graph.GraphRelationship{
			ID: "calls:3", Type: graph.RelCalls, Source: "function:d.py:qux", Target: "function:b.py:bar",
		}
		err = backend.AddRelationships(ctx, []*graph.GraphRelationship{rel})
		assert.NoError(t, err)

		callers, err := backend.GetCallers(ctx, "function:b.py:bar")
		assert.NoError(t, err)
		assert.Len(t, callers, 2)
	})
}

func TestBadgerBackend_GetCallees(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	// Setup: foo -> bar -> baz
	nodes := []*graph.GraphNode{
		{ID: "function:a.py:foo", Label: graph.NodeFunction, Name: "foo", FilePath: "a.py"},
		{ID: "function:b.py:bar", Label: graph.NodeFunction, Name: "bar", FilePath: "b.py"},
		{ID: "function:c.py:baz", Label: graph.NodeFunction, Name: "baz", FilePath: "c.py"},
	}
	err := backend.AddNodes(ctx, nodes)
	require.NoError(t, err)

	rels := []*graph.GraphRelationship{
		{ID: "calls:1", Type: graph.RelCalls, Source: "function:a.py:foo", Target: "function:b.py:bar"},
		{ID: "calls:2", Type: graph.RelCalls, Source: "function:b.py:bar", Target: "function:c.py:baz"},
	}
	err = backend.AddRelationships(ctx, rels)
	require.NoError(t, err)

	t.Run("DirectCallees", func(t *testing.T) {
		callees, err := backend.GetCallees(ctx, "function:a.py:foo")
		assert.NoError(t, err)
		assert.Len(t, callees, 1)
		assert.Equal(t, "bar", callees[0].Name)
	})

	t.Run("NoCallees", func(t *testing.T) {
		callees, err := backend.GetCallees(ctx, "function:c.py:baz")
		assert.NoError(t, err)
		assert.Empty(t, callees)
	})
}

func TestBadgerBackend_Traverse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	// Create a call chain: main -> foo -> bar -> baz
	nodes := []*graph.GraphNode{
		{ID: "function:main.py:main", Label: graph.NodeFunction, Name: "main", FilePath: "main.py"},
		{ID: "function:foo.py:foo", Label: graph.NodeFunction, Name: "foo", FilePath: "foo.py"},
		{ID: "function:bar.py:bar", Label: graph.NodeFunction, Name: "bar", FilePath: "bar.py"},
		{ID: "function:baz.py:baz", Label: graph.NodeFunction, Name: "baz", FilePath: "baz.py"},
	}
	err := backend.AddNodes(ctx, nodes)
	require.NoError(t, err)

	rels := []*graph.GraphRelationship{
		{ID: "calls:1", Type: graph.RelCalls, Source: "function:main.py:main", Target: "function:foo.py:foo"},
		{ID: "calls:2", Type: graph.RelCalls, Source: "function:foo.py:foo", Target: "function:bar.py:bar"},
		{ID: "calls:3", Type: graph.RelCalls, Source: "function:bar.py:bar", Target: "function:baz.py:baz"},
	}
	err = backend.AddRelationships(ctx, rels)
	require.NoError(t, err)

	t.Run("TraverseCallersDepth1", func(t *testing.T) {
		nodes, err := backend.Traverse(ctx, "function:bar.py:bar", 1, "callers")
		assert.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "foo", nodes[0].Name)
	})

	t.Run("TraverseCallersDepth2", func(t *testing.T) {
		nodes, err := backend.Traverse(ctx, "function:bar.py:bar", 2, "callers")
		assert.NoError(t, err)
		assert.Len(t, nodes, 2)
		// Should include foo and main
		names := []string{nodes[0].Name, nodes[1].Name}
		assert.Contains(t, names, "foo")
		assert.Contains(t, names, "main")
	})

	t.Run("TraverseCalleesDepth1", func(t *testing.T) {
		nodes, err := backend.Traverse(ctx, "function:foo.py:foo", 1, "callees")
		assert.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "bar", nodes[0].Name)
	})

	t.Run("TraverseMaxDepth", func(t *testing.T) {
		// Should not exceed max depth (10)
		nodes, err := backend.Traverse(ctx, "function:main.py:main", 15, "callees")
		assert.NoError(t, err)
		assert.Len(t, nodes, 3) // foo, bar, baz
	})
}

func TestBadgerBackend_BulkLoad(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	g := graph.NewKnowledgeGraph()
	g.AddNode(&graph.GraphNode{ID: "node1", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"})
	g.AddNode(&graph.GraphNode{ID: "node2", Label: graph.NodeClass, Name: "Bar", FilePath: "test.py"})
	g.AddRelationship(&graph.GraphRelationship{
		ID: "rel1", Type: graph.RelDefines, Source: "node1", Target: "node2",
	})

	err := backend.BulkLoad(ctx, g)
	assert.NoError(t, err)

	// Verify nodes were loaded
	node1, err := backend.GetNode(ctx, "node1")
	assert.NoError(t, err)
	assert.NotNil(t, node1)

	node2, err := backend.GetNode(ctx, "node2")
	assert.NoError(t, err)
	assert.NotNil(t, node2)
}

func TestBadgerBackend_Close(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger")

	backend := NewBadgerBackend()
	err := backend.Initialize(dbPath, false)
	require.NoError(t, err)

	err = backend.Close()
	assert.NoError(t, err)

	// Verify DB is closed
	assert.Nil(t, backend.db)
}

func TestBadgerBackend_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend, cleanup := setupTestBadgerBackend(t)
	defer cleanup()

	done := make(chan bool, 10)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			node := &graph.GraphNode{
				ID:       "function:test" + string(rune(id)) + ".py:foo",
				Label:    graph.NodeFunction,
				Name:     "foo",
				FilePath: "test" + string(rune(id)) + ".py",
			}
			_ = backend.AddNodes(ctx, []*graph.GraphNode{node})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all nodes were stored
	for i := 0; i < 10; i++ {
		nodeID := "function:test" + string(rune(i)) + ".py:foo"
		node, err := backend.GetNode(ctx, nodeID)
		assert.NoError(t, err)
		assert.NotNil(t, node)
	}
}
