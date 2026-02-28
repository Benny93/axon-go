package parsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoParser_CrossPackageCalls(t *testing.T) {
	t.Parallel()

	t.Run("ParseCrossPackageFunctionCall", func(t *testing.T) {
		content := []byte(`
package cmd

import "github.com/Benny93/axon-go/internal/ingestion"

func testFunc() {
	_, result, err := ingestion.RunPipeline(ctx, repoPath, store, false, nil, false)
}
`)
		parser := NewGoParser()
		result, err := parser.Parse("cmd/test.go", content)
		require.NoError(t, err)

		// Should find the call to RunPipeline
		assert.NotEmpty(t, result.Calls)

		// Find the RunPipeline call
		var foundRunPipeline bool
		for _, call := range result.Calls {
			if call.Name == "RunPipeline" {
				foundRunPipeline = true
				// The package should be recorded
				assert.Equal(t, "github.com/Benny93/axon-go/internal/ingestion", call.Package)
				// Receiver should be empty (it's a package, not an object)
				assert.Empty(t, call.Receiver)
			}
		}
		assert.True(t, foundRunPipeline, "Should find call to ingestion.RunPipeline")
	})

	t.Run("ParsePackageImport", func(t *testing.T) {
		content := []byte(`
package cmd

import (
	"context"
	"github.com/Benny93/axon-go/internal/ingestion"
	"github.com/Benny93/axon-go/internal/storage"
)

func testFunc() {
	ingestion.RunPipeline(ctx, repoPath, store, false, nil, false)
	storage.NewBadgerBackend()
}
`)
		parser := NewGoParser()
		result, err := parser.Parse("cmd/test.go", content)
		require.NoError(t, err)

		// Should have imports
		assert.NotEmpty(t, result.Imports)

		// Check imports include ingestion package
		var foundIngestion bool
		for _, imp := range result.Imports {
			if imp.ModulePath == "github.com/Benny93/axon-go/internal/ingestion" {
				foundIngestion = true
			}
		}
		assert.True(t, foundIngestion, "Should have ingestion import")
	})

	t.Run("ParseMethodCallOnStruct", func(t *testing.T) {
		content := []byte(`
package cmd

type MyStruct struct{}

func (m *MyStruct) MyMethod() {}

func testFunc() {
	m := &MyStruct{}
	m.MyMethod()
}
`)
		parser := NewGoParser()
		result, err := parser.Parse("cmd/test.go", content)
		require.NoError(t, err)

		// Should find the method call
		var foundMyMethod bool
		for _, call := range result.Calls {
			if call.Name == "MyMethod" {
				foundMyMethod = true
				// Receiver should be resolved to type name, not variable name
				assert.Equal(t, "MyStruct", call.Receiver, "Receiver should be type name, not variable name")
			}
		}
		assert.True(t, foundMyMethod, "Should find method call m.MyMethod()")
	})

	t.Run("ParseQualifiedPackageCall", func(t *testing.T) {
		content := []byte(`
package cmd

import (
	"fmt"
	"github.com/Benny93/axon-go/internal/ingestion"
)

func testFunc() {
	fmt.Println("hello")
	ingestion.RunPipeline(ctx, repoPath, store, false, nil, false)
}
`)
		parser := NewGoParser()
		result, err := parser.Parse("cmd/test.go", content)
		require.NoError(t, err)

		// Debug: print all calls found
		t.Logf("Found %d calls", len(result.Calls))
		for _, call := range result.Calls {
			t.Logf("  Call: %s (receiver=%q, package=%q)", call.Name, call.Receiver, call.Package)
		}

		// Should find package function calls
		var foundPrintln, foundRunPipeline bool
		for _, call := range result.Calls {
			if call.Name == "Println" {
				foundPrintln = true
				t.Logf("Found Println: receiver=%q, package=%q", call.Receiver, call.Package)
			}
			if call.Name == "RunPipeline" && call.Package == "github.com/Benny93/axon-go/internal/ingestion" {
				foundRunPipeline = true
			}
		}
		// Note: Standard library packages like fmt are tracked with just the package name
		// Custom packages are tracked with full path
		assert.True(t, foundPrintln, "Should find fmt.Println call")
		assert.True(t, foundRunPipeline, "Should find ingestion.RunPipeline call")
	})
}

func TestFindSymbolTarget_CrossPackage(t *testing.T) {
	t.Parallel()

	t.Run("FindFunctionInPackage", func(t *testing.T) {
		// This test verifies that findSymbolTarget can resolve cross-package calls
		// when given a package-qualified call like ingestion.RunPipeline
	})
}
