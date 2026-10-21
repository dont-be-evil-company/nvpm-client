package files

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryCacheMaxAgeConfigAndOverride(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("registry:\n  cache-max-age: 7d\n"), 0o644))

	oldFS := fileSystem
	fileSystem = &defaultFileSystem{fs: afero.NewOsFs()}
	t.Setenv("NVPM_HOME", dir)
	t.Cleanup(func() {
		fileSystem = oldFS
		ClearRegistryCacheMaxAgeOverride()
	})

	ClearRegistryCacheMaxAgeOverride()
	assert.Equal(t, 7*24*time.Hour, getRegistryCacheMaxAge())

	SetRegistryCacheMaxAgeOverride(time.Hour)
	assert.Equal(t, time.Hour, getRegistryCacheMaxAge())

	SetRegistryCacheMaxAgeOverride(0)
	assert.Equal(t, time.Duration(0), getRegistryCacheMaxAge())
	assert.False(t, IsCacheValid(filepath.Join(dir, "missing.zip"), getRegistryCacheMaxAge()))
}
