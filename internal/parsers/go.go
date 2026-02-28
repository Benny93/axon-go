package parsers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// GoParser parses Go source code using the standard library's go/parser.
type GoParser struct{}

// NewGoParser creates a new Go parser.
func NewGoParser() *GoParser {
	return &GoParser{}
}

// Language returns the language this parser handles.
func (p *GoParser) Language() string {
	return "go"
}

// Parse parses Go source code and extracts symbols, imports, calls, etc.
func (p *GoParser) Parse(filePath string, content []byte) (*ParseResult, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing Go code: %w", err)
	}

	result := &ParseResult{
		Symbols:        []ParsedSymbol{},
		Imports:        []ImportStatement{},
		Calls:          []CallSite{},
		TypeRefs:       []TypeAnnotation{},
		Heritage:       []ClassHeritage{},
		Package:        file.Name.Name,
		PackageImports: make(map[string]string),
	}

	// Parse imports
	p.parseImports(file, result)

	// Parse declarations
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			p.parseFuncDecl(d, fset, content, result)
		case *ast.GenDecl:
			p.parseGenDecl(d, fset, content, result)
		}
	}

	// Parse function calls (walk AST)
	p.parseCalls(file, fset, content, result)

	return result, nil
}

func (p *GoParser) parseImports(file *ast.File, result *ParseResult) {
	for _, imp := range file.Imports {
		impStmt := ImportStatement{
			ModulePath: strings.Trim(imp.Path.Value, `"`),
		}

		if imp.Name != nil {
			impStmt.Alias = imp.Name.Name
			// Map alias to package path
			result.PackageImports[imp.Name.Name] = impStmt.ModulePath
		} else {
			// Use last part of path as implicit alias
			parts := strings.Split(impStmt.ModulePath, "/")
			alias := parts[len(parts)-1]
			result.PackageImports[alias] = impStmt.ModulePath
		}

		// Get line number
		if imp.Path != nil {
			impStmt.StartLine = int(imp.Path.Pos())
		}

		result.Imports = append(result.Imports, impStmt)
	}
}

func (p *GoParser) parseFuncDecl(fn *ast.FuncDecl, fset *token.FileSet, content []byte, result *ParseResult) {
	sym := ParsedSymbol{
		Name:       fn.Name.Name,
		StartLine:  fset.Position(fn.Pos()).Line,
		EndLine:    fset.Position(fn.End()).Line,
		IsExported: fn.Name.IsExported(),
	}

	// Determine if it's a function or method
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		// It's a method
		sym.Kind = graph.NodeMethod
		recv := fn.Recv.List[0]
		if typeName, ok := recv.Type.(*ast.StarExpr); ok {
			if ident, ok := typeName.X.(*ast.Ident); ok {
				sym.ClassName = ident.Name
			}
		} else if ident, ok := recv.Type.(*ast.Ident); ok {
			sym.ClassName = ident.Name
		}
	} else {
		// It's a function
		sym.Kind = graph.NodeFunction
	}

	// Build signature
	sig := p.buildSignature(fn, fset, content)
	sym.Signature = sig

	// Get content
	start := fset.Position(fn.Pos()).Offset
	end := fset.Position(fn.End()).Offset
	if start >= 0 && end <= len(content) {
		sym.Content = string(content[start:end])
	}

	result.Symbols = append(result.Symbols, sym)
}

func (p *GoParser) buildSignature(fn *ast.FuncDecl, fset *token.FileSet, content []byte) string {
	sig := fn.Name.Name

	// Add parameters
	params := []string{}
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			paramStr := p.nodeText(param, fset, content)
			params = append(params, paramStr)
		}
	}
	sig += "(" + strings.Join(params, ", ") + ")"

	// Add return types
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		returns := []string{}
		for _, ret := range fn.Type.Results.List {
			retStr := p.nodeText(ret, fset, content)
			returns = append(returns, retStr)
		}
		if len(returns) == 1 {
			sig += " " + returns[0]
		} else {
			sig += " (" + strings.Join(returns, ", ") + ")"
		}
	}

	return sig
}

func (p *GoParser) parseGenDecl(decl *ast.GenDecl, fset *token.FileSet, content []byte, result *ParseResult) {
	switch decl.Tok {
	case token.TYPE:
		for _, spec := range decl.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				p.parseTypeSpec(typeSpec, decl, fset, content, result)
			}
		}
	case token.VAR, token.CONST:
		// Could extract variables/constants if needed
	}
}

func (p *GoParser) parseTypeSpec(typeSpec *ast.TypeSpec, decl *ast.GenDecl, fset *token.FileSet, content []byte, result *ParseResult) {
	sym := ParsedSymbol{
		Name:       typeSpec.Name.Name,
		StartLine:  fset.Position(decl.Pos()).Line,
		EndLine:    fset.Position(decl.End()).Line,
		IsExported: typeSpec.Name.IsExported(),
	}

	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		sym.Kind = graph.NodeClass
		sym.Signature = "type " + typeSpec.Name.Name + " struct"
		// Extract fields as type references
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				fieldType := p.nodeText(field.Type, fset, content)
				result.TypeRefs = append(result.TypeRefs, TypeAnnotation{
					Name:      fieldType,
					Role:      "field",
					StartLine: fset.Position(field.Pos()).Line,
				})
			}
		}

	case *ast.InterfaceType:
		sym.Kind = graph.NodeInterface
		sym.Signature = "type " + typeSpec.Name.Name + " interface"
		// Extract methods
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if ident, ok := method.Type.(*ast.Ident); ok {
					result.TypeRefs = append(result.TypeRefs, TypeAnnotation{
						Name:      ident.Name,
						Role:      "method",
						StartLine: fset.Position(method.Pos()).Line,
					})
				}
			}
		}

	default:
		// Type alias or other type
		sym.Kind = graph.NodeTypeAlias
		typeStr := p.nodeText(t, fset, content)
		sym.Signature = "type " + typeSpec.Name.Name + " " + typeStr
	}

	// Get content
	start := fset.Position(decl.Pos()).Offset
	end := fset.Position(decl.End()).Offset
	if start >= 0 && end <= len(content) {
		sym.Content = string(content[start:end])
	}

	result.Symbols = append(result.Symbols, sym)
}

func (p *GoParser) parseCalls(file *ast.File, fset *token.FileSet, content []byte, result *ParseResult) {
	// Build receiver variable map for this file
	receiverMap := p.buildReceiverMap(file, fset)

	ast.Inspect(file, func(n ast.Node) bool {
		if callExpr, ok := n.(*ast.CallExpr); ok {
			call := p.extractCall(callExpr, fset, content, result, receiverMap)
			if call.Name != "" {
				result.Calls = append(result.Calls, call)
			}
		}
		return true
	})
}

// buildReceiverMap builds a map of receiver variable names to their type names.
func (p *GoParser) buildReceiverMap(file *ast.File, fset *token.FileSet) map[string]string {
	receiverMap := make(map[string]string)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}

		// Get receiver type name
		var typeName string
		recv := fn.Recv.List[0]
		if typeNameExpr, ok := recv.Type.(*ast.StarExpr); ok {
			if ident, ok := typeNameExpr.X.(*ast.Ident); ok {
				typeName = ident.Name
			}
		} else if ident, ok := recv.Type.(*ast.Ident); ok {
			typeName = ident.Name
		}

		if typeName == "" {
			continue
		}

		// Get receiver variable name
		if len(recv.Names) > 0 && recv.Names[0] != nil {
			receiverMap[recv.Names[0].Name] = typeName
		}
	}

	return receiverMap
}

func (p *GoParser) extractCall(callExpr *ast.CallExpr, fset *token.FileSet, content []byte, result *ParseResult, receiverMap map[string]string) CallSite {
	call := CallSite{
		StartLine: fset.Position(callExpr.Pos()).Line,
		EndLine:   fset.Position(callExpr.End()).Line,
	}

	switch fun := callExpr.Fun.(type) {
	case *ast.Ident:
		// Simple function call: foo()
		call.Name = fun.Name
	case *ast.SelectorExpr:
		// Method or package call: pkg.Method() or obj.Method()
		call.Name = fun.Sel.Name
		if xIdent, ok := fun.X.(*ast.Ident); ok {
			receiverVar := xIdent.Name
			call.Receiver = receiverVar

			// Check if this is a package-qualified call
			if pkgPath, ok := result.PackageImports[receiverVar]; ok {
				// This is a package.Function call
				call.Package = pkgPath
				call.Receiver = "" // Clear receiver since it's a package, not an object
			} else if typeName, ok := receiverMap[receiverVar]; ok {
				// This is a method call - replace variable name with type name
				call.Receiver = typeName
			}
			// Otherwise it's a method call on an object - keep receiver as is
		}
	case *ast.FuncLit:
		// Anonymous function
		call.Name = "func"
	}

	return call
}

func (p *GoParser) nodeText(n ast.Node, fset *token.FileSet, content []byte) string {
	if n == nil {
		return ""
	}
	start := fset.Position(n.Pos()).Offset
	end := fset.Position(n.End()).Offset
	if start >= 0 && end <= len(content) {
		return string(content[start:end])
	}
	return ""
}
