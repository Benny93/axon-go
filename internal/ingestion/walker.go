// Package ingestion provides the data ingestion pipeline for Axon.
package ingestion

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// FileEntry represents a file to be processed.
type FileEntry struct {
	// Path is the absolute file path.
	Path string

	// RelPath is the path relative to the repo root.
	RelPath string

	// Language is the detected programming language.
	Language string

	// Content is the file content.
	Content []byte

	// SHA256 is the hash of the file content.
	SHA256 string

	// IsDir indicates if this is a directory.
	IsDir bool
}

// Supported file extensions and their languages.
var supportedExtensions = map[string]string{
	".py":  "python",
	".ts":  "typescript",
	".tsx": "typescript",
	".js":  "javascript",
	".jsx": "javascript",
	".mjs": "javascript",
	".cjs": "javascript",
	".go":  "go",
}

// Default patterns to ignore (in addition to .gitignore).
var defaultIgnorePatterns = []string{
	".git/",
	"node_modules/",
	".axon/",
	"__pycache__/",
	".venv/",
	"venv/",
	".tox/",
	".eggs/",
	"*.egg-info/",
	".pytest_cache/",
	".mypy_cache/",
	"coverage/",
	"htmlcov/",
	".coverage",
	"*.pyc",
	"*.pyo",
	"*.pyd",
	".DS_Store",
	"Thumbs.db",
}

// WalkRepo walks the repository and returns all supported files.
func WalkRepo(repoPath string, patterns []gitignore.Pattern) ([]FileEntry, error) {
	var entries []FileEntry

	// Combine default patterns with loaded patterns
	allPatterns := make([]gitignore.Pattern, 0, len(defaultIgnorePatterns)+len(patterns))

	// Add default patterns
	for _, p := range defaultIgnorePatterns {
		allPatterns = append(allPatterns, gitignore.ParsePattern(p, nil))
	}

	// Add loaded patterns from .gitignore
	allPatterns = append(allPatterns, patterns...)

	matcher := gitignore.NewMatcher(allPatterns)

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories we don't want to traverse
		if d.IsDir() {
			// Check if directory should be skipped
			if shouldSkipDir(d.Name(), path, repoPath, matcher) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file is supported
		if !isSupportedFile(d.Name()) {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			return err
		}

		// Check gitignore patterns
		pathParts := splitPath(relPath)
		if matcher.Match(pathParts, false) {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Compute SHA256 hash
		hash := sha256.Sum256(content)

		entries = append(entries, FileEntry{
			Path:     path,
			RelPath:  relPath,
			Language: getLanguage(d.Name()),
			Content:  content,
			SHA256:   hex.EncodeToString(hash[:]),
			IsDir:    false,
		})

		return nil
	})

	return entries, err
}

// loadGitignore loads .gitignore patterns from the repository root.
func loadGitignore(repoPath string) ([]gitignore.Pattern, error) {
	gitignorePath := filepath.Join(repoPath, ".gitignore")

	// Check if .gitignore exists
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		return nil, nil
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
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

		pattern := gitignore.ParsePattern(line, nil)
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// isSupportedFile checks if a file has a supported extension.
func isSupportedFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	_, ok := supportedExtensions[ext]
	return ok
}

// getLanguage returns the language for a file extension.
func getLanguage(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	return supportedExtensions[ext]
}

// shouldSkipDir checks if a directory should be skipped.
func shouldSkipDir(name, path, repoRoot string, matcher gitignore.Matcher) bool {
	// Always skip .git
	if name == ".git" {
		return true
	}

	// Check matcher
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}

	pathParts := splitPath(relPath)
	return matcher.Match(pathParts, true)
}

// splitPath splits a path into its components.
func splitPath(path string) []string {
	return strings.Split(path, string(filepath.Separator))
}
