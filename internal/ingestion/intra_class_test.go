package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
	"github.com/Benny93/axon-go/internal/parsers"
)

func TestGoParser_ReceiverTypeTracking(t *testing.T) {
	t.Parallel()

	t.Run("TracksReceiverVariableType", func(t *testing.T) {
		content := []byte(`
package main

type Parser struct{}

func (p *Parser) parseFunc() {
	p.helper()
}

func (p *Parser) helper() {}
`)
		parser := parsers.NewGoParser()
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		// Should have methods (may also have type alias)
		var methods []parsers.ParsedSymbol
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeMethod {
				methods = append(methods, sym)
			}
		}
		assert.Len(t, methods, 2, "Should have 2 methods")

		// Check that methods have ClassName set
		for _, sym := range methods {
			assert.Equal(t, "Parser", sym.ClassName, "Method should have ClassName set")
		}

		// Should have calls with receiver info
		assert.NotEmpty(t, result.Calls)

		// Find the helper call - should have receiver type (class name, not variable name)
		var foundHelperCall bool
		for _, call := range result.Calls {
			if call.Name == "helper" {
				foundHelperCall = true
				// Receiver should be the class name, not variable name
				assert.Equal(t, "Parser", call.Receiver, "Receiver should be class name, not variable name")
			}
		}
		assert.True(t, foundHelperCall, "Should find helper call")
	})

	t.Run("TracksMultipleReceiverVariables", func(t *testing.T) {
		content := []byte(`
package main

type Service struct{}

func (s *Service) Process() {
	s.validate()
	s.execute()
}

func (s *Service) validate() {}
func (s *Service) execute() {}
`)
		parser := parsers.NewGoParser()
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		// Should have 3 methods
		methodCount := 0
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeMethod {
				methodCount++
				assert.Equal(t, "Service", sym.ClassName)
			}
		}
		assert.Equal(t, 3, methodCount)

		// Should find calls to validate and execute with class name as receiver
		var foundValidate, foundExecute bool
		for _, call := range result.Calls {
			if call.Name == "validate" {
				foundValidate = true
				assert.Equal(t, "Service", call.Receiver, "Receiver should be class name")
			}
			if call.Name == "execute" {
				foundExecute = true
				assert.Equal(t, "Service", call.Receiver, "Receiver should be class name")
			}
		}
		assert.True(t, foundValidate, "Should find validate call")
		assert.True(t, foundExecute, "Should find execute call")
	})

	t.Run("HandlesDifferentReceiverNames", func(t *testing.T) {
		content := []byte(`
package main

type Handler struct{}

func (h *Handler) Serve() {
	h.process()
}

func (handler *Handler) process() {}
`)
		parser := parsers.NewGoParser()
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		// Both methods should have same ClassName despite different receiver names
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeMethod {
				assert.Equal(t, "Handler", sym.ClassName)
			}
		}

		// Call should have class name as receiver
		for _, call := range result.Calls {
			if call.Name == "process" {
				assert.Equal(t, "Handler", call.Receiver, "Receiver should be class name")
			}
		}
	})

	t.Run("DistinguishesPackageCallsFromMethodCalls", func(t *testing.T) {
		content := []byte(`
package main

import "fmt"

type Parser struct{}

func (p *Parser) Parse() {
	p.helper()
	fmt.Println("done")
}

func (p *Parser) helper() {}
`)
		parser := parsers.NewGoParser()
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		// Should find both method call and package call
		var foundHelper, foundPrintln bool
		for _, call := range result.Calls {
			if call.Name == "helper" {
				foundHelper = true
				assert.Equal(t, "Parser", call.Receiver, "Method call should have class name")
				assert.Empty(t, call.Package, "Method call should not have package")
			}
			if call.Name == "Println" {
				foundPrintln = true
				assert.Empty(t, call.Receiver, "Package call should not have receiver")
				assert.Equal(t, "fmt", call.Package, "Package call should have package")
			}
		}
		assert.True(t, foundHelper, "Should find helper call")
		assert.True(t, foundPrintln, "Should find Println call")
	})
}

func TestFindSymbolTarget_MethodCalls(t *testing.T) {
	t.Parallel()

	t.Run("FindsMethodByReceiverAndName", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add method nodes
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:Parser.parseFunc",
			Label:     graph.NodeMethod,
			Name:      "parseFunc",
			FilePath:  "test.go",
			ClassName: "Parser",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:Parser.helper",
			Label:     graph.NodeMethod,
			Name:      "helper",
			FilePath:  "test.go",
			ClassName: "Parser",
		})

		// Test findSymbolTarget with receiver (class name)
		targetID := FindSymbolTargetForTest(g, "helper", "Parser", "", "test.go")

		assert.Equal(t, "function:test.go:Parser.helper", targetID)
	})

	t.Run("FindsMethodByClassName", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:Service.validate",
			Label:     graph.NodeMethod,
			Name:      "validate",
			FilePath:  "test.go",
			ClassName: "Service",
		})

		// Should find method by ClassName even if receiver variable name differs
		targetID := FindSymbolTargetForTest(g, "validate", "Service", "", "test.go")

		assert.Equal(t, "function:test.go:Service.validate", targetID)
	})

	t.Run("PrefersClassNameMatchOverNameOnly", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add methods with same name but different classes
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:Parser.validate",
			Label:     graph.NodeMethod,
			Name:      "validate",
			FilePath:  "test.go",
			ClassName: "Parser",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:Service.validate",
			Label:     graph.NodeMethod,
			Name:      "validate",
			FilePath:  "test.go",
			ClassName: "Service",
		})

		// Should find Service.validate when receiver is Service
		targetID := FindSymbolTargetForTest(g, "validate", "Service", "", "test.go")

		assert.Equal(t, "function:test.go:Service.validate", targetID)
	})
}

func TestProcessCalls_IntraClassTracking(t *testing.T) {
	t.Parallel()

	t.Run("CreatesCallsRelationshipsForIntraClassCalls", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		// Add method nodes with correct IDs (matching graph.GenerateID format)
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:parseFunc",
			Label:     graph.NodeMethod,
			Name:      "parseFunc",
			FilePath:  "test.go",
			ClassName: "Parser",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:helper",
			Label:     graph.NodeMethod,
			Name:      "helper",
			FilePath:  "test.go",
			ClassName: "Parser",
		})

		// Create parse data with intra-class call (receiver is class name)
		parseData := NewParseData()
		parseData.AddFile("test.go", &parsers.ParseResult{
			Symbols: []parsers.ParsedSymbol{
				{Name: "parseFunc", Kind: graph.NodeMethod, ClassName: "Parser"},
				{Name: "helper", Kind: graph.NodeMethod, ClassName: "Parser"},
			},
			Calls: []parsers.CallSite{
				{Name: "helper", Receiver: "Parser"}, // Receiver is class name
			},
		})

		// Process calls
		ProcessCalls(parseData, g)

		// Should have CALLS relationship
		rels := g.GetRelationshipsByType(graph.RelCalls)
		assert.NotEmpty(t, rels, "Should create CALLS relationship for intra-class call")

		// Verify the relationship has correct target (helper method found by ClassName)
		found := false
		for _, rel := range rels {
			if rel.Target == "function:test.go:helper" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should have CALLS relationship targeting helper method")
	})

	t.Run("HandlesReceiverVariableNameResolvedToClassName", func(t *testing.T) {
		g := graph.NewKnowledgeGraph()

		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:process",
			Label:     graph.NodeMethod,
			Name:      "process",
			FilePath:  "test.go",
			ClassName: "Service",
		})
		g.AddNode(&graph.GraphNode{
			ID:        "function:test.go:execute",
			Label:     graph.NodeMethod,
			Name:      "execute",
			FilePath:  "test.go",
			ClassName: "Service",
		})

		// Parser now resolves receiver variable to class name
		parseData := NewParseData()
		parseData.AddFile("test.go", &parsers.ParseResult{
			Symbols: []parsers.ParsedSymbol{
				{Name: "process", Kind: graph.NodeMethod, ClassName: "Service"},
				{Name: "execute", Kind: graph.NodeMethod, ClassName: "Service"},
			},
			Calls: []parsers.CallSite{
				{Name: "execute", Receiver: "Service"}, // Parser resolved 's' to 'Service'
			},
		})

		// Process calls should work
		ProcessCalls(parseData, g)

		// Should find the call by matching ClassName
		rels := g.GetRelationshipsByType(graph.RelCalls)
		assert.NotEmpty(t, rels, "Should create CALLS relationship")

		// Verify correct target
		found := false
		for _, rel := range rels {
			if rel.Target == "function:test.go:execute" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should have CALLS relationship targeting execute method")
	})
}
