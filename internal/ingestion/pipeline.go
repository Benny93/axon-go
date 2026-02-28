// Package ingestion provides the data ingestion pipeline for Axon.
package ingestion

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Benny93/axon-go/internal/embeddings"
	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/parsers"
	"github.com/Benny93/axon-go/internal/storage"
)

// ParseData holds parsing results for all files.
type ParseData struct {
	mu    sync.RWMutex
	Files map[string]*parsers.ParseResult
}

// NewParseData creates a new ParseData instance.
func NewParseData() *ParseData {
	return &ParseData{
		Files: make(map[string]*parsers.ParseResult),
	}
}

// AddFile adds parsing results for a file.
func (p *ParseData) AddFile(relPath string, result *parsers.ParseResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Files[relPath] = result
}

// PipelineResult summarizes a pipeline run.
type PipelineResult struct {
	Files         int
	Symbols       int
	Relationships int
	DeadCode      int
	CoupledPairs  int
	DurationSecs  float64
}

// ProgressCallback is called with phase name and progress (0.0-1.0).
type ProgressCallback func(phase string, progress float64)

// RunPipeline runs the full ingestion pipeline.
func RunPipeline(
	ctx context.Context,
	repoPath string,
	store storage.StorageBackend,
	full bool,
	progress ProgressCallback,
	embeddings bool,
) (*graph.KnowledgeGraph, *PipelineResult, error) {
	result := &PipelineResult{}

	if progress != nil {
		progress("Walking files", 0.0)
	}

	// Phase 1: File walking
	patterns, _ := loadGitignore(repoPath)
	entries, err := WalkRepo(repoPath, patterns)
	if err != nil {
		return nil, nil, fmt.Errorf("walking repo: %w", err)
	}
	result.Files = len(entries)

	if progress != nil {
		progress("Walking files", 1.0)
	}

	g := graph.NewKnowledgeGraph()

	// Phase 2: Structure
	if progress != nil {
		progress("Processing structure", 0.0)
	}
	ProcessStructure(entries, g)
	if progress != nil {
		progress("Processing structure", 1.0)
	}

	// Phase 3: Parsing
	if progress != nil {
		progress("Parsing code", 0.0)
	}
	parseData := ProcessParsing(entries, g)
	if progress != nil {
		progress("Parsing code", 1.0)
	}

	// Phase 4: Imports
	if progress != nil {
		progress("Resolving imports", 0.0)
	}
	ProcessImports(parseData, g)
	if progress != nil {
		progress("Resolving imports", 1.0)
	}

	// Phase 5: Calls
	if progress != nil {
		progress("Tracing calls", 0.0)
	}
	ProcessCalls(parseData, g)
	if progress != nil {
		progress("Tracing calls", 1.0)
	}

	// Phase 6: Heritage
	if progress != nil {
		progress("Extracting heritage", 0.0)
	}
	ProcessHeritage(parseData, g)
	if progress != nil {
		progress("Extracting heritage", 1.0)
	}

	// Phase 7: Types
	if progress != nil {
		progress("Analyzing types", 0.0)
	}
	ProcessTypes(parseData, g)
	if progress != nil {
		progress("Analyzing types", 1.0)
	}

	// Phase 9: Process/Flow Detection
	if progress != nil {
		progress("Detecting execution flows", 0.0)
	}
	processCount := ProcessProcesses(g)
	_ = processCount // Could add to result if needed
	if progress != nil {
		progress("Detecting execution flows", 1.0)
	}

	// Phase 8: Community Detection
	if progress != nil {
		progress("Detecting communities", 0.0)
	}
	communityCount := DetectCommunities(g)
	_ = communityCount // Could add to result if needed
	if progress != nil {
		progress("Detecting communities", 1.0)
	}

	// Phase 11: Git Coupling Analysis
	if progress != nil {
		progress("Analyzing git history", 0.0)
	}
	coupledCount := ProcessCoupling(g, repoPath)
	result.CoupledPairs = coupledCount
	if progress != nil {
		progress("Analyzing git history", 1.0)
	}

	// Phase 10: Dead Code Detection
	if progress != nil {
		progress("Detecting dead code", 0.0)
	}
	deadCodeCount := ProcessDeadCode(g)
	result.DeadCode = deadCodeCount
	if progress != nil {
		progress("Detecting dead code", 1.0)
	}

	// Phase 12: Embeddings (if enabled)
	if embeddings && progress != nil {
		progress("Generating embeddings", 0.0)
	}
	if embeddings {
		if err := GenerateAndStoreEmbeddings(ctx, g, store); err != nil {
			// Log error but don't fail the pipeline
			fmt.Printf("Warning: embedding generation failed: %v\n", err)
		}
	}
	if embeddings && progress != nil {
		progress("Generating embeddings", 1.0)
	}

	// Count results
	result.Symbols = countSymbols(g)
	result.Relationships = g.RelationshipCount()

	// Store in backend
	if store != nil {
		if progress != nil {
			progress("Loading to storage", 0.0)
		}
		if err := store.BulkLoad(ctx, g); err != nil {
			return nil, nil, fmt.Errorf("bulk load: %w", err)
		}
		if progress != nil {
			progress("Loading to storage", 1.0)
		}
	}

	return g, result, nil
}

// ProcessStructure creates File and Folder nodes with CONTAINS relationships.
func ProcessStructure(entries []FileEntry, g *graph.KnowledgeGraph) {
	for _, entry := range entries {
		// Create file node
		fileNode := &graph.GraphNode{
			ID:       graph.GenerateID(graph.NodeFile, entry.RelPath, ""),
			Label:    graph.NodeFile,
			Name:     filepath.Base(entry.RelPath),
			FilePath: entry.RelPath,
			Language: entry.Language,
			Content:  string(entry.Content),
		}
		g.AddNode(fileNode)

		// Create folder nodes for each directory level
		dir := filepath.Dir(entry.RelPath)
		if dir != "." {
			parts := strings.Split(dir, string(filepath.Separator))
			for i := range parts {
				folderPath := filepath.Join(parts[:i+1]...)
				folderNode := &graph.GraphNode{
					ID:       graph.GenerateID(graph.NodeFolder, folderPath, ""),
					Label:    graph.NodeFolder,
					Name:     parts[i],
					FilePath: folderPath,
				}
				g.AddNode(folderNode)

				// Create CONTAINS relationship from parent folder
				if i > 0 {
					parentPath := filepath.Join(parts[:i]...)
					parentID := graph.GenerateID(graph.NodeFolder, parentPath, "")
					rel := &graph.GraphRelationship{
						ID:     graph.GenerateID(graph.NodeFolder, folderPath, parts[i]),
						Type:   graph.RelContains,
						Source: parentID,
						Target: folderNode.ID,
					}
					g.AddRelationship(rel)
				}
			}

			// Link last folder to file
			lastFolderID := graph.GenerateID(graph.NodeFolder, dir, "")
			rel := &graph.GraphRelationship{
				ID:     graph.GenerateID(graph.NodeFolder, dir, filepath.Base(entry.RelPath)),
				Type:   graph.RelContains,
				Source: lastFolderID,
				Target: fileNode.ID,
			}
			g.AddRelationship(rel)
		}
	}
}

// ProcessParsing parses all files and extracts symbols.
func ProcessParsing(entries []FileEntry, g *graph.KnowledgeGraph) *ParseData {
	parseData := NewParseData()

	for _, entry := range entries {
		parser := getParserForLanguage(entry.Language)
		if parser == nil {
			continue
		}

		result, err := parser.Parse(entry.RelPath, entry.Content)
		if err != nil {
			continue
		}

		parseData.AddFile(entry.RelPath, result)

		// Create symbol nodes
		for _, sym := range result.Symbols {
			var label graph.NodeLabel
			switch sym.Kind {
			case graph.NodeFunction, graph.NodeMethod:
				label = sym.Kind
			case graph.NodeClass:
				label = graph.NodeClass
			case graph.NodeInterface:
				label = graph.NodeInterface
			case graph.NodeTypeAlias:
				label = graph.NodeTypeAlias
			default:
				label = graph.NodeFunction
			}

			nodeID := graph.GenerateID(label, entry.RelPath, sym.Name)
			node := &graph.GraphNode{
				ID:         nodeID,
				Label:      label,
				Name:       sym.Name,
				FilePath:   entry.RelPath,
				StartLine:  sym.StartLine,
				EndLine:    sym.EndLine,
				Content:    sym.Content,
				Signature:  sym.Signature,
				Language:   entry.Language,
				ClassName:  sym.ClassName,
				IsExported: sym.IsExported,
			}
			g.AddNode(node)

			// Create DEFINES relationship from file
			fileID := graph.GenerateID(graph.NodeFile, entry.RelPath, "")
			rel := &graph.GraphRelationship{
				ID:     graph.GenerateID(graph.NodeFunction, entry.RelPath, sym.Name),
				Type:   graph.RelDefines,
				Source: fileID,
				Target: nodeID,
			}
			g.AddRelationship(rel)
		}
	}

	return parseData
}

// ProcessImports creates IMPORTS relationships between files.
func ProcessImports(parseData *ParseData, g *graph.KnowledgeGraph) {
	for filePath, result := range parseData.Files {
		sourceFileID := graph.GenerateID(graph.NodeFile, filePath, "")

		for _, imp := range result.Imports {
			// Try to find target file
			targetPath := findImportTarget(filePath, imp.ModulePath)
			if targetPath == "" {
				continue
			}

			targetFileID := graph.GenerateID(graph.NodeFile, targetPath, "")

			// Only create relationship if target exists
			if g.GetNode(targetFileID) != nil {
				rel := &graph.GraphRelationship{
					ID:     graph.GenerateID(graph.NodeFile, filePath, imp.ModulePath),
					Type:   graph.RelImports,
					Source: sourceFileID,
					Target: targetFileID,
					Properties: map[string]any{
						"symbols": imp.Symbols,
					},
				}
				g.AddRelationship(rel)
			}
		}
	}
}

// ProcessCalls creates CALLS relationships between symbols.
func ProcessCalls(parseData *ParseData, g *graph.KnowledgeGraph) {
	for filePath, result := range parseData.Files {
		for _, sym := range result.Symbols {
			sourceID := graph.GenerateID(sym.Kind, filePath, sym.Name)

			for _, call := range result.Calls {
				// Try to find target symbol
				targetID := findSymbolTarget(g, call.Name, call.Receiver, call.Package, filePath)
				if targetID == "" {
					continue
				}

				rel := &graph.GraphRelationship{
					ID:     graph.GenerateID(graph.NodeFunction, filePath, fmt.Sprintf("%s->%s", sym.Name, call.Name)),
					Type:   graph.RelCalls,
					Source: sourceID,
					Target: targetID,
					Properties: map[string]any{
						"confidence": 0.8,
					},
				}
				g.AddRelationship(rel)
			}
		}
	}
}

// ProcessHeritage creates EXTENDS and IMPLEMENTS relationships.
func ProcessHeritage(parseData *ParseData, g *graph.KnowledgeGraph) {
	for filePath, result := range parseData.Files {
		for _, h := range result.Heritage {
			sourceID := graph.GenerateID(graph.NodeClass, filePath, h.ClassName)

			for _, base := range h.Extends {
				targetID := findSymbolTarget(g, base, "", "", filePath)
				if targetID == "" {
					continue
				}

				rel := &graph.GraphRelationship{
					ID:     graph.GenerateID(graph.NodeClass, filePath, fmt.Sprintf("%s->%s", h.ClassName, base)),
					Type:   graph.RelExtends,
					Source: sourceID,
					Target: targetID,
				}
				g.AddRelationship(rel)
			}

			for _, iface := range h.Implements {
				targetID := findSymbolTarget(g, iface, "", "", filePath)
				if targetID == "" {
					continue
				}

				rel := &graph.GraphRelationship{
					ID:     graph.GenerateID(graph.NodeClass, filePath, fmt.Sprintf("%s->%s", h.ClassName, iface)),
					Type:   graph.RelImplements,
					Source: sourceID,
					Target: targetID,
				}
				g.AddRelationship(rel)
			}
		}
	}
}

// ProcessTypes creates USES_TYPE relationships.
func ProcessTypes(parseData *ParseData, g *graph.KnowledgeGraph) {
	for filePath, result := range parseData.Files {
		for _, sym := range result.Symbols {
			sourceID := graph.GenerateID(sym.Kind, filePath, sym.Name)

			for _, typeRef := range result.TypeRefs {
				targetID := findSymbolTarget(g, typeRef.Name, "", "", filePath)
				if targetID == "" {
					continue
				}

				rel := &graph.GraphRelationship{
					ID:     graph.GenerateID(graph.NodeFunction, filePath, fmt.Sprintf("%s->%s", sym.Name, typeRef.Name)),
					Type:   graph.RelUsesType,
					Source: sourceID,
					Target: targetID,
					Properties: map[string]any{
						"role": typeRef.Role,
					},
				}
				g.AddRelationship(rel)
			}
		}
	}
}

// Helper functions

func getParserForLanguage(language string) parsers.Parser {
	switch language {
	case "go":
		return parsers.NewGoParser()
	case "python":
		return parsers.NewPythonParser()
	case "typescript":
		return parsers.NewTypeScriptParser()
	default:
		return nil
	}
}

func findImportTarget(sourceFile, modulePath string) string {
	// Simplified import resolution
	// In production, this would use Go's module resolution
	if strings.HasPrefix(modulePath, ".") || strings.HasPrefix(modulePath, "/") {
		return modulePath + ".go"
	}
	// For standard imports, would need module path resolution
	return ""
}

// FindSymbolTargetForTest is an exported wrapper for testing.
func FindSymbolTargetForTest(g *graph.KnowledgeGraph, name, receiver, pkgPath, sourceFile string) string {
	return findSymbolTarget(g, name, receiver, pkgPath, sourceFile)
}

func findSymbolTarget(g *graph.KnowledgeGraph, name, receiver, pkgPath, sourceFile string) string {
	// If package path is specified, look for functions in that package
	if pkgPath != "" {
		// Look for function by name across all files
		nodes := g.GetNodesByLabel(graph.NodeFunction)

		// Extract package name from path (last component)
		pkgName := pkgPath
		if idx := strings.LastIndex(pkgPath, "/"); idx >= 0 {
			pkgName = pkgPath[idx+1:]
		}

		// First try to match by package name in file path
		for _, n := range nodes {
			if n.Name == name {
				// Check if file is in the expected package directory
				dir := filepath.Dir(n.FilePath)
				if strings.HasSuffix(dir, pkgName) || strings.Contains(dir, pkgName) {
					return n.ID
				}
			}
		}

		// If no exact package match, just match by name as fallback
		for _, n := range nodes {
			if n.Name == name {
				return n.ID
			}
		}
	}

	// Check for method call with receiver
	if receiver != "" {
		// Look for method on receiver type
		methods := g.GetNodesByLabel(graph.NodeMethod)
		for _, m := range methods {
			if m.Name == name && m.ClassName == receiver {
				return m.ID
			}
		}
	}

	// Look for function/type by name
	labels := []graph.NodeLabel{
		graph.NodeFunction,
		graph.NodeClass,
		graph.NodeInterface,
		graph.NodeTypeAlias,
	}

	for _, label := range labels {
		nodes := g.GetNodesByLabel(label)
		for _, n := range nodes {
			if n.Name == name {
				return n.ID
			}
		}
	}

	return ""
}

func countSymbols(g *graph.KnowledgeGraph) int {
	count := 0
	labels := []graph.NodeLabel{
		graph.NodeFunction,
		graph.NodeMethod,
		graph.NodeClass,
		graph.NodeInterface,
		graph.NodeTypeAlias,
		graph.NodeEnum,
	}
	for _, label := range labels {
		count += g.CountNodesByLabel(label)
	}
	return count
}

// GenerateAndStoreEmbeddings generates TF-IDF embeddings for all nodes and stores them.
func GenerateAndStoreEmbeddings(ctx context.Context, g *graph.KnowledgeGraph, store storage.StorageBackend) error {
	// Collect all nodes
	var nodes []*graph.GraphNode
	for node := range g.IterNodes() {
		nodes = append(nodes, node)
	}

	if len(nodes) == 0 {
		return nil
	}

	// Create TF-IDF embedder
	embedder := embeddings.NewTFIDFEmbedder()

	// Generate embeddings
	embeddingList := embedder.EmbedNodes(nodes)

	// Convert to storage embeddings
	storageEmbeddings := make([]storage.NodeEmbedding, len(nodes))
	for i, node := range nodes {
		storageEmbeddings[i] = storage.NodeEmbedding{
			NodeID:    node.ID,
			Embedding: embeddingList[i],
		}
	}

	// Store embeddings
	return store.StoreEmbeddings(ctx, storageEmbeddings)
}
