// Package cmd provides CLI command implementations for Axon.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/fatih/color"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/ingestion"
	"github.com/Benny93/axon-go/internal/storage"
	"github.com/Benny93/axon-go/mcp"
)

// Version is set at build time via ldflags.
var Version = "dev"

// AnalyzeCmd indexes a repository into a knowledge graph.
type AnalyzeCmd struct {
	Path         string `arg:"" optional:"" default:"." help:"Path to repository"`
	Full         bool   `help:"Perform full re-index"`
	NoEmbeddings bool   `help:"Skip vector embedding generation"`
}

// Run executes the analyze command.
func (c *AnalyzeCmd) Run() error {
	ctx := context.Background()
	repoPath, err := filepath.Abs(c.Path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(repoPath)
	if err != nil {
		return fmt.Errorf("accessing %s: %w", repoPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", repoPath)
	}

	color.Green("Indexing %s", repoPath)

	// Create .axon directory
	axonDir := filepath.Join(repoPath, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		return fmt.Errorf("creating .axon directory: %w", err)
	}

	// Initialize BadgerDB storage
	dbPath := filepath.Join(axonDir, "badger")
	store := storage.NewBadgerBackend()
	if err := store.Initialize(dbPath, false); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Run pipeline
	var result *ingestion.PipelineResult

	progress := func(phase string, pct float64) {
		fmt.Printf("\r\033[K%s (%.0f%%)", phase, pct*100)
	}

	_, result, err = ingestion.RunPipeline(
		ctx,
		repoPath,
		store,
		c.Full,
		progress,
		!c.NoEmbeddings,
	)
	if err != nil {
		return fmt.Errorf("running pipeline: %w", err)
	}

	fmt.Println() // Newline after progress

	// Write meta.json
	meta := map[string]any{
		"version":    Version,
		"name":       filepath.Base(repoPath),
		"path":       repoPath,
		"stats":      result,
		"indexed_at": time.Now().UTC().Format(time.RFC3339),
	}

	metaPath := filepath.Join(axonDir, "meta.json")
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metaPath, metaJSON, 0o644); err != nil {
		return fmt.Errorf("writing meta.json: %w", err)
	}

	// Print summary
	color.Green("\n✓ Indexing complete")
	fmt.Printf("  Files:          %d\n", result.Files)
	fmt.Printf("  Symbols:        %d\n", result.Symbols)
	fmt.Printf("  Relationships:  %d\n", result.Relationships)
	fmt.Printf("  Duration:       %.2fs\n", result.DurationSecs)

	return nil
}

// QueryCmd searches the knowledge graph.
type QueryCmd struct {
	Query string `arg:"" help:"Search query"`
	Limit int    `short:"n" default:"20" help:"Maximum results"`
}

// Run executes the query command.
func (c *QueryCmd) Run() error {
	ctx := context.Background()
	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	results, err := store.FTSSearch(ctx, c.Query, c.Limit)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found")
		return nil
	}

	for i, r := range results {
		fmt.Printf("\n%d. %s (%s)\n", i+1, r.NodeName, r.Label)
		fmt.Printf("   File: %s\n", r.FilePath)
		fmt.Printf("   Score: %.3f\n", r.Score)
		if r.Snippet != "" {
			fmt.Printf("   %s\n", r.Snippet[:min(200, len(r.Snippet))])
		}
	}

	return nil
}

// ContextCmd shows 360-degree view of a symbol.
type ContextCmd struct {
	Symbol string `arg:"" help:"Symbol name to inspect"`
}

// Run executes the context command.
func (c *ContextCmd) Run() error {
	if c.Symbol == "" {
		return fmt.Errorf("symbol name required. Usage: axon context <symbol>")
	}

	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Find the symbol by name
	nodeID, err := findSymbolByName(store, c.Symbol)
	if err != nil {
		return err
	}
	if nodeID == "" {
		fmt.Printf("Symbol '%s' not found in the knowledge graph.\n", c.Symbol)
		return nil
	}

	// Get the node details
	node, err := store.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		fmt.Printf("Symbol '%s' not found.\n", c.Symbol)
		return nil
	}

	// Print context information
	fmt.Printf("## Context for: **%s** (%s)\n\n", node.Name, node.Label)
	fmt.Printf("**File:** %s\n", node.FilePath)
	if node.StartLine > 0 && node.EndLine > 0 {
		fmt.Printf("**Lines:** %d-%d\n", node.StartLine, node.EndLine)
	}
	fmt.Println()

	// Get callers
	callers, err := store.GetCallers(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(callers) > 0 {
		fmt.Printf("### Callers (%d)\n", len(callers))
		for _, caller := range callers {
			fmt.Printf("- %s (%s) in %s\n", caller.Name, caller.Label, caller.FilePath)
		}
		fmt.Println()
	} else {
		fmt.Println("### Callers")
		fmt.Println("None")
		fmt.Println()
	}

	// Get callees
	callees, err := store.GetCallees(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(callees) > 0 {
		fmt.Printf("### Callees (%d)\n", len(callees))
		for _, callee := range callees {
			fmt.Printf("- %s (%s) in %s\n", callee.Name, callee.Label, callee.FilePath)
		}
		fmt.Println()
	} else {
		fmt.Println("### Callees")
		fmt.Println("None")
		fmt.Println()
	}

	fmt.Println("Next: Use `axon impact` if planning changes to this symbol.")

	return nil
}

// ImpactCmd shows blast radius of changing a symbol.
type ImpactCmd struct {
	Symbol string `arg:"" help:"Symbol to analyze"`
	Depth  int    `short:"d" default:"3" help:"Traversal depth"`
}

// Run executes the impact command.
func (c *ImpactCmd) Run() error {
	if c.Symbol == "" {
		return fmt.Errorf("symbol name required. Usage: axon impact <symbol>")
	}

	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Find the symbol by name
	nodeID, err := findSymbolByName(store, c.Symbol)
	if err != nil {
		return err
	}
	if nodeID == "" {
		fmt.Printf("Symbol '%s' not found in the knowledge graph.\n", c.Symbol)
		return nil
	}

	// Get the node details
	_, err = store.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}

	// Print impact header
	fmt.Printf("## Impact Analysis for: **%s** (depth: %d)\n\n", c.Symbol, c.Depth)

	// Traverse callers (blast radius) - who calls this symbol?
	affected, err := store.Traverse(ctx, nodeID, c.Depth, "callers")
	if err != nil {
		return err
	}

	if len(affected) == 0 {
		fmt.Println("No affected symbols found. This symbol appears to be isolated (no callers).")
		fmt.Println("\nTip: This might be an entry point or unused code.")
		return nil
	}

	fmt.Printf("## Affected Symbols (%d)\n\n", len(affected))

	// Group by depth
	byDepth := make(map[int][]*graph.GraphNode)
	for i, n := range affected {
		d := (i % c.Depth) + 1
		byDepth[d] = append(byDepth[d], n)
	}

	for d := 1; d <= c.Depth && d <= len(byDepth); d++ {
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

		fmt.Printf("### Depth %d (%s) - %d symbols\n", d, depthLabel, len(nodes))
		for _, n := range nodes {
			fmt.Printf("- %s (%s) in %s\n", n.Name, n.Label, n.FilePath)
		}
		fmt.Println()
	}

	fmt.Println("Tip: Review each affected symbol before making changes.")

	return nil
}

// DeadCodeCmd lists all detected dead code.
type DeadCodeCmd struct{}

// Run executes the dead-code command.
func (c *DeadCodeCmd) Run() error {
	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Get dead code nodes from storage
	deadNodes, err := store.GetDeadCode(ctx)
	if err != nil {
		return fmt.Errorf("retrieving dead code: %w", err)
	}

	fmt.Println("## Dead Code Report")

	if len(deadNodes) == 0 {
		fmt.Println("✅ No dead code detected!")
		fmt.Println()
		fmt.Println("All symbols in the knowledge graph are either:")
		fmt.Println("- Called by other symbols")
		fmt.Println("- Entry points (main functions, test functions)")
		fmt.Println("- Exported/public symbols")
		fmt.Println("- Constructors or lifecycle methods")
		fmt.Println("- Dunder methods or overrides")
		fmt.Println()
		fmt.Printf("Knowledge graph contains %d nodes, all reachable.\n", store.NodeCount())
	} else {
		fmt.Printf("⚠️ Found %d dead code symbols\n\n", len(deadNodes))
		fmt.Println("Exempt from dead code detection:")
		fmt.Println("- Entry points (main functions, test functions)")
		fmt.Println("- Exported/public symbols")
		fmt.Println("- Constructors and lifecycle methods")
		fmt.Println("- Dunder methods (__init__, __str__, etc.)")
		fmt.Println("- Override methods")
		fmt.Println()

		// Group by file
		byFile := make(map[string][]*graph.GraphNode)
		for _, node := range deadNodes {
			byFile[node.FilePath] = append(byFile[node.FilePath], node)
		}

		fmt.Println("Dead code by file:")
		fmt.Println()
		for filePath, nodes := range byFile {
			fmt.Printf("### %s (%d symbols)\n", filePath, len(nodes))
			for _, node := range nodes {
				fmt.Printf("  - %s (%s) at line %d\n", node.Name, node.Label, node.StartLine)
			}
			fmt.Println()
		}
	}

	fmt.Println("\nNext: Review dead code symbols and consider removing or integrating them.")

	return nil
}

// CypherCmd executes raw Cypher queries.
type CypherCmd struct {
	Query string `arg:"" help:"Cypher query"`
}

// Run executes the cypher command.
func (c *CypherCmd) Run() error {
	if c.Query == "" {
		return fmt.Errorf("query required. Usage: axon cypher <query>")
	}

	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	fmt.Println("## Cypher Query Support")
	fmt.Println("Raw Cypher queries are not yet supported with BadgerDB backend.")
	fmt.Println()
	fmt.Println("The storage backend uses BadgerDB (key-value store) instead of KuzuDB (graph DB).")
	fmt.Println("Direct graph queries are available through the Go API.")
	fmt.Println()
	fmt.Printf("Your query was: `%s`\n", c.Query)
	fmt.Println()
	fmt.Println("Alternative approaches:")
	fmt.Println("- Use `axon query <search>` for full-text search")
	fmt.Println("- Use `axon context <symbol>` for symbol relationships")
	fmt.Println("- Use `axon impact <symbol>` for blast radius analysis")

	return nil
}

// WatchCmd enables watch mode with live re-indexing.
type WatchCmd struct{}

// Run executes the watch command.
func (c *WatchCmd) Run() error {
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	fmt.Println("## Watch Mode")
	fmt.Printf("Watching %s for changes (Ctrl+C to stop)\n\n", repoPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	go func() {
		<-osSignalChannel()
		fmt.Println("\nStopping watch mode...")
		cancel()
	}()

	err = ingestion.WatchRepo(ctx, repoPath, store)
	if err != nil && err != context.Canceled {
		return fmt.Errorf("watch error: %w", err)
	}

	fmt.Println("Watch mode stopped.")
	return nil
}

// DiffCmd compares branches structurally.
type DiffCmd struct {
	BranchRange string `arg:"" help:"Branch range (e.g., main..feature)"`
}

// Run executes the diff command.
func (c *DiffCmd) Run() error {
	if c.BranchRange == "" {
		return fmt.Errorf("branch range required. Usage: axon diff <base>..<head>")
	}

	fmt.Println("## Branch Diff")
	fmt.Println("Branch comparison is not yet implemented.")
	fmt.Println()
	fmt.Println("This feature will:")
	fmt.Println("- Compare two git branches at the symbol level")
	fmt.Println("- Show added, modified, and removed symbols")
	fmt.Println("- Use git worktrees for comparison (no stashing required)")
	fmt.Println()
	fmt.Printf("Requested diff: %s\n", c.BranchRange)
	fmt.Println()
	fmt.Println("Workaround: Use `git diff` for text-level comparison.")

	return nil
}

// SetupCmd configures MCP for various AI clients.
type SetupCmd struct {
	Qwen     bool   `help:"Configure for Qwen CLI"`
	Claude   bool   `help:"Configure for Claude Code"`
	Cursor   bool   `help:"Configure for Cursor"`
	Local    bool   `help:"Create project-local configuration"`
	Global   bool   `help:"Create global configuration"`
	Format   string `help:"Output format (json|text)" enum:"json,text" default:"json"`
	FilePath string `help:"Custom file path for configuration"`
}

// Run executes the setup command.
func (c *SetupCmd) Run() error {
	// Validate format
	if c.Format != "json" && c.Format != "text" {
		return fmt.Errorf("invalid format: %s (must be json or text)", c.Format)
	}

	// If no specific client is specified, output config to stdout
	if !c.Qwen && !c.Claude && !c.Cursor {
		return c.outputDefaultConfig()
	}

	// If neither local nor global is specified, default to local
	if !c.Local && !c.Global {
		c.Local = true
	}

	// Setup for each requested client
	if c.Qwen {
		if err := c.setupQwen(); err != nil {
			return err
		}
	}

	if c.Claude {
		if err := c.setupClaude(); err != nil {
			return err
		}
	}

	if c.Cursor {
		if err := c.setupCursor(); err != nil {
			return err
		}
	}

	return nil
}

func (c *SetupCmd) outputDefaultConfig() error {
	config := generateAxonConfig()

	if c.Format == "json" {
		jsonBytes, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Println("# Add this to your MCP client configuration:")
		fmt.Println()
		for key, value := range config {
			fmt.Printf("%s: %s\n", key, toJSON(value))
		}
	}

	return nil
}

func (c *SetupCmd) setupQwen() error {
	config := generateAxonConfig()

	if c.Global {
		globalPath := getGlobalConfigPath("qwen")
		if err := writeConfig(globalPath, config, c.Format); err != nil {
			return err
		}
		color.Green("✓ Created global Qwen MCP config at %s", globalPath)
	}

	if c.Local {
		var localPath string
		if c.FilePath != "" {
			localPath = filepath.Join(c.FilePath, "mcp.json")
		} else {
			localPath = getLocalConfigPath(".", "qwen")
		}
		if err := writeConfig(localPath, config, c.Format); err != nil {
			return err
		}
		color.Green("✓ Created local Qwen MCP config at %s", localPath)
	}

	return nil
}

func (c *SetupCmd) setupClaude() error {
	config := generateClaudeConfig()

	if c.Global {
		globalPath := getGlobalConfigPath("claude")
		if err := writeConfig(globalPath, config, c.Format); err != nil {
			return err
		}
		color.Green("✓ Created global Claude MCP config at %s", globalPath)
	}

	if c.Local {
		var localPath string
		if c.FilePath != "" {
			localPath = filepath.Join(c.FilePath, "settings.json")
		} else {
			localPath = getLocalConfigPath(".", "claude")
		}
		if err := writeConfig(localPath, config, c.Format); err != nil {
			return err
		}
		color.Green("✓ Created local Claude MCP config at %s", localPath)
	}

	return nil
}

func (c *SetupCmd) setupCursor() error {
	config := generateCursorConfig()

	if c.Global {
		globalPath := getGlobalConfigPath("cursor")
		if err := writeConfig(globalPath, config, c.Format); err != nil {
			return err
		}
		color.Green("✓ Created global Cursor MCP config at %s", globalPath)
	}

	if c.Local {
		var localPath string
		if c.FilePath != "" {
			localPath = filepath.Join(c.FilePath, "mcp.json")
		} else {
			localPath = getLocalConfigPath(".", "cursor")
		}
		if err := writeConfig(localPath, config, c.Format); err != nil {
			return err
		}
		color.Green("✓ Created local Cursor MCP config at %s", localPath)
	}

	return nil
}

// Configuration generators

func generateAxonConfig() map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"axon-go": map[string]any{
				"command": "axon-go",
				"args":    []string{"serve", "--watch"},
			},
		},
	}
}

func generateClaudeConfig() map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"axon-go": map[string]any{
				"command": "axon-go",
				"args":    []string{"serve", "--watch"},
			},
		},
	}
}

func generateCursorConfig() map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"axon-go": map[string]any{
				"command": "axon-go",
				"args":    []string{"serve", "--watch"},
			},
		},
	}
}

// Path helpers

func getLocalConfigPath(basePath, client string) string {
	configDir := getClientConfigDir(client)
	return filepath.Join(basePath, configDir, "mcp.json")
}

func getGlobalConfigPath(client string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}

	configDir := getClientConfigDir(client)
	return filepath.Join(homeDir, configDir, "global", "mcp.json")
}

func getClientConfigDir(client string) string {
	switch client {
	case "qwen":
		return ".qwen"
	case "claude":
		return ".claude"
	case "cursor":
		return ".cursor"
	default:
		return ".qwen"
	}
}

// Config writers

func writeConfig(configPath string, config map[string]any, format string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	var content []byte
	var err error

	if format == "json" {
		content, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		content = append(content, '\n')
	} else {
		// Text format - just output key-value pairs
		var sb strings.Builder
		sb.WriteString("# MCP Configuration for Axon\n")
		sb.WriteString("# Generated by axon setup\n\n")

		for key, value := range config {
			sb.WriteString(fmt.Sprintf("%s: %s\n", key, toJSON(value)))
		}
		content = []byte(sb.String())
	}

	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// MCPCmd starts the MCP server.
type MCPCmd struct{}

// Run executes the mcp command.
func (c *MCPCmd) Run() error {
	ctx := context.Background()
	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Note: No output to stderr - MCP server uses stdio for JSON-RPC only
	return server.Run(ctx, os.Stdin, os.Stdout)
}

// ServeCmd starts the MCP server with optional watch mode.
type ServeCmd struct {
	Watch bool `short:"w" help:"Enable file watching"`
}

// Run executes the serve command.
func (c *ServeCmd) Run() error {
	ctx := context.Background()
	store, err := loadStorage()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	if c.Watch {
		fmt.Fprintln(os.Stderr, "Starting MCP server with watch mode...")

		// Get repo path
		repoPath, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		// Start watch mode in background
		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		go func() {
			err := ingestion.WatchRepo(watchCtx, repoPath, store)
			if err != nil && err != context.Canceled {
				fmt.Fprintf(os.Stderr, "Watch error: %v\n", err)
			}
		}()

		fmt.Fprintln(os.Stderr, "File watching enabled")
	} else {
		fmt.Fprintln(os.Stderr, "Starting MCP server...")
	}

	return server.Run(ctx, os.Stdin, os.Stdout)
}

// ListCmd lists all indexed repositories.
type ListCmd struct{}

// Run executes the list command.
func (c *ListCmd) Run() error {
	// List repos from global registry
	registryRoot := filepath.Join(os.Getenv("HOME"), ".axon", "repos")

	entries, err := os.ReadDir(registryRoot)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No indexed repositories found")
			return nil
		}
		return fmt.Errorf("reading registry: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No indexed repositories found")
		return nil
	}

	fmt.Println("Indexed repositories:")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metaPath := filepath.Join(registryRoot, entry.Name(), "meta.json")
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta map[string]any
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}

		fmt.Printf("\n  %s\n", entry.Name())
		if path, ok := meta["path"].(string); ok {
			fmt.Printf("    Path: %s\n", path)
		}
		if stats, ok := meta["stats"].(map[string]any); ok {
			if files, ok := stats["files"].(float64); ok {
				fmt.Printf("    Files: %.0f\n", files)
			}
			if symbols, ok := stats["symbols"].(float64); ok {
				fmt.Printf("    Symbols: %.0f\n", symbols)
			}
		}
		if indexedAt, ok := meta["indexed_at"].(string); ok {
			fmt.Printf("    Indexed: %s\n", indexedAt)
		}
	}

	return nil
}

// StatusCmd shows index status for current repository.
type StatusCmd struct{}

// Run executes the status command.
func (c *StatusCmd) Run() error {
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	metaPath := filepath.Join(repoPath, ".axon", "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no index found at %s. Run 'axon analyze' first", repoPath)
		}
		return fmt.Errorf("reading meta.json: %w", err)
	}

	var meta map[string]any
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return fmt.Errorf("parsing meta.json: %w", err)
	}

	fmt.Printf("Index status for %s\n", repoPath)
	if version, ok := meta["version"].(string); ok {
		fmt.Printf("  Version:        %s\n", version)
	}
	if indexedAt, ok := meta["indexed_at"].(string); ok {
		fmt.Printf("  Last indexed:   %s\n", indexedAt)
	}
	if stats, ok := meta["stats"].(map[string]any); ok {
		if files, ok := stats["files"].(float64); ok {
			fmt.Printf("  Files:          %.0f\n", files)
		}
		if symbols, ok := stats["symbols"].(float64); ok {
			fmt.Printf("  Symbols:        %.0f\n", symbols)
		}
		if relationships, ok := stats["relationships"].(float64); ok {
			fmt.Printf("  Relationships:  %.0f\n", relationships)
		}
	}

	return nil
}

// CleanCmd deletes index for current repository.
type CleanCmd struct {
	Force bool `short:"f" help:"Skip confirmation"`
}

// Run executes the clean command.
func (c *CleanCmd) Run() error {
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	axonDir := filepath.Join(repoPath, ".axon")
	if _, err := os.Stat(axonDir); os.IsNotExist(err) {
		return fmt.Errorf("no index found at %s. Nothing to clean", repoPath)
	}

	if !c.Force {
		fmt.Printf("Delete index at %s? [y/N] ", axonDir)
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return nil
		}
	}

	if err := os.RemoveAll(axonDir); err != nil {
		return fmt.Errorf("deleting index: %w", err)
	}

	color.Green("Deleted %s", axonDir)
	return nil
}

// Helper functions

// osSignalChannel returns a channel that receives OS signals for graceful shutdown.
func osSignalChannel() <-chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	return sigChan
}

func loadStorage() (*storage.BadgerBackend, error) {
	repoPath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	dbPath := filepath.Join(repoPath, ".axon", "badger")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no index found at %s. Run 'axon analyze' first", repoPath)
	}

	store := storage.NewBadgerBackend()
	if err := store.Initialize(dbPath, true); err != nil {
		return nil, fmt.Errorf("initializing storage: %w", err)
	}

	return store, nil
}

// findSymbolByName searches for a symbol by name across all node types.
func findSymbolByName(store *storage.BadgerBackend, name string) (string, error) {
	ctx := context.Background()

	// Try FTS search first
	results, err := store.FTSSearch(ctx, name, 10)
	if err != nil {
		return "", err
	}

	// Look for exact match
	for _, result := range results {
		if result.NodeName == name {
			return result.NodeID, nil
		}
	}

	// If no exact match, return the first result (best match)
	if len(results) > 0 {
		return results[0].NodeID, nil
	}

	return "", nil
}

func toJSON(v any) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CLI is the root Kong command structure.
type CLI struct {
	Version kong.VersionFlag `help:"Show version information"`
	Verbose bool             `short:"v" help:"Enable verbose output"`
	Quiet   bool             `short:"q" help:"Suppress non-essential output"`

	// Commands
	Analyze  AnalyzeCmd  `cmd:"" help:"Index a repository into a knowledge graph"`
	Query    QueryCmd    `cmd:"" help:"Search the knowledge graph"`
	Context  ContextCmd  `cmd:"" help:"Show 360-degree view of a symbol"`
	Impact   ImpactCmd   `cmd:"" help:"Show blast radius of changing a symbol"`
	DeadCode DeadCodeCmd `cmd:"" help:"List all detected dead code"`
	Cypher   CypherCmd   `cmd:"" help:"Execute raw Cypher query"`
	Watch    WatchCmd    `cmd:"" help:"Watch mode with live re-indexing"`
	Diff     DiffCmd     `cmd:"" help:"Structural branch comparison"`
	Setup    SetupCmd    `cmd:"" help:"Configure MCP for Claude Code / Cursor"`
	MCP      MCPCmd      `cmd:"" help:"Start MCP server (stdio transport)"`
	Serve    ServeCmd    `cmd:"" help:"Start MCP server with optional watch mode"`
	List     ListCmd     `cmd:"" help:"List all indexed repositories"`
	Status   StatusCmd   `cmd:"" help:"Show index status for current repo"`
	Clean    CleanCmd    `cmd:"" help:"Delete index for current repository"`
}

// NewCLI creates a new CLI instance.
func NewCLI() *CLI {
	return &CLI{}
}

// Execute parses command-line arguments and executes the selected command.
func (c *CLI) Execute(args []string) error {
	kongCtx := kong.Parse(c,
		kong.Name("axon-go"),
		kong.Description("Graph-powered code intelligence engine for Go"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:             true,
			NoExpandSubcommands: true,
		}),
		kong.Vars{
			"version": Version,
		},
	)

	return kongCtx.Run()
}
