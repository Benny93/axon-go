package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestIsEntryPoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		node     *graph.GraphNode
		expected bool
	}{
		{
			name: "MainFunction",
			node: &graph.GraphNode{
				Name:     "main",
				Label:    graph.NodeFunction,
				FilePath: "main.go",
			},
			expected: true,
		},
		{
			name: "TestFunction",
			node: &graph.GraphNode{
				Name:     "TestSomething",
				Label:    graph.NodeFunction,
				FilePath: "something_test.go",
			},
			expected: true,
		},
		{
			name: "TestFunctionPython",
			node: &graph.GraphNode{
				Name:     "test_something",
				Label:    graph.NodeFunction,
				FilePath: "test_something.py",
			},
			expected: true,
		},
		{
			name: "HTTPHandler",
			node: &graph.GraphNode{
				Name:       "HandleUsers",
				Label:      graph.NodeFunction,
				FilePath:   "handlers.go",
				Decorators: []string{"http.HandleFunc"},
			},
			expected: true,
		},
		{
			name: "RegularFunction",
			node: &graph.GraphNode{
				Name:     "helper",
				Label:    graph.NodeFunction,
				FilePath: "utils.go",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isEntryPoint(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessProcesses(t *testing.T) {
	t.Parallel()

	t.Run("CreatesProcessNodes", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add entry point
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})

		// Add called function
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:helper",
			Label:    graph.NodeFunction,
			Name:     "helper",
			FilePath: "main.go",
		})

		// Add call relationship
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:main.go:helper",
		})

		count := ProcessProcesses(g)

		assert.Greater(t, count, 0)

		// Verify PROCESS node was created
		processNodes := g.GetNodesByLabel(graph.NodeProcess)
		assert.NotEmpty(t, processNodes)
	})

	t.Run("CreatesStepInProcessEdges", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add entry point and callees
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:step1",
			Label:    graph.NodeFunction,
			Name:     "step1",
			FilePath: "main.go",
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:main.go:step1",
		})

		_ = ProcessProcesses(g)

		// Verify STEP_IN_PROCESS edges were created
		stepEdges := g.GetRelationshipsByType(graph.RelStepInProcess)
		assert.NotEmpty(t, stepEdges)
	})

	t.Run("MultipleEntryPoints", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add multiple entry points
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})
		g.AddNode(&graph.GraphNode{
			ID:           "function:test.go:TestSomething",
			Label:        graph.NodeFunction,
			Name:         "TestSomething",
			FilePath:     "test.go",
			IsEntryPoint: true,
		})

		count := ProcessProcesses(g)

		// Should create at least 2 processes (one per entry point)
		assert.GreaterOrEqual(t, count, 2)
	})

	t.Run("NoEntryPoints", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add regular functions (no entry points)
		g.AddNode(&graph.GraphNode{
			ID:       "function:utils.go:helper",
			Label:    graph.NodeFunction,
			Name:     "helper",
			FilePath: "utils.go",
		})

		count := ProcessProcesses(g)

		assert.Equal(t, 0, count)
	})
}

func TestTraceFlow(t *testing.T) {
	t.Parallel()

	t.Run("TracesCallChain", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create call chain: main -> step1 -> step2
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:step1",
			Label:    graph.NodeFunction,
			Name:     "step1",
			FilePath: "main.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:step2",
			Label:    graph.NodeFunction,
			Name:     "step2",
			FilePath: "main.go",
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:main.go:step1",
		})
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:main.go:step1",
			Target: "function:main.go:step2",
		})

		flow := traceFlow(g, "function:main.go:main", 10)

		// Should include all nodes in the chain
		assert.Len(t, flow, 3)
	})

	t.Run("RespectsMaxDepth", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create deep call chain
		for i := 0; i < 20; i++ {
			g.AddNode(&graph.GraphNode{
				ID:       "function:main.go:func" + string(rune('A'+i)),
				Label:    graph.NodeFunction,
				Name:     "func" + string(rune('A'+i)),
				FilePath: "main.go",
			})
		}

		for i := 0; i < 19; i++ {
			g.AddRelationship(&graph.GraphRelationship{
				ID:     "calls:" + string(rune('A'+i)),
				Type:   graph.RelCalls,
				Source: "function:main.go:func" + string(rune('A'+i)),
				Target: "function:main.go:func" + string(rune('A'+i+1)),
			})
		}

		flow := traceFlow(g, "function:main.go:funcA", 5)

		// Should respect max depth
		assert.LessOrEqual(t, len(flow), 6) // Entry point + 5 levels
	})

	t.Run("HandlesCycles", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create cycle: A -> B -> C -> A
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:A",
			Label:    graph.NodeFunction,
			Name:     "A",
			FilePath: "main.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:B",
			Label:    graph.NodeFunction,
			Name:     "B",
			FilePath: "main.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:C",
			Label:    graph.NodeFunction,
			Name:     "C",
			FilePath: "main.go",
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:A",
			Target: "function:main.go:B",
		})
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:main.go:B",
			Target: "function:main.go:C",
		})
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:3",
			Type:   graph.RelCalls,
			Source: "function:main.go:C",
			Target: "function:main.go:A",
		})

		// Should not infinite loop
		flow := traceFlow(g, "function:main.go:A", 10)
		assert.NotEmpty(t, flow)
	})
}

func TestDeduplicateFlows(t *testing.T) {
	t.Parallel()

	t.Run("RemovesDuplicates", func(t *testing.T) {
		flows := [][]string{
			{"A", "B", "C"},
			{"A", "B", "C"}, // Duplicate
			{"A", "B", "D"},
		}

		deduped := deduplicateFlows(flows)

		assert.Len(t, deduped, 2)
	})

	t.Run("PreservesUnique", func(t *testing.T) {
		flows := [][]string{
			{"A", "B"},
			{"C", "D"},
			{"E", "F"},
		}

		deduped := deduplicateFlows(flows)

		assert.Len(t, deduped, 3)
	})
}
