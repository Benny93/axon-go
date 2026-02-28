package storage

import (
	"context"
	"math"
	"sort"
)

// HybridSearch combines FTS and vector search using Reciprocal Rank Fusion (RRF).
// k is the RRF constant (typically 60).
func HybridSearch(ctx context.Context, storage StorageBackend, query string, queryVector []float32, limit, k int) ([]HybridSearchResult, error) {
	// Get FTS results
	ftsResults, err := storage.FTSSearch(ctx, query, limit*2)
	if err != nil {
		ftsResults = []SearchResult{}
	}

	// Get vector results
	var vectorResults []SearchResult
	if len(queryVector) > 0 {
		vectorResults, err = storage.VectorSearch(ctx, queryVector, limit*2)
		if err != nil {
			vectorResults = []SearchResult{}
		}
	}

	// Apply RRF fusion
	rrfScores := make(map[string]float64)
	metadata := make(map[string]SearchResult)

	// Add FTS scores
	for i, result := range ftsResults {
		rrfScores[result.NodeID] += 1.0 / float64(k+i)
		if _, exists := metadata[result.NodeID]; !exists {
			metadata[result.NodeID] = result
		}
	}

	// Add vector scores
	for i, result := range vectorResults {
		rrfScores[result.NodeID] += 1.0 / float64(k+i)
		if _, exists := metadata[result.NodeID]; !exists {
			metadata[result.NodeID] = result
		}
	}

	// Convert to results and sort
	results := make([]HybridSearchResult, 0, len(rrfScores))
	for nodeID, score := range rrfScores {
		meta := metadata[nodeID]
		results = append(results, HybridSearchResult{
			NodeID:   nodeID,
			Score:    score,
			NodeName: meta.NodeName,
			FilePath: meta.FilePath,
			Label:    meta.Label,
			Snippet:  meta.Snippet,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
