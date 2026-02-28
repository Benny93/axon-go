package embeddings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestTFIDFEmbedder(t *testing.T) {
	t.Parallel()

	t.Run("BuildVocabulary", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		docs := []string{
			"function ProcessDeadCode in internal/ingestion",
			"function RunPipeline in internal/ingestion",
			"class KnowledgeGraph in internal/graph",
		}

		embedder.BuildVocabulary(docs)

		assert.Greater(t, len(embedder.vocab), 0)
		assert.Equal(t, len(docs), embedder.docCount)
	})

	t.Run("ComputeIDF", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		docs := []string{
			"function process dead code",
			"function run pipeline",
			"class knowledge graph",
		}

		embedder.BuildVocabulary(docs)
		embedder.ComputeIDF(docs)

		// Common term "function" should have lower IDF (appears in 2/3 docs)
		assert.Greater(t, embedder.idf["function"], float64(0))
		// Rare terms should have higher IDF (appear in 1/3 docs)
		// Note: IDF = log(N/df), so rarer terms have higher IDF
		assert.Greater(t, embedder.idf["dead"], embedder.idf["function"])
		assert.Greater(t, embedder.idf["graph"], embedder.idf["function"])
	})

	t.Run("Embed", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		docs := []string{
			"function ProcessDeadCode detects unused code",
			"function RunPipeline processes files",
			"class KnowledgeGraph stores nodes",
		}

		embedder.BuildVocabulary(docs)
		embedder.ComputeIDF(docs)

		embedding := embedder.Embed("function ProcessDeadCode detects unused code")

		assert.Len(t, embedding, EmbeddingDimension)

		// Embedding should be L2 normalized (norm ≈ 1)
		norm := 0.0
		for _, v := range embedding {
			norm += float64(v * v)
		}
		assert.InDelta(t, 1.0, norm, 0.01)
	})

	t.Run("EmbedSimilar", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		docs := []string{
			"function ProcessDeadCode detects unused code",
			"function RunPipeline processes files",
		}

		embedder.BuildVocabulary(docs)
		embedder.ComputeIDF(docs)

		// Similar documents should have similar embeddings
		emb1 := embedder.Embed("function ProcessDeadCode detects unused code")
		emb2 := embedder.Embed("function ProcessDeadCode detects unused code")

		// Dot product should be close to 1 (identical embeddings)
		dotProduct := 0.0
		for i := range emb1 {
			dotProduct += float64(emb1[i] * emb2[i])
		}
		assert.InDelta(t, 1.0, dotProduct, 0.01)
	})

	t.Run("EmbedDissimilar", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		docs := []string{
			"function ProcessDeadCode detects unused code",
			"class KnowledgeGraph stores nodes and relationships",
		}

		embedder.BuildVocabulary(docs)
		embedder.ComputeIDF(docs)

		// Dissimilar documents should have lower similarity
		emb1 := embedder.Embed("function ProcessDeadCode detects unused code")
		emb2 := embedder.Embed("class KnowledgeGraph stores nodes")

		// Dot product should be less than 1
		dotProduct := 0.0
		for i := range emb1 {
			dotProduct += float64(emb1[i] * emb2[i])
		}
		assert.Less(t, dotProduct, 0.9)
	})

	t.Run("EmbedEmpty", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		embedding := embedder.Embed("")
		assert.Len(t, embedding, EmbeddingDimension)
	})

	t.Run("EmbedNodes", func(t *testing.T) {
		embedder := NewTFIDFEmbedder()
		nodes := []*graph.GraphNode{
			{
				Label:     graph.NodeFunction,
				Name:      "ProcessDeadCode",
				FilePath:  "internal/ingestion/dead_code.go",
				Signature: "func ProcessDeadCode(g *graph.KnowledgeGraph) int",
			},
			{
				Label:     graph.NodeFunction,
				Name:      "RunPipeline",
				FilePath:  "internal/ingestion/pipeline.go",
				Signature: "func RunPipeline(...) (*graph.KnowledgeGraph, *PipelineResult, error)",
			},
		}

		embeddings := embedder.EmbedNodes(nodes)

		require.Len(t, embeddings, len(nodes))
		for _, emb := range embeddings {
			assert.Len(t, emb, EmbeddingDimension)
		}
	})
}

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "SimpleText",
			input:    "hello world",
			expected: []string{"hello", "world"},
		},
		{
			name:     "WithSeparators",
			input:    "hello_world test-case",
			expected: []string{"hello", "world", "test", "case"},
		},
		{
			name:     "CamelCase",
			input:    "UserService",
			expected: []string{"userservice"},
		},
		{
			name:     "WithNumbers",
			input:    "HTTP2Client",
			expected: []string{"http2client"},
		},
		{
			name:     "ShortTermsFiltered",
			input:    "a b cd",
			expected: []string{"cd"},
		},
		{
			name:     "MixedCase",
			input:    "Hello WORLD Test",
			expected: []string{"hello", "world", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tokenize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
