package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHybridSearch(t *testing.T) {
	t.Parallel()

	t.Run("RRFFusion", func(t *testing.T) {
		store := NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Hybrid search with empty results should work
		results, err := HybridSearch(t.Context(), store, "test", nil, 10, 60)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("CosineSimilarity", func(t *testing.T) {
		// Identical vectors
		v1 := []float32{1.0, 0.0, 0.0}
		v2 := []float32{1.0, 0.0, 0.0}
		sim := CosineSimilarity(v1, v2)
		assert.InDelta(t, 1.0, sim, 0.001)

		// Orthogonal vectors
		v3 := []float32{0.0, 1.0, 0.0}
		sim = CosineSimilarity(v1, v3)
		assert.InDelta(t, 0.0, sim, 0.001)

		// Opposite vectors
		v4 := []float32{-1.0, 0.0, 0.0}
		sim = CosineSimilarity(v1, v4)
		assert.InDelta(t, -1.0, sim, 0.001)
	})
}
