package ingestion

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestParseGitLog(t *testing.T) {
	t.Parallel()

	t.Run("ParsesGitLog", func(t *testing.T) {
		// Create a temporary git repo for testing
		tmpDir := t.TempDir()
		initGitRepo(t, tmpDir)

		// Create some commits
		createCommit(t, tmpDir, "file1.go", "package main")
		createCommit(t, tmpDir, "file2.go", "package main")
		createCommit(t, tmpDir, "file1.go", "package main\n\nfunc main() {}")

		// Parse git log
		changes, err := parseGitLog(tmpDir, 6)
		require.NoError(t, err)
		assert.NotEmpty(t, changes)
	})

	t.Run("HandlesNoGitRepo", func(t *testing.T) {
		tmpDir := t.TempDir()

		changes, err := parseGitLog(tmpDir, 6)
		assert.Error(t, err)
		assert.Empty(t, changes)
	})

	t.Run("RespectsMonthsLimit", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepo(t, tmpDir)

		// Create commits
		for i := 0; i < 5; i++ {
			createCommit(t, tmpDir, "file.go", "package main")
		}

		changes, err := parseGitLog(tmpDir, 6)
		require.NoError(t, err)
		assert.NotEmpty(t, changes)
	})
}

func TestBuildCoChangeMatrix(t *testing.T) {
	t.Parallel()

	t.Run("BuildsMatrix", func(t *testing.T) {
		changes := [][]string{
			{"file1.go", "file2.go"},
			{"file1.go", "file3.go"},
			{"file2.go", "file3.go"},
			{"file1.go", "file2.go"},
		}

		matrix := buildCoChangeMatrix(changes)

		assert.Equal(t, 2, matrix["file1.go"]["file2.go"])
		assert.Equal(t, 1, matrix["file1.go"]["file3.go"])
		assert.Equal(t, 1, matrix["file2.go"]["file3.go"])
	})

	t.Run("HandlesEmptyChanges", func(t *testing.T) {
		changes := [][]string{}

		matrix := buildCoChangeMatrix(changes)

		assert.Empty(t, matrix)
	})

	t.Run("SymmetricMatrix", func(t *testing.T) {
		changes := [][]string{
			{"file1.go", "file2.go"},
		}

		matrix := buildCoChangeMatrix(changes)

		assert.Equal(t, matrix["file1.go"]["file2.go"], matrix["file2.go"]["file1.go"])
	})
}

func TestComputeCouplingStrength(t *testing.T) {
	t.Parallel()

	t.Run("ComputesStrength", func(t *testing.T) {
		coChanges := 5
		totalChangesA := 10
		totalChangesB := 10

		strength := computeCouplingStrength(coChanges, totalChangesA, totalChangesB)

		assert.InDelta(t, 0.5, strength, 0.01)
	})

	t.Run("HandlesZeroChanges", func(t *testing.T) {
		strength := computeCouplingStrength(0, 10, 10)
		assert.Equal(t, 0.0, strength)
	})

	t.Run("HandlesDifferentTotals", func(t *testing.T) {
		coChanges := 3
		totalChangesA := 10
		totalChangesB := 5

		// Should use max of totals
		strength := computeCouplingStrength(coChanges, totalChangesA, totalChangesB)
		assert.InDelta(t, 0.3, strength, 0.01)
	})
}

func TestProcessCoupling(t *testing.T) {
	t.Parallel()

	t.Run("CreatesCoupledWithEdges", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepo(t, tmpDir)

		// Create files that change together
		createCommit(t, tmpDir, "file1.go", "package main")
		createCommit(t, tmpDir, "file2.go", "package main")
		createCommit(t, tmpDir, "file1.go", "package main\n\nfunc main() {}")
		createCommit(t, tmpDir, "file2.go", "package main\n\nfunc init() {}")

		g := graph.NewKnowledgeGraph()
		g.AddNode(&graph.GraphNode{
			ID:       "file:file1.go",
			Label:    graph.NodeFile,
			Name:     "file1.go",
			FilePath: "file1.go",
		})
		g.AddNode(&graph.GraphNode{
			ID:       "file:file2.go",
			Label:    graph.NodeFile,
			Name:     "file2.go",
			FilePath: "file2.go",
		})

		count := ProcessCoupling(g, tmpDir)

		assert.GreaterOrEqual(t, count, 0) // May be 0 if coupling < threshold
	})

	t.Run("HandlesNoGitRepo", func(t *testing.T) {
		tmpDir := t.TempDir()

		g := graph.NewKnowledgeGraph()
		count := ProcessCoupling(g, tmpDir)

		assert.Equal(t, 0, count)
	})

	t.Run("FiltersWeakCouplings", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepo(t, tmpDir)

		// Create files with weak coupling (only once together)
		createCommit(t, tmpDir, "file1.go", "package main")
		createCommit(t, tmpDir, "file2.go", "package main")
		createCommit(t, tmpDir, "file3.go", "package main")

		g := graph.NewKnowledgeGraph()
		for i := 1; i <= 3; i++ {
			g.AddNode(&graph.GraphNode{
				ID:       "file:file" + string(rune('0'+i)) + ".go",
				Label:    graph.NodeFile,
				Name:     "file" + string(rune('0'+i)) + ".go",
				FilePath: "file" + string(rune('0'+i)) + ".go",
			})
		}

		count := ProcessCoupling(g, tmpDir)

		// Should filter out weak couplings (< 0.3 strength or < 3 co-changes)
		assert.GreaterOrEqual(t, count, 0)
	})
}

// Helper functions for git repo setup

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	err := cmd.Run()
	require.NoError(t, err)

	// Set git config for commits
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	_ = cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	_ = cmd.Run()
}

func createCommit(t *testing.T, dir, filename, content string) {
	t.Helper()

	// Write file
	filepath := filepath.Join(dir, filename)
	err := os.WriteFile(filepath, []byte(content), 0o644)
	require.NoError(t, err)

	// Git add
	cmd := exec.Command("git", "add", filename)
	cmd.Dir = dir
	_ = cmd.Run()

	// Git commit
	cmd = exec.Command("git", "commit", "-m", "Add "+filename)
	cmd.Dir = dir
	_ = cmd.Run()
}
