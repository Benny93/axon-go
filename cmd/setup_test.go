package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupCmd_Run(t *testing.T) {
	t.Run("SetupQwenLocal", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &SetupCmd{
			Qwen:   true,
			Local:  true,
			Format: "json",
		}

		err := cmd.Run()
		assert.NoError(t, err)

		// Verify .qwen directory was created
		qwenDir := filepath.Join(tmpDir, ".qwen")
		_, err = os.Stat(qwenDir)
		assert.NoError(t, err)

		// Verify mcp.json was created
		mcpPath := filepath.Join(qwenDir, "mcp.json")
		_, err = os.Stat(mcpPath)
		assert.NoError(t, err)
	})

	t.Run("SetupQwenGlobal", func(t *testing.T) {
		tmpHome := t.TempDir()
		origHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		cmd := &SetupCmd{
			Qwen:   true,
			Global: true,
			Format: "json",
		}

		err := cmd.Run()
		assert.NoError(t, err)

		// Verify global config was created
		globalPath := filepath.Join(tmpHome, ".qwen", "global", "mcp.json")
		_, err = os.Stat(globalPath)
		assert.NoError(t, err)
	})

	t.Run("SetupClaude", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &SetupCmd{
			Claude: true,
			Local:  true,
			Format: "json",
		}

		err := cmd.Run()
		assert.NoError(t, err)

		// Verify .claude directory was created
		claudeDir := filepath.Join(tmpDir, ".claude")
		_, err = os.Stat(claudeDir)
		assert.NoError(t, err)

		// Verify mcp.json was created
		mcpPath := filepath.Join(claudeDir, "mcp.json")
		_, err = os.Stat(mcpPath)
		assert.NoError(t, err)
	})

	t.Run("SetupCursor", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		cmd := &SetupCmd{
			Cursor: true,
			Local:  true,
			Format: "json",
		}

		err := cmd.Run()
		assert.NoError(t, err)

		// Verify .cursor directory was created
		cursorDir := filepath.Join(tmpDir, ".cursor")
		_, err = os.Stat(cursorDir)
		assert.NoError(t, err)

		// Verify mcp.json was created
		mcpPath := filepath.Join(cursorDir, "mcp.json")
		_, err = os.Stat(mcpPath)
		assert.NoError(t, err)
	})

	t.Run("SetupDefault", func(t *testing.T) {
		// When no specific client is specified, should output to stdout
		cmd := &SetupCmd{
			Format: "json",
		}

		err := cmd.Run()
		assert.NoError(t, err)
	})
}

func TestMCPConfigGeneration(t *testing.T) {
	t.Run("GenerateAxonConfig", func(t *testing.T) {
		config := generateAxonConfig()

		assert.NotNil(t, config)
		assert.Contains(t, config, "mcpServers")

		mcpServers := config["mcpServers"].(map[string]any)
		assert.Contains(t, mcpServers, "axon-go")

		axon := mcpServers["axon-go"].(map[string]any)
		assert.Equal(t, "axon-go", axon["command"])
		assert.Contains(t, axon["args"], "serve")
		assert.Contains(t, axon["args"], "--watch")
	})

	t.Run("GenerateClaudeConfig", func(t *testing.T) {
		config := generateClaudeConfig()

		assert.NotNil(t, config)
		assert.Contains(t, config, "mcpServers")
	})

	t.Run("GenerateCursorConfig", func(t *testing.T) {
		config := generateCursorConfig()

		assert.NotNil(t, config)
		assert.Contains(t, config, "mcpServers")
	})
}

func TestConfigPaths(t *testing.T) {
	t.Run("GetLocalConfigPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := getLocalConfigPath(tmpDir, "qwen")
		assert.Equal(t, filepath.Join(tmpDir, ".qwen", "mcp.json"), path)
	})

	t.Run("GetClientConfigDir", func(t *testing.T) {
		assert.Equal(t, ".qwen", getClientConfigDir("qwen"))
		assert.Equal(t, ".claude", getClientConfigDir("claude"))
		assert.Equal(t, ".cursor", getClientConfigDir("cursor"))
	})
}

func TestWriteConfig(t *testing.T) {
	t.Run("WriteJSONConfig", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		config := map[string]any{
			"mcpServers": map[string]any{
				"axon-go": map[string]any{
					"command": "axon-go",
					"args":    []string{"serve", "--watch"},
				},
			},
		}

		err := writeConfig(configPath, config, "json")
		assert.NoError(t, err)

		// Verify file was created
		_, err = os.Stat(configPath)
		assert.NoError(t, err)

		// Verify content
		content, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var loaded map[string]any
		err = json.Unmarshal(content, &loaded)
		assert.NoError(t, err)
	})

	t.Run("WriteConfigCreatesDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "nested", "dir", "config.json")

		config := map[string]any{"test": "value"}

		err := writeConfig(configPath, config, "json")
		assert.NoError(t, err)

		// Verify file was created
		_, err = os.Stat(configPath)
		assert.NoError(t, err)
	})
}

func TestSetupCmd_Validation(t *testing.T) {
	t.Run("NoClientSpecified", func(t *testing.T) {
		cmd := &SetupCmd{
			Format: "json",
		}

		// Should not error, just output to stdout
		err := cmd.Run()
		assert.NoError(t, err)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		cmd := &SetupCmd{
			Qwen:   true,
			Format: "invalid",
		}

		err := cmd.Run()
		assert.Error(t, err)
	})
}
