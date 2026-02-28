// Package parsers provides tree-sitter based code parsers for multiple languages.
package parsers

import "github.com/Benny93/axon-go/internal/graph"

// ParsedSymbol represents a code entity extracted from source.
type ParsedSymbol struct {
	// Name is the symbol name (function, class, variable, etc.)
	Name string

	// Kind is the symbol kind (function, class, method, interface, etc.)
	Kind graph.NodeLabel

	// StartLine is the starting line number (1-based)
	StartLine int

	// EndLine is the ending line number (1-based)
	EndLine int

	// Content is the source code content
	Content string

	// Signature is the function/method signature
	Signature string

	// ClassName is the parent class name (for methods)
	ClassName string

	// IsExported indicates if the symbol is exported/public
	IsExported bool

	// Decorators contains decorator names (Python/TS)
	Decorators []string
}

// ImportStatement represents an import statement.
type ImportStatement struct {
	// ModulePath is the imported module/file path
	ModulePath string

	// Symbols is the list of imported symbol names
	Symbols []string

	// Alias is the import alias (if any)
	Alias string

	// IsRelative indicates if it's a relative import
	IsRelative bool

	// StartLine is the line number of the import
	StartLine int
}

// CallSite represents a function/method call.
type CallSite struct {
	// Name is the called function/method name
	Name string

	// Receiver is the receiver object (for method calls)
	Receiver string

	// Package is the package qualifier (for package.Function calls)
	Package string

	// StartLine is the line number of the call
	StartLine int

	// EndLine is the ending line number
	EndLine int
}

// TypeAnnotation represents a type reference.
type TypeAnnotation struct {
	// Name is the type name
	Name string

	// Role is the usage role (param, return, variable)
	Role string

	// StartLine is the line number
	StartLine int
}

// ClassHeritage represents class inheritance information.
type ClassHeritage struct {
	// ClassName is the class name
	ClassName string

	// Extends is the list of base classes
	Extends []string

	// Implements is the list of implemented interfaces
	Implements []string
}

// ParseResult contains all parsed information from a source file.
type ParseResult struct {
	// Package is the package name
	Package string

	// PackageImports maps import aliases to package paths
	PackageImports map[string]string

	// Symbols extracted from the file
	Symbols []ParsedSymbol

	// Imports found in the file
	Imports []ImportStatement

	// Call sites found in the file
	Calls []CallSite

	// Type annotations found in the file
	TypeRefs []TypeAnnotation

	// Class heritage information
	Heritage []ClassHeritage
}

// Parser defines the interface for language-specific parsers.
type Parser interface {
	// Parse parses source code and extracts symbols, imports, calls, etc.
	Parse(filePath string, content []byte) (*ParseResult, error)

	// Language returns the language this parser handles
	Language() string
}
