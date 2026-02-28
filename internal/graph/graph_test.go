package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKnowledgeGraph(t *testing.T) {
	t.Parallel()

	g := NewKnowledgeGraph()

	assert.NotNil(t, g)
	assert.Equal(t, 0, g.NodeCount())
	assert.Equal(t, 0, g.RelationshipCount())
}

func TestKnowledgeGraph_AddNode(t *testing.T) {
	t.Parallel()

	t.Run("AddSingle", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()
		node := &GraphNode{
			ID:       "function:test.py:foo",
			Label:    NodeFunction,
			Name:     "foo",
			FilePath: "test.py",
		}

		g.AddNode(node)

		assert.Equal(t, 1, g.NodeCount())
		assert.Equal(t, node, g.GetNode("function:test.py:foo"))
	})

	t.Run("AddMultiple", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:a.py:foo", Label: NodeFunction, Name: "foo", FilePath: "a.py"})
		g.AddNode(&GraphNode{ID: "function:b.py:bar", Label: NodeFunction, Name: "bar", FilePath: "b.py"})
		g.AddNode(&GraphNode{ID: "class:c.py:MyClass", Label: NodeClass, Name: "MyClass", FilePath: "c.py"})

		assert.Equal(t, 3, g.NodeCount())
		assert.Equal(t, 2, g.CountNodesByLabel(NodeFunction))
		assert.Equal(t, 1, g.CountNodesByLabel(NodeClass))
	})

	t.Run("ReplaceExisting", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		original := &GraphNode{
			ID:       "function:test.py:foo",
			Label:    NodeFunction,
			Name:     "foo",
			FilePath: "test.py",
			StartLine: 10,
		}
		g.AddNode(original)

		updated := &GraphNode{
			ID:       "function:test.py:foo",
			Label:    NodeFunction,
			Name:     "foo",
			FilePath: "test.py",
			StartLine: 20,
		}
		g.AddNode(updated)

		assert.Equal(t, 1, g.NodeCount())
		assert.Equal(t, 20, g.GetNode("function:test.py:foo").StartLine)
	})

	t.Run("ReplaceWithDifferentLabel", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "id1", Label: NodeFunction, Name: "foo", FilePath: "test.py"})
		assert.Equal(t, 1, g.CountNodesByLabel(NodeFunction))

		g.AddNode(&GraphNode{ID: "id1", Label: NodeClass, Name: "Foo", FilePath: "test.py"})
		assert.Equal(t, 0, g.CountNodesByLabel(NodeFunction))
		assert.Equal(t, 1, g.CountNodesByLabel(NodeClass))
		assert.Equal(t, 1, g.NodeCount())
	})
}

func TestKnowledgeGraph_RemoveNode(t *testing.T) {
	t.Parallel()

	t.Run("RemoveExisting", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()
		node := &GraphNode{ID: "function:test.py:foo", Label: NodeFunction, Name: "foo", FilePath: "test.py"}
		g.AddNode(node)

		removed := g.RemoveNode("function:test.py:foo")

		assert.True(t, removed)
		assert.Equal(t, 0, g.NodeCount())
		assert.Nil(t, g.GetNode("function:test.py:foo"))
	})

	t.Run("RemoveNonExistent", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		removed := g.RemoveNode("function:test.py:foo")

		assert.False(t, removed)
	})

	t.Run("RemoveCascadesRelationships", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:a.py:foo", Label: NodeFunction, Name: "foo", FilePath: "a.py"})
		g.AddNode(&GraphNode{ID: "function:b.py:bar", Label: NodeFunction, Name: "bar", FilePath: "b.py"})
		g.AddRelationship(&GraphRelationship{
			ID:     "rel1",
			Type:   RelCalls,
			Source: "function:a.py:foo",
			Target: "function:b.py:bar",
		})

		assert.Equal(t, 2, g.NodeCount())
		assert.Equal(t, 1, g.RelationshipCount())

		g.RemoveNode("function:a.py:foo")

		assert.Equal(t, 1, g.NodeCount())
		assert.Equal(t, 0, g.RelationshipCount())
	})
}

func TestKnowledgeGraph_RemoveNodesByFile(t *testing.T) {
	t.Parallel()

	t.Run("RemoveSingleFile", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:test.py:foo", Label: NodeFunction, Name: "foo", FilePath: "test.py"})
		g.AddNode(&GraphNode{ID: "function:other.py:bar", Label: NodeFunction, Name: "bar", FilePath: "other.py"})

		removed := g.RemoveNodesByFile("test.py")

		assert.Equal(t, 1, removed)
		assert.Equal(t, 1, g.NodeCount())
		assert.NotNil(t, g.GetNode("function:other.py:bar"))
		assert.Nil(t, g.GetNode("function:test.py:foo"))
	})

	t.Run("RemoveMultipleNodes", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:test.py:foo", Label: NodeFunction, Name: "foo", FilePath: "test.py"})
		g.AddNode(&GraphNode{ID: "class:test.py:Foo", Label: NodeClass, Name: "Foo", FilePath: "test.py"})
		g.AddNode(&GraphNode{ID: "function:other.py:bar", Label: NodeFunction, Name: "bar", FilePath: "other.py"})

		removed := g.RemoveNodesByFile("test.py")

		assert.Equal(t, 2, removed)
		assert.Equal(t, 1, g.NodeCount())
	})

	t.Run("RemoveNonExistentFile", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:test.py:foo", Label: NodeFunction, Name: "foo", FilePath: "test.py"})

		removed := g.RemoveNodesByFile("nonexistent.py")

		assert.Equal(t, 0, removed)
		assert.Equal(t, 1, g.NodeCount())
	})

	t.Run("RemoveCascadesRelationships", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:test.py:foo", Label: NodeFunction, Name: "foo", FilePath: "test.py"})
		g.AddNode(&GraphNode{ID: "function:other.py:bar", Label: NodeFunction, Name: "bar", FilePath: "other.py"})
		g.AddRelationship(&GraphRelationship{
			ID:     "rel1",
			Type:   RelCalls,
			Source: "function:test.py:foo",
			Target: "function:other.py:bar",
		})

		removed := g.RemoveNodesByFile("test.py")

		assert.Equal(t, 1, removed)
		assert.Equal(t, 0, g.RelationshipCount())
	})
}

func TestKnowledgeGraph_AddRelationship(t *testing.T) {
	t.Parallel()

	t.Run("AddSingle", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		rel := &GraphRelationship{
			ID:     "calls:1",
			Type:   RelCalls,
			Source: "function:a.py:foo",
			Target: "function:b.py:bar",
		}
		g.AddRelationship(rel)

		assert.Equal(t, 1, g.RelationshipCount())
		rels := g.GetRelationshipsByType(RelCalls)
		assert.Len(t, rels, 1)
		assert.Equal(t, rel, rels[0])
	})

	t.Run("ReplaceExisting", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		original := &GraphRelationship{
			ID:     "calls:1",
			Type:   RelCalls,
			Source: "function:a.py:foo",
			Target: "function:b.py:bar",
			Properties: map[string]any{"confidence": 0.5},
		}
		g.AddRelationship(original)

		updated := &GraphRelationship{
			ID:     "calls:1",
			Type:   RelCalls,
			Source: "function:a.py:foo",
			Target: "function:b.py:bar",
			Properties: map[string]any{"confidence": 0.9},
		}
		g.AddRelationship(updated)

		assert.Equal(t, 1, g.RelationshipCount())
		rels := g.GetRelationshipsByType(RelCalls)
		assert.Equal(t, 0.9, rels[0].Properties["confidence"])
	})
}

func TestKnowledgeGraph_GetOutgoing(t *testing.T) {
	t.Parallel()

	t.Run("GetAllOutgoing", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel2", Type: RelCalls, Source: "node1", Target: "node3"})
		g.AddRelationship(&GraphRelationship{ID: "rel3", Type: RelExtends, Source: "node1", Target: "node4"})

		rels := g.GetOutgoing("node1")
		assert.Len(t, rels, 3)
	})

	t.Run("GetOutgoingByType", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel2", Type: RelCalls, Source: "node1", Target: "node3"})
		g.AddRelationship(&GraphRelationship{ID: "rel3", Type: RelExtends, Source: "node1", Target: "node4"})

		rels := g.GetOutgoing("node1", RelCalls)
		assert.Len(t, rels, 2)
	})

	t.Run("NoOutgoing", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		rels := g.GetOutgoing("nonexistent")
		assert.Nil(t, rels)
	})
}

func TestKnowledgeGraph_GetIncoming(t *testing.T) {
	t.Parallel()

	t.Run("GetAllIncoming", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel2", Type: RelCalls, Source: "node3", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel3", Type: RelExtends, Source: "node4", Target: "node2"})

		rels := g.GetIncoming("node2")
		assert.Len(t, rels, 3)
	})

	t.Run("GetIncomingByType", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel2", Type: RelCalls, Source: "node3", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel3", Type: RelExtends, Source: "node4", Target: "node2"})

		rels := g.GetIncoming("node2", RelCalls)
		assert.Len(t, rels, 2)
	})

	t.Run("NoIncoming", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		rels := g.GetIncoming("nonexistent")
		assert.Nil(t, rels)
	})
}

func TestKnowledgeGraph_HasIncoming(t *testing.T) {
	t.Parallel()

	t.Run("HasIncomingTrue", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})

		assert.True(t, g.HasIncoming("node2", RelCalls))
	})

	t.Run("HasIncomingFalse", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})

		assert.False(t, g.HasIncoming("node2", RelExtends))
		assert.False(t, g.HasIncoming("nonexistent", RelCalls))
	})
}

func TestKnowledgeGraph_GetNodesByLabel(t *testing.T) {
	t.Parallel()

	t.Run("GetFunctions", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddNode(&GraphNode{ID: "function:a.py:foo", Label: NodeFunction, Name: "foo", FilePath: "a.py"})
		g.AddNode(&GraphNode{ID: "function:b.py:bar", Label: NodeFunction, Name: "bar", FilePath: "b.py"})
		g.AddNode(&GraphNode{ID: "class:c.py:MyClass", Label: NodeClass, Name: "MyClass", FilePath: "c.py"})

		functions := g.GetNodesByLabel(NodeFunction)
		classes := g.GetNodesByLabel(NodeClass)

		assert.Len(t, functions, 2)
		assert.Len(t, classes, 1)
	})

	t.Run("GetNonExistentLabel", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		nodes := g.GetNodesByLabel(NodeProcess)
		assert.Nil(t, nodes)
	})
}

func TestKnowledgeGraph_GetRelationshipsByType(t *testing.T) {
	t.Parallel()

	t.Run("GetCalls", func(t *testing.T) {
		t.Parallel()
		g := NewKnowledgeGraph()

		g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})
		g.AddRelationship(&GraphRelationship{ID: "rel2", Type: RelCalls, Source: "node3", Target: "node4"})
		g.AddRelationship(&GraphRelationship{ID: "rel3", Type: RelImports, Source: "node5", Target: "node6"})

		calls := g.GetRelationshipsByType(RelCalls)
		imports := g.GetRelationshipsByType(RelImports)

		assert.Len(t, calls, 2)
		assert.Len(t, imports, 1)
	})
}

func TestKnowledgeGraph_Stats(t *testing.T) {
	t.Parallel()

	g := NewKnowledgeGraph()

	g.AddNode(&GraphNode{ID: "node1", Label: NodeFunction, Name: "foo", FilePath: "test.py"})
	g.AddNode(&GraphNode{ID: "node2", Label: NodeClass, Name: "Bar", FilePath: "test.py"})
	g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelDefines, Source: "node1", Target: "node2"})

	stats := g.Stats()

	assert.Equal(t, 2, stats["nodes"])
	assert.Equal(t, 1, stats["relationships"])
}

func TestKnowledgeGraph_IterNodes(t *testing.T) {
	t.Parallel()

	g := NewKnowledgeGraph()
	g.AddNode(&GraphNode{ID: "node1", Label: NodeFunction, Name: "foo", FilePath: "test.py"})
	g.AddNode(&GraphNode{ID: "node2", Label: NodeClass, Name: "Bar", FilePath: "test.py"})

	count := 0
	for range g.IterNodes() {
		count++
	}

	assert.Equal(t, 2, count)
}

func TestKnowledgeGraph_IterRelationships(t *testing.T) {
	t.Parallel()

	g := NewKnowledgeGraph()
	g.AddRelationship(&GraphRelationship{ID: "rel1", Type: RelCalls, Source: "node1", Target: "node2"})
	g.AddRelationship(&GraphRelationship{ID: "rel2", Type: RelExtends, Source: "node3", Target: "node4"})

	count := 0
	for range g.IterRelationships() {
		count++
	}

	assert.Equal(t, 2, count)
}

func TestKnowledgeGraph_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	g := NewKnowledgeGraph()

	// Add nodes concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			g.AddNode(&GraphNode{
				ID:       "function:test" + string(rune(id)) + ".py:foo",
				Label:    NodeFunction,
				Name:     "foo",
				FilePath: "test" + string(rune(id)) + ".py",
			})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 10, g.NodeCount())
}
