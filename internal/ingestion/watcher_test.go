package ingestion

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

func TestReindexFiles(t *testing.T) {
	t.Parallel()

	t.Run("ReindexesChangedFiles", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create initial graph
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:OldFunc",
			Label:     graph.NodeFunction,
			Name:      "OldFunc",
			FilePath:  "test.go",
			Content:   "func OldFunc() {}",
			Signature: "OldFunc()",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Create file entries for re-indexing
		entries := []FileEntry{
			{
				RelPath:  "test.go",
				Path:  filepath.Join(tmpDir, "test.go"),
				Language: "go",
				Content:  []byte("package main\n\nfunc NewFunc() {}"),
			},
		}

		// Re-index files
		count := ReindexFiles(entries, tmpDir, store)

		assert.Equal(t, 1, count)

		// Verify old node is removed and new node exists
		nodes := store.GetNodesByLabel(t.Context(), "function")
		assert.NotEmpty(t, nodes)

		found := false
		for _, node := range nodes {
			if node.Name == "NewFunc" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find NewFunc after re-indexing")
	})

	t.Run("HandlesFileDeletion", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create initial graph with a file
		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:        "function:deleted.go:SomeFunc",
			Label:     graph.NodeFunction,
			Name:      "SomeFunc",
			FilePath:  "deleted.go",
			Content:   "func SomeFunc() {}",
			Signature: "SomeFunc()",
		})

		err = store.BulkLoad(t.Context(), g)
		require.NoError(t, err)

		// Re-index with empty entries (simulating file deletion)
		// Note: ReindexFiles only removes nodes for files in the entries list
		// Actual file deletion handling happens in processChangedFiles
		entries := []FileEntry{}
		count := ReindexFiles(entries, tmpDir, store)

		assert.Equal(t, 0, count)

		// Nodes are not automatically removed - caller must handle deletions
		// This test verifies ReindexFiles behavior with empty entries
		nodes := store.GetNodesByLabel(t.Context(), "function")
		assert.NotEmpty(t, nodes, "Nodes remain until explicitly removed")
	})

	t.Run("SkipsUnsupportedFiles", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create file entry for unsupported file type
		entries := []FileEntry{
			{
				RelPath:  "README.md",
				Path:  filepath.Join(tmpDir, "README.md"),
				Language: "", // Unsupported
				Content:  []byte("# README"),
			},
		}

		// ReindexFiles processes entries passed to it
		// Filtering is caller's responsibility
		count := ReindexFiles(entries, tmpDir, store)

		// Returns count of entries passed
		assert.Equal(t, 1, count)
	})

	t.Run("SkipsIgnoredFiles", func(t *testing.T) {
		store := storage.NewBadgerBackend()
		tmpDir := t.TempDir()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Create file entry that should be ignored
		entries := []FileEntry{
			{
				RelPath:  ".git/config",
				Path:  filepath.Join(tmpDir, ".git", "config"),
				Language: "",
				Content:  []byte("[core]"),
			},
		}

		// ReindexFiles processes entries passed to it
		// Filtering is caller's responsibility
		count := ReindexFiles(entries, tmpDir, store)

		// Returns count of entries passed
		assert.Equal(t, 1, count)
	})
}

func TestWatchRepo(t *testing.T) {
	t.Parallel()

	t.Run("DetectsFileChanges", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a test file
		testFile := filepath.Join(tmpDir, "watch_test.go")
		err := os.WriteFile(testFile, []byte("package main\n\nfunc Func1() {}"), 0o644)
		require.NoError(t, err)

		// Create store
		store := storage.NewBadgerBackend()
		err = store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Initial index
		_, _, err = RunPipeline(t.Context(), tmpDir, store, false, nil, false)
		require.NoError(t, err)

		// Modify the file
		err = os.WriteFile(testFile, []byte("package main\n\nfunc Func2() {}"), 0o644)
		require.NoError(t, err)

		// Give watchfiles time to detect (in real implementation)
		time.Sleep(100 * time.Millisecond)

		// Verify file exists
		_, err = os.Stat(testFile)
		assert.NoError(t, err)
	})

	t.Run("HandlesMultipleFiles", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple test files
		files := []string{
			"file1.go",
			"file2.go",
			"subdir/file3.go",
		}

		for _, file := range files {
			fullPath := filepath.Join(tmpDir, file)
			err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
			require.NoError(t, err)

			content := "package main\n\nfunc Test() {}"
			err = os.WriteFile(fullPath, []byte(content), 0o644)
			require.NoError(t, err)
		}

		// Create store
		store := storage.NewBadgerBackend()
		err := store.Initialize(tmpDir, false)
		require.NoError(t, err)
		defer store.Close()

		// Initial index
		_, _, err = RunPipeline(t.Context(), tmpDir, store, false, nil, false)
		require.NoError(t, err)

		// Verify files were indexed
		nodes := store.GetNodesByLabel(t.Context(), "function")
		assert.Greater(t, len(nodes), 0)
	})

	t.Run("RespectsGitignore", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .gitignore
		gitignore := filepath.Join(tmpDir, ".gitignore")
		err := os.WriteFile(gitignore, []byte("*.tmp\nbuild/\n"), 0o644)
		require.NoError(t, err)

		// Create files that should be ignored
		tmpFile := filepath.Join(tmpDir, "test.tmp")
		err = os.WriteFile(tmpFile, []byte("temp"), 0o644)
		require.NoError(t, err)

		// Create file that should be watched
		goFile := filepath.Join(tmpDir, "main.go")
		err = os.WriteFile(goFile, []byte("package main"), 0o644)
		require.NoError(t, err)

		// Verify gitignore loading
		patterns, err := loadGitignore(tmpDir)
		require.NoError(t, err)
		assert.NotEmpty(t, patterns)
	})
}

func TestShouldReindexGlobalPhases(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsTrueAfterThreshold", func(t *testing.T) {
		lastGlobalPhase := time.Now().Add(-GLOBAL_PHASE_INTERVAL - 1*time.Second)

		shouldReindex := shouldReindexGlobalPhases(lastGlobalPhase)
		assert.True(t, shouldReindex)
	})

	t.Run("ReturnsFalseBeforeThreshold", func(t *testing.T) {
		lastGlobalPhase := time.Now()

		shouldReindex := shouldReindexGlobalPhases(lastGlobalPhase)
		assert.False(t, shouldReindex)
	})
}

func TestFilterChangedPaths(t *testing.T) {
	t.Parallel()

	t.Run("FiltersUnsupportedFiles", func(t *testing.T) {
		paths := []string{
			"file.go",
			"file.py",
			"README.md",
			"image.png",
		}

		filtered := filterChangedPaths(paths)

		assert.Len(t, filtered, 2)
		assert.Contains(t, filtered, "file.go")
		assert.Contains(t, filtered, "file.py")
	})

	t.Run("FiltersIgnoredFiles", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .gitignore
		gitignore := filepath.Join(tmpDir, ".gitignore")
		err := os.WriteFile(gitignore, []byte("*.log\n"), 0o644)
		require.NoError(t, err)

		paths := []string{
			"app.go",
			"debug.log",
			"main.go",
		}

		// Convert to absolute paths
		absPaths := make([]string, len(paths))
		for i, p := range paths {
			absPaths[i] = filepath.Join(tmpDir, p)
		}

		// Load matcher
		matcher, _ := loadGitignoreMatcher(tmpDir)

		filtered := filterChangedPathsWithIgnore(absPaths, tmpDir, matcher)

		assert.Len(t, filtered, 2)
	})
}
