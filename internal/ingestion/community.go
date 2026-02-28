package ingestion

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/Benny93/axon-go/internal/graph"
)

// DetectCommunities detects communities in the code graph using a Louvain-style algorithm.
// Returns the number of communities detected.
func DetectCommunities(g *graph.KnowledgeGraph) int {
	// Get only symbol nodes (not files/folders)
	symbolNodes := getSymbolNodes(g)
	if len(symbolNodes) == 0 {
		return 0
	}

	// Build adjacency matrix
	matrix, _, indexNode := buildAdjacencyMatrix(g)
	if len(matrix) == 0 {
		return 0
	}

	// Detect communities using Louvain-style algorithm
	communities := assignCommunities(matrix)

	// Create COMMUNITY nodes and MEMBER_OF edges
	communityMap := make(map[int][]string)
	for nodeIdx, commID := range communities {
		nodeID := indexNode[nodeIdx]
		communityMap[commID] = append(communityMap[commID], nodeID)
	}

	// Create community nodes and edges
	communityCount := 0
	for commID, members := range communityMap {
		if len(members) == 0 {
			continue
		}

		// Create COMMUNITY node
		communityID := fmt.Sprintf("community:%d", commID)
		communityLabel := generateCommunityLabel(g, members)

		communityNode := &graph.GraphNode{
			ID:       communityID,
			Label:    graph.NodeCommunity,
			Name:     communityLabel,
			FilePath: "",
			Properties: map[string]any{
				"member_count": len(members),
				"members":      members,
			},
		}
		g.AddNode(communityNode)
		communityCount++

		// Create MEMBER_OF edges
		for _, memberID := range members {
			edge := &graph.GraphRelationship{
				ID:     fmt.Sprintf("member:%s:%s", memberID, communityID),
				Type:   graph.RelMemberOf,
				Source: memberID,
				Target: communityID,
				Properties: map[string]any{
					"step_number": 0,
				},
			}
			g.AddRelationship(edge)
		}
	}

	return communityCount
}

// getSymbolNodes returns all symbol nodes (functions, classes, etc.)
func getSymbolNodes(g *graph.KnowledgeGraph) []*graph.GraphNode {
	var symbols []*graph.GraphNode
	for node := range g.IterNodes() {
		if node.Label == graph.NodeFunction ||
			node.Label == graph.NodeMethod ||
			node.Label == graph.NodeClass ||
			node.Label == graph.NodeInterface {
			symbols = append(symbols, node)
		}
	}
	return symbols
}

// buildAdjacencyMatrix builds an undirected adjacency matrix from the graph.
func buildAdjacencyMatrix(g *graph.KnowledgeGraph) ([][]float64, map[string]int, []string) {
	symbolNodes := getSymbolNodes(g)
	n := len(symbolNodes)

	if n == 0 {
		return nil, nil, nil
	}

	// Create index mappings
	nodeIndex := make(map[string]int)
	indexNode := make([]string, n)
	for i, node := range symbolNodes {
		nodeIndex[node.ID] = i
		indexNode[i] = node.ID
	}

	// Build adjacency matrix (undirected, weighted)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
	}

	// Add edges from CALLS relationships
	for rel := range g.IterRelationships() {
		if rel.Type != graph.RelCalls {
			continue
		}

		srcIdx, srcOk := nodeIndex[rel.Source]
		tgtIdx, tgtOk := nodeIndex[rel.Target]

		if srcOk && tgtOk {
			// Undirected: add weight to both directions
			matrix[srcIdx][tgtIdx] += 1.0
			matrix[tgtIdx][srcIdx] += 1.0
		}
	}

	return matrix, nodeIndex, indexNode
}

// assignCommunities assigns communities to nodes using a simplified Louvain algorithm.
// Returns a slice where index i contains the community ID for node i.
func assignCommunities(adjMatrix [][]float64) []int {
	n := len(adjMatrix)
	if n == 0 {
		return []int{}
	}

	if n == 1 {
		return []int{0}
	}

	// Initialize: each node in its own community
	communities := make([]int, n)
	for i := range communities {
		communities[i] = i
	}

	// Calculate total edge weight
	var totalWeight float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			totalWeight += adjMatrix[i][j]
		}
	}
	if totalWeight == 0 {
		return communities
	}

	// Calculate node degrees
	degrees := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			degrees[i] += adjMatrix[i][j]
		}
	}

	// Louvain-style optimization
	improved := true
	iterations := 0
	maxIterations := 100

	for improved && iterations < maxIterations {
		improved = false
		iterations++

		// Shuffle node order for better convergence
		nodeOrder := rand.Perm(n)

		for _, node := range nodeOrder {
			bestComm := communities[node]
			bestGain := 0.0

			// Try moving to neighbor communities
			neighborComms := make(map[int]bool)
			for j := 0; j < n; j++ {
				if adjMatrix[node][j] > 0 {
					neighborComms[communities[j]] = true
				}
			}

			for comm := range neighborComms {
				if comm == bestComm {
					continue
				}

				// Temporarily move node to this community
				communities[node] = comm
				gain := calculateModularityGain(node, comm, communities, adjMatrix, degrees, totalWeight)

				if gain > bestGain {
					bestGain = gain
					bestComm = comm
					improved = true
				}
			}

			// Move to best community
			communities[node] = bestComm
		}
	}

	// Renumber communities to be consecutive
	communityMap := make(map[int]int)
	nextComm := 0
	for i := range communities {
		if _, exists := communityMap[communities[i]]; !exists {
			communityMap[communities[i]] = nextComm
			nextComm++
		}
		communities[i] = communityMap[communities[i]]
	}

	return communities
}

// calculateModularityGain calculates the modularity gain of moving a node to a community.
func calculateModularityGain(node, comm int, communities []int, adjMatrix [][]float64, degrees []float64, totalWeight float64) float64 {
	n := len(communities)

	// Sum of weights to community
	var sigmaIn float64
	// Sum of degrees in community
	var sigmaTot float64

	for j := 0; j < n; j++ {
		if communities[j] == comm && j != node {
			sigmaIn += adjMatrix[node][j]
			sigmaTot += degrees[j]
		}
	}

	// Add node's own degree
	sigmaTot += degrees[node]

	// Modularity gain formula
	ki := degrees[node]
	gain := (sigmaIn / totalWeight) - ((ki * sigmaTot) / (totalWeight * totalWeight))

	return gain
}

// generateCommunityLabel generates a human-readable label for a community.
func generateCommunityLabel(g *graph.KnowledgeGraph, members []string) string {
	if len(members) == 0 {
		return "Community (empty)"
	}

	// Get member names
	var names []string
	for _, memberID := range members {
		node := g.GetNode(memberID)
		if node != nil {
			names = append(names, node.Name)
		}
	}

	if len(names) == 0 {
		return fmt.Sprintf("Community (%d members)", len(members))
	}

	// Sort names for consistency
	sort.Strings(names)

	// Use first few names
	if len(names) <= 3 {
		return fmt.Sprintf("Community (%s)", joinNames(names))
	}

	return fmt.Sprintf("Community (%s, +%d more)", joinNames(names[:3]), len(names)-3)
}

// joinNames joins names with commas.
func joinNames(names []string) string {
	result := ""
	for i, name := range names {
		if i > 0 {
			result += ", "
		}
		result += name
	}
	return result
}
