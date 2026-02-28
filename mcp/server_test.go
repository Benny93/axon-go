package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

// mockStorage is a mock storage backend for testing.
type mockStorage struct {
	nodes         int
	relationships int
	searchResults []storage.SearchResult
	callers       []*graph.GraphNode
	callees       []*graph.GraphNode
	traverseNodes []*graph.GraphNode
}

func (m *mockStorage) FTSSearch(ctx context.Context, query string, limit int) ([]storage.SearchResult, error) {
	if m.searchResults != nil {
		return m.searchResults, nil
	}
	return []storage.SearchResult{
		{NodeID: "func:test.go:Foo", NodeName: "Foo", Label: "function", FilePath: "test.go", Score: 1.0},
	}, nil
}

func (m *mockStorage) GetCallers(ctx context.Context, nodeID string) ([]*graph.GraphNode, error) {
	return m.callers, nil
}

func (m *mockStorage) GetCallees(ctx context.Context, nodeID string) ([]*graph.GraphNode, error) {
	return m.callees, nil
}

func (m *mockStorage) Traverse(ctx context.Context, startID string, depth int, direction string) ([]*graph.GraphNode, error) {
	return m.traverseNodes, nil
}

func (m *mockStorage) NodeCount() int {
	return m.nodes
}

func (m *mockStorage) RelationshipCount() int {
	return m.relationships
}

func (m *mockStorage) Close() error {
	return nil
}

func (m *mockStorage) GetDeadCode(ctx context.Context) ([]*graph.GraphNode, error) {
	return nil, nil
}

func (m *mockStorage) HybridSearch(ctx context.Context, query string, queryVector []float32, limit int) ([]storage.HybridSearchResult, error) {
	return nil, nil
}

func (m *mockStorage) GetNodesByLabel(ctx context.Context, label string) []*graph.GraphNode {
	return nil
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		nodes:         10,
		relationships: 20,
		callers: []*graph.GraphNode{
			{ID: "func:a.go:Bar", Label: graph.NodeFunction, Name: "Bar", FilePath: "a.go"},
		},
		callees: []*graph.GraphNode{
			{ID: "func:b.go:Baz", Label: graph.NodeFunction, Name: "Baz", FilePath: "b.go"},
		},
		traverseNodes: []*graph.GraphNode{
			{ID: "func:a.go:Bar", Label: graph.NodeFunction, Name: "Bar", FilePath: "a.go"},
			{ID: "func:c.go:Qux", Label: graph.NodeFunction, Name: "Qux", FilePath: "c.go"},
		},
	}
}

func TestNewServer(t *testing.T) {
	t.Parallel()

	t.Run("CreatesServer", func(t *testing.T) {
		store := newMockStorage()
		server := NewServer(store)

		assert.NotNil(t, server)
		assert.NotNil(t, server.storage)
	})
}

func TestServer_Tools(t *testing.T) {
	t.Parallel()

	store := newMockStorage()
	server := NewServer(store)

	t.Run("ListTools", func(t *testing.T) {
		tools := server.ListTools()

		assert.NotEmpty(t, tools)
		assert.GreaterOrEqual(t, len(tools), 5)

		// Check expected tools exist
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		expectedTools := []string{
			"axon_query",
			"axon_context",
			"axon_impact",
			"axon_dead_code",
			"axon_list_repos",
		}

		for _, expected := range expectedTools {
			assert.True(t, toolNames[expected], "Should have tool: %s", expected)
		}
	})

	t.Run("ToolDescriptions", func(t *testing.T) {
		tools := server.ListTools()

		for _, tool := range tools {
			assert.NotEmpty(t, tool.Description)
			assert.NotNil(t, tool.InputSchema)
		}
	})
}

func TestServer_HandleToolCalls(t *testing.T) {
	t.Parallel()

	store := newMockStorage()
	server := NewServer(store)
	ctx := context.Background()

	t.Run("AxonListRepos", func(t *testing.T) {
		result, err := server.CallTool(ctx, "axon_list_repos", map[string]any{})
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("AxonQuery", func(t *testing.T) {
		result, err := server.CallTool(ctx, "axon_query", map[string]any{
			"query": "test",
			"limit": 10,
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("AxonQueryMissingQuery", func(t *testing.T) {
		result, err := server.CallTool(ctx, "axon_query", map[string]any{})
		assert.NoError(t, err)
		assert.Contains(t, result, "No query provided")
	})

	t.Run("UnknownTool", func(t *testing.T) {
		result, err := server.CallTool(ctx, "unknown_tool", map[string]any{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown tool")
		assert.Empty(t, result)
	})
}

func TestServer_Resources(t *testing.T) {
	t.Parallel()

	store := newMockStorage()
	server := NewServer(store)

	t.Run("ListResources", func(t *testing.T) {
		resources := server.ListResources()

		assert.NotEmpty(t, resources)
		assert.GreaterOrEqual(t, len(resources), 3)

		// Check expected resources exist
		resourceURIs := make(map[string]bool)
		for _, res := range resources {
			resourceURIs[res.URI] = true
		}

		expectedResources := []string{
			"axon://overview",
			"axon://dead-code",
			"axon://schema",
		}

		for _, expected := range expectedResources {
			assert.True(t, resourceURIs[expected], "Should have resource: %s", expected)
		}
	})

	t.Run("ResourceMetadata", func(t *testing.T) {
		resources := server.ListResources()

		for _, res := range resources {
			assert.NotEmpty(t, res.Name)
			assert.NotEmpty(t, res.Description)
			assert.NotEmpty(t, res.MimeType)
		}
	})
}

func TestServer_HandleResourceReads(t *testing.T) {
	t.Parallel()

	store := newMockStorage()
	server := NewServer(store)
	ctx := context.Background()

	t.Run("ReadOverview", func(t *testing.T) {
		content, err := server.ReadResource(ctx, "axon://overview")
		assert.NoError(t, err)
		assert.NotEmpty(t, content)
	})

	t.Run("ReadSchema", func(t *testing.T) {
		content, err := server.ReadResource(ctx, "axon://schema")
		assert.NoError(t, err)
		assert.NotEmpty(t, content)
		assert.Contains(t, content, "Node")
		assert.Contains(t, content, "Relationship")
	})

	t.Run("ReadDeadCode", func(t *testing.T) {
		content, err := server.ReadResource(ctx, "axon://dead-code")
		assert.NoError(t, err)
		assert.NotNil(t, content)
		// May be empty if no dead code detected
	})

	t.Run("ReadUnknownResource", func(t *testing.T) {
		content, err := server.ReadResource(ctx, "axon://unknown")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource")
		assert.Empty(t, content)
	})
}

func TestToolHandlers(t *testing.T) {
	t.Parallel()

	store := newMockStorage()

	t.Run("HandleQuery", func(t *testing.T) {
		result, err := handleQuery(store, "test", 10)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("HandleQueryEmpty", func(t *testing.T) {
		result, err := handleQuery(store, "", 10)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("HandleListRepos", func(t *testing.T) {
		result, err := handleListRepos()
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("HandleContext", func(t *testing.T) {
		result, err := handleContext(store, "SomeSymbol")
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("HandleImpact", func(t *testing.T) {
		result, err := handleImpact(store, "SomeSymbol", 3)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("HandleDeadCode", func(t *testing.T) {
		result, err := handleDeadCode(store)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("HandleCypher", func(t *testing.T) {
		result, err := handleCypher(store, "MATCH (n) RETURN n")
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestResourceHandlers(t *testing.T) {
	t.Parallel()

	store := newMockStorage()

	t.Run("GetOverview", func(t *testing.T) {
		content := getOverview(store)
		assert.NotEmpty(t, content)
		assert.Contains(t, content, "Nodes")
		assert.Contains(t, content, "Relationships")
	})

	t.Run("GetSchema", func(t *testing.T) {
		content := getSchema()
		assert.NotEmpty(t, content)
		assert.Contains(t, content, "Node")
		assert.Contains(t, content, "Relationship")
	})

	t.Run("GetDeadCodeList", func(t *testing.T) {
		content := getDeadCodeList(store)
		assert.NotNil(t, content)
		// May be empty if no dead code
	})
}

func TestServer_Run(t *testing.T) {
	t.Parallel()

	store := newMockStorage()
	server := NewServer(store)

	t.Run("RunWithNilStreams", func(t *testing.T) {
		// Should not panic with nil streams
		err := server.Run(context.Background(), nil, nil)
		assert.Error(t, err) // Should error with nil streams
	})
}
