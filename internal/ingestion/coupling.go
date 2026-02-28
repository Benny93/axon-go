package ingestion

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// ProcessCoupling analyzes git history to find files that change together.
// Returns the number of COUPLED_WITH edges created.
func ProcessCoupling(g *graph.KnowledgeGraph, repoPath string) int {
	// Parse git log for last 6 months
	changes, err := parseGitLog(repoPath, 6)
	if err != nil {
		return 0
	}

	if len(changes) == 0 {
		return 0
	}

	// Build co-change matrix
	matrix := buildCoChangeMatrix(changes)

	// Count total changes per file
	totalChanges := make(map[string]int)
	for _, commit := range changes {
		for _, file := range commit {
			totalChanges[file]++
		}
	}

	// Create COUPLED_WITH edges for strong couplings
	edgeCount := 0
	for fileA, coChanges := range matrix {
		for fileB, count := range coChanges {
			if fileA >= fileB {
				continue // Avoid duplicates
			}

			// Calculate coupling strength
			strength := computeCouplingStrength(count, totalChanges[fileA], totalChanges[fileB])

			// Filter weak couplings
			if strength < 0.3 || count < 3 {
				continue
			}

			// Check if both files exist in graph
			nodeA := findFileNode(g, fileA)
			nodeB := findFileNode(g, fileB)
			if nodeA == nil || nodeB == nil {
				continue
			}

			// Create COUPLED_WITH edge
			edge := &graph.GraphRelationship{
				ID:     fmt.Sprintf("coupled:%s:%s", fileA, fileB),
				Type:   graph.RelCoupledWith,
				Source: nodeA.ID,
				Target: nodeB.ID,
				Properties: map[string]any{
					"strength":   strength,
					"co_changes": count,
				},
			}
			g.AddRelationship(edge)
			edgeCount++
		}
	}

	return edgeCount
}

// parseGitLog parses git log for the last N months.
// Returns a list of commits, where each commit is a list of changed files.
func parseGitLog(repoPath string, months int) ([][]string, error) {
	// Run git log
	cmd := exec.Command("git", "log",
		fmt.Sprintf("--since=%d months ago", months),
		"--name-only",
		"--pretty=format:COMMIT:%H")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output
	var changes [][]string
	var currentCommit []string

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "COMMIT:") {
			if len(currentCommit) > 0 {
				changes = append(changes, currentCommit)
			}
			currentCommit = []string{}
		} else {
			// File path
			if line != "" && !strings.HasPrefix(line, "COMMIT:") {
				currentCommit = append(currentCommit, line)
			}
		}
	}

	// Add last commit
	if len(currentCommit) > 0 {
		changes = append(changes, currentCommit)
	}

	return changes, scanner.Err()
}

// buildCoChangeMatrix builds a matrix of file co-changes.
// Returns map[fileA]map[fileB]count
func buildCoChangeMatrix(changes [][]string) map[string]map[string]int {
	matrix := make(map[string]map[string]int)

	for _, commit := range changes {
		// For each pair of files in the commit
		for i := 0; i < len(commit); i++ {
			for j := i + 1; j < len(commit); j++ {
				fileA := commit[i]
				fileB := commit[j]

				// Initialize maps if needed
				if matrix[fileA] == nil {
					matrix[fileA] = make(map[string]int)
				}
				if matrix[fileB] == nil {
					matrix[fileB] = make(map[string]int)
				}

				// Increment co-change count (symmetric)
				matrix[fileA][fileB]++
				matrix[fileB][fileA]++
			}
		}
	}

	return matrix
}

// computeCouplingStrength calculates the coupling strength between two files.
// Formula: co_changes / max(total_changes_A, total_changes_B)
func computeCouplingStrength(coChanges, totalA, totalB int) float64 {
	if totalA == 0 || totalB == 0 {
		return 0.0
	}

	maxTotal := totalA
	if totalB > maxTotal {
		maxTotal = totalB
	}

	if maxTotal == 0 {
		return 0.0
	}

	return float64(coChanges) / float64(maxTotal)
}

// findFileNode finds a file node by file path.
func findFileNode(g *graph.KnowledgeGraph, filePath string) *graph.GraphNode {
	// Try exact match first
	nodeID := "file:" + filePath
	node := g.GetNode(nodeID)
	if node != nil {
		return node
	}

	// Try with different prefixes
	files := g.GetNodesByLabel(graph.NodeFile)
	for _, file := range files {
		if file.FilePath == filePath || file.Name == filePath {
			return file
		}
	}

	return nil
}
