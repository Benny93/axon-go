package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestDetectCommunities(t *testing.T) {
	t.Parallel()

	t.Run("DetectsCommunities", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create two distinct clusters
		// Cluster 1: A -> B -> C
		g.AddNode(&graph.GraphNode{ID: "function:a.go:A", Label: graph.NodeFunction, Name: "A", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "function:b.go:B", Label: graph.NodeFunction, Name: "B", FilePath: "b.go"})
		g.AddNode(&graph.GraphNode{ID: "function:c.go:C", Label: graph.NodeFunction, Name: "C", FilePath: "c.go"})

		g.AddRelationship(&graph.GraphRelationship{ID: "calls:1", Type: graph.RelCalls, Source: "function:a.go:A", Target: "function:b.go:B"})
		g.AddRelationship(&graph.GraphRelationship{ID: "calls:2", Type: graph.RelCalls, Source: "function:b.go:B", Target: "function:c.go:C"})

		// Cluster 2: D -> E -> F
		g.AddNode(&graph.GraphNode{ID: "function:d.go:D", Label: graph.NodeFunction, Name: "D", FilePath: "d.go"})
		g.AddNode(&graph.GraphNode{ID: "function:e.go:E", Label: graph.NodeFunction, Name: "E", FilePath: "e.go"})
		g.AddNode(&graph.GraphNode{ID: "function:f.go:F", Label: graph.NodeFunction, Name: "F", FilePath: "f.go"})

		g.AddRelationship(&graph.GraphRelationship{ID: "calls:3", Type: graph.RelCalls, Source: "function:d.go:D", Target: "function:e.go:E"})
		g.AddRelationship(&graph.GraphRelationship{ID: "calls:4", Type: graph.RelCalls, Source: "function:e.go:E", Target: "function:f.go:F"})

		count := DetectCommunities(g)

		assert.Greater(t, count, 0)

		// Verify COMMUNITY nodes were created
		communities := g.GetNodesByLabel(graph.NodeCommunity)
		assert.NotEmpty(t, communities)

		// Verify MEMBER_OF edges were created
		memberEdges := g.GetRelationshipsByType(graph.RelMemberOf)
		assert.NotEmpty(t, memberEdges)
	})

	t.Run("HandlesDisconnectedGraph", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add isolated nodes
		g.AddNode(&graph.GraphNode{ID: "function:a.go:A", Label: graph.NodeFunction, Name: "A", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "function:b.go:B", Label: graph.NodeFunction, Name: "B", FilePath: "b.go"})

		count := DetectCommunities(g)

		// Each isolated node should be its own community
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("HandlesEmptyGraph", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		count := DetectCommunities(g)

		assert.Equal(t, 0, count)
	})

	t.Run("SingleCommunity", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create tightly connected nodes
		for i := 0; i < 5; i++ {
			nodeID := "function:test.go:Func" + string(rune('A'+i))
			g.AddNode(&graph.GraphNode{ID: nodeID, Label: graph.NodeFunction, Name: "Func" + string(rune('A'+i)), FilePath: "test.go"})
		}

		// Connect all nodes to each other
		for i := 0; i < 5; i++ {
			for j := i + 1; j < 5; j++ {
				sourceID := "function:test.go:Func" + string(rune('A'+i))
				targetID := "function:test.go:Func" + string(rune('A'+j))
				g.AddRelationship(&graph.GraphRelationship{
					ID:     "calls:" + string(rune('A'+i)) + string(rune('A'+j)),
					Type:   graph.RelCalls,
					Source: sourceID,
					Target: targetID,
				})
			}
		}

		count := DetectCommunities(g)

		// Should detect at least 1 community
		assert.GreaterOrEqual(t, count, 1)
	})
}

func TestBuildAdjacencyMatrix(t *testing.T) {
	t.Parallel()

	t.Run("BuildsMatrix", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		g.AddNode(&graph.GraphNode{ID: "node:A", Label: graph.NodeFunction, Name: "A", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "node:B", Label: graph.NodeFunction, Name: "B", FilePath: "b.go"})
		g.AddNode(&graph.GraphNode{ID: "node:C", Label: graph.NodeFunction, Name: "C", FilePath: "c.go"})

		g.AddRelationship(&graph.GraphRelationship{ID: "calls:1", Type: graph.RelCalls, Source: "node:A", Target: "node:B"})
		g.AddRelationship(&graph.GraphRelationship{ID: "calls:2", Type: graph.RelCalls, Source: "node:B", Target: "node:C"})

		matrix, nodeIndex, indexNode := buildAdjacencyMatrix(g)

		assert.NotNil(t, matrix)
		assert.NotEmpty(t, nodeIndex)
		assert.NotEmpty(t, indexNode)

		// Verify A -> B connection
		aIdx := nodeIndex["node:A"]
		bIdx := nodeIndex["node:B"]
		assert.Equal(t, 1.0, matrix[aIdx][bIdx])
	})

	t.Run("SymmetricMatrix", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		g.AddNode(&graph.GraphNode{ID: "node:A", Label: graph.NodeFunction, Name: "A", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "node:B", Label: graph.NodeFunction, Name: "B", FilePath: "b.go"})

		g.AddRelationship(&graph.GraphRelationship{ID: "calls:1", Type: graph.RelCalls, Source: "node:A", Target: "node:B"})

		matrix, _, _ := buildAdjacencyMatrix(g)

		// Matrix should be symmetric for undirected graph
		assert.Equal(t, matrix[0][1], matrix[1][0])
	})
}

func TestAssignCommunities(t *testing.T) {
	t.Parallel()

	t.Run("AssignsCommunities", func(t *testing.T) {
		// Simple adjacency matrix for 4 nodes in 2 clusters
		matrix := [][]float64{
			{0, 1, 0, 0},
			{1, 0, 0, 0},
			{0, 0, 0, 1},
			{0, 0, 1, 0},
		}

		communities := assignCommunities(matrix)

		assert.Len(t, communities, 4)

		// Nodes 0,1 should be in same community
		// Nodes 2,3 should be in same community
		assert.Equal(t, communities[0], communities[1])
		assert.Equal(t, communities[2], communities[3])
		assert.NotEqual(t, communities[0], communities[2])
	})

	t.Run("HandlesSingleNode", func(t *testing.T) {
		matrix := [][]float64{{0}}

		communities := assignCommunities(matrix)

		assert.Len(t, communities, 1)
		assert.Equal(t, 0, communities[0])
	})
}

func TestGenerateCommunityLabel(t *testing.T) {
	t.Parallel()

	t.Run("GeneratesLabel", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{ID: "node:A", Label: graph.NodeFunction, Name: "FuncA", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "node:B", Label: graph.NodeFunction, Name: "FuncB", FilePath: "b.go"})

		members := []string{"node:A", "node:B"}

		label := generateCommunityLabel(g, members)

		assert.NotEmpty(t, label)
		assert.Contains(t, label, "Community")
	})

	t.Run("HandlesEmptyMembers", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		label := generateCommunityLabel(g, []string{})

		assert.Contains(t, label, "Community")
	})
}
