package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/parsers"
	"github.com/Benny93/axon-go/internal/storage"
)

func TestProcessCalls_CrossPackage(t *testing.T) {
	t.Parallel()

	t.Run("ResolveCrossPackageCall", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add function in ingestion package
		g.AddNode(&graph.GraphNode{
			ID:       "function:internal/ingestion/pipeline.go:RunPipeline",
			Label:    graph.NodeFunction,
			Name:     "RunPipeline",
			FilePath: "internal/ingestion/pipeline.go",
		})

		// Add function in cmd package that calls RunPipeline
		g.AddNode(&graph.GraphNode{
			ID:       "function:cmd/cmd.go:AnalyzeCmd_Run",
			Label:    graph.NodeFunction,
			Name:     "AnalyzeCmd_Run",
			FilePath: "cmd/cmd.go",
		})

		// Create parse data with a cross-package call
		parseData := NewParseData()
		parseData.AddFile("cmd/cmd.go", &parsers.ParseResult{
			Package: "cmd",
			PackageImports: map[string]string{
				"ingestion": "github.com/Benny93/axon-go/internal/ingestion",
			},
			Symbols: []parsers.ParsedSymbol{
				{Name: "AnalyzeCmd_Run", Kind: graph.NodeFunction},
			},
			Calls: []parsers.CallSite{
				{Name: "RunPipeline", Package: "github.com/Benny93/axon-go/internal/ingestion"},
			},
		})

		// Process calls
		ProcessCalls(parseData, g)

		// Verify CALLS relationship was created
		rels := g.GetRelationshipsByType(graph.RelCalls)
		assert.NotEmpty(t, rels, "Should have CALLS relationships")

		// Find the specific call
		var foundCall bool
		for _, rel := range rels {
			if rel.Source == "function:cmd/cmd.go:AnalyzeCmd_Run" &&
				rel.Target == "function:internal/ingestion/pipeline.go:RunPipeline" {
				foundCall = true
				break
			}
		}
		assert.True(t, foundCall, "Should have call from AnalyzeCmd_Run to RunPipeline")
	})
}

func TestDeadCodeIntegration(t *testing.T) {
	t.Parallel()

	t.Run("DeadCodeInPipeline", func(t *testing.T) {
		// Create test graph with dead code
		g := graph.NewKnowledgeGraph()

		// Add entry point (not dead)
		g.AddNode(&graph.GraphNode{
			ID:           "function:main.go:main",
			Label:        graph.NodeFunction,
			Name:         "main",
			FilePath:     "main.go",
			IsEntryPoint: true,
		})

		// Add called function (not dead)
		g.AddNode(&graph.GraphNode{
			ID:       "function:main.go:helper",
			Label:    graph.NodeFunction,
			Name:     "helper",
			FilePath: "main.go",
		})

		// Add unused function (dead)
		g.AddNode(&graph.GraphNode{
			ID:       "function:unused.go:deadFunction",
			Label:    graph.NodeFunction,
			Name:     "deadFunction",
			FilePath: "unused.go",
		})

		// Add call relationship
		g.AddRelationship(&graph.GraphRelationship{
			ID:     "calls:1",
			Type:   graph.RelCalls,
			Source: "function:main.go:main",
			Target: "function:main.go:helper",
		})

		// Run dead code detection
		count := ProcessDeadCode(g)

		assert.Equal(t, 1, count)

		// Verify dead code flag
		deadFunc := g.GetNode("function:unused.go:deadFunction")
		assert.NotNil(t, deadFunc)
		assert.True(t, deadFunc.IsDead)

		// Verify live code is not flagged
		mainFunc := g.GetNode("function:main.go:main")
		assert.False(t, mainFunc.IsDead)

		helperFunc := g.GetNode("function:main.go:helper")
		assert.False(t, helperFunc.IsDead)
	})

	t.Run("GetDeadCodeList", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add some dead code
		g.AddNode(&graph.GraphNode{
			ID:       "function:a.go:dead1",
			Label:    graph.NodeFunction,
			Name:     "dead1",
			FilePath: "a.go",
			IsDead:   true,
		})

		g.AddNode(&graph.GraphNode{
			ID:       "function:b.go:dead2",
			Label:    graph.NodeFunction,
			Name:     "dead2",
			FilePath: "b.go",
			IsDead:   true,
		})

		g.AddNode(&graph.GraphNode{
			ID:       "function:c.go:live",
			Label:    graph.NodeFunction,
			Name:     "live",
			FilePath: "c.go",
			IsDead:   false,
		})

		// Get dead code list
		deadCode := GetDeadCodeList(g)

		assert.Len(t, deadCode, 2)

		// Verify grouping by file
		files := make(map[string][]string)
		for _, node := range deadCode {
			files[node.FilePath] = append(files[node.FilePath], node.Name)
		}

		assert.Contains(t, files, "a.go")
		assert.Contains(t, files, "b.go")
		assert.NotContains(t, files, "c.go")
	})

	t.Run("StoreDeadCodeInBackend", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create graph with dead code
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:       "function:test.go:dead",
			Label:    graph.NodeFunction,
			Name:     "dead",
			FilePath: "test.go",
			IsDead:   true,
		})

		// Store in backend
		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Verify node was stored with IsDead flag
		node, err := store.GetNode(t.Context(), "function:test.go:dead")
		require.NoError(t, err)
		assert.NotNil(t, node)
		assert.True(t, node.IsDead)
	})
}

func TestDeadCodeExemptions(t *testing.T) {
	t.Parallel()

	t.Run("TestFunctionsNotDead", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add test functions (should not be dead)
		g.AddNode(&graph.GraphNode{
			ID:       "function:main_test.go:TestFoo",
			Label:    graph.NodeFunction,
			Name:     "TestFoo",
			FilePath: "main_test.go",
		})

		count := ProcessDeadCode(g)
		assert.Equal(t, 0, count)

		node := g.GetNode("function:main_test.go:TestFoo")
		assert.False(t, node.IsDead)
	})

	t.Run("ExportedFunctionsNotDead", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add exported function (should not be dead)
		g.AddNode(&graph.GraphNode{
			ID:         "function:api.go:PublicAPI",
			Label:      graph.NodeFunction,
			Name:       "PublicAPI",
			FilePath:   "api.go",
			IsExported: true,
		})

		count := ProcessDeadCode(g)
		assert.Equal(t, 0, count)

		node := g.GetNode("function:api.go:PublicAPI")
		assert.False(t, node.IsDead)
	})

	t.Run("DunderMethodsNotDead", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add dunder methods (should not be dead)
		g.AddNode(&graph.GraphNode{
			ID:        "method:class.go:MyClass.__init__",
			Label:     graph.NodeMethod,
			Name:      "__init__",
			ClassName: "MyClass",
			FilePath:  "class.go",
		})

		g.AddNode(&graph.GraphNode{
			ID:        "method:class.go:MyClass.__str__",
			Label:     graph.NodeMethod,
			Name:      "__str__",
			ClassName: "MyClass",
			FilePath:  "class.go",
		})

		count := ProcessDeadCode(g)
		assert.Equal(t, 0, count)
	})
}
