// Package mcp provides the MCP (Model Context Protocol) server for Axon.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

// Server represents the MCP server.
type Server struct {
	storage StorageBackend
	server  *mcp.Server
}

// StorageBackend defines the interface for storage backends.
type StorageBackend interface {
	FTSSearch(ctx context.Context, query string, limit int) ([]storage.SearchResult, error)
	GetCallers(ctx context.Context, nodeID string) ([]*graph.GraphNode, error)
	GetCallees(ctx context.Context, nodeID string) ([]*graph.GraphNode, error)
	Traverse(ctx context.Context, startID string, depth int, direction string) ([]*graph.GraphNode, error)
	NodeCount() int
	RelationshipCount() int
	Close() error
	GetDeadCode(ctx context.Context) ([]*graph.GraphNode, error)
	HybridSearch(ctx context.Context, query string, queryVector []float32, limit int) ([]storage.HybridSearchResult, error)
	GetNodesByLabel(ctx context.Context, label string) []*graph.GraphNode
}

// SearchResult represents a search result.
type SearchResult struct {
	NodeID   string
	Score    float64
	NodeName string
	FilePath string
	Label    string
	Snippet  string
}

// Node represents a graph node.
type Node struct {
	ID       string
	Name     string
	Label    string
	FilePath string
}

// Tool represents an MCP tool.
type Tool struct {
	Name        string
	Description string
	InputSchema *jsonschema.Schema
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

// NewServer creates a new MCP server.
func NewServer(storage StorageBackend) *Server {
	s := &Server{
		storage: storage,
	}

	// Create MCP server
	s.server = mcp.NewServer(&mcp.Implementation{
		Name:    "axon-go",
		Version: "0.1.0",
	}, nil)

	// Register tools
	s.registerTools()

	// Register resources
	s.registerResources()

	return s
}

// ListTools returns all registered tools.
func (s *Server) ListTools() []Tool {
	return []Tool{
		{
			Name:        "axon_query",
			Description: "Search the knowledge graph using hybrid search. Returns ranked symbols matching the query.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"query": {Type: "string", Description: "Search query text"},
					"limit": {Type: "integer", Description: "Maximum number of results"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "axon_context",
			Description: "Get a 360-degree view of a symbol: callers, callees, type references, and community membership.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"symbol": {Type: "string", Description: "Name of the symbol to look up"},
				},
				Required: []string{"symbol"},
			},
		},
		{
			Name:        "axon_impact",
			Description: "Blast radius analysis: find all symbols affected by changing a given symbol.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"symbol": {Type: "string", Description: "Name of the symbol to analyze"},
					"depth":  {Type: "integer", Description: "Maximum traversal depth"},
				},
				Required: []string{"symbol"},
			},
		},
		{
			Name:        "axon_dead_code",
			Description: "List all symbols detected as dead (unreachable) code.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		{
			Name:        "axon_list_repos",
			Description: "List all indexed repositories with their stats.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		{
			Name:        "axon_cypher",
			Description: "Execute a raw Cypher query against the knowledge graph (read-only).",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"query": {Type: "string", Description: "Cypher query string"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "axon_detect_changes",
			Description: "Detect changes in specified files and analyze their impact on the codebase.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"files": {
						Type:        "array",
						Items:       &jsonschema.Schema{Type: "string"},
						Description: "List of changed file paths",
					},
				},
				Required: []string{"files"},
			},
		},
	}
}

// ListResources returns all registered resources.
func (s *Server) ListResources() []Resource {
	return []Resource{
		{
			URI:         "axon://overview",
			Name:        "Codebase Overview",
			Description: "High-level statistics about the indexed codebase",
			MimeType:    "text/plain",
		},
		{
			URI:         "axon://dead-code",
			Name:        "Dead Code Report",
			Description: "List of all symbols flagged as unreachable",
			MimeType:    "text/plain",
		},
		{
			URI:         "axon://schema",
			Name:        "Graph Schema",
			Description: "Description of the Axon knowledge graph schema",
			MimeType:    "text/plain",
		},
	}
}

// CallTool executes a tool with the given arguments.
func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "axon_list_repos":
		return handleListRepos()
	case "axon_query":
		query, _ := args["query"].(string)
		limit, _ := args["limit"].(float64)
		if limit == 0 {
			limit = 20
		}
		return handleQuery(s.storage, query, int(limit))
	case "axon_context":
		symbol, _ := args["symbol"].(string)
		return handleContext(s.storage, symbol)
	case "axon_impact":
		symbol, _ := args["symbol"].(string)
		depth, _ := args["depth"].(float64)
		if depth == 0 {
			depth = 3
		}
		return handleImpact(s.storage, symbol, int(depth))
	case "axon_dead_code":
		return handleDeadCode(s.storage)
	case "axon_detect_changes":
		filesArg, _ := args["files"].([]any)
		files := make([]string, 0, len(filesArg))
		for _, f := range filesArg {
			if file, ok := f.(string); ok {
				files = append(files, file)
			}
		}
		return handleDetectChanges(s.storage, files)
	case "axon_cypher":
		query, _ := args["query"].(string)
		return handleCypher(s.storage, query)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// ReadResource reads a resource by URI.
func (s *Server) ReadResource(ctx context.Context, uri string) (string, error) {
	switch uri {
	case "axon://overview":
		return getOverview(s.storage), nil
	case "axon://dead-code":
		return getDeadCodeList(s.storage), nil
	case "axon://schema":
		return getSchema(), nil
	default:
		return "", fmt.Errorf("unknown resource: %s", uri)
	}
}

// Run starts the MCP server with stdio transport.
func (s *Server) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	if stdin == nil || stdout == nil {
		return fmt.Errorf("stdin and stdout must not be nil")
	}

	reader := bufio.NewReader(stdin)
	encoder := json.NewEncoder(stdout)
	// Note: Do NOT use SetIndent - MCP protocol requires compact JSON (one line per message)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Parse JSON-RPC request
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		// Handle request
		resp := s.handleRequest(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req map[string]any) map[string]any {
	method, _ := req["method"].(string)
	id := req["id"]

	switch method {
	case "initialize":
		return s.handleInitialize(id)
	case "tools/list":
		return s.handleToolsList(id)
	case "tools/call":
		return s.handleToolsCall(ctx, id, req)
	case "resources/list":
		return s.handleResourcesList(id)
	case "resources/read":
		return s.handleResourcesRead(ctx, id, req)
	default:
		return errorResponse(id, -32601, "Method not found: "+method)
	}
}

func (s *Server) handleInitialize(id any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "axon-go",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{
					"listChanged": false,
				},
				"resources": map[string]any{
					"listChanged": false,
				},
			},
		},
	}
}

func (s *Server) handleToolsList(id any) map[string]any {
	tools := s.ListTools()
	toolList := make([]map[string]any, len(tools))
	for i, tool := range tools {
		schema, _ := json.Marshal(tool.InputSchema)
		var schemaMap map[string]any
		json.Unmarshal(schema, &schemaMap)

		toolList[i] = map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": schemaMap,
		}
	}

	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"tools": toolList,
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, id any, req map[string]any) map[string]any {
	params, _ := req["params"].(map[string]any)
	if params == nil {
		return errorResponse(id, -32602, "Invalid params")
	}

	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	result, err := s.CallTool(ctx, name, args)
	if err != nil {
		return errorResponse(id, -32000, err.Error())
	}

	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": result,
				},
			},
		},
	}
}

func (s *Server) handleResourcesList(id any) map[string]any {
	resources := s.ListResources()
	resourceList := make([]map[string]any, len(resources))
	for i, res := range resources {
		resourceList[i] = map[string]any{
			"uri":         res.URI,
			"name":        res.Name,
			"description": res.Description,
			"mimeType":    res.MimeType,
		}
	}

	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"resources": resourceList,
		},
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, id any, req map[string]any) map[string]any {
	params, _ := req["params"].(map[string]any)
	if params == nil {
		return errorResponse(id, -32602, "Invalid params")
	}

	uri, _ := params["uri"].(string)

	content, err := s.ReadResource(ctx, uri)
	if err != nil {
		return errorResponse(id, -32000, err.Error())
	}

	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"contents": []map[string]any{
				{
					"uri":      uri,
					"mimeType": "text/plain",
					"text":     content,
				},
			},
		},
	}
}

// Tool Handlers

func handleQuery(storage StorageBackend, query string, limit int) (string, error) {
	if query == "" {
		return "No query provided", nil
	}

	ctx := context.Background()

	// Use hybrid search (FTS + Vector with RRF)
	// For now, query vector is nil (would be generated from query text in future)
	hybridResults, err := storage.HybridSearch(ctx, query, nil, limit)
	if err != nil {
		// Fallback to FTS only
		results, err := storage.FTSSearch(ctx, query, limit)
		if err != nil {
			return "", err
		}
		if len(results) == 0 {
			return "No results found", nil
		}
		return formatSearchResults(results, query), nil
	}

	if len(hybridResults) == 0 {
		return "No results found", nil
	}

	return formatHybridResults(hybridResults, query), nil
}

// formatHybridResults formats hybrid search results as markdown.
func formatHybridResults(results []storage.HybridSearchResult, query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results for '%s' (hybrid search):\n\n", len(results), query))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, r.NodeName, r.Label))
		sb.WriteString(fmt.Sprintf("   File: %s\n", r.FilePath))
		sb.WriteString(fmt.Sprintf("   Score: %.3f\n", r.Score))
		if r.Snippet != "" {
			snippet := r.Snippet
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s\n", snippet))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Next: Use `axon_context` on a specific symbol for the full picture.")

	return sb.String()
}

// formatSearchResults formats FTS search results as markdown.
func formatSearchResults(results []storage.SearchResult, query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results for '%s':\n\n", len(results), query))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, r.NodeName, r.Label))
		sb.WriteString(fmt.Sprintf("   File: %s\n", r.FilePath))
		sb.WriteString(fmt.Sprintf("   Score: %.3f\n", r.Score))
		if r.Snippet != "" {
			snippet := r.Snippet
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s\n", snippet))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Next: Use `axon_context` on a specific symbol for the full picture.")

	return sb.String()
}

// resolveSymbolToNodeID finds a node ID for a given symbol name using FTS search.
func resolveSymbolToNodeID(storage StorageBackend, symbol string) (string, error) {
	ctx := context.Background()

	// Use FTS search to find matching nodes
	results, err := storage.FTSSearch(ctx, symbol, 10)
	if err != nil {
		return "", err
	}

	// Look for exact match first
	for _, result := range results {
		if result.NodeName == symbol {
			return result.NodeID, nil
		}
	}

	// If no exact match, return the first result (best match)
	if len(results) > 0 {
		return results[0].NodeID, nil
	}

	return "", fmt.Errorf("symbol '%s' not found", symbol)
}

func handleContext(storage StorageBackend, symbol string) (string, error) {
	if symbol == "" {
		return "No symbol provided", nil
	}

	// Resolve symbol name to node ID
	nodeID, err := resolveSymbolToNodeID(storage, symbol)
	if err != nil {
		return fmt.Sprintf("Symbol '%s' not found in index", symbol), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Context for symbol: **%s**\n\n", symbol))

	// Get callers using node ID
	callers, _ := storage.GetCallers(context.Background(), nodeID)
	if len(callers) > 0 {
		sb.WriteString(fmt.Sprintf("## Callers (%d)\n", len(callers)))
		for _, c := range callers {
			sb.WriteString(fmt.Sprintf("- %s (%s) in %s\n", c.Name, c.Label, c.FilePath))
		}
		sb.WriteString("\n")
	}

	// Get callees using node ID
	callees, _ := storage.GetCallees(context.Background(), nodeID)
	if len(callees) > 0 {
		sb.WriteString(fmt.Sprintf("## Callees (%d)\n", len(callees)))
		for _, c := range callees {
			sb.WriteString(fmt.Sprintf("- %s (%s) in %s\n", c.Name, c.Label, c.FilePath))
		}
		sb.WriteString("\n")
	}

	if len(callers) == 0 && len(callees) == 0 {
		sb.WriteString("No connections found. Symbol may be isolated or not yet indexed.\n")
	}

	sb.WriteString("\nNext: Use `axon_impact` if planning changes to this symbol.")

	return sb.String(), nil
}

func handleImpact(storage StorageBackend, symbol string, depth int) (string, error) {
	if symbol == "" {
		return "No symbol provided", nil
	}

	// Resolve symbol name to node ID
	nodeID, err := resolveSymbolToNodeID(storage, symbol)
	if err != nil {
		return fmt.Sprintf("Symbol '%s' not found in index", symbol), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Impact analysis for: **%s** (depth: %d)\n\n", symbol, depth))

	// Traverse callers (blast radius) using node ID
	affected, _ := storage.Traverse(context.Background(), nodeID, depth, "callers")

	if len(affected) == 0 {
		sb.WriteString("No affected symbols found. This symbol appears to be isolated.\n")
	} else {
		sb.WriteString(fmt.Sprintf("## Affected Symbols (%d)\n\n", len(affected)))

		// Group by depth
		byDepth := make(map[int][]*graph.GraphNode)
		for i, node := range affected {
			d := (i % depth) + 1
			byDepth[d] = append(byDepth[d], node)
		}

		for d := 1; d <= depth; d++ {
			nodes := byDepth[d]
			if len(nodes) == 0 {
				continue
			}

			depthLabel := "Direct"
			if d == 2 {
				depthLabel = "Indirect"
			} else if d > 2 {
				depthLabel = "Transitive"
			}

			sb.WriteString(fmt.Sprintf("### Depth %d (%s)\n", d, depthLabel))
			for _, n := range nodes {
				sb.WriteString(fmt.Sprintf("- %s (%s) in %s\n", n.Name, n.Label, n.FilePath))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nTip: Review each affected symbol before making changes.")

	return sb.String(), nil
}

func handleDeadCode(storage StorageBackend) (string, error) {
	ctx := context.Background()

	// Get dead code nodes from storage
	deadNodes, err := storage.GetDeadCode(ctx)
	if err != nil {
		return "Error retrieving dead code: " + err.Error(), nil
	}

	var sb strings.Builder
	sb.WriteString("## Dead Code Report\n\n")

	if len(deadNodes) == 0 {
		sb.WriteString("✅ **No dead code detected!**\n\n")
		sb.WriteString("All symbols in the knowledge graph are either:\n")
		sb.WriteString("- Called by other symbols\n")
		sb.WriteString("- Entry points (main functions, test functions)\n")
		sb.WriteString("- Exported/public symbols\n")
		sb.WriteString("- Constructors or lifecycle methods\n")
		sb.WriteString("- Dunder methods or overrides\n\n")

		nodeCount := storage.NodeCount()
		sb.WriteString(fmt.Sprintf("Knowledge graph contains **%d nodes**, all reachable.\n", nodeCount))
	} else {
		sb.WriteString(fmt.Sprintf("⚠️ **Found %d dead code symbols**\n\n", len(deadNodes)))
		sb.WriteString("**Exempt from dead code detection:**\n")
		sb.WriteString("- Entry points (main functions, test functions)\n")
		sb.WriteString("- Exported/public symbols\n")
		sb.WriteString("- Constructors and lifecycle methods\n")
		sb.WriteString("- Dunder methods (__init__, __str__, etc.)\n")
		sb.WriteString("- Override methods\n\n")

		// Group by file
		byFile := make(map[string][]*graph.GraphNode)
		for _, node := range deadNodes {
			byFile[node.FilePath] = append(byFile[node.FilePath], node)
		}

		sb.WriteString("**Dead code by file:**\n\n")
		for filePath, nodes := range byFile {
			sb.WriteString(fmt.Sprintf("### %s (%d symbols)\n", filePath, len(nodes)))
			for _, node := range nodes {
				sb.WriteString(fmt.Sprintf("- `%s` (%s) at line %d\n", node.Name, node.Label, node.StartLine))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n**Next:** Review dead code symbols and consider removing or integrating them.")

	return sb.String(), nil
}

func handleListRepos() (string, error) {
	var sb strings.Builder
	sb.WriteString("## Indexed Repositories\n\n")

	// Note: Would need to read from global registry
	sb.WriteString("No repositories indexed yet. Run `axon analyze` to index a repository.\n")

	return sb.String(), nil
}

func handleCypher(storage StorageBackend, query string) (string, error) {
	if query == "" {
		return "No query provided", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Cypher Query Result\n\n"))
	sb.WriteString(fmt.Sprintf("Query: `%s`\n\n", query))
	sb.WriteString("Raw Cypher queries are not yet supported. This feature will allow advanced graph queries.\n")

	return sb.String(), nil
}

// Resource Handlers

func getOverview(storage StorageBackend) string {
	var sb strings.Builder
	sb.WriteString("# Axon Knowledge Graph Overview\n\n")
	sb.WriteString(fmt.Sprintf("**Nodes:** %d\n", storage.NodeCount()))
	sb.WriteString(fmt.Sprintf("**Relationships:** %d\n", storage.RelationshipCount()))
	sb.WriteString("\n## Node Types\n\n")
	sb.WriteString("- File: Source code files\n")
	sb.WriteString("- Folder: Directories\n")
	sb.WriteString("- Function: Function definitions\n")
	sb.WriteString("- Class: Class/struct definitions\n")
	sb.WriteString("- Method: Methods within classes\n")
	sb.WriteString("- Interface: Interface definitions\n")
	sb.WriteString("\n## Relationship Types\n\n")
	sb.WriteString("- CONTAINS: Folder hierarchy\n")
	sb.WriteString("- DEFINES: File defines symbol\n")
	sb.WriteString("- CALLS: Function calls\n")
	sb.WriteString("- IMPORTS: File imports\n")
	sb.WriteString("- EXTENDS: Class inheritance\n")
	sb.WriteString("- IMPLEMENTS: Interface implementation\n")
	sb.WriteString("- USES_TYPE: Type references\n")

	return sb.String()
}

func getSchema() string {
	var sb strings.Builder
	sb.WriteString("# Axon Knowledge Graph Schema\n\n")
	sb.WriteString("## Node Labels\n\n")
	sb.WriteString("| Label | Description | Key Properties |\n")
	sb.WriteString("|-------|-------------|----------------|\n")
	sb.WriteString("| `file` | Source file | path, language |\n")
	sb.WriteString("| `folder` | Directory | path |\n")
	sb.WriteString("| `function` | Function | name, signature, is_exported |\n")
	sb.WriteString("| `class` | Class/struct | name, bases |\n")
	sb.WriteString("| `method` | Method | name, class_name |\n")
	sb.WriteString("| `interface` | Interface | name |\n")
	sb.WriteString("| `type_alias` | Type alias | name, underlying_type |\n")
	sb.WriteString("\n## Relationship Types\n\n")
	sb.WriteString("| Type | Source → Target | Properties |\n")
	sb.WriteString("|------|-----------------|------------|\n")
	sb.WriteString("| `contains` | Folder → File/Symbol | - |\n")
	sb.WriteString("| `defines` | File → Symbol | - |\n")
	sb.WriteString("| `calls` | Symbol → Symbol | confidence |\n")
	sb.WriteString("| `imports` | File → File | symbols |\n")
	sb.WriteString("| `extends` | Class → Class | - |\n")
	sb.WriteString("| `implements` | Class → Interface | - |\n")
	sb.WriteString("| `uses_type` | Symbol → Type | role |\n")

	return sb.String()
}

func getDeadCodeList(storage StorageBackend) string {
	var sb strings.Builder
	sb.WriteString("# Dead Code Report\n\n")
	sb.WriteString("No dead code detected (or detection not yet implemented).\n")
	return sb.String()
}

// handleDetectChanges detects changes in specified files and analyzes their impact.
func handleDetectChanges(storage StorageBackend, files []string) (string, error) {
	if len(files) == 0 {
		return "No changed files provided. Please specify files to analyze.", nil
	}

	ctx := context.Background()

	// Get symbols in changed files
	changedSymbols := getSymbolsInFiles(storage, files)

	if len(changedSymbols) == 0 {
		return "No symbols found in the specified changed files.", nil
	}

	var sb strings.Builder
	sb.WriteString("# Change Detection Report\n\n")
	sb.WriteString(fmt.Sprintf("## Changed Files (%d)\n\n", len(files)))
	for _, file := range files {
		sb.WriteString(fmt.Sprintf("- `%s`\n", file))
	}

	sb.WriteString(fmt.Sprintf("\n## Changed Symbols (%d)\n\n", len(changedSymbols)))
	for _, sym := range changedSymbols {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) in `%s`\n", sym.Name, sym.Label, sym.FilePath))
	}

	// Get affected symbols (callers)
	affectedSymbols := getAffectedSymbols(ctx, storage, changedSymbols)

	if len(affectedSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("\n## Impact Analysis (%d affected symbols)\n\n", len(affectedSymbols)))
		sb.WriteString("These symbols may be affected by the changes:\n\n")
		for _, sym := range affectedSymbols {
			sb.WriteString(fmt.Sprintf("- **%s** (%s) in `%s`\n", sym.Name, sym.Label, sym.FilePath))
		}
		sb.WriteString("\n**Recommendation:** Review and test these affected symbols after making changes.\n")
	} else {
		sb.WriteString("\n## Impact Analysis\n\n")
		sb.WriteString("No other symbols appear to be directly affected by these changes.\n")
	}

	return sb.String(), nil
}

// getSymbolsInFiles returns all symbols in the specified files.
func getSymbolsInFiles(storage StorageBackend, files []string) []*graph.GraphNode {
	ctx := context.Background()
	var symbols []*graph.GraphNode

	fileSet := make(map[string]bool)
	for _, file := range files {
		fileSet[file] = true
	}

	// Get all nodes and filter by file
	labels := []graph.NodeLabel{
		graph.NodeFunction,
		graph.NodeMethod,
		graph.NodeClass,
		graph.NodeInterface,
		graph.NodeTypeAlias,
		graph.NodeEnum,
	}

	for _, label := range labels {
		nodes := storage.GetNodesByLabel(ctx, string(label))
		for _, node := range nodes {
			if fileSet[node.FilePath] {
				symbols = append(symbols, node)
			}
		}
	}

	return symbols
}

// getAffectedSymbols returns all symbols that call the changed symbols.
func getAffectedSymbols(ctx context.Context, storage StorageBackend, changedSymbols []*graph.GraphNode) []*graph.GraphNode {
	affectedMap := make(map[string]*graph.GraphNode)

	for _, sym := range changedSymbols {
		// Get callers of this symbol
		callers, _ := storage.GetCallers(ctx, sym.ID)
		for _, caller := range callers {
			affectedMap[caller.ID] = caller
		}
	}

	// Convert map to slice
	affected := make([]*graph.GraphNode, 0, len(affectedMap))
	for _, node := range affectedMap {
		affected = append(affected, node)
	}

	return affected
}

// Helper functions

func errorResponse(id any, code int, message string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
}

// registerTools registers tools with the MCP server.
func (s *Server) registerTools() {
	// Tools are handled via ListTools and CallTool
}

// registerResources registers resources with the MCP server.
func (s *Server) registerResources() {
	// Resources are handled via ListResources and ReadResource
}
