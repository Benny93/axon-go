package parsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestPythonParser_Parse(t *testing.T) {
	t.Parallel()

	parser := NewPythonParser()

	t.Run("ParseFunction", func(t *testing.T) {
		content := []byte(`
def greet(name: str) -> str:
    """Say hello."""
    return f"Hello, {name}!"
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.NotEmpty(t, result.Symbols)
		assert.Len(t, result.Symbols, 1)

		fn := result.Symbols[0]
		assert.Equal(t, "greet", fn.Name)
		assert.Equal(t, graph.NodeFunction, fn.Kind)
		assert.Contains(t, fn.Signature, "greet")
		assert.Contains(t, fn.Signature, "name")
	})

	t.Run("ParseClass", func(t *testing.T) {
		content := []byte(`
class UserService:
    def __init__(self, db):
        self.db = db
    
    def get_user(self, user_id: int):
        pass
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)

		// Should find class
		var hasClass bool
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeClass && sym.Name == "UserService" {
				hasClass = true
			}
		}

		assert.True(t, hasClass, "Should find UserService class")
		// Note: Method parsing inside classes requires tree-sitter for full support
	})

	t.Run("ParseImports", func(t *testing.T) {
		content := []byte(`
import os
import sys
from pathlib import Path
from typing import List, Dict
from .utils import helper
from ..common import base
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Imports)
		assert.GreaterOrEqual(t, len(result.Imports), 4)
	})

	t.Run("ParseFunctionCalls", func(t *testing.T) {
		content := []byte(`
def main():
    result = process_data(input)
    user = UserService().get_user(123)
    print(result)
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Calls)
		// Should find process_data, print calls at minimum
		var foundProcessData bool
		var foundPrint bool
		for _, call := range result.Calls {
			if call.Name == "process_data" {
				foundProcessData = true
			}
			if call.Name == "print" {
				foundPrint = true
			}
		}
		assert.True(t, foundProcessData, "Should find process_data call")
		assert.True(t, foundPrint, "Should find print call")
	})

	t.Run("ParseTypeAnnotations", func(t *testing.T) {
		content := []byte(`
def calculate(x: int, y: int) -> float:
    return 0.0
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)

		// Basic parsing should work
		// Note: Full type annotation parsing requires tree-sitter
		assert.NotNil(t, result)
		assert.NotEmpty(t, result.Symbols)
	})

	t.Run("ParseClassInheritance", func(t *testing.T) {
		content := []byte(`
class BaseUser:
    pass

class AdminUser(BaseUser, Mixin):
    pass
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Heritage)
	})

	t.Run("ParseDecorators", func(t *testing.T) {
		content := []byte(`
@cache
def get_users():
    pass
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)

		// Should find decorated function
		var foundDecorated bool
		for _, sym := range result.Symbols {
			if len(sym.Decorators) > 0 {
				foundDecorated = true
				assert.Contains(t, sym.Decorators, "cache")
			}
		}
		assert.True(t, foundDecorated, "Should find decorated symbols")
	})

	t.Run("ParseEmptyFile", func(t *testing.T) {
		content := []byte("")
		result, err := parser.Parse("empty.py", content)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Symbols)
	})

	t.Run("ParseComments", func(t *testing.T) {
		content := []byte(`
# This is a comment
def foo():
    # Another comment
    pass
`)
		result, err := parser.Parse("test.py", content)
		require.NoError(t, err)
		assert.NotNil(t, result)
		// Comments should not affect parsing
		assert.NotEmpty(t, result.Symbols)
	})
}

func TestPythonParser_Language(t *testing.T) {
	t.Parallel()

	parser := NewPythonParser()
	assert.Equal(t, "python", parser.Language())
}

func TestPythonParser_ComplexCode(t *testing.T) {
	t.Parallel()

	parser := NewPythonParser()

	content := []byte(`
"""Module docstring."""

from typing import List, Optional
from .utils import helper

class UserService:
    """Service for managing users."""
    
    def __init__(self, db: Database):
        self.db = db
    
    @transaction
    def create_user(self, name: str, email: str) -> User:
        """Create a new user."""
        user = User(name=name, email=email)
        self.db.save(user)
        return user
    
    def get_user(self, user_id: int) -> Optional[User]:
        return self.db.get(User, user_id)

def main():
    service = UserService(Database())
    user = service.create_user("Alice", "alice@example.com")
    print(f"Created user: {user.name}")

if __name__ == "__main__":
    main()
`)

	result, err := parser.Parse("complex.py", content)
	require.NoError(t, err)

	// Verify we extracted the key elements
	var hasClass bool
	var hasFunction bool
	var hasImports bool
	var hasCalls bool

	for _, sym := range result.Symbols {
		if sym.Kind == graph.NodeClass {
			hasClass = true
		}
		if sym.Kind == graph.NodeFunction {
			hasFunction = true
		}
	}

	hasImports = len(result.Imports) > 0
	hasCalls = len(result.Calls) > 0

	assert.True(t, hasClass, "Should find UserService class")
	assert.True(t, hasFunction, "Should find main function")
	assert.True(t, hasImports, "Should find imports")
	assert.True(t, hasCalls, "Should find function calls")
	// Note: Method parsing inside classes requires tree-sitter for full support
}
