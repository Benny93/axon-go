package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestMemoryBackend_Initialize(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		backend := NewMemoryBackend()
		err := backend.Initialize("/tmp/test", false)

		assert.NoError(t, err)
		assert.True(t, backend.IsIndexed())
	})

	t.Run("ReadOnly", func(t *testing.T) {
		t.Parallel()
		backend := NewMemoryBackend()
		err := backend.Initialize("/tmp/test", true)

		assert.NoError(t, err)
		assert.True(t, backend.IsIndexed())
	})
}

func TestMemoryBackend_Close(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	err := backend.Close()

	assert.NoError(t, err)
}

func TestMemoryBackend_BulkLoad(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()

	g := graph.NewKnowledgeGraph()
	g.AddNode(&graph.GraphNode{ID: "node1", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"})
	g.AddNode(&graph.GraphNode{ID: "node2", Label: graph.NodeClass, Name: "Bar", FilePath: "test.py"})

	err := backend.BulkLoad(ctx, g)

	assert.NoError(t, err)
	assert.Equal(t, 2, backend.NodeCount())
	assert.True(t, backend.IsIndexed())
}

func TestMemoryBackend_AddNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	nodes := []*graph.GraphNode{
		{ID: "node1", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"},
		{ID: "node2", Label: graph.NodeClass, Name: "Bar", FilePath: "test.py"},
	}

	err := backend.AddNodes(ctx, nodes)

	assert.NoError(t, err)
	assert.Equal(t, 2, backend.NodeCount())

	node, err := backend.GetNode(ctx, "node1")
	assert.NoError(t, err)
	assert.Equal(t, "foo", node.Name)
}

func TestMemoryBackend_RemoveNodesByFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	backend.AddNodes(ctx, []*graph.GraphNode{
		{ID: "node1", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"},
		{ID: "node2", Label: graph.NodeClass, Name: "Bar", FilePath: "test.py"},
		{ID: "node3", Label: graph.NodeFunction, Name: "baz", FilePath: "other.py"},
	})

	count, err := backend.RemoveNodesByFile(ctx, "test.py")

	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, 1, backend.NodeCount())

	node, err := backend.GetNode(ctx, "node3")
	assert.NoError(t, err)
	assert.Equal(t, "baz", node.Name)
}

func TestMemoryBackend_GetNode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	backend.AddNodes(ctx, []*graph.GraphNode{
		{ID: "node1", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"},
	})

	t.Run("Exists", func(t *testing.T) {
		t.Parallel()
		node, err := backend.GetNode(ctx, "node1")

		assert.NoError(t, err)
		assert.NotNil(t, node)
		assert.Equal(t, "foo", node.Name)
	})

	t.Run("NotExists", func(t *testing.T) {
		t.Parallel()
		node, err := backend.GetNode(ctx, "nonexistent")

		assert.NoError(t, err)
		assert.Nil(t, node)
	})
}

func TestMemoryBackend_FTSSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	backend.AddNodes(ctx, []*graph.GraphNode{
		{ID: "node1", Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"},
		{ID: "node2", Label: graph.NodeClass, Name: "Bar", FilePath: "test.py"},
		{ID: "node3", Label: graph.NodeFunction, Name: "baz", FilePath: "other.py"},
	})

	results, err := backend.FTSSearch(ctx, "test", 2)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "node1", results[0].NodeID)
	assert.Equal(t, "node2", results[1].NodeID)
}

func TestMemoryBackend_RebuildFTSIndexes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	err := backend.RebuildFTSIndexes(ctx)

	assert.NoError(t, err)
	assert.True(t, backend.IsFTSIndexed())
}

func TestMemoryBackend_StoreEmbeddings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	embeddings := []NodeEmbedding{
		{NodeID: "node1", Embedding: []float32{0.1, 0.2, 0.3}},
		{NodeID: "node2", Embedding: []float32{0.4, 0.5, 0.6}},
	}

	err := backend.StoreEmbeddings(ctx, embeddings)

	assert.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, backend.GetEmbedding("node1"))
	assert.Equal(t, []float32{0.4, 0.5, 0.6}, backend.GetEmbedding("node2"))
}

func TestMemoryBackend_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			_ = backend.AddNodes(ctx, []*graph.GraphNode{
				{ID: "node" + string(rune(id)), Label: graph.NodeFunction, Name: "foo", FilePath: "test.py"},
			})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 10, backend.NodeCount())
}

func TestMemoryBackend_AddRelationships(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	rels := []*graph.GraphRelationship{
		{ID: "rel1", Type: graph.RelCalls, Source: "node1", Target: "node2"},
	}

	err := backend.AddRelationships(ctx, rels)

	assert.NoError(t, err)
}

func TestMemoryBackend_Traversal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	t.Run("GetCallers", func(t *testing.T) {
		t.Parallel()
		callers, err := backend.GetCallers(ctx, "node1")
		assert.NoError(t, err)
		assert.Nil(t, callers)
	})

	t.Run("GetCallees", func(t *testing.T) {
		t.Parallel()
		callees, err := backend.GetCallees(ctx, "node1")
		assert.NoError(t, err)
		assert.Nil(t, callees)
	})

	t.Run("Traverse", func(t *testing.T) {
		t.Parallel()
		nodes, err := backend.Traverse(ctx, "node1", 3, "callers")
		assert.NoError(t, err)
		assert.Nil(t, nodes)
	})
}

func TestMemoryBackend_VectorSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.Initialize("/tmp/test", false)

	results, err := backend.VectorSearch(ctx, []float32{0.1, 0.2, 0.3}, 10)

	assert.NoError(t, err)
	assert.Nil(t, results)
}
