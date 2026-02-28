package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeLabelConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		label    NodeLabel
		expected string
	}{
		{"File", NodeFile, "file"},
		{"Folder", NodeFolder, "folder"},
		{"Function", NodeFunction, "function"},
		{"Class", NodeClass, "class"},
		{"Method", NodeMethod, "method"},
		{"Interface", NodeInterface, "interface"},
		{"TypeAlias", NodeTypeAlias, "type_alias"},
		{"Enum", NodeEnum, "enum"},
		{"Community", NodeCommunity, "community"},
		{"Process", NodeProcess, "process"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, string(tt.label))
		})
	}
}

func TestRelTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		relType  RelType
		expected string
	}{
		{"Contains", RelContains, "contains"},
		{"Defines", RelDefines, "defines"},
		{"Calls", RelCalls, "calls"},
		{"Imports", RelImports, "imports"},
		{"Extends", RelExtends, "extends"},
		{"Implements", RelImplements, "implements"},
		{"MemberOf", RelMemberOf, "member_of"},
		{"StepInProcess", RelStepInProcess, "step_in_process"},
		{"UsesType", RelUsesType, "uses_type"},
		{"Exports", RelExports, "exports"},
		{"CoupledWith", RelCoupledWith, "coupled_with"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, string(tt.relType))
		})
	}
}

func TestGenerateID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		label       NodeLabel
		filePath    string
		symbolName  string
		expected    string
		description string
	}{
		{
			name:        "function with symbol name",
			label:       NodeFunction,
			filePath:    "src/auth/validate.py",
			symbolName:  "validate_user",
			expected:    "function:src/auth/validate.py:validate_user",
			description: "should generate ID with label, path, and symbol",
		},
		{
			name:        "class with symbol name",
			label:       NodeClass,
			filePath:    "src/models/user.py",
			symbolName:  "User",
			expected:    "class:src/models/user.py:User",
			description: "should generate ID for class",
		},
		{
			name:        "method with symbol name",
			label:       NodeMethod,
			filePath:    "src/models/user.py",
			symbolName:  "User.save",
			expected:    "method:src/models/user.py:User.save",
			description: "should generate ID for method",
		},
		{
			name:        "file without symbol name",
			label:       NodeFile,
			filePath:    "src/main.py",
			symbolName:  "",
			expected:    "file:src/main.py",
			description: "should generate ID without symbol name for files",
		},
		{
			name:        "folder without symbol name",
			label:       NodeFolder,
			filePath:    "src/auth",
			symbolName:  "",
			expected:    "folder:src/auth",
			description: "should generate ID without symbol name for folders",
		},
		{
			name:        "empty symbol name",
			label:       NodeFunction,
			filePath:    "test.py",
			symbolName:  "",
			expected:    "function:test.py",
			description: "should handle empty symbol name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateID(tt.label, tt.filePath, tt.symbolName)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGraphNode(t *testing.T) {
	t.Parallel()

	t.Run("NewNode", func(t *testing.T) {
		t.Parallel()
		node := &GraphNode{
			ID:        "function:test.py:foo",
			Label:     NodeFunction,
			Name:      "foo",
			FilePath:  "test.py",
			StartLine: 10,
			EndLine:   20,
			Language:  "python",
		}

		assert.Equal(t, "function:test.py:foo", node.ID)
		assert.Equal(t, NodeFunction, node.Label)
		assert.Equal(t, "foo", node.Name)
		assert.Equal(t, "test.py", node.FilePath)
		assert.Equal(t, 10, node.StartLine)
		assert.Equal(t, 20, node.EndLine)
		assert.Equal(t, "python", node.Language)
		assert.False(t, node.IsDead)
		assert.False(t, node.IsEntryPoint)
		assert.False(t, node.IsExported)
		assert.Nil(t, node.Properties)
	})

	t.Run("NodeWithProperties", func(t *testing.T) {
		t.Parallel()
		node := &GraphNode{
			ID:         "class:test.py:User",
			Label:      NodeClass,
			Name:       "User",
			FilePath:   "test.py",
			Properties: map[string]any{"bases": []string{"object"}, "decorators": []string{"dataclass"}},
		}

		assert.Equal(t, "class:test.py:User", node.ID)
		assert.NotNil(t, node.Properties)
		assert.Equal(t, []string{"object"}, node.Properties["bases"])
	})

	t.Run("NodeWithContent", func(t *testing.T) {
		t.Parallel()
		content := "def foo():\n    pass"
		node := &GraphNode{
			ID:        "function:test.py:foo",
			Label:     NodeFunction,
			Name:      "foo",
			FilePath:  "test.py",
			Content:   content,
			Signature: "foo()",
		}

		assert.Equal(t, content, node.Content)
		assert.Equal(t, "foo()", node.Signature)
	})
}

func TestGraphRelationship(t *testing.T) {
	t.Parallel()

	t.Run("NewRelationship", func(t *testing.T) {
		t.Parallel()
		rel := &GraphRelationship{
			ID:     "calls:1",
			Type:   RelCalls,
			Source: "function:test.py:foo",
			Target: "function:bar.py:bar",
		}

		assert.Equal(t, "calls:1", rel.ID)
		assert.Equal(t, RelCalls, rel.Type)
		assert.Equal(t, "function:test.py:foo", rel.Source)
		assert.Equal(t, "function:bar.py:bar", rel.Target)
		assert.Nil(t, rel.Properties)
	})

	t.Run("RelationshipWithConfidence", func(t *testing.T) {
		t.Parallel()
		rel := &GraphRelationship{
			ID:         "calls:2",
			Type:       RelCalls,
			Source:     "function:test.py:foo",
			Target:     "function:bar.py:bar",
			Properties: map[string]any{"confidence": 0.9},
		}

		assert.Equal(t, 0.9, rel.Properties["confidence"])
	})

	t.Run("RelationshipWithRole", func(t *testing.T) {
		t.Parallel()
		rel := &GraphRelationship{
			ID:         "uses_type:1",
			Type:       RelUsesType,
			Source:     "function:test.py:foo",
			Target:     "class:types.py:User",
			Properties: map[string]any{"role": "return"},
		}

		assert.Equal(t, "return", rel.Properties["role"])
	})

	t.Run("RelationshipWithStepNumber", func(t *testing.T) {
		t.Parallel()
		rel := &GraphRelationship{
			ID:         "step_in_process:1",
			Type:       RelStepInProcess,
			Source:     "function:main.py:main",
			Target:     "process:main.py:main_process",
			Properties: map[string]any{"step_number": 1},
		}

		assert.Equal(t, 1, rel.Properties["step_number"])
	})

	t.Run("RelationshipWithCouplingStrength", func(t *testing.T) {
		t.Parallel()
		rel := &GraphRelationship{
			ID:         "coupled_with:1",
			Type:       RelCoupledWith,
			Source:     "file:user.py",
			Target:     "file:auth.py",
			Properties: map[string]any{"strength": 0.8, "co_changes": 15},
		}

		assert.Equal(t, 0.8, rel.Properties["strength"])
		assert.Equal(t, 15, rel.Properties["co_changes"])
	})
}

func TestGenerateID_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("EmptyFilePath", func(t *testing.T) {
		t.Parallel()
		result := GenerateID(NodeFunction, "", "foo")
		assert.Equal(t, "function::foo", result)
	})

	t.Run("SpecialCharactersInPath", func(t *testing.T) {
		t.Parallel()
		result := GenerateID(NodeFunction, "src/my-file_test.py", "foo")
		assert.Equal(t, "function:src/my-file_test.py:foo", result)
	})

	t.Run("NestedPath", func(t *testing.T) {
		t.Parallel()
		result := GenerateID(NodeFunction, "src/auth/validators/password.py", "validate")
		assert.Equal(t, "function:src/auth/validators/password.py:validate", result)
	})

	t.Run("SymbolWithDots", func(t *testing.T) {
		t.Parallel()
		result := GenerateID(NodeMethod, "src/user.py", "User.save")
		assert.Equal(t, "method:src/user.py:User.save", result)
	})
}
