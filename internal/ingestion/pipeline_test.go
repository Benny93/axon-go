package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/parsers"
	"github.com/Benny93/axon-go/internal/storage"
)

func TestProcessStructure(t *testing.T) {
	t.Parallel()

	t.Run("CreatesFolderAndFileNodes", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		entries := []FileEntry{
			{Path: "/repo/main.go", RelPath: "main.go", Language: "go"},
			{Path: "/repo/src/app.go", RelPath: "src/app.go", Language: "go"},
		}

		ProcessStructure(entries, g)

		// Should have folder and file nodes
		assert.GreaterOrEqual(t, g.NodeCount(), 2)

		// Check for main.go file node
		fileNode := g.GetNode("file:main.go")
		assert.NotNil(t, fileNode)
		assert.Equal(t, graph.NodeFile, fileNode.Label)
	})

	t.Run("CreatesContainsRelationships", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		entries := []FileEntry{
			{Path: "/repo/src/app.go", RelPath: "src/app.go", Language: "go"},
		}

		ProcessStructure(entries, g)

		// Should have CONTAINS relationships
		rels := g.GetRelationshipsByType(graph.RelContains)
		assert.NotEmpty(t, rels)
	})
}

func TestProcessParsing(t *testing.T) {
	t.Parallel()

	t.Run("ParsesGoFiles", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		content := []byte(`
package main

func greet(name string) string {
	return "Hello, " + name
}

type User struct {
	Name string
}
`)
		entries := []FileEntry{
			{Path: "/repo/test.go", RelPath: "test.go", Language: "go", Content: content},
		}

		parseData := ProcessParsing(entries, g)

		assert.NotNil(t, parseData)
		assert.NotEmpty(t, parseData.Files)

		// Check parsed symbols
		fileData, ok := parseData.Files["test.go"]
		assert.True(t, ok)
		assert.NotEmpty(t, fileData.Symbols)
	})

	t.Run("HandlesMultipleFiles", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		entries := []FileEntry{
			{Path: "/repo/a.go", RelPath: "a.go", Language: "go", Content: []byte("package main\nfunc A() {}")},
			{Path: "/repo/b.go", RelPath: "b.go", Language: "go", Content: []byte("package main\nfunc B() {}")},
		}

		parseData := ProcessParsing(entries, g)

		assert.Len(t, parseData.Files, 2)
	})
}

func TestProcessImports(t *testing.T) {
	t.Parallel()

	t.Run("CreatesImportRelationships", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create file nodes first
		g.AddNode(&graph.GraphNode{ID: "file:a.go", Label: graph.NodeFile, FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "file:./b.go", Label: graph.NodeFile, FilePath: "./b.go"})

		parseData := &ParseData{
			Files: map[string]*parsers.ParseResult{
				"a.go": {
					Imports: []parsers.ImportStatement{
						{ModulePath: "./b", Symbols: []string{"B"}},
					},
				},
			},
		}

		ProcessImports(parseData, g)

		// Should have IMPORTS relationships for relative imports
		rels := g.GetRelationshipsByType(graph.RelImports)
		assert.NotEmpty(t, rels)
	})
}

func TestProcessCalls(t *testing.T) {
	t.Parallel()

	t.Run("CreatesCallRelationships", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Create function nodes
		g.AddNode(&graph.GraphNode{ID: "function:a.go:Foo", Label: graph.NodeFunction, Name: "Foo", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "function:b.go:Bar", Label: graph.NodeFunction, Name: "Bar", FilePath: "b.go"})

		parseData := &ParseData{
			Files: map[string]*parsers.ParseResult{
				"a.go": {
					Calls: []parsers.CallSite{
						{Name: "Bar", Receiver: ""},
					},
					Symbols: []parsers.ParsedSymbol{
						{Name: "Foo", Kind: graph.NodeFunction},
					},
				},
			},
		}

		ProcessCalls(parseData, g)

		// Should have CALLS relationships
		rels := g.GetRelationshipsByType(graph.RelCalls)
		assert.NotEmpty(t, rels)
	})
}

func TestProcessHeritage(t *testing.T) {
	t.Parallel()

	t.Run("CreatesExtendsRelationships", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		g.AddNode(&graph.GraphNode{ID: "class:a.go:Base", Label: graph.NodeClass, Name: "Base", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "class:a.go:Derived", Label: graph.NodeClass, Name: "Derived", FilePath: "a.go"})

		parseData := &ParseData{
			Files: map[string]*parsers.ParseResult{
				"a.go": {
					Heritage: []parsers.ClassHeritage{
						{ClassName: "Derived", Extends: []string{"Base"}},
					},
				},
			},
		}

		ProcessHeritage(parseData, g)

		// Should have EXTENDS relationships
		rels := g.GetRelationshipsByType(graph.RelExtends)
		assert.NotEmpty(t, rels)
	})
}

func TestProcessTypes(t *testing.T) {
	t.Parallel()

	t.Run("CreatesUsesTypeRelationships", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		g.AddNode(&graph.GraphNode{ID: "function:a.go:Foo", Label: graph.NodeFunction, Name: "Foo", FilePath: "a.go"})
		g.AddNode(&graph.GraphNode{ID: "class:a.go:User", Label: graph.NodeClass, Name: "User", FilePath: "a.go"})

		parseData := &ParseData{
			Files: map[string]*parsers.ParseResult{
				"a.go": {
					TypeRefs: []parsers.TypeAnnotation{
						{Name: "User", Role: "param"},
					},
					Symbols: []parsers.ParsedSymbol{
						{Name: "Foo", Kind: graph.NodeFunction},
					},
				},
			},
		}

		ProcessTypes(parseData, g)

		// Should have USES_TYPE relationships
		rels := g.GetRelationshipsByType(graph.RelUsesType)
		assert.NotEmpty(t, rels)
	})
}

func TestRunPipeline(t *testing.T) {
	t.Parallel()

	t.Run("FullPipeline", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test Go files
		files := map[string]string{
			"main.go": `
package main

import "fmt"

func main() {
	fmt.Println("Hello")
}

type Service struct {}

func (s *Service) Run() {}
`,
			"service.go": `
package main

type Database struct {}
`,
		}

		for path, content := range files {
			fullPath := filepath.Join(tmpDir, path)
			err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
			require.NoError(t, err)
			err = os.WriteFile(fullPath, []byte(content), 0o644)
			require.NoError(t, err)
		}

		// Create in-memory storage
		store := storage.NewMemoryBackend()
		err := store.Initialize(filepath.Join(tmpDir, "db"), false)
		require.NoError(t, err)
		defer store.Close()

		// Run pipeline
		ctx := context.Background()
		graph, result, err := RunPipeline(ctx, tmpDir, store, false, nil, false)

		assert.NoError(t, err)
		assert.NotNil(t, graph)
		assert.NotNil(t, result)

		// Check results
		assert.Greater(t, result.Files, 0)
		assert.Greater(t, result.Symbols, 0)
		assert.Greater(t, result.Relationships, 0)
	})
}

func TestParseData(t *testing.T) {
	t.Parallel()

	t.Run("NewParseData", func(t *testing.T) {
		data := NewParseData()
		assert.NotNil(t, data)
		assert.NotNil(t, data.Files)
	})

	t.Run("AddFile", func(t *testing.T) {
		data := NewParseData()

		result := &parsers.ParseResult{
			Package: "main",
			Symbols: []parsers.ParsedSymbol{{Name: "Foo"}},
		}

		data.AddFile("test.go", result)

		assert.Len(t, data.Files, 1)
		assert.Equal(t, "main", data.Files["test.go"].Package)
	})
}
