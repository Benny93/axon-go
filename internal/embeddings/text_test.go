package embeddings

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestGenerateEmbeddingText(t *testing.T) {
	t.Parallel()

	t.Run("FunctionNode", func(t *testing.T) {
		node := &graph.GraphNode{
			Label:     graph.NodeFunction,
			Name:      "ProcessDeadCode",
			FilePath:  "internal/ingestion/dead_code.go",
			Signature: "func ProcessDeadCode(g *graph.KnowledgeGraph) int",
			Content:   "func ProcessDeadCode(g *graph.KnowledgeGraph) int {\n\t// Implementation\n}",
		}

		text := GenerateEmbeddingText(node)

		assert.Contains(t, text, "function ProcessDeadCode")
		assert.Contains(t, text, "in file internal/ingestion/dead_code.go")
		assert.Contains(t, text, "Signature: func ProcessDeadCode")
		assert.Contains(t, text, "Code: func ProcessDeadCode")
	})

	t.Run("MethodNode", func(t *testing.T) {
		node := &graph.GraphNode{
			Label:     graph.NodeMethod,
			Name:      "Run",
			FilePath:  "cmd/cmd.go",
			ClassName: "AnalyzeCmd",
			Signature: "func (c *AnalyzeCmd) Run() error",
			Content:   "func (c *AnalyzeCmd) Run() error {\n\t// Implementation\n}",
		}

		text := GenerateEmbeddingText(node)

		assert.Contains(t, text, "method Run")
		assert.Contains(t, text, "Method of class AnalyzeCmd")
		assert.Contains(t, text, "Signature: func (c *AnalyzeCmd) Run()")
	})

	t.Run("ClassNode", func(t *testing.T) {
		node := &graph.GraphNode{
			Label:     graph.NodeClass,
			Name:      "KnowledgeGraph",
			FilePath:  "internal/graph/graph.go",
			Signature: "type KnowledgeGraph struct",
			Content:   "type KnowledgeGraph struct {\n\t// Fields\n}",
		}

		text := GenerateEmbeddingText(node)

		assert.Contains(t, text, "class KnowledgeGraph")
		assert.Contains(t, text, "in file internal/graph/graph.go")
		assert.Contains(t, text, "Signature: type KnowledgeGraph struct")
	})

	t.Run("NodeWithLongContent", func(t *testing.T) {
		longContent := "func Test() {\n"
		for i := 0; i < 100; i++ {
			longContent += "\t// Line " + string(rune(i)) + "\n"
		}
		longContent += "}"

		node := &graph.GraphNode{
			Label:     graph.NodeFunction,
			Name:      "Test",
			FilePath:  "test.go",
			Signature: "func Test()",
			Content:   longContent,
		}

		text := GenerateEmbeddingText(node)

		// Should truncate to 500 chars
		assert.Contains(t, text, "Code: func Test()")
		assert.Less(t, len(text), 1000)
	})

	t.Run("NilNode", func(t *testing.T) {
		text := GenerateEmbeddingText(nil)
		assert.Empty(t, text)
	})

	t.Run("MinimalNode", func(t *testing.T) {
		node := &graph.GraphNode{
			Label: graph.NodeFunction,
			Name:  "SimpleFunc",
		}

		text := GenerateEmbeddingText(node)
		assert.Contains(t, text, "function SimpleFunc")
	})
}

func TestGenerateNodeText(t *testing.T) {
	t.Parallel()

	t.Run("FunctionNode", func(t *testing.T) {
		node := &graph.GraphNode{
			Label:     graph.NodeFunction,
			Name:      "RunPipeline",
			FilePath:  "internal/ingestion/pipeline.go",
			Signature: "func RunPipeline(...) (*graph.KnowledgeGraph, *PipelineResult, error)",
		}

		text := GenerateNodeText(node)

		assert.Contains(t, text, "function RunPipeline")
		assert.Contains(t, text, "internal/ingestion/pipeline.go")
		assert.Contains(t, text, "func RunPipeline")
	})

	t.Run("NilNode", func(t *testing.T) {
		text := GenerateNodeText(nil)
		assert.Empty(t, text)
	})

	t.Run("NodeWithoutSignature", func(t *testing.T) {
		node := &graph.GraphNode{
			Label:    graph.NodeClass,
			Name:     "MyClass",
			FilePath: "myclass.go",
		}

		text := GenerateNodeText(node)
		assert.Contains(t, text, "class MyClass")
		assert.Contains(t, text, "myclass.go")
	})
}
