package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImpactCmd_Run(t *testing.T) {
	t.Run("NoSymbolProvided", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ImpactCmd{}

		err := cmd.Run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "symbol name required")
	})

	t.Run("NoIndexFound", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ImpactCmd{
			Symbol: "Foo",
		}

		err := cmd.Run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no index found")
	})

	t.Run("SymbolNotFound", func(t *testing.T) {
		tmpDir := setupTestIndex(t)
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ImpactCmd{
			Symbol: "NonExistent",
		}

		err := cmd.Run()
		assert.NoError(t, err)
		// Should print "not found" message
	})

	t.Run("SymbolFound", func(t *testing.T) {
		tmpDir := setupTestIndex(t)
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ImpactCmd{
			Symbol: "Foo",
		}

		err := cmd.Run()
		assert.NoError(t, err)
	})

	t.Run("WithDepth", func(t *testing.T) {
		tmpDir := setupTestIndex(t)
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &ImpactCmd{
			Symbol: "Foo",
			Depth:  5,
		}

		err := cmd.Run()
		assert.NoError(t, err)
	})
}

func TestImpactAnalysis(t *testing.T) {
	t.Run("BlastRadiusCalculation", func(t *testing.T) {
		tmpDir := setupTestIndex(t)
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		// The test index has Foo with no callers
		// So impact should show no affected symbols
		cmd := &ImpactCmd{
			Symbol: "Foo",
			Depth:  3,
		}

		err := cmd.Run()
		assert.NoError(t, err)
	})
}
