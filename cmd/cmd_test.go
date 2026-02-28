package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/storage"
)

func TestAnalyzeCmd_Run(t *testing.T) {
	t.Parallel()

	t.Run("AnalyzeGoRepo", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a simple Go project
		files := map[string]string{
			"main.go": `
package main

import "fmt"

func main() {
	fmt.Println("Hello")
}
`,
			"service.go": `
package main

type Service struct{}

func (s *Service) Run() {}
`,
		}

		for path, content := range files {
			fullPath := filepath.Join(tmpDir, path)
			err := os.WriteFile(fullPath, []byte(content), 0o644)
			require.NoError(t, err)
		}

		cmd := &AnalyzeCmd{
			Path:         tmpDir,
			Full:         true,
			NoEmbeddings: true,
		}

		err := cmd.Run()
		assert.NoError(t, err)

		// Verify .axon directory was created
		axonDir := filepath.Join(tmpDir, ".axon")
		_, err = os.Stat(axonDir)
		assert.NoError(t, err)

		// Verify meta.json was created
		metaPath := filepath.Join(axonDir, "meta.json")
		_, err = os.Stat(metaPath)
		assert.NoError(t, err)
	})

	t.Run("InvalidPath", func(t *testing.T) {
		cmd := &AnalyzeCmd{
			Path: "/nonexistent/path",
		}

		err := cmd.Run()
		assert.Error(t, err)
	})

	t.Run("NotADirectory", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "file.txt")
		err := os.WriteFile(tmpFile, []byte("test"), 0o644)
		require.NoError(t, err)

		cmd := &AnalyzeCmd{
			Path: tmpFile,
		}

		err = cmd.Run()
		assert.Error(t, err)
	})
}

func TestQueryCmd_Run(t *testing.T) {
	t.Parallel()

	t.Run("QueryWithNoIndex", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &QueryCmd{
			Query: "test",
			Limit: 10,
		}

		err = cmd.Run()
		assert.Error(t, err) // Should error because no index exists
	})
}

func TestStatusCmd_Run(t *testing.T) {
	t.Parallel()

	t.Run("StatusWithNoIndex", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &StatusCmd{}

		err = cmd.Run()
		assert.Error(t, err) // Should error because no index exists
	})
}

func TestListCmd_Run(t *testing.T) {
	t.Parallel()

	t.Run("ListRepos", func(t *testing.T) {
		cmd := &ListCmd{}

		err := cmd.Run()
		assert.NoError(t, err)
		// Should not error even if no repos are indexed
	})
}

func TestCleanCmd_Run(t *testing.T) {
	t.Parallel()

	t.Run("CleanWithNoIndex", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &CleanCmd{
			Force: true,
		}

		err = cmd.Run()
		assert.Error(t, err) // Should error because no index exists
	})

	t.Run("CleanWithIndex", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		// Create a fake .axon directory
		axonDir := filepath.Join(tmpDir, ".axon")
		err := os.MkdirAll(axonDir, 0o755)
		require.NoError(t, err)

		cmd := &CleanCmd{
			Force: true,
		}

		err = cmd.Run()
		assert.NoError(t, err)

		// Verify .axon was deleted
		_, err = os.Stat(axonDir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestStorageHelpers(t *testing.T) {
	// Note: Not using t.Parallel() because tests change directories

	t.Run("LoadStorageWithNoIndex", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		store, err := loadStorage()
		assert.Error(t, err)
		assert.Nil(t, store)
	})

	t.Run("LoadStorageWithIndex", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		// Create a fake index
		axonDir := filepath.Join(tmpDir, ".axon")
		dbPath := filepath.Join(axonDir, "badger")
		err := os.MkdirAll(dbPath, 0o755)
		require.NoError(t, err)

		// Create actual BadgerDB index
		store := storage.NewBadgerBackend()
		err = store.Initialize(dbPath, false)
		require.NoError(t, err)
		err = store.Close()
		require.NoError(t, err)

		// Now try to load
		loadedStore, err := loadStorage()
		assert.NoError(t, err)
		if loadedStore != nil {
			loadedStore.Close()
		}
	})
}
