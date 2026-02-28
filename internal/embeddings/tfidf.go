package embeddings

import (
	"math"
	"strings"
	"sync"

	"github.com/Benny93/axon-go/internal/graph"
)

// EmbeddingDimension is the dimension of generated embeddings.
const EmbeddingDimension = 100

// TFIDFEmbedder generates TF-IDF based embeddings for code symbols.
// This is a simple embedding model that doesn't require external ML models.
type TFIDFEmbedder struct {
	mu       sync.RWMutex
	idf      map[string]float64 // term -> IDF score
	docCount int                // number of documents processed
	vocab    map[string]int     // term -> index in embedding vector
}

// NewTFIDFEmbedder creates a new TF-IDF embedder.
func NewTFIDFEmbedder() *TFIDFEmbedder {
	return &TFIDFEmbedder{
		idf:   make(map[string]float64),
		vocab: make(map[string]int),
	}
}

// BuildVocabulary builds the vocabulary from a set of documents.
func (e *TFIDFEmbedder) BuildVocabulary(docs []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	termIndex := 0
	for _, doc := range docs {
		terms := tokenize(doc)
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				if _, exists := e.vocab[term]; !exists {
					e.vocab[term] = termIndex
					termIndex++
					if termIndex >= EmbeddingDimension {
						return
					}
				}
				seen[term] = true
			}
		}
	}
	e.docCount = len(docs)
}

// ComputeIDF computes IDF scores for all terms in the vocabulary.
func (e *TFIDFEmbedder) ComputeIDF(docs []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Count document frequency for each term
	docFreq := make(map[string]int)
	for _, doc := range docs {
		terms := tokenize(doc)
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	// Compute IDF: log(N / df)
	for term, df := range docFreq {
		if df > 0 {
			e.idf[term] = math.Log(float64(e.docCount) / float64(df))
		}
	}
}

// Embed generates a TF-IDF embedding for a document.
func (e *TFIDFEmbedder) Embed(doc string) []float32 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	embedding := make([]float32, EmbeddingDimension)

	// Compute term frequency
	tf := make(map[string]int)
	terms := tokenize(doc)
	for _, term := range terms {
		tf[term]++
	}

	// Normalize TF
	maxTF := 0.0
	for _, count := range tf {
		if float64(count) > maxTF {
			maxTF = float64(count)
		}
	}

	// Compute TF-IDF for each term
	for term, count := range tf {
		normalizedTF := float64(count) / maxTF
		idf := e.idf[term]
		if idf == 0 {
			idf = 1.0 // Default IDF for unseen terms
		}

		tfidf := normalizedTF * idf

		if idx, exists := e.vocab[term]; exists {
			embedding[idx] = float32(tfidf)
		}
	}

	// L2 normalize the embedding
	norm := 0.0
	for _, v := range embedding {
		norm += float64(v * v)
	}
	norm = math.Sqrt(norm)

	if norm > 0 && !math.IsNaN(norm) {
		for i := range embedding {
			val := embedding[i] / float32(norm)
			if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
				embedding[i] = 0
			} else {
				embedding[i] = val
			}
		}
	}

	return embedding
}

// EmbedNode generates an embedding for a graph node.
func (e *TFIDFEmbedder) EmbedNode(node *graph.GraphNode) []float32 {
	text := GenerateEmbeddingText(node)
	return e.Embed(text)
}

// EmbedNodes generates embeddings for multiple nodes.
func (e *TFIDFEmbedder) EmbedNodes(nodes []*graph.GraphNode) [][]float32 {
	// First pass: build vocabulary
	docs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		docs = append(docs, GenerateEmbeddingText(node))
	}

	e.BuildVocabulary(docs)
	e.ComputeIDF(docs)

	// Second pass: generate embeddings
	embeddings := make([][]float32, len(nodes))
	for i, node := range nodes {
		embeddings[i] = e.EmbedNode(node)
	}

	return embeddings
}

// tokenize splits text into terms.
func tokenize(text string) []string {
	text = strings.ToLower(text)

	// Split on non-alphanumeric characters
	terms := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})

	// Filter out very short terms
	filtered := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) >= 2 {
			filtered = append(filtered, term)
		}
	}

	return filtered
}
