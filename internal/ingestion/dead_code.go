package ingestion

import (
	"strings"

	"github.com/Benny93/axon-go/internal/graph"
)

// ProcessDeadCode performs multi-pass dead code detection.
// Returns the count of dead code symbols found.
//
// Dead code detection phases:
// 1. Call pattern detection - identify dynamic dispatch, framework patterns
// 2. Initial scan - flag symbols with no incoming CALLS
// 3. Intra-class call tracking - un-flag methods called within same class
// 4. Exemptions - un-flag entry points, exports, test code, dunder methods
// 5. Allowlist exemptions - un-flag framework handlers, CLI commands
// 6. Override pass - un-flag methods overriding non-dead base class methods
// 7. Confidence scoring - assign confidence levels to remaining dead code
func ProcessDeadCode(g *graph.KnowledgeGraph) int {
	// Phase 1: Detect call patterns (dynamic dispatch, framework patterns)
	detectCallPatterns(g)

	// Phase 2: Initial scan - flag all symbols with no incoming calls
	flagUnreachable(g)

	// Phase 3: Intra-class call tracking
	trackIntraClassCalls(g)

	// Phase 4: Apply exemptions
	applyExemptions(g)

	// Phase 5: Apply allowlist exemptions
	applyAllowlistExemptions(g)

	// Phase 6: Override pass
	applyOverridePass(g)

	// Phase 7: Assign confidence scores
	assignConfidenceScores(g)

	// Count dead code
	count := 0
	for node := range g.IterNodes() {
		if node.IsDead {
			count++
		}
	}

	return count
}

// flagUnreachable marks all symbols with no incoming CALLS as potentially dead.
func flagUnreachable(g *graph.KnowledgeGraph) {
	for node := range g.IterNodes() {
		// Skip non-symbol nodes
		if node.Label == graph.NodeFile || node.Label == graph.NodeFolder {
			continue
		}

		// Skip structural nodes (communities, processes) - they're not callable
		if node.Label == graph.NodeCommunity || node.Label == graph.NodeProcess {
			continue
		}

		// Check if node has any incoming CALLS relationships
		hasIncomingCalls := g.HasIncoming(node.ID, graph.RelCalls)

		if !hasIncomingCalls {
			node.IsDead = true
		}
	}
}

// applyExemptions removes dead code flags from exempt symbols.
func applyExemptions(g *graph.KnowledgeGraph) {
	for node := range g.IterNodes() {
		if !node.IsDead {
			continue
		}

		if isDeadCodeExempt(node) {
			node.IsDead = false
		}
	}
}

// isDeadCodeExempt checks if a symbol is exempt from dead code detection.
func isDeadCodeExempt(node *graph.GraphNode) bool {
	// Entry points are never dead
	if node.IsEntryPoint {
		return true
	}

	// Exported symbols are never dead (may be used externally)
	if node.IsExported {
		return true
	}

	// Test functions are never dead
	if isTestFunction(node) {
		return true
	}

	// Dunder methods (Python) are never dead
	if isDunderMethod(node) {
		return true
	}

	// Constructors are never dead
	if isConstructor(node) {
		return true
	}

	// Structural nodes (communities, processes) are never dead
	if node.Label == graph.NodeCommunity || node.Label == graph.NodeProcess {
		return true
	}

	return false
}

// isTestFunction checks if a function is a test function.
func isTestFunction(node *graph.GraphNode) bool {
	// Check if file is a test file
	if strings.HasSuffix(node.FilePath, "_test.go") ||
		strings.HasSuffix(node.FilePath, "_test.py") {
		return true
	}

	// Check if function name starts with Test (Go) or test_ (Python)
	if node.Label == graph.NodeFunction {
		if strings.HasPrefix(node.Name, "Test") ||
			strings.HasPrefix(node.Name, "test_") {
			return true
		}
	}

	return false
}

// isDunderMethod checks if a method is a Python dunder method.
func isDunderMethod(node *graph.GraphNode) bool {
	// Check for Python dunder methods
	if strings.HasPrefix(node.Name, "__") && strings.HasSuffix(node.Name, "__") {
		return true
	}

	// Common dunder methods
	dunderMethods := []string{
		"__init__", "__new__", "__del__",
		"__repr__", "__str__",
		"__lt__", "__le__", "__eq__", "__ne__", "__gt__", "__ge__",
		"__hash__",
		"__bool__", "__len__",
	}

	for _, method := range dunderMethods {
		if node.Name == method {
			return true
		}
	}

	return false
}

// isConstructor checks if a method is a constructor.
func isConstructor(node *graph.GraphNode) bool {
	if node.Label != graph.NodeMethod {
		return false
	}

	// Go constructors
	if node.Name == "New"+node.ClassName {
		return true
	}

	// Python constructors
	if node.Name == "__init__" {
		return true
	}

	return false
}

// applyOverridePass removes dead code flags from methods that override
// non-dead base class methods.
func applyOverridePass(g *graph.KnowledgeGraph) {
	for node := range g.IterNodes() {
		if !node.IsDead {
			continue
		}

		if node.Label != graph.NodeMethod {
			continue
		}

		// Check if this method overrides a base class method
		if isOverrideOfNonDeadMethod(g, node) {
			node.IsDead = false
		}
	}
}

// isOverrideOfNonDeadMethod checks if a method overrides a non-dead base class method.
func isOverrideOfNonDeadMethod(g *graph.KnowledgeGraph, method *graph.GraphNode) bool {
	if method.ClassName == "" {
		return false
	}

	// Find the class this method belongs to
	classNode := findClassByName(g, method.ClassName)
	if classNode == nil {
		return false
	}

	// Find base classes
	baseClasses := findBaseClasses(g, classNode)

	// Check if any base class has a non-dead method with the same name
	for _, baseClass := range baseClasses {
		baseMethod := findMethodInClass(g, baseClass, method.Name)
		if baseMethod != nil && !baseMethod.IsDead {
			return true
		}
	}

	return false
}

// findClassByName finds a class node by name.
func findClassByName(g *graph.KnowledgeGraph, className string) *graph.GraphNode {
	for node := range g.IterNodes() {
		if node.Label == graph.NodeClass && node.Name == className {
			return node
		}
	}
	return nil
}

// findBaseClasses finds all base classes of a class.
func findBaseClasses(g *graph.KnowledgeGraph, class *graph.GraphNode) []*graph.GraphNode {
	var baseClasses []*graph.GraphNode

	rels := g.GetOutgoing(class.ID, graph.RelExtends)
	for _, rel := range rels {
		baseClass := g.GetNode(rel.Target)
		if baseClass != nil && baseClass.Label == graph.NodeClass {
			baseClasses = append(baseClasses, baseClass)
		}
	}

	return baseClasses
}

// findMethodInClass finds a method with the given name in a class.
func findMethodInClass(g *graph.KnowledgeGraph, class *graph.GraphNode, methodName string) *graph.GraphNode {
	for node := range g.IterNodes() {
		if node.Label == graph.NodeMethod &&
			node.ClassName == class.Name &&
			node.Name == methodName {
			return node
		}
	}
	return nil
}

// detectCallPatterns identifies dynamic dispatch and framework patterns.
func detectCallPatterns(g *graph.KnowledgeGraph) {
	for caller := range g.IterNodes() {
		// Check if caller has dynamic dispatch pattern
		if hasDynamicDispatch(caller) {
			// Mark all callees as dynamic dispatch
			rels := g.GetOutgoing(caller.ID, graph.RelCalls)
			for _, rel := range rels {
				callee := g.GetNode(rel.Target)
				if callee != nil {
					if callee.Properties == nil {
						callee.Properties = make(map[string]any)
					}
					callee.Properties["call_pattern"] = "dynamic_dispatch"
				}
			}
		}

		// Check if caller has switch-case dispatch (framework pattern)
		if hasSwitchDispatch(caller) {
			rels := g.GetOutgoing(caller.ID, graph.RelCalls)
			for _, rel := range rels {
				callee := g.GetNode(rel.Target)
				if callee != nil {
					if callee.Properties == nil {
						callee.Properties = make(map[string]any)
					}
					callee.Properties["call_pattern"] = "framework_dispatch"
				}
			}
		}
	}
}

// hasDynamicDispatch checks if a node uses dynamic dispatch (e.g., MCP handlers).
func hasDynamicDispatch(node *graph.GraphNode) bool {
	// Check if node has dynamic dispatch indicator
	if val, ok := node.Properties["has_dynamic_dispatch"]; ok {
		if b, ok := val.(bool); ok && b {
			return true
		}
	}

	// Check for MCP handler patterns
	if strings.Contains(node.FilePath, "server.go") &&
		strings.Contains(node.Name, "handle") {
		return true
	}

	return false
}

// hasSwitchDispatch checks if a node uses switch-case dispatch (CLI patterns).
func hasSwitchDispatch(node *graph.GraphNode) bool {
	// Check if node has switch dispatch indicator
	if val, ok := node.Properties["has_switch_dispatch"]; ok {
		if b, ok := val.(bool); ok && b {
			return true
		}
	}

	// Check for CLI command patterns
	if strings.Contains(node.FilePath, "cmd.go") &&
		(node.Name == "Run" || strings.HasSuffix(node.Name, "Cmd")) {
		return true
	}

	return false
}

// trackIntraClassCalls un-flags methods that are called within the same class or file.
func trackIntraClassCalls(g *graph.KnowledgeGraph) {
	// Build a map of class methods
	classMethods := make(map[string]map[string]*graph.GraphNode)
	for node := range g.IterNodes() {
		if node.Label == graph.NodeMethod && node.ClassName != "" {
			if classMethods[node.ClassName] == nil {
				classMethods[node.ClassName] = make(map[string]*graph.GraphNode)
			}
			classMethods[node.ClassName][node.Name] = node
		}
	}

	// Build a map of methods by file for intra-file call tracking
	fileMethods := make(map[string]map[string]*graph.GraphNode)
	for node := range g.IterNodes() {
		if node.Label == graph.NodeMethod {
			if fileMethods[node.FilePath] == nil {
				fileMethods[node.FilePath] = make(map[string]*graph.GraphNode)
			}
			fileMethods[node.FilePath][node.Name] = node
		}
	}

	// Find all method calls within classes
	for node := range g.IterNodes() {
		if node.Label == graph.NodeMethod && node.ClassName != "" {
			// Get all methods this method calls
			rels := g.GetOutgoing(node.ID, graph.RelCalls)
			for _, rel := range rels {
				callee := g.GetNode(rel.Target)
				if callee != nil && callee.Label == graph.NodeMethod {
					// Check if callee is in the same class
					if callee.ClassName == node.ClassName {
						// Called within same class - un-flag as dead
						callee.IsDead = false
						if callee.Properties == nil {
							callee.Properties = make(map[string]any)
						}
						callee.Properties["call_pattern"] = "intra_class"
					}
				}
			}
		}
	}

	// Also track intra-file method calls (for cases where ClassName isn't set or receiver tracking fails)
	for filePath, methods := range fileMethods {
		// Get all method nodes in this file
		methodNodes := make(map[string]*graph.GraphNode)
		for _, method := range methods {
			methodNodes[method.Name] = method
		}

		// Check each method for calls to other methods in the same file
		for _, caller := range methodNodes {
			rels := g.GetOutgoing(caller.ID, graph.RelCalls)
			for _, rel := range rels {
				callee := g.GetNode(rel.Target)
				if callee != nil && callee.Label == graph.NodeMethod {
					// Check if callee is in the same file
					if callee.FilePath == filePath {
						// Called within same file - un-flag as dead
						callee.IsDead = false
						if callee.Properties == nil {
							callee.Properties = make(map[string]any)
						}
						callee.Properties["call_pattern"] = "intra_file"
					}
				}
			}
		}
	}
}

// applyAllowlistExemptions un-flags methods matching known framework patterns.
func applyAllowlistExemptions(g *graph.KnowledgeGraph) {
	exemptCount := 0
	for node := range g.IterNodes() {
		if !node.IsDead {
			continue
		}

		if isAllowlistExempt(node) {
			node.IsDead = false
			if node.Properties == nil {
				node.Properties = make(map[string]any)
			}
			node.Properties["dead_code_exempt"] = true
			node.Properties["exempt_reason"] = "framework_pattern"
			exemptCount++
		}
	}
	_ = exemptCount // Could add debug logging
}

// isAllowlistExempt checks if a node matches an allowlisted pattern.
func isAllowlistExempt(node *graph.GraphNode) bool {
	// MCP handler methods
	if strings.Contains(node.FilePath, "server.go") &&
		strings.HasPrefix(node.Name, "handle") {
		return true
	}

	// CLI setup/configure methods
	if strings.Contains(node.FilePath, "cmd.go") &&
		(strings.HasPrefix(node.Name, "setup") ||
			strings.HasPrefix(node.Name, "configure") ||
			strings.HasPrefix(node.Name, "output")) {
		return true
	}

	// Register methods
	if strings.HasPrefix(node.Name, "register") {
		return true
	}

	// Methods with dynamic dispatch pattern
	if val, ok := node.Properties["call_pattern"]; ok {
		if pattern, ok := val.(string); ok {
			if pattern == "dynamic_dispatch" || pattern == "framework_dispatch" ||
				pattern == "mcp_handler" || pattern == "cli_subcommand" {
				return true
			}
		}
	}

	return false
}

// assignConfidenceScores assigns confidence levels to dead code flags.
func assignConfidenceScores(g *graph.KnowledgeGraph) {
	for node := range g.IterNodes() {
		if !node.IsDead {
			continue
		}

		if node.Properties == nil {
			node.Properties = make(map[string]any)
		}

		confidence := "high"

		// Low confidence: has callers but they're dynamic
		if val, ok := node.Properties["call_pattern"]; ok {
			if pattern, ok := val.(string); ok {
				if pattern == "dynamic_dispatch" || pattern == "framework_dispatch" {
					confidence = "low"
				}
			}
		}

		// Medium confidence: intra-class calls only
		if val, ok := node.Properties["call_pattern"]; ok {
			if pattern, ok := val.(string); ok && pattern == "intra_class" {
				confidence = "medium"
			}
		}

		// Medium confidence: test code
		if isTestFunction(node) || strings.Contains(node.FilePath, "_test") {
			confidence = "medium"
		}

		// Medium confidence: private methods (lowercase first letter in Go)
		if node.Label == graph.NodeMethod || node.Label == graph.NodeFunction {
			if len(node.Name) > 0 && node.Name[0] >= 'a' && node.Name[0] <= 'z' {
				if confidence == "high" {
					confidence = "medium"
				}
			}
		}

		node.Properties["dead_code_confidence"] = confidence
	}
}

// GetDeadCodeList returns all nodes marked as dead code, grouped by file.
func GetDeadCodeList(g *graph.KnowledgeGraph) []*graph.GraphNode {
	var deadCode []*graph.GraphNode

	for node := range g.IterNodes() {
		if node.IsDead {
			deadCode = append(deadCode, node)
		}
	}

	return deadCode
}
