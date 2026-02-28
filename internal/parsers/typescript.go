package parsers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// TypeScriptParser parses TypeScript/TSX source code.
type TypeScriptParser struct {
	functionRegex *regexp.Regexp
	classRegex    *regexp.Regexp
	interfaceRegex *regexp.Regexp
	typeRegex     *regexp.Regexp
	importRegex   *regexp.Regexp
	callRegex     *regexp.Regexp
}

// NewTypeScriptParser creates a new TypeScript parser.
func NewTypeScriptParser() *TypeScriptParser {
	return &TypeScriptParser{
		functionRegex:  regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(([^)]*)\)(?:\s*:\s*(\S+))?`),
		classRegex:     regexp.MustCompile(`(?m)^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?(?:\s+implements\s+(\w+))?`),
		interfaceRegex: regexp.MustCompile(`(?m)^(?:export\s+)?interface\s+(\w+)`),
		typeRegex:      regexp.MustCompile(`(?m)^(?:export\s+)?type\s+(\w+)\s*=`),
		importRegex:    regexp.MustCompile(`(?m)^import\s+(?:{([^}]+)}|\*\s+as\s+(\w+)|(\w+))\s+from\s+['"]([^'"]+)['"]`),
		callRegex:      regexp.MustCompile(`(\w+)\s*\(([^)]*)\)`),
	}
}

// Language returns the language this parser handles.
func (p *TypeScriptParser) Language() string {
	return "typescript"
}

// SupportsFile checks if this parser can handle the given file.
func (p *TypeScriptParser) SupportsFile(filename string) bool {
	return strings.HasSuffix(filename, ".ts") || strings.HasSuffix(filename, ".tsx")
}

// Parse parses TypeScript source code and extracts symbols, imports, calls, etc.
func (p *TypeScriptParser) Parse(filePath string, content []byte) (*ParseResult, error) {
	source := string(content)

	result := &ParseResult{
		Symbols:  []ParsedSymbol{},
		Imports:  []ImportStatement{},
		Calls:    []CallSite{},
		TypeRefs: []TypeAnnotation{},
		Heritage: []ClassHeritage{},
		Package:  "",
	}

	// Parse functions (including arrow functions)
	p.parseFunctions(source, filePath, result)

	// Parse classes
	p.parseClasses(source, filePath, result)

	// Parse interfaces
	p.parseInterfaces(source, filePath, result)

	// Parse types
	p.parseTypes(source, filePath, result)

	// Parse imports
	p.parseImports(source, filePath, result)

	// Parse function calls
	p.parseCalls(source, filePath, result)

	return result, nil
}

func (p *TypeScriptParser) parseFunctions(source, filePath string, result *ParseResult) {
	matches := p.functionRegex.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := match[1]
		params := ""
		returnType := ""

		if len(match) > 2 {
			params = strings.TrimSpace(match[2])
		}
		if len(match) > 3 && match[3] != "" {
			returnType = match[3]
		}

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		signature := fmt.Sprintf("function %s(%s)", name, params)
		if returnType != "" {
			signature += fmt.Sprintf(": %s", returnType)
		}

		sym := ParsedSymbol{
			Name:       name,
			Kind:       graph.NodeFunction,
			StartLine:  lineNum,
			EndLine:    lineNum,
			Signature:  signature,
			IsExported: strings.Contains(source[:strings.Index(source, match[0])], "export"),
		}

		result.Symbols = append(result.Symbols, sym)

		// Extract type references from parameters and return type
		p.extractTypeRefs(params, returnType, filePath, lineNum, result)
	}

	// Parse arrow functions assigned to constants
	arrowRegex := regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:\(([^)]*)\)|(\w+))\s*=>`)
	arrowMatches := arrowRegex.FindAllStringSubmatch(source, -1)
	for _, match := range arrowMatches {
		if len(match) < 2 {
			continue
		}

		name := match[1]
		params := ""
		if len(match) > 2 && match[2] != "" {
			params = match[2]
		} else if len(match) > 3 && match[3] != "" {
			params = match[3]
		}

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		sym := ParsedSymbol{
			Name:       name,
			Kind:       graph.NodeFunction,
			
			StartLine:  lineNum,
			EndLine:    lineNum,
			Signature:  fmt.Sprintf("const %s = (%s) => ...", name, params),
			IsExported: strings.Contains(source[:strings.Index(source, match[0])], "export"),
		}

		result.Symbols = append(result.Symbols, sym)
	}
}

func (p *TypeScriptParser) parseClasses(source, filePath string, result *ParseResult) {
	matches := p.classRegex.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := match[1]
		extends := ""
		implements := ""

		if len(match) > 2 && match[2] != "" {
			extends = match[2]
		}
		if len(match) > 3 && match[3] != "" {
			implements = match[3]
		}

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		sym := ParsedSymbol{
			Name:       name,
			Kind:       graph.NodeClass,
			
			StartLine:  lineNum,
			EndLine:    lineNum,
			Signature:  fmt.Sprintf("class %s", name),
			IsExported: strings.Contains(source[:strings.Index(source, match[0])], "export"),
		}

		result.Symbols = append(result.Symbols, sym)

		// Add heritage information
		if extends != "" || implements != "" {
			heritage := ClassHeritage{
				ClassName: name,
			}
			if extends != "" {
				heritage.Extends = []string{extends}
			}
			if implements != "" {
				heritage.Implements = []string{implements}
			}
			result.Heritage = append(result.Heritage, heritage)
		}

		// Parse methods in class
		p.parseClassMethods(source, filePath, name, result)
	}
}

func (p *TypeScriptParser) parseClassMethods(source, filePath, className string, result *ParseResult) {
	// Simple method regex - looks for methods within class body
	methodRegex := regexp.MustCompile(fmt.Sprintf(`(?m)(?:async\s+)?(?:get|set)?\s*(\w+)\s*\(([^)]*)\)(?:\s*:\s*(\S+))?`))

	// Find class body
	classBodyRegex := regexp.MustCompile(fmt.Sprintf(`(?m)class\s+%s\s*{([^}]+)}`, className))
	classMatch := classBodyRegex.FindStringSubmatch(source)
	if len(classMatch) < 2 {
		return
	}

	classBody := classMatch[1]
	matches := methodRegex.FindAllStringSubmatch(classBody, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := match[1]
		// Skip constructor and keywords
		if name == "constructor" || name == "if" || name == "for" || name == "while" {
			continue
		}

		params := ""
		returnType := ""
		if len(match) > 2 {
			params = match[2]
		}
		if len(match) > 3 && match[3] != "" {
			returnType = match[3]
		}

		signature := fmt.Sprintf("%s(%s)", name, params)
		if returnType != "" {
			signature += fmt.Sprintf(": %s", returnType)
		}

		sym := ParsedSymbol{
			Name:       name,
			Kind:       graph.NodeMethod,
			ClassName:  className,
			
			Signature:  signature,
			IsExported: false, // Methods inherit export status from class
		}

		result.Symbols = append(result.Symbols, sym)
	}
}

func (p *TypeScriptParser) parseInterfaces(source, filePath string, result *ParseResult) {
	matches := p.interfaceRegex.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := match[1]

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		sym := ParsedSymbol{
			Name:       name,
			Kind:       graph.NodeInterface,
			
			StartLine:  lineNum,
			EndLine:    lineNum,
			Signature:  fmt.Sprintf("interface %s", name),
			IsExported: strings.Contains(source[:strings.Index(source, match[0])], "export"),
		}

		result.Symbols = append(result.Symbols, sym)
	}
}

func (p *TypeScriptParser) parseTypes(source, filePath string, result *ParseResult) {
	matches := p.typeRegex.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := match[1]

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		sym := ParsedSymbol{
			Name:       name,
			Kind:       graph.NodeTypeAlias,
			
			StartLine:  lineNum,
			EndLine:    lineNum,
			Signature:  fmt.Sprintf("type %s = ...", name),
			IsExported: strings.Contains(source[:strings.Index(source, match[0])], "export"),
		}

		result.Symbols = append(result.Symbols, sym)
	}
}

func (p *TypeScriptParser) parseImports(source, filePath string, result *ParseResult) {
	matches := p.importRegex.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		if len(match) < 5 {
			continue
		}

		modulePath := match[4]
		symbols := []string{}

		// Named imports: { User, Post }
		if match[1] != "" {
			parts := strings.Split(match[1], ",")
			for _, part := range parts {
				symbols = append(symbols, strings.TrimSpace(part))
			}
		}

		// Namespace import: * as db
		if match[2] != "" {
			symbols = append(symbols, match[2])
		}

		// Default import: express
		if match[3] != "" {
			symbols = append(symbols, match[3])
		}

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		imp := ImportStatement{
			ModulePath: modulePath,
			Symbols:    symbols,
			IsRelative: strings.HasPrefix(modulePath, "."),
			StartLine:  lineNum,
		}

		result.Imports = append(result.Imports, imp)
	}
}

func (p *TypeScriptParser) parseCalls(source, filePath string, result *ParseResult) {
	matches := p.callRegex.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name := match[1]

		// Skip keywords and common non-function calls
		if name == "if" || name == "for" || name == "while" || name == "switch" ||
			name == "catch" || name == "function" || name == "class" || name == "return" {
			continue
		}

		// Find line number
		lineNum := 1
		for i := 0; i < strings.Index(source, match[0]); i++ {
			if source[i] == '\n' {
				lineNum++
			}
		}

		call := CallSite{
			Name:      name,
			StartLine: lineNum,
			EndLine:   lineNum,
		}

		result.Calls = append(result.Calls, call)
	}
}

func (p *TypeScriptParser) extractTypeRefs(params, returnType, filePath string, lineNum int, result *ParseResult) {
	// Extract type references from parameters
	if params != "" {
		typeRegex := regexp.MustCompile(`(\w+)\s*:\s*(\w+)`)
		matches := typeRegex.FindAllStringSubmatch(params, -1)
		for _, match := range matches {
			if len(match) > 2 {
				result.TypeRefs = append(result.TypeRefs, TypeAnnotation{
					Name:      match[2],
					Role:      "param",
					StartLine: lineNum,
				})
			}
		}
	}

	// Extract type reference from return type
	if returnType != "" {
		result.TypeRefs = append(result.TypeRefs, TypeAnnotation{
			Name:      returnType,
			Role:      "return",
			StartLine: lineNum,
		})
	}
}
