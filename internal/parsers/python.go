package parsers

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// PythonParser parses Python source code using regex-based approach.
// Note: This is a simplified implementation. For production, use tree-sitter.
type PythonParser struct {
	functionRegex *regexp.Regexp
	classRegex    *regexp.Regexp
	importRegex   *regexp.Regexp
	callRegex     *regexp.Regexp
}

// NewPythonParser creates a new Python parser.
func NewPythonParser() *PythonParser {
	return &PythonParser{
		functionRegex: regexp.MustCompile(`^(?P<decorator>@\w+(?:\.\w+)?(?:\(.*\))?)?\s*(?:async\s+)?def\s+(?P<name>\w+)\s*\((?P<params>[^)]*)\)\s*(?:->\s*(?P<return>\S+))?`),
		classRegex:    regexp.MustCompile(`^class\s+(\w+)(?:\(([^)]+)\))?`),
		importRegex:   regexp.MustCompile(`^(?:from\s+([\w.]+)\s+)?import\s+(.+)`),
		callRegex:     regexp.MustCompile(`(\w+)\s*\(([^)]*)\)`),
	}
}

// Language returns the language this parser handles.
func (p *PythonParser) Language() string {
	return "python"
}

// Parse parses Python source code and extracts symbols, imports, calls, etc.
func (p *PythonParser) Parse(filePath string, content []byte) (*ParseResult, error) {
	result := &ParseResult{
		Symbols:  []ParsedSymbol{},
		Imports:  []ImportStatement{},
		Calls:    []CallSite{},
		TypeRefs: []TypeAnnotation{},
		Heritage: []ClassHeritage{},
	}

	lines := strings.Split(string(content), "\n")
	var currentClass string
	var decorators []string

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Collect decorators
		if strings.HasPrefix(trimmed, "@") {
			dec := strings.TrimPrefix(trimmed, "@")
			if idx := strings.Index(dec, "("); idx > 0 {
				dec = dec[:idx]
			}
			decorators = append(decorators, dec)
			continue
		}

		// Parse functions
		if matches := p.functionRegex.FindStringSubmatch(trimmed); matches != nil {
			nameIdx := p.functionRegex.SubexpIndex("name")
			paramsIdx := p.functionRegex.SubexpIndex("params")
			returnIdx := p.functionRegex.SubexpIndex("return")

			kind := graph.NodeFunction
			className := ""
			if currentClass != "" {
				kind = graph.NodeMethod
				className = currentClass
			}

			sym := ParsedSymbol{
				Name:       matches[nameIdx],
				Kind:       kind,
				ClassName:  className,
				StartLine:  lineNum + 1,
				EndLine:    lineNum + 1,
				Signature:  trimmed,
				Content:    trimmed,
				IsExported: !strings.HasPrefix(matches[nameIdx], "_"),
				Decorators: decorators,
			}

			if returnIdx >= 0 && matches[returnIdx] != "" {
				sym.Signature = fmt.Sprintf("%s(%s) -> %s", matches[nameIdx], matches[paramsIdx], matches[returnIdx])
			}

			result.Symbols = append(result.Symbols, sym)
			decorators = nil // Reset decorators
			continue
		}

		// Parse classes
		if matches := p.classRegex.FindStringSubmatch(trimmed); matches != nil {
			className := matches[1]
			heritage := ""
			if len(matches) > 2 {
				heritage = matches[2]
			}

			sym := ParsedSymbol{
				Name:       className,
				Kind:       graph.NodeClass,
				StartLine:  lineNum + 1,
				EndLine:    lineNum + 1,
				Signature:  trimmed,
				Content:    trimmed,
				IsExported: !strings.HasPrefix(className, "_"),
				Decorators: decorators,
			}
			result.Symbols = append(result.Symbols, sym)

			// Parse heritage
			if heritage != "" {
				h := ClassHeritage{
					ClassName: className,
				}
				bases := strings.Split(heritage, ",")
				for _, base := range bases {
					base = strings.TrimSpace(base)
					if strings.HasSuffix(base, "Mixin") || strings.HasSuffix(base, "Protocol") {
						h.Implements = append(h.Implements, base)
					} else {
						h.Extends = append(h.Extends, base)
					}
				}
				result.Heritage = append(result.Heritage, h)
			}

			currentClass = className
			decorators = nil
			continue
		}

		// Reset class context on dedent (simplified)
		if trimmed != "" && !strings.HasPrefix(trimmed, " ") && !strings.HasPrefix(trimmed, "\t") {
			if !strings.HasPrefix(trimmed, "def ") && !strings.HasPrefix(trimmed, "class ") {
				currentClass = ""
			}
		}

		// Parse imports
		if matches := p.importRegex.FindStringSubmatch(trimmed); matches != nil {
			imp := ImportStatement{
				StartLine: lineNum + 1,
			}

			if matches[1] != "" {
				// from X import Y
				imp.ModulePath = matches[1]
				imp.IsRelative = strings.HasPrefix(matches[1], ".")
				symbols := strings.Split(matches[2], ",")
				for _, s := range symbols {
					s = strings.TrimSpace(s)
					// Handle "X as Y" aliases
					if parts := strings.Split(s, " as "); len(parts) > 1 {
						s = strings.TrimSpace(parts[0])
					}
					if s != "" {
						imp.Symbols = append(imp.Symbols, s)
					}
				}
			} else {
				// import X
				parts := strings.Split(matches[2], ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if idx := strings.Index(part, " as "); idx > 0 {
						part = part[:idx]
					}
					imp.ModulePath = strings.TrimSpace(part)
				}
			}

			if imp.ModulePath != "" {
				result.Imports = append(result.Imports, imp)
			}
		}

		// Parse function calls (simplified)
		if strings.Contains(trimmed, "(") && !strings.HasPrefix(trimmed, "def ") && !strings.HasPrefix(trimmed, "class ") && !strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "from ") {
			calls := p.extractCalls(trimmed, lineNum+1)
			result.Calls = append(result.Calls, calls...)
		}

		// Parse type annotations (simplified)
		if strings.Contains(trimmed, ":") && (strings.HasPrefix(trimmed, "def ") || strings.Contains(trimmed, "->")) {
			typeRefs := p.extractTypeAnnotations(trimmed, lineNum+1)
			result.TypeRefs = append(result.TypeRefs, typeRefs...)
		}
	}

	return result, nil
}

func (p *PythonParser) extractCalls(line string, lineNum int) []CallSite {
	var calls []CallSite

	matches := p.callRegex.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		funcName := match[1]
		// Skip keywords
		if funcName == "def" || funcName == "class" || funcName == "if" || funcName == "for" || funcName == "while" || funcName == "with" || funcName == "except" {
			continue
		}

		call := CallSite{
			Name:      funcName,
			StartLine: lineNum,
			EndLine:   lineNum,
		}

		// Check for method call (obj.method())
		if idx := strings.LastIndex(line[:strings.Index(line, funcName)], "."); idx >= 0 {
			before := line[:idx]
			parts := strings.Fields(before)
			if len(parts) > 0 {
				call.Receiver = strings.TrimSpace(parts[len(parts)-1])
			}
		}

		calls = append(calls, call)
	}

	return calls
}

func (p *PythonParser) extractTypeAnnotations(line string, lineNum int) []TypeAnnotation {
	var refs []TypeAnnotation

	// Extract return type
	if idx := strings.Index(line, "->"); idx >= 0 {
		returnType := strings.TrimSpace(line[idx+2:])
		if idx := strings.Index(returnType, ":"); idx >= 0 {
			returnType = returnType[:idx]
		}
		returnType = strings.TrimSpace(returnType)
		if returnType != "" {
			refs = append(refs, TypeAnnotation{
				Name:      returnType,
				Role:      "return",
				StartLine: lineNum,
			})
		}
	}

	// Extract parameter types
	if idx := strings.Index(line, "("); idx >= 0 {
		endIdx := strings.Index(line, ")")
		if endIdx > idx {
			params := line[idx+1 : endIdx]
			scanner := bufio.NewScanner(strings.NewReader(params))
			scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
				if atEOF && len(data) == 0 {
					return 0, nil, nil
				}
				if idx := strings.IndexByte(string(data), ','); idx >= 0 {
					return idx + 1, data[0:idx], nil
				}
				if atEOF {
					return len(data), data, nil
				}
				return 0, nil, nil
			})
			for scanner.Scan() {
				param := strings.TrimSpace(scanner.Text())
				if colonIdx := strings.Index(param, ":"); colonIdx >= 0 {
					typeName := strings.TrimSpace(param[colonIdx+1:])
					if typeName != "" {
						refs = append(refs, TypeAnnotation{
							Name:      typeName,
							Role:      "param",
							StartLine: lineNum,
						})
					}
				}
			}
		}
	}

	return refs
}
