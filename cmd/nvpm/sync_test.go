package nvpm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dont-be-evil-company/nvpm-client/internal/lib/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncPackagesForceFlagDefaultsOff(t *testing.T) {
	flag := syncPackagesCmd.Flags().Lookup("force")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.False(t, flag.Changed)
}

func TestClearLocalInstallCacheKeepsLockfile(t *testing.T) {
	home := t.TempDir()
	files.ResetDependencies()
	t.Cleanup(files.ResetDependencies)
	t.Setenv("NVPM_HOME", home)

	require.Equal(t, filepath.Join(home, "packages"), files.GetAppPackagesPath())
	require.Equal(t, filepath.Join(home, "plugins"), files.GetAppNeovimPluginsPath())
	require.Equal(t, filepath.Join(home, "bin"), files.GetAppBinPath())

	plant := []string{
		filepath.Join(home, "packages", "github", "owner_repo", "README.md"),
		filepath.Join(home, "plugins", "github", "owner_plugin", "plugin.lua"),
		filepath.Join(home, "artifacts", "treesitter", "github_owner_repo", "v1", "lang.so"),
		filepath.Join(home, "bin", "tool"),
	}
	for _, path := range plant {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("cached"), 0o644))
	}
	lockPath := filepath.Join(home, "nvpm-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":[]}`), 0o644))
	configPath := filepath.Join(home, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("ui:\n  color: auto\n"), 0o644))

	require.NoError(t, clearLocalInstallCache())

	for _, path := range plant {
		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err), "expected %s to be removed", path)
	}
	for _, dir := range []string{"packages", "plugins", "artifacts", "bin"} {
		_, err := os.Stat(filepath.Join(home, dir))
		assert.True(t, os.IsNotExist(err), "expected %s to be removed", dir)
	}

	lock, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, `{"packages":[]}`, string(lock))

	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(config), "color: auto")
}

func TestClearLocalInstallCacheMissingDirs(t *testing.T) {
	home := t.TempDir()
	files.ResetDependencies()
	t.Cleanup(files.ResetDependencies)
	t.Setenv("NVPM_HOME", home)
	require.Equal(t, home, files.GetAppDataSharePath())
	require.NoError(t, clearLocalInstallCache())
}
