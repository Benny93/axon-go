package ingestion

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkRepo(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"main.py":          "print('hello')",
		"utils.py":         "def helper(): pass",
		"test_main.py":     "def test_main(): pass",
		"src/app.py":       "class App: pass",
		"src/lib/utils.py": "def util(): pass",
		"README.md":        "# README",
		".gitignore":       "*.pyc\n__pycache__/",
		"cache.pyc":        "binary",
		"__pycache__/mod.pyc": "binary",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(t, err)
	}

	t.Run("WalkAllSupportedFiles", func(t *testing.T) {
		entries, err := WalkRepo(tmpDir, nil)
		assert.NoError(t, err)

		// Should find Python files
		var pyFiles []FileEntry
		for _, e := range entries {
			if strings.HasSuffix(e.Path, ".py") {
				pyFiles = append(pyFiles, e)
			}
		}

		assert.GreaterOrEqual(t, len(pyFiles), 4) // main.py, utils.py, test_main.py, src/app.py, src/lib/utils.py
	})

	t.Run("RespectGitignore", func(t *testing.T) {
		// Load gitignore patterns
		patterns, err := loadGitignore(tmpDir)
		require.NoError(t, err)

		entries, err := WalkRepo(tmpDir, patterns)
		assert.NoError(t, err)

		// Should NOT find .pyc files
		for _, e := range entries {
			assert.NotContains(t, e.Path, ".pyc")
			assert.NotContains(t, e.Path, "__pycache__")
		}
	})

	t.Run("SkipUnsupportedExtensions", func(t *testing.T) {
		entries, err := WalkRepo(tmpDir, nil)
		assert.NoError(t, err)

		// Should NOT find .md files
		for _, e := range entries {
			assert.False(t, strings.HasSuffix(e.Path, ".md"))
		}
	})

	t.Run("SkipDirectories", func(t *testing.T) {
		entries, err := WalkRepo(tmpDir, nil)
		assert.NoError(t, err)

		// All entries should be files
		for _, e := range entries {
			assert.False(t, e.IsDir)
		}
	})

	t.Run("ComputeSHA256", func(t *testing.T) {
		entries, err := WalkRepo(tmpDir, nil)
		assert.NoError(t, err)

		// Each entry should have a valid SHA256 hash
		for _, e := range entries {
			assert.NotEmpty(t, e.SHA256)
			assert.Len(t, e.SHA256, 64) // SHA256 hex length
		}
	})

	t.Run("DetectLanguage", func(t *testing.T) {
		entries, err := WalkRepo(tmpDir, nil)
		assert.NoError(t, err)

		// Check language detection
		for _, e := range entries {
			if strings.HasSuffix(e.Path, ".py") {
				assert.Equal(t, "python", e.Language)
			}
		}
	})
}

func TestLoadGitignore(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	t.Run("NoGitignore", func(t *testing.T) {
		patterns, err := loadGitignore(tmpDir)
		assert.NoError(t, err)
		assert.Empty(t, patterns)
	})

	t.Run("WithGitignore", func(t *testing.T) {
		gitignoreContent := "*.pyc\n__pycache__/\n.env"
		err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignoreContent), 0o644)
		require.NoError(t, err)

		patterns, err := loadGitignore(tmpDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, patterns)
	})

	t.Run("DefaultPatterns", func(t *testing.T) {
		patterns, err := loadGitignore(tmpDir)
		assert.NoError(t, err)

		// Should include default patterns like .git, node_modules, .axon
		assert.GreaterOrEqual(t, len(patterns), 3)
	})
}

func TestIsSupportedFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{"Python", "main.py", true},
		{"TypeScript", "app.ts", true},
		{"TSX", "component.tsx", true},
		{"JavaScript", "script.js", true},
		{"JSX", "component.jsx", true},
		{"MJS", "module.mjs", true},
		{"CJS", "module.cjs", true},
		{"Go", "main.go", true}, // Go support added
		{"Markdown", "README.md", false},
		{"Text", "file.txt", false},
		{"Binary", "image.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isSupportedFile(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		expected string
	}{
		{"Python", "main.py", "python"},
		{"TypeScript", "app.ts", "typescript"},
		{"TSX", "component.tsx", "typescript"},
		{"JavaScript", "script.js", "javascript"},
		{"JSX", "component.jsx", "javascript"},
		{"MJS", "module.mjs", "javascript"},
		{"CJS", "module.cjs", "javascript"},
		{"Go", "main.go", "go"}, // Go support added
		{"Unknown", "file.txt", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getLanguage(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileEntry_HashConsistency(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a file with known content
	content := "hello world"
	filePath := filepath.Join(tmpDir, "test.py")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	// Walk the repo
	entries, err := WalkRepo(tmpDir, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]

	// Verify hash is correct
	expectedHash := sha256.Sum256([]byte(content))
	expectedHex := hex.EncodeToString(expectedHash[:])
	assert.Equal(t, expectedHex, entry.SHA256)
}
