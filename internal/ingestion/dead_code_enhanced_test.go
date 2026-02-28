package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestDeadCodeWithIntraClassCalls(t *testing.T) {
	t.Parallel()

	t.Run("TracksIntraClassMethodCalls", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create a class with methods
		g.AddNode(&graph.GraphNode{
			ID:        "function:parser.go:GoParser",
			Label:     graph.NodeClass,
			Name:      "GoParser",
			FilePath:  "parser.go",
			ClassName: "GoParser",
		})

		// Main Parse method
		g.AddNode(&graph.GraphNode{
			ID:        "function:parser.go:Parse",
			Label:     graph.NodeMethod,
			Name:      "Parse",
			FilePath:  "parser.go",
			ClassName: "GoParser",
		})

		// Helper method parseFuncDecl
		g.AddNode(&graph.GraphNode{
			ID:        "function:parser.go:parseFuncDecl",
			Label:     graph.NodeMethod,
			Name:      "parseFuncDecl",
			FilePath:  "parser.go",
			ClassName: "GoParser",
		})

		// Helper method parseImports
		g.AddNode(&graph.GraphNode{
			ID:        "function:parser.go:parseImports",
			Label:     graph.NodeMethod,
			Name:      "parseImports",
			FilePath:  "parser.go",
			ClassName: "GoParser",
		})

		// Add CALLS relationship: Parse -> parseFuncDecl
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:parser.go:Parse",
			Target: "function:parser.go:parseFuncDecl",
		})

		// Add CALLS relationship: Parse -> parseImports
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:parser.go:Parse",
			Target: "function:parser.go:parseImports",
		})

		// Add entry point that calls Parse
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:3",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:parser.go:Parse",
		})

		// Run dead code detection
		_ = ProcessDeadCode(g)

		// Verify no methods are marked as dead
		for node := range g.IterNodes() {
			if node.Label == graph.NodeMethod {
				assert.False(t, node.IsDead, "Method %s should not be dead", node.Name)
			}
			// Entry point should not be dead
			if node.IsEntryPoint {
				assert.False(t, node.IsDead, "Entry point %s should not be dead", node.Name)
			}
		}
	})

	t.Run("HandlesPrivateMethods", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create class with private helper method
		g.AddNode(&graph.GraphNode{
			ID:        "function:backend.go:BadgerBackend",
			Label:     graph.NodeClass,
			Name:      "BadgerBackend",
			FilePath:  "backend.go",
			ClassName: "BadgerBackend",
		})

		// Public method
		g.AddNode(&graph.GraphNode{
			ID:        "function:backend.go:GetNode",
			Label:     graph.NodeMethod,
			Name:      "GetNode",
			FilePath:  "backend.go",
			ClassName: "BadgerBackend",
		})

		// Private helper method
		g.AddNode(&graph.GraphNode{
			ID:        "function:backend.go:getNode",
			Label:     graph.NodeMethod,
			Name:      "getNode",
			FilePath:  "backend.go",
			ClassName: "BadgerBackend",
		})

		// GetNode calls getNode
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:backend.go:GetNode",
			Target: "function:backend.go:getNode",
		})

		// External caller (entry point)
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:2",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:backend.go:GetNode",
		})

		_ = ProcessDeadCode(g)

		// GetNode and getNode should NOT be dead
		for node := range g.IterNodes() {
			if node.Label == graph.NodeMethod && node.ClassName == "BadgerBackend" {
				assert.False(t, node.IsDead, "Method %s should not be dead", node.Name)
			}
		}
	})
}

func TestDeadCodeWithConfidenceScores(t *testing.T) {
	t.Parallel()

	t.Run("AssignsConfidenceScores", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Function with no callers (high confidence if dead)
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:UnusedFunc",
			Label:    graph.NodeFunction,
			Name:     "UnusedFunc",
			FilePath: "test.go",
		})

		// Private function with no callers (medium confidence)
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:helperFunc",
			Label:    graph.NodeFunction,
			Name:     "helperFunc",
			FilePath: "test.go",
		})

		_ = ProcessDeadCode(g)

		// Check confidence scores
		for node := range g.IterNodes() {
			if node.Name == "UnusedFunc" && node.IsDead {
				// Should have high confidence (public, no callers)
				confidence := node.Properties["dead_code_confidence"]
				assert.Equal(t, "high", confidence)
			}
			if node.Name == "helperFunc" && node.IsDead {
				// Should have medium confidence (private function)
				confidence := node.Properties["dead_code_confidence"]
				assert.Equal(t, "medium", confidence)
			}
		}
	})

	t.Run("MarksFrameworkPatternsAsLowConfidence", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// CLI command method (framework pattern)
		g.AddNode(&graph.GraphNode{
			ID:        "function:cmd.go:setupQwen",
			Label:     graph.NodeMethod,
			Name:      "setupQwen",
			FilePath:  "cmd.go",
			ClassName: "SetupCmd",
			Properties: map[string]any{
				"call_pattern": "framework_dispatch",
			},
		})

		_ = ProcessDeadCode(g)

		// Should not be marked as dead, or marked with low confidence
		for node := range g.IterNodes() {
			if node.Name == "setupQwen" {
				if node.IsDead {
					confidence := node.Properties["dead_code_confidence"]
					assert.Equal(t, "low", confidence, "Framework methods should have low confidence")
				}
			}
		}
	})
}

func TestDeadCodeWithAllowlist(t *testing.T) {
	t.Parallel()

	t.Run("ExemptsAllowlistedPatterns", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// MCP handler method (should be allowlisted)
		g.AddNode(&graph.GraphNode{
			ID:        "function:server.go:handleInitialize",
			Label:     graph.NodeMethod,
			Name:      "handleInitialize",
			FilePath:  "server.go",
			ClassName: "Server",
			Properties: map[string]any{
				"call_pattern": "mcp_handler",
			},
		})

		// CLI setup method (should be allowlisted)
		g.AddNode(&graph.GraphNode{
			ID:        "function:cmd.go:setupClaude",
			Label:     graph.NodeMethod,
			Name:      "setupClaude",
			FilePath:  "cmd.go",
			ClassName: "SetupCmd",
			Properties: map[string]any{
				"call_pattern": "cli_subcommand",
			},
		})

		// Regular unused function (should be detected as dead)
		g.AddNode(&graph.GraphNode{
			ID:       "function:utils.go:unusedHelper",
			Label:    graph.NodeFunction,
			Name:     "unusedHelper",
			FilePath: "utils.go",
		})

		// Manually flag as dead (simulating flagUnreachable phase)
		for node := range g.IterNodes() {
			if node.Label != graph.NodeFile && node.Label != graph.NodeFolder {
				node.IsDead = true
			}
		}

		// Apply allowlist exemptions
		applyAllowlistExemptions(g)

		// MCP and CLI methods should be exempt
		for node := range g.IterNodes() {
			if node.Name == "handleInitialize" || node.Name == "setupClaude" {
				exempt := node.Properties["dead_code_exempt"]
				assert.Equal(t, true, exempt, "%s should be exempt", node.Name)
				assert.False(t, node.IsDead, "%s should not be dead after exemption", node.Name)
			}
			// Regular function should still be dead
			if node.Name == "unusedHelper" {
				assert.True(t, node.IsDead, "Regular unused function should be dead")
			}
		}
	})

	t.Run("AllowlistPatterns", func(t *testing.T) {
		tests := []struct {
			name     string
			node     *graph.GraphNode
			expected bool // should be exempt
		}{
			{
				name: "MCPHandler",
				node: &graph.GraphNode{
					Name:     "handleInitialize",
					FilePath: "server.go",
					Properties: map[string]any{
						"call_pattern": "mcp_handler",
					},
				},
				expected: true,
			},
			{
				name: "CLISubcommand",
				node: &graph.GraphNode{
					Name:     "setupQwen",
					FilePath: "cmd.go",
					Properties: map[string]any{
						"call_pattern": "cli_subcommand",
					},
				},
				expected: true,
			},
			{
				name: "RegularFunction",
				node: &graph.GraphNode{
					Name:       "helper",
					FilePath:   "utils.go",
					Properties: map[string]any{},
				},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				exempt := isAllowlistExempt(tt.node)
				assert.Equal(t, tt.expected, exempt)
			})
		}
	})
}

func TestDeadCodeCallPatternDetection(t *testing.T) {
	t.Parallel()

	t.Run("DetectsDynamicDispatch", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add handler method
		g.AddNode(&graph.GraphNode{
			ID:       "function:server.go:handleTools",
			Label:    graph.NodeMethod,
			Name:     "handleTools",
			FilePath: "server.go",
		})

		// Add caller with dynamic dispatch pattern
		g.AddNode(&graph.GraphNode{
			ID:       "function:server.go:handleRequest",
			Label:    graph.NodeFunction,
			Name:     "handleRequest",
			FilePath: "server.go",
			Properties: map[string]any{
				"has_dynamic_dispatch": true,
			},
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:server.go:handleRequest",
			Target: "function:server.go:handleTools",
		})

		// Detect call patterns
		detectCallPatterns(g)

		// handleTools should be marked as dynamic dispatch
		for node := range g.IterNodes() {
			if node.Name == "handleTools" {
				pattern := node.Properties["call_pattern"]
				assert.Equal(t, "dynamic_dispatch", pattern)
			}
		}
	})

	t.Run("DetectsFrameworkDispatch", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add CLI command
		g.AddNode(&graph.GraphNode{
			ID:       "function:cmd.go:setupCursor",
			Label:    graph.NodeMethod,
			Name:     "setupCursor",
			FilePath: "cmd.go",
		})

		// Add switch-case caller
		g.AddNode(&graph.GraphNode{
			ID:       "function:cmd.go:Run",
			Label:    graph.NodeMethod,
			Name:     "Run",
			FilePath: "cmd.go",
			Properties: map[string]any{
				"has_switch_dispatch": true,
			},
		})

		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:cmd.go:Run",
			Target: "function:cmd.go:setupCursor",
		})

		detectCallPatterns(g)

		for node := range g.IterNodes() {
			if node.Name == "setupCursor" {
				pattern := node.Properties["call_pattern"]
				assert.Equal(t, "framework_dispatch", pattern)
			}
		}
	})
}
