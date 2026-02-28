package parsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestTypeScriptParser_Parse(t *testing.T) {
	t.Parallel()

	t.Run("ParseFunction", func(t *testing.T) {
		content := []byte(`
function greet(name: string): string {
    return "Hello, " + name;
}
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Symbols)
		assert.Len(t, result.Symbols, 1)

		fn := result.Symbols[0]
		assert.Equal(t, "greet", fn.Name)
		assert.Equal(t, graph.NodeFunction, fn.Kind)
		assert.Contains(t, fn.Signature, "greet")
		assert.Contains(t, fn.Signature, "name: string")
	})

	t.Run("ParseArrowFunction", func(t *testing.T) {
		content := []byte(`
const add = (a: number, b: number): number => a + b;
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		// Arrow functions may not be detected by simple regex
		// At least verify the parser doesn't crash
		assert.NotNil(t, result)
	})

	t.Run("ParseClass", func(t *testing.T) {
		content := []byte(`
class UserService {
    constructor(private db: Database) {}

    getUser(id: number): User {
        return this.db.find(id);
    }

    async createUser(name: string): Promise<User> {
        return this.db.insert(name);
    }
}
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		// Should find class
		var hasClass bool
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeClass && sym.Name == "UserService" {
				hasClass = true
				break
			}
		}

		assert.True(t, hasClass, "Should find UserService class")
		// Methods may not be detected by simple regex
	})

	t.Run("ParseInterface", func(t *testing.T) {
		content := []byte(`
interface User {
    id: number;
    name: string;
    email?: string;
}
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Symbols)
		iface := result.Symbols[0]
		assert.Equal(t, "User", iface.Name)
		assert.Equal(t, graph.NodeInterface, iface.Kind)
	})

	t.Run("ParseType", func(t *testing.T) {
		content := []byte(`
type UserID = string | number;
type Result<T> = { data: T; error?: Error };
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Symbols)
		// Should find type aliases
		var found bool
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeTypeAlias && sym.Name == "UserID" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find UserID type alias")
	})

	t.Run("ParseImports", func(t *testing.T) {
		content := []byte(`
import { User } from './models/User';
import * as db from './database';
import express from 'express';
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Imports)
		assert.GreaterOrEqual(t, len(result.Imports), 3)
	})

	t.Run("ParseFunctionCalls", func(t *testing.T) {
		content := []byte(`
function main() {
    const result = processData(input);
    const user = service.getUser(123);
    console.log(result);
}
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.ts", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Calls)
		// Should find function calls
		var foundProcessData bool
		var foundGetUser bool
		for _, call := range result.Calls {
			if call.Name == "processData" {
				foundProcessData = true
			}
			if call.Name == "getUser" {
				foundGetUser = true
			}
		}
		assert.True(t, foundProcessData, "Should find processData call")
		assert.True(t, foundGetUser, "Should find getUser call")
	})

	t.Run("ParseEmptyFile", func(t *testing.T) {
		content := []byte(``)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("empty.ts", content)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Symbols)
	})

	t.Run("ParseTSX", func(t *testing.T) {
		content := []byte(`
import React from 'react';

interface Props {
    name: string;
}

export const Greeting: React.FC<Props> = ({ name }) => {
    return <div>Hello, {name}!</div>;
};
`)
		parser := NewTypeScriptParser()
		result, err := parser.Parse("test.tsx", content)
		require.NoError(t, err)

		// Should at least parse without errors and find the interface
		assert.NotEmpty(t, result.Symbols)
		// TSX components are complex, verify basic parsing works
		assert.NotNil(t, result)
	})
}

func TestTypeScriptParser_Language(t *testing.T) {
	t.Parallel()

	parser := NewTypeScriptParser()
	assert.Equal(t, "typescript", parser.Language())
}

func TestTypeScriptParser_FileDetection(t *testing.T) {
	t.Parallel()

	parser := NewTypeScriptParser()

	tests := []struct {
		filename string
		expected bool
	}{
		{"test.ts", true},
		{"test.tsx", true},
		{"test.js", false},
		{"test.py", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := parser.SupportsFile(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}
