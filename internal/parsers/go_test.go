package parsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Benny93/axon-go/internal/graph"
)

func TestGoParser_Parse(t *testing.T) {
	t.Parallel()

	parser := NewGoParser()

	t.Run("ParseFunction", func(t *testing.T) {
		content := []byte(`
package main

func greet(name string) string {
	return "Hello, " + name
}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.NotEmpty(t, result.Symbols)
		assert.Len(t, result.Symbols, 1)

		fn := result.Symbols[0]
		assert.Equal(t, "greet", fn.Name)
		assert.Equal(t, graph.NodeFunction, fn.Kind)
		assert.Contains(t, fn.Signature, "greet")
		assert.Equal(t, "main", result.Package)
	})

	t.Run("ParseFunctionWithMultipleParams", func(t *testing.T) {
		content := []byte(`
package main

func add(x int, y int) int {
	return x + y
}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Symbols)
		fn := result.Symbols[0]
		assert.Equal(t, "add", fn.Name)
		assert.Contains(t, fn.Signature, "x int")
		assert.Contains(t, fn.Signature, "y int")
	})

	t.Run("ParseMethod", func(t *testing.T) {
		content := []byte(`
package main

type UserService struct {
	db *Database
}

func (s *UserService) GetUser(id int) *User {
	return nil
}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		// Should find struct and method
		var hasStruct bool
		var hasMethod bool
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeClass && sym.Name == "UserService" {
				hasStruct = true
			}
			if sym.Kind == graph.NodeMethod && sym.Name == "GetUser" {
				hasMethod = true
				assert.Equal(t, "UserService", sym.ClassName)
			}
		}

		assert.True(t, hasStruct, "Should find UserService struct")
		assert.True(t, hasMethod, "Should find GetUser method")
	})

	t.Run("ParseInterface", func(t *testing.T) {
		content := []byte(`
package main

type Reader interface {
	Read(p []byte) (n int, err error)
}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		var hasInterface bool
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeInterface && sym.Name == "Reader" {
				hasInterface = true
			}
		}

		assert.True(t, hasInterface, "Should find Reader interface")
	})

	t.Run("ParseTypeAlias", func(t *testing.T) {
		content := []byte(`
package main

type UserID string
type Count int
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		var hasAlias bool
		for _, sym := range result.Symbols {
			if sym.Kind == graph.NodeTypeAlias && sym.Name == "UserID" {
				hasAlias = true
			}
		}

		assert.True(t, hasAlias, "Should find UserID type alias")
	})

	t.Run("ParseImports", func(t *testing.T) {
		content := []byte(`
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"github.com/pkg/errors"
)
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Imports)
		assert.GreaterOrEqual(t, len(result.Imports), 4)
	})

	t.Run("ParseFunctionCalls", func(t *testing.T) {
		content := []byte(`
package main

func main() {
	fmt.Println("Hello")
	result := processData(input)
	os.Exit(0)
}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Calls)
		// Should find function calls
		var foundPrintln bool
		var foundProcessData bool
		for _, call := range result.Calls {
			if call.Name == "Println" {
				foundPrintln = true
			}
			if call.Name == "processData" {
				foundProcessData = true
			}
		}
		assert.True(t, foundPrintln, "Should find Println call")
		assert.True(t, foundProcessData, "Should find processData call")
	})

	t.Run("ParseEmptyFile", func(t *testing.T) {
		content := []byte(`package main`)
		result, err := parser.Parse("empty.go", content)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("ParsePackageDeclaration", func(t *testing.T) {
		content := []byte(`
package mypackage

func foo() {}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)
		assert.Equal(t, "mypackage", result.Package)
	})

	t.Run("ParseExportedVsUnexported", func(t *testing.T) {
		content := []byte(`
package main

func PublicFunc() {}
func privateFunc() {}

type PublicStruct struct {}
type privateStruct struct {}
`)
		result, err := parser.Parse("test.go", content)
		require.NoError(t, err)

		var publicFunc, privateFunc, publicStruct, privateStruct bool
		for _, sym := range result.Symbols {
			switch sym.Name {
			case "PublicFunc":
				publicFunc = true
				assert.True(t, sym.IsExported)
			case "privateFunc":
				privateFunc = true
				assert.False(t, sym.IsExported)
			case "PublicStruct":
				publicStruct = true
				assert.True(t, sym.IsExported)
			case "privateStruct":
				privateStruct = true
				assert.False(t, sym.IsExported)
			}
		}

		assert.True(t, publicFunc)
		assert.True(t, privateFunc)
		assert.True(t, publicStruct)
		assert.True(t, privateStruct)
	})
}

func TestGoParser_Language(t *testing.T) {
	t.Parallel()

	parser := NewGoParser()
	assert.Equal(t, "go", parser.Language())
}

func TestGoParser_ComplexCode(t *testing.T) {
	t.Parallel()

	parser := NewGoParser()

	content := []byte(`
package service

import (
	"context"
	"database/sql"
	"github.com/pkg/errors"
)

type UserService struct {
	db *sql.DB
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
	query := "SELECT * FROM users WHERE id = $1"
	row := s.db.QueryRowContext(ctx, query, id)
	
	user := &User{}
	err := row.Scan(&user.ID, &user.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user")
	}
	
	return user, nil
}

func (s *UserService) CreateUser(ctx context.Context, name string) (*User, error) {
	user := &User{Name: name}
	err := s.db.QueryRowContext(
		ctx,
		"INSERT INTO users (name) VALUES ($1) RETURNING id",
		name,
	).Scan(&user.ID)
	
	if err != nil {
		return nil, errors.Wrap(err, "failed to create user")
	}
	
	return user, nil
}

func main() {
	db, _ := sql.Open("postgres", "dsn")
	service := NewUserService(db)
	user, _ := service.GetUser(context.Background(), 1)
	println(user.Name)
}
`)

	result, err := parser.Parse("complex.go", content)
	require.NoError(t, err)

	// Verify we extracted the key elements
	var hasStruct bool
	var hasFunction bool
	var hasMethod bool
	var hasImports bool
	var hasCalls bool

	for _, sym := range result.Symbols {
		if sym.Kind == graph.NodeClass {
			hasStruct = true
		}
		if sym.Kind == graph.NodeFunction {
			hasFunction = true
		}
		if sym.Kind == graph.NodeMethod {
			hasMethod = true
		}
	}

	hasImports = len(result.Imports) > 0
	hasCalls = len(result.Calls) > 0

	assert.True(t, hasStruct, "Should find UserService struct")
	assert.True(t, hasFunction, "Should find NewUserService and main functions")
	assert.True(t, hasMethod, "Should find GetUser and CreateUser methods")
	assert.True(t, hasImports, "Should find imports")
	assert.True(t, hasCalls, "Should find function calls")
	assert.Equal(t, "service", result.Package)
}
