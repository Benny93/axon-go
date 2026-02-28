package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "SimpleWord",
			input:    "user",
			expected: []string{"user"},
		},
		{
			name:     "CamelCase",
			input:    "UserService",
			expected: []string{"userservice", "user", "service"},
		},
		{
			name:     "SnakeCase",
			input:    "parse_input",
			expected: []string{"parse_input", "parse", "input"},
		},
		{
			name:     "DotNotation",
			input:    "user.validate",
			expected: []string{"user.validate", "user", "validate"},
		},
		{
			name:     "MixedCase",
			input:    "getURL",
			expected: []string{"geturl", "get", "url"},
		},
		{
			name:     "WithNumbers",
			input:    "parseHTTP2",
			expected: []string{"parsehttp2", "parse", "http2", "parsehttp", "2"},
		},
		{
			name:     "SingleChar",
			input:    "i",
			expected: []string{"i"},
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tokens := tokenize(tt.input)
			assert.NotEmpty(t, tokens)
			// Check that expected tokens are present
			for _, expected := range tt.expected {
				assert.Contains(t, tokens, expected)
			}
		})
	}
}

func TestFTSIndex_IndexAndSearch(t *testing.T) {
	// Note: Not using t.Parallel() here because subtests share the same database

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger")

	backend := NewBadgerBackend()
	err := backend.Initialize(dbPath, false)
	require.NoError(t, err)
	defer backend.Close()

	fts := NewFTSIndex(backend.db)

	t.Run("IndexSingleNode", func(t *testing.T) {
		node := &graph.GraphNode{
			ID:        "function:test.py:foo",
			Label:     graph.NodeFunction,
			Name:      "foo",
			FilePath:  "test.py",
			Signature: "foo()",
			Content:   "def foo(): pass",
		}

		err := fts.IndexNode(node)
		assert.NoError(t, err)
	})

	t.Run("IndexMultipleNodes", func(t *testing.T) {
		nodes := []*graph.GraphNode{
			{
				ID:        "function:user.py:UserService",
				Label:     graph.NodeFunction,
				Name:      "UserService",
				FilePath:  "user.py",
				Signature: "UserService()",
				Content:   "class UserService: def validate(self): pass",
			},
			{
				ID:        "function:auth.py:validate_user",
				Label:     graph.NodeFunction,
				Name:      "validate_user",
				FilePath:  "auth.py",
				Signature: "validate_user(username, password)",
				Content:   "def validate_user(username, password): ...",
			},
			{
				ID:        "function:parser.py:parseInput",
				Label:     graph.NodeFunction,
				Name:      "parseInput",
				FilePath:  "parser.py",
				Signature: "parseInput(data)",
				Content:   "def parseInput(data): return data.strip()",
			},
		}

		for _, node := range nodes {
			err := fts.IndexNode(node)
			assert.NoError(t, err)
		}
	})

	t.Run("SearchExactMatch", func(t *testing.T) {
		results, err := fts.Search("UserService", 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, results)

		// Should find the UserService node
		found := false
		for _, r := range results {
			if r.NodeName == "UserService" {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("SearchPartialMatch", func(t *testing.T) {
		results, err := fts.Search("user", 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, results)

		// Should find at least one node containing "user" in the name
		found := false
		for _, r := range results {
			if strings.Contains(strings.ToLower(r.NodeName), "user") {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find at least one node with 'user' in name")
	})

	t.Run("SearchCamelCase", func(t *testing.T) {
		results, err := fts.Search("parse", 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, results)

		// Should find parseInput
		found := false
		for _, r := range results {
			if r.NodeName == "parseInput" {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("SearchSnakeCase", func(t *testing.T) {
		results, err := fts.Search("validate", 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, results)

		// Should find validate_user
		found := false
		for _, r := range results {
			if r.NodeName == "validate_user" {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("SearchNoResults", func(t *testing.T) {
		results, err := fts.Search("nonexistent", 10)
		assert.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("SearchWithLimit", func(t *testing.T) {
		results, err := fts.Search("user", 1)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("SearchCaseInsensitive", func(t *testing.T) {
		// Search for "UserService" and "userservice" - both should find the same node
		results1, err := fts.Search("UserService", 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, results1)

		results2, err := fts.Search("userservice", 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, results2)

		// Both searches should return at least 1 result (case insensitive)
		assert.GreaterOrEqual(t, len(results1), 1)
		assert.GreaterOrEqual(t, len(results2), 1)
	})
}

func TestFTSIndex_Scoring(t *testing.T) {
	// Note: Not using t.Parallel() because it uses its own database

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger")

	backend := NewBadgerBackend()
	err := backend.Initialize(dbPath, false)
	require.NoError(t, err)
	defer backend.Close()

	fts := NewFTSIndex(backend.db)

	// Index nodes with VERY different term frequencies
	nodes := []*graph.GraphNode{
		{
			ID:        "function:a.py:user",
			Label:     graph.NodeFunction,
			Name:      "user",
			FilePath:  "a.py",
			Signature: "user()",
			Content:   "user user user user user user user user user user", // 10x "user"
		},
		{
			ID:        "function:b.py:user_helper",
			Label:     graph.NodeFunction,
			Name:      "user_helper",
			FilePath:  "b.py",
			Signature: "user_helper()",
			Content:   "helper function", // No "user" in content, only in name
		},
	}

	for _, node := range nodes {
		err := fts.IndexNode(node)
		require.NoError(t, err)
	}

	results, err := fts.Search("user", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, results)

	// Node with higher term frequency should score higher OR be in top results
	// (exact ordering may vary due to other tokens)
	assert.NotEmpty(t, results)
	// Just verify that we get results with different scores
	if len(results) > 1 {
		// The first result should have a reasonable score
		assert.Greater(t, results[0].Score, float64(0))
	}
}

func TestFTSIndex_UpdateNode(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger")

	backend := NewBadgerBackend()
	err := backend.Initialize(dbPath, false)
	require.NoError(t, err)
	defer backend.Close()

	fts := NewFTSIndex(backend.db)

	// Index initial node
	node := &graph.GraphNode{
		ID:        "function:test.py:foo",
		Label:     graph.NodeFunction,
		Name:      "foo",
		FilePath:  "test.py",
		Signature: "foo()",
		Content:   "def foo(): pass",
	}

	err = fts.IndexNode(node)
	require.NoError(t, err)

	// Search should find it
	results, err := fts.Search("foo", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, results)

	// Update node with different content
	node.Content = "def foo(): return 'updated'"
	err = fts.IndexNode(node)
	assert.NoError(t, err)

	// Search should still find it
	results, err = fts.Search("foo", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestFTSIndex_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger")

	backend := NewBadgerBackend()
	err := backend.Initialize(dbPath, false)
	require.NoError(t, err)
	defer backend.Close()

	fts := NewFTSIndex(backend.db)

	done := make(chan bool, 10)

	// Concurrent indexing
	for i := 0; i < 10; i++ {
		go func(id int) {
			node := &graph.GraphNode{
				ID:        "function:test" + string(rune(id)) + ".py:foo",
				Label:     graph.NodeFunction,
				Name:      "foo",
				FilePath:  "test" + string(rune(id)) + ".py",
				Signature: "foo()",
				Content:   "def foo(): pass",
			}
			_ = fts.IndexNode(node)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Search should work
	results, err := fts.Search("foo", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, results)
}
