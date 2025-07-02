package config_test

import (
	"os"
	"testing"

	"github.com/SayantanSaha/toron/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadValidConfig(t *testing.T) {
	tempFile := createTempConfig(t, `
server:
  address: ":8080"
routes:
  - path: "/"
    backend: "http://localhost:9000"
  - path: "/api"
    backend: "http://localhost:9001"
`)
	defer os.Remove(tempFile)

	cfg, err := config.Load(tempFile)
	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Server.Address)
	assert.Len(t, cfg.Routes, 2)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load("nonexistent.yaml")
	assert.Error(t, err)
}

func TestLoadInvalidYAML(t *testing.T) {
	tempFile := createTempConfig(t, `not: valid: yaml`)
	defer os.Remove(tempFile)

	_, err := config.Load(tempFile)
	assert.Error(t, err)
}

func createTempConfig(t *testing.T, content string) string {
	tempFile, err := os.CreateTemp("", "toron-config-*.yaml")
	require.NoError(t, err)

	_, err = tempFile.WriteString(content)
	require.NoError(t, err)
	tempFile.Close()

	return tempFile.Name()
}
