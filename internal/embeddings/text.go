package embeddings

import (
	"fmt"
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// GenerateEmbeddingText generates natural language text from a graph node for embedding.
// This text is used to create semantic embeddings that capture the meaning of code symbols.
func GenerateEmbeddingText(node *graph.GraphNode) string {
	if node == nil {
		return ""
	}

	var parts []string

	// Add node type and name
	parts = append(parts, fmt.Sprintf("%s %s", node.Label, node.Name))

	// Add file location
	if node.FilePath != "" {
		parts = append(parts, fmt.Sprintf("in file %s", node.FilePath))
	}

	// Add signature for functions/methods
	if node.Signature != "" {
		parts = append(parts, fmt.Sprintf("Signature: %s", node.Signature))
	}

	// Add content (first 500 chars)
	if node.Content != "" {
		content := node.Content
		if len(content) > 500 {
			content = content[:500]
		}
		parts = append(parts, fmt.Sprintf("Code: %s", content))
	}

	// Add class name for methods
	if node.ClassName != "" {
		parts = append(parts, fmt.Sprintf("Method of class %s", node.ClassName))
	}

	return strings.Join(parts, ". ")
}

// GenerateNodeText generates a shorter text representation for a node.
// Used for quick indexing and search.
func GenerateNodeText(node *graph.GraphNode) string {
	if node == nil {
		return ""
	}

	var parts []string

	// Add node type and name
	parts = append(parts, fmt.Sprintf("%s %s", node.Label, node.Name))

	// Add signature
	if node.Signature != "" {
		parts = append(parts, node.Signature)
	}

	// Add file path
	if node.FilePath != "" {
		parts = append(parts, node.FilePath)
	}

	return strings.Join(parts, " ")
}
