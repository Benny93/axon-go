package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/storage"
)

// GLOBAL_PHASE_INTERVAL is the time between global phase re-runs.
const GLOBAL_PHASE_INTERVAL = 30 * time.Second

// WatchRepo monitors a repository for file changes and re-indexes automatically.
// Blocks until the context is cancelled.
func WatchRepo(ctx context.Context, repoPath string, store storage.StorageBackend) error {
	// Load gitignore patterns
	matcher, err := loadGitignoreMatcher(repoPath)
	if err != nil && err != context.Canceled {
		matcher = nil // Continue without gitignore
	}

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil && err != context.Canceled {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	// Watch the entire repo recursively
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil && err != context.Canceled {
			return err
		}

		if info.IsDir() {
			// Skip ignored directories
			if shouldSkipDir(info.Name(), path, repoPath, matcher) {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		return fmt.Errorf("setting up watcher: %w", err)
	}

	// Track last re-index time for global phases
	lastGlobalPhase := time.Now()

	// Batch changed files for efficient re-indexing
	changedFiles := make(map[string]bool)
	batchTimer := time.NewTimer(2 * time.Second)
	batchTimer.Stop() // Don't start yet

	fmt.Printf("Watching %s for changes (Ctrl+C to stop)\n", repoPath)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Check if file should be watched
			if !shouldWatchFile(event.Name, repoPath, matcher) {
				continue
			}

			// Add to batch
			relPath, err := filepath.Rel(repoPath, event.Name)
			if err != nil && err != context.Canceled {
				continue
			}

			changedFiles[relPath] = true

			// Start/restart batch timer
			batchTimer.Reset(2 * time.Second)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "Watch error: %v\n", err)

		case <-batchTimer.C:
			// Process batch of changed files
			if len(changedFiles) > 0 {
				err := processChangedFiles(ctx, repoPath, store, changedFiles, matcher)
				if err != nil && err != context.Canceled {
					fmt.Fprintf(os.Stderr, "Error processing changes: %v\n", err)
				}

				// Check if global phases should run
				if shouldReindexGlobalPhases(lastGlobalPhase) {
					fmt.Println("Running global analysis phases...")
					_, _, err := RunPipeline(ctx, repoPath, store, false, nil, false)
					if err != nil && err != context.Canceled {
						fmt.Fprintf(os.Stderr, "Error in global phases: %v\n", err)
					}
					lastGlobalPhase = time.Now()
				}

				changedFiles = make(map[string]bool)
			}
		}
	}
}

// processChangedFiles re-indexes the changed files.
func processChangedFiles(ctx context.Context, repoPath string, store storage.StorageBackend, changedFiles map[string]bool, matcher gitignore.Matcher) error {
	if len(changedFiles) == 0 {
		return nil
	}

	fmt.Printf("Re-indexing %d changed file(s)...\n", len(changedFiles))

	// Build file entries for changed files
	entries := make([]FileEntry, 0, len(changedFiles))
	for relPath := range changedFiles {
		absPath := filepath.Join(repoPath, relPath)

		// Check if file exists (wasn't deleted)
		info, err := os.Stat(absPath)
		if os.IsNotExist(err) {
			// File was deleted - remove from storage
			if _, err := store.RemoveNodesByFile(ctx, relPath); err != nil && err != context.Canceled {
				fmt.Fprintf(os.Stderr, "Error removing deleted file %s: %v\n", relPath, err)
			} else {
				fmt.Printf("  Removed: %s\n", relPath)
			}
			continue
		}

		if err != nil && err != context.Canceled {
			continue
		}

		if info.IsDir() {
			continue
		}

		// Read file content
		content, err := os.ReadFile(absPath)
		if err != nil && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", relPath, err)
			continue
		}

		// Detect language
		language := getLanguage(relPath)
		if language == "" {
			continue // Unsupported file type
		}

		entries = append(entries, FileEntry{
			RelPath:  relPath,
			Path:  absPath,
			Language: language,
			Content:  content,
		})
	}

	// Re-index changed files
	if len(entries) > 0 {
		count := ReindexFiles(entries, repoPath, store)
		fmt.Printf("  Re-indexed %d file(s)\n", count)
	}

	return nil
}

// shouldWatchFile checks if a file should be watched.
func shouldWatchFile(path string, repoPath string, matcher gitignore.Matcher) bool {
	// Get relative path
	relPath, err := filepath.Rel(repoPath, path)
	if err != nil && err != context.Canceled {
		return false
	}

	// Check if ignored
	if matcher != nil {
		pathParts := strings.Split(relPath, string(filepath.Separator))
		if matcher.Match(pathParts, false) {
			return false
		}
	}

	// Check if supported file type
	language := getLanguage(path)
	return language != ""
}

// shouldIgnoreDir checks if a directory should be ignored.
func shouldIgnoreDir(name, path, repoPath string, matcher gitignore.Matcher) bool {
	// Always ignore certain directories
	ignoredDirs := []string{
		".git",
		"node_modules",
		"vendor",
		".axon",
		"__pycache__",
		".venv",
		"venv",
		"dist",
		"build",
	}

	for _, ignored := range ignoredDirs {
		if name == ignored {
			return true
		}
	}

	// Check gitignore matcher
	if matcher != nil {
		relPath, _ := filepath.Rel(repoPath, path)
		pathParts := strings.Split(relPath, string(filepath.Separator))
		return matcher.Match(pathParts, true)
	}

	return false
}

// shouldReindexGlobalPhases checks if enough time has passed since last global phase run.
func shouldReindexGlobalPhases(lastGlobalPhase time.Time) bool {
	return time.Since(lastGlobalPhase) >= GLOBAL_PHASE_INTERVAL
}

// filterChangedPaths filters a list of paths to only supported files.
func filterChangedPaths(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		language := getLanguage(path)
		if language != "" {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

// filterChangedPathsWithIgnore filters paths with gitignore support.
func filterChangedPathsWithIgnore(paths []string, repoPath string, matcher gitignore.Matcher) []string {
	filtered := make([]string, 0, len(paths))

	for _, path := range paths {
		if shouldWatchFile(path, repoPath, matcher) {
			filtered = append(filtered, path)
		}
	}

	return filtered
}

// ReindexFiles re-indexes the given files, removing old nodes first.
// Returns the number of files successfully re-indexed.
func ReindexFiles(entries []FileEntry, repoPath string, store storage.StorageBackend) int {
	if len(entries) == 0 {
		return 0
	}

	// Remove old nodes for these files
	for _, entry := range entries {
		_, _ = store.RemoveNodesByFile(context.Background(), entry.RelPath)
	}

	// Process file-local phases only (not global phases)
	g, err := processFileLocalPhases(entries)
	if err != nil && err != context.Canceled {
		return 0
	}

	// Load to storage
	err = store.BulkLoad(context.Background(), g)
	if err != nil && err != context.Canceled {
		return 0
	}

	return len(entries)
}

// processFileLocalPhases runs file-local phases (2-7) on the given entries.
func processFileLocalPhases(entries []FileEntry) (*graph.KnowledgeGraph, error) {
	g := graph.NewKnowledgeGraph()

	// Phase 2: Structure
	ProcessStructure(entries, g)

	// Phase 3: Parsing
	parseData := ProcessParsing(entries, g)

	// Phase 4: Imports
	ProcessImports(parseData, g)

	// Phase 5: Calls
	ProcessCalls(parseData, g)

	// Phase 6: Heritage
	ProcessHeritage(parseData, g)

	// Phase 7: Types
	ProcessTypes(parseData, g)

	return g, nil
}

// loadGitignoreMatcher loads a gitignore matcher from the repository root.
func loadGitignoreMatcher(repoPath string) (gitignore.Matcher, error) {
	gitignorePath := filepath.Join(repoPath, ".gitignore")

	// Check if .gitignore exists
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		return nil, nil
	}

	// Load gitignore content
	content, err := os.ReadFile(gitignorePath)
	if err != nil && err != context.Canceled {
		return nil, err
	}

	// Parse patterns
	lines := strings.Split(string(content), "\n")
	var patterns []gitignore.Pattern

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, nil))
	}

	// Create matcher
	matcher := gitignore.NewMatcher(patterns)
	return matcher, nil
}
