// Package graph provides the knowledge graph data model for Axon.
//
// It defines the core node and relationship types that represent code-level
// entities (files, functions, classes, etc.) and the edges between them
// (calls, imports, contains, etc.).
package graph

// NodeLabel represents the type of a graph node.
type NodeLabel string

const (
	NodeFile      NodeLabel = "file"
	NodeFolder    NodeLabel = "folder"
	NodeFunction  NodeLabel = "function"
	NodeClass     NodeLabel = "class"
	NodeMethod    NodeLabel = "method"
	NodeInterface NodeLabel = "interface"
	NodeTypeAlias NodeLabel = "type_alias"
	NodeEnum      NodeLabel = "enum"
	NodeCommunity NodeLabel = "community"
	NodeProcess   NodeLabel = "process"
)

// RelType represents the type of relationship between graph nodes.
type RelType string

const (
	RelContains      RelType = "contains"
	RelDefines       RelType = "defines"
	RelCalls         RelType = "calls"
	RelImports       RelType = "imports"
	RelExtends       RelType = "extends"
	RelImplements    RelType = "implements"
	RelMemberOf      RelType = "member_of"
	RelStepInProcess RelType = "step_in_process"
	RelUsesType      RelType = "uses_type"
	RelExports       RelType = "exports"
	RelCoupledWith   RelType = "coupled_with"
)

// GraphNode represents a node in the knowledge graph.
type GraphNode struct {
	// ID is the unique identifier for the node.
	// Format: {label}:{file_path}:{symbol_name}
	ID string

	// Label is the type of the node.
	Label NodeLabel

	// Name is the name of the entity (e.g., function name, class name).
	Name string

	// FilePath is the path to the file containing this entity.
	FilePath string

	// StartLine is the starting line number in the file.
	StartLine int

	// EndLine is the ending line number in the file.
	EndLine int

	// Content is the source code content.
	Content string

	// Signature is the function/method signature.
	Signature string

	// Language is the programming language (e.g., "python", "typescript").
	Language string

	// ClassName is the parent class name (for methods).
	ClassName string

	// IsDead indicates if the symbol is unreachable/dead code.
	IsDead bool

	// IsEntryPoint indicates if the symbol is an entry point.
	IsEntryPoint bool

	// IsExported indicates if the symbol is exported.
	IsExported bool

	// Decorators holds decorator names (for Python/TS decorators, Go attributes).
	Decorators []string

	// Properties holds additional metadata.
	Properties map[string]any
}

// GraphRelationship represents a directed edge in the knowledge graph.
type GraphRelationship struct {
	// ID is the unique identifier for the relationship.
	ID string

	// Type is the type of relationship.
	Type RelType

	// Source is the ID of the source node.
	Source string

	// Target is the ID of the target node.
	Target string

	// Properties holds additional metadata (e.g., confidence, role).
	Properties map[string]any
}

// GenerateID creates a deterministic node ID from label, file path, and symbol name.
// Format: {label}:{file_path}:{symbol_name}
func GenerateID(label NodeLabel, filePath, symbolName string) string {
	if symbolName == "" {
		return string(label) + ":" + filePath
	}
	return string(label) + ":" + filePath + ":" + symbolName
}
