package ingestion

import (
	"fmt"
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// ProcessProcesses detects execution flows from entry points.
// Returns the number of PROCESS nodes created.
func ProcessProcesses(g *graph.KnowledgeGraph) int {
	// Find all entry points
	entryPoints := findEntryPoints(g)

	// Trace flows from each entry point
	allFlows := make([][]string, 0, len(entryPoints))
	for _, entryPoint := range entryPoints {
		flow := traceFlow(g, entryPoint.ID, 10)
		if len(flow) > 0 {
			allFlows = append(allFlows, flow)
		}
	}

	// Deduplicate flows
	allFlows = deduplicateFlows(allFlows)

	// Create PROCESS nodes and STEP_IN_PROCESS edges
	processCount := 0
	for i, flow := range allFlows {
		if len(flow) == 0 {
			continue
		}

		// Create PROCESS node
		processID := fmt.Sprintf("process:flow-%d", i)
		processNode := &graph.GraphNode{
			ID:       processID,
			Label:    graph.NodeProcess,
			Name:     generateProcessLabel(g, flow),
			FilePath: "",
		}
		g.AddNode(processNode)
		processCount++

		// Create STEP_IN_PROCESS edges
		for step, nodeID := range flow {
			edge := &graph.GraphRelationship{
				ID:     fmt.Sprintf("step:%s:%d", processID, step),
				Type:   graph.RelStepInProcess,
				Source: nodeID,
				Target: processID,
				Properties: map[string]any{
					"step_number": step,
				},
			}
			g.AddRelationship(edge)
		}
	}

	return processCount
}

// findEntryPoints finds all entry point nodes in the graph.
func findEntryPoints(g *graph.KnowledgeGraph) []*graph.GraphNode {
	var entryPoints []*graph.GraphNode

	for node := range g.IterNodes() {
		if isEntryPoint(node) {
			entryPoints = append(entryPoints, node)
		}
	}

	return entryPoints
}

// isEntryPoint checks if a node is an entry point.
func isEntryPoint(node *graph.GraphNode) bool {
	if node.IsEntryPoint {
		return true
	}

	// Check for main function
	if node.Label == graph.NodeFunction && node.Name == "main" {
		return true
	}

	// Check for test functions
	if node.Label == graph.NodeFunction {
		if strings.HasPrefix(node.Name, "Test") ||
			strings.HasPrefix(node.Name, "test_") {
			return true
		}
	}

	// Check for HTTP handlers (decorated functions)
	for _, decorator := range node.Decorators {
		if strings.Contains(decorator, "HandleFunc") ||
			strings.Contains(decorator, "Handle") ||
			strings.Contains(decorator, "http.") {
			return true
		}
	}

	// Check for CLI commands
	if strings.Contains(node.Name, "Cmd") ||
		strings.Contains(node.Name, "Command") {
		return true
	}

	return false
}

// traceFlow traces the call flow from a starting node.
// Returns a list of node IDs in the flow.
func traceFlow(g *graph.KnowledgeGraph, startNodeID string, maxDepth int) []string {
	flow := []string{startNodeID}
	visited := map[string]bool{startNodeID: true}

	// BFS traversal
	queue := []string{startNodeID}
	depth := map[string]int{startNodeID: 0}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if depth[current] >= maxDepth {
			continue
		}

		// Get callees
		callees := g.GetCallees(current)
		for _, callee := range callees {
			if !visited[callee.ID] {
				visited[callee.ID] = true
				flow = append(flow, callee.ID)
				queue = append(queue, callee.ID)
				depth[callee.ID] = depth[current] + 1
			}
		}
	}

	return flow
}

// generateProcessLabel generates a human-readable label for a process.
func generateProcessLabel(g *graph.KnowledgeGraph, flow []string) string {
	if len(flow) == 0 {
		return "Unknown Process"
	}

	// Get the entry point name
	entryNode := g.GetNode(flow[0])
	if entryNode != nil {
		return fmt.Sprintf("Flow from %s", entryNode.Name)
	}

	return fmt.Sprintf("Flow %d", len(flow))
}

// deduplicateFlows removes duplicate flows.
func deduplicateFlows(flows [][]string) [][]string {
	seen := make(map[string]bool)
	result := make([][]string, 0, len(flows))

	for _, flow := range flows {
		// Create a key from the flow
		key := strings.Join(flow, "->")
		if !seen[key] {
			seen[key] = true
			result = append(result, flow)
		}
	}

	return result
}
