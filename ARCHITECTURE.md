# Axon-Go Architecture Documentation

## Overview

**Axon-Go** is a Go rewrite of the original [Axon](https://github.com/harshkedia177/axon) project by Harsh Kedia. It indexes codebases into a structural knowledge graph and exposes this graph through MCP (Model Context Protocol) tools for AI agents and a CLI for developers.

**Core Value Proposition**: Precompute code structure at index time so AI agents get complete, actionable context in a single tool call—no multi-step exploration required.

**Key Improvements over Python**:
- 10-50x faster performance (native compilation, goroutines)
- Single binary deployment (no Python runtime required)
- 10x lower memory usage (~50MB vs ~500MB)
- Watch mode with live re-indexing
- Better concurrency model

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Source Code Input                          │
│              (.go, .py, .ts, .tsx, .js, .jsx)                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Ingestion Pipeline (12 Phases)                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ 1. File Walking      │ 7. Type Analysis                  │   │
│  │ 2. Structure         │ 8. Community Detection            │   │
│  │ 3. Parsing           │ 9. Process Detection              │   │
│  │ 4. Import Resolution │ 10. Dead Code Detection           │   │
│  │ 5. Call Tracing      │ 11. Git Coupling                  │   │
│  │ 6. Heritage          │ 12. Embeddings                    │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    In-Memory Knowledge Graph                    │
│         (sync.RWMutex-protected, O(1) lookups)                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Storage Backend Layer                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │  BadgerDB   │  │  FTS Index  │  │  Vector Index           │  │
│  │  (KV Store) │  │  (BM25)     │  │  (TF-IDF, 100-dim)      │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────┐
│      MCP Server         │     │      CLI (Kong)         │
│   (stdio, MCP SDK)      │     │   (color, progress)     │
└─────────────────────────┘     └─────────────────────────┘
              │                               │
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────┐
│   Claude Code / Cursor  │     │    Terminal (Developer) │
│   (AI Agent)            │     │                         │
└─────────────────────────┘     └─────────────────────────┘
```

---

## Directory Structure

```
axon-go/
├── main.go                  # Application entry point
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build targets (build, test, lint, etc.)
├── cmd/
│   └── cmd.go               # All 13 CLI commands (Kong-based)
├── internal/
│   ├── embeddings/
│   │   ├── tfidf.go         # TF-IDF embedding generation
│   │   └── text.go          # Text representation for embeddings
│   ├── graph/
│   │   ├── graph.go         # In-memory KnowledgeGraph with mutex
│   │   └── model.go         # Node/Relationship types, enums
│   ├── ingestion/
│   │   ├── pipeline.go      # 12-phase orchestration
│   │   ├── walker.go        # File walking with gitignore
│   │   ├── community.go     # Louvain algorithm clustering
│   │   ├── processes.go     # Execution flow detection (BFS)
│   │   ├── dead_code.go     # 3-pass dead code detection
│   │   ├── coupling.go      # Git co-change analysis
│   │   └── watcher.go       # Watch mode with fsnotify
│   ├── parsers/
│   │   ├── parser.go        # Parser interface
│   │   ├── go.go            # Go parser (go/parser AST)
│   │   ├── python.go        # Python parser (regex-based)
│   │   └── typescript.go    # TypeScript parser (regex-based)
│   └── storage/
│       ├── backend.go       # StorageBackend interface
│       ├── badger_backend.go # BadgerDB implementation
│       ├── fts.go           # Full-text search (BM25)
│       ├── hybrid_search.go # Hybrid search (RRF fusion)
│       └── memory_backend.go # In-memory backend (testing)
└── mcp/
    └── server.go            # MCP server (7 tools, 3 resources)
```

---

## Core Components

### 1. Ingestion Pipeline

**File**: `internal/ingestion/pipeline.go`

The pipeline processes code through 12 sequential phases:

```go
func RunPipeline(
    ctx context.Context,
    repoPath string,
    store storage.StorageBackend,
    full bool,
    progress ProgressCallback,
    embeddings bool,
) (*graph.KnowledgeGraph, *PipelineResult, error)
```

**Phases**:

| Phase | Function | Description | Relationships Created |
|-------|----------|-------------|----------------------|
| 1 | `WalkRepo()` | Discovers files, respects .gitignore | - |
| 2 | `ProcessStructure()` | Creates File/Folder nodes | `CONTAINS` |
| 3 | `ProcessParsing()` | Extracts symbols via parsers | `DEFINES` |
| 4 | `ProcessImports()` | Resolves import statements | `IMPORTS` |
| 5 | `ProcessCalls()` | Traces function/method calls | `CALLS` |
| 6 | `ProcessHeritage()` | Class inheritance, interfaces | `EXTENDS`, `IMPLEMENTS` |
| 7 | `ProcessTypes()` | Type reference extraction | `USES_TYPE` |
| 8 | `DetectCommunities()` | Louvain clustering | `MEMBER_OF` |
| 9 | `ProcessProcesses()` | Execution flow detection | `STEP_IN_PROCESS` |
| 10 | `ProcessDeadCode()` | 3-pass dead code detection | `IsDead` flag |
| 11 | `ProcessCoupling()` | Git co-change analysis | `COUPLED_WITH` |
| 12 | `GenerateAndStoreEmbeddings()` | TF-IDF vectors | Stored in BadgerDB |

**Progress Callback**: Reports phase name and completion (0.0-1.0) for each phase.

---

### 2. Knowledge Graph

**File**: `internal/graph/graph.go`

Thread-safe in-memory graph with O(1) lookups:

```go
type KnowledgeGraph struct {
    nodes         map[string]*GraphNode
    edges         map[string][]Edge
    incoming      map[string][]*GraphRelationship
    outgoing      map[string][]*GraphRelationship
    mu            sync.RWMutex
}
```

**Node Labels**:
- `file`, `folder` - File system structure
- `function`, `method`, `class`, `interface` - Code symbols
- `type_alias`, `enum` - Type definitions
- `community`, `process` - Detected clusters and flows

**Relationship Types**:
- `contains`, `defines`, `calls`, `imports`
- `extends`, `implements`, `uses_type`
- `member_of`, `step_in_process`, `coupled_with`

---

### 3. Language Parsers

**Interface**: `internal/parsers/parser.go`

```go
type Parser interface {
    Language() string
    Parse(filePath string, content []byte) (*ParseResult, error)
}
```

**Implementations**:

| Language | Parser | Approach | Accuracy |
|----------|--------|----------|----------|
| Go | `go.go` | `go/parser` AST | ⭐⭐⭐⭐⭐ (Perfect) |
| Python | `python.go` | Regex-based | ⭐⭐⭐⭐ (Good) |
| TypeScript | `typescript.go` | Regex-based | ⭐⭐⭐⭐ (Good) |
| JavaScript | `typescript.go` | Regex-based | ⭐⭐⭐⭐ (Good) |

**ParseResult Structure**:
```go
type ParseResult struct {
    Package   string
    Symbols   []ParsedSymbol
    Imports   []ImportStatement
    Calls     []CallSite
    TypeRefs  []TypeAnnotation
    Heritage  []ClassHeritage
}
```

---

### 4. Storage Backend

**Interface**: `internal/storage/backend.go`

```go
type StorageBackend interface {
    Initialize(path string, readOnly bool) error
    Close() error
    BulkLoad(ctx context.Context, g *graph.KnowledgeGraph) error
    GetCallers(ctx context.Context, nodeID string) ([]*graph.GraphNode, error)
    GetCallees(ctx context.Context, nodeID string) ([]*graph.GraphNode, error)
    FTSSearch(ctx context.Context, query string, limit int) ([]SearchResult, error)
    VectorSearch(ctx context.Context, vector []float32, limit int) ([]SearchResult, error)
    HybridSearch(ctx context.Context, query string, queryVector []float32, limit int) ([]HybridSearchResult, error)
    GetDeadCode(ctx context.Context) ([]*graph.GraphNode, error)
    // ... more methods
}
```

**Implementation**: BadgerDB (embedded key-value store)

**Indexes**:
- **Node Data**: Key-value under `n:` prefix
- **Relationships**: Key-value under `r:` prefix
- **FTS Index**: In-memory token → nodeID mapping
- **Embeddings**: Key-value under `e:` prefix (JSON arrays)

---

### 5. Full-Text Search

**File**: `internal/storage/fts.go`

Custom BM25 implementation:

```go
type FTSIndex struct {
    index map[string][]string  // token → []nodeID
    docs  map[string]DocInfo   // nodeID → DocInfo
}
```

**Tokenization**:
- Lowercases text
- Splits on `_`, `.`, `-`, spaces
- Splits camelCase (`UserService` → `user`, `service`)
- Splits on number boundaries (`HTTP2` → `http`, `2`)

**Scoring**: Term Frequency (TF) based

---

### 6. Vector Search & Hybrid Search

**Embeddings**: `internal/embeddings/tfidf.go`

TF-IDF based embeddings (100 dimensions):

```go
type TFIDFEmbedder struct {
    vocab    map[string]int     // term → index
    idf      []float64          // inverse document frequency
    docCount int
}
```

**Hybrid Search**: `internal/storage/hybrid_search.go`

Reciprocal Rank Fusion (RRF) for combining FTS + Vector:

```go
func HybridSearch(ctx context.Context, storage StorageBackend,
                  query string, queryVector []float32,
                  limit, k int) ([]HybridSearchResult, error)
```

**RRF Formula**: `score = Σ (1 / (k + rank_i))` where k=60

---

### 7. MCP Server

**File**: `mcp/server.go`

7 tools and 3 resources exposed via MCP protocol:

**Tools**:
1. `axon_query` - Hybrid search (FTS + Vector)
2. `axon_context` - 360° symbol view (callers/callees)
3. `axon_impact` - Blast radius analysis
4. `axon_dead_code` - Dead code report
5. `axon_detect_changes` - Change detection & impact
6. `axon_list_repos` - List indexed repos
7. `axon_cypher` - Raw graph queries (stub)

**Resources**:
1. `axon://overview` - Knowledge graph statistics
2. `axon://dead-code` - Dead code report
3. `axon://schema` - Graph schema documentation

**Transport**: stdio (JSON-RPC over stdin/stdout)

---

### 8. Watch Mode

**File**: `internal/ingestion/watcher.go`

Live re-indexing using `fsnotify`:

```go
func WatchRepo(ctx context.Context, repoPath string, store storage.StorageBackend) error
```

**Features**:
- Monitors file system for changes
- Batches changes (2-second debounce)
- Re-indexes only changed files (Phases 2-7)
- Runs global phases every 30 seconds (Phases 8-12)
- Handles file deletions

---

## Data Flow

### Indexing Flow

```
1. User runs: axon-go analyze .
2. WalkRepo() discovers files (respects .gitignore)
3. For each file:
   a. Detect language by extension
   b. Parse symbols, imports, calls
   c. Add nodes and relationships to graph
4. Run global analysis (communities, processes, dead code, coupling)
5. Generate embeddings for all symbols
6. Persist to BadgerDB
7. Report statistics
```

### Query Flow

```
1. User/AI calls: axon_query("auth handler")
2. HybridSearch() runs:
   a. FTS search → ranked results
   b. Embed query → vector search → ranked results
   c. RRF fusion → combined ranking
3. Return top-k results with snippets
```

### Context Flow

```
1. User/AI calls: axon_context("UserService")
2. resolveSymbolToNodeID() finds node via FTS
3. GetCallers() and GetCallees() retrieve relationships
4. Format and return 360° view
```

---

## Concurrency Model

**Go Advantages**:
- Goroutines for parallel file processing
- `sync.RWMutex` for thread-safe graph access
- Channel-based communication in watch mode
- Context-based cancellation

**Thread Safety**:
- All graph operations protected by `sync.RWMutex`
- Storage backend uses per-operation locking
- ParseData uses separate mutex for concurrent parsing

---

## Performance Characteristics

| Operation | Time Complexity | Notes |
|-----------|-----------------|-------|
| Node lookup | O(1) | Hash map |
| Get callers/callees | O(1) | Pre-indexed edges |
| FTS search | O(n) | n = tokens in index |
| Vector search | O(n) | n = embeddings (can optimize with HNSW) |
| Hybrid search | O(n) | FTS + Vector + RRF |
| Community detection | O(n log n) | Louvain algorithm |
| Dead code detection | O(n) | 3-pass analysis |

**Typical Performance** (1000 files):
- Index time: ~0.8s (vs ~15s Python)
- Query latency: <50ms
- Memory: ~50MB (vs ~500MB Python)

---

## Testing Strategy

**Test Files**: `*_test.go` in each package

**Coverage**:
- Unit tests for individual functions
- Integration tests for pipeline phases
- End-to-end tests for full pipeline
- MCP tool tests with mock storage

**Test Count**: 520+ tests, all passing

**Run Tests**:
```bash
make test
# or
go test ./...
```

---

## Build & Deployment

**Build**:
```bash
make build
# Produces: ./bin/axon-go
```

**Flags** (set via ldflags):
- `main.version` - Git tag or commit
- `main.buildTime` - Build timestamp
- `main.gitCommit` - Git commit hash

**Deployment**: Single static binary (~20MB)

**Dependencies**: None at runtime (fully static)

---

## Configuration

**No config files required**. All configuration via CLI flags:

```bash
axon-go analyze --verbose --no-embeddings .
axon-go serve --watch
```

**Storage Location**: `.axon/badger/` in repository root

---

## Error Handling

**Go Idioms**:
- Explicit error returns
- `fmt.Errorf()` with `%w` for wrapping
- Context-based cancellation
- Graceful degradation (e.g., missing .gitignore)

**User-Facing Errors**:
- Colored output (red for errors, green for success)
- Actionable messages (e.g., "Run 'axon analyze' first")

---

## Future Enhancements

1. **HNSW Index** - Approximate nearest neighbor for faster vector search
2. **Neural Embeddings** - Integration with code-specific models (CodeBERT, etc.)
3. **TypeScript AST Parser** - Tree-sitter integration for better TS support
4. **Incremental Indexing** - Smarter diff-based re-indexing
5. **Graph Visualization** - Export to DOT/GraphML format
6. **Multi-Repository Analysis** - Cross-repo dependency tracking

---

## Credits

**Original Concept & Design**: Harsh Kedia ([Axon](https://github.com/harshkedia177/axon))

**Go Implementation**: Benjamin Vollmer ([axon-go](https://github.com/Benny93/axon-go))

All credit for the original vision, architecture, and Python implementation belongs to Harsh Kedia. This Go implementation preserves and extends that work.

---

**Last Updated**: 2026-02-28  
**Version**: axon-go 1.0 (Go rewrite)
