package nvpm

import (
	"testing"

	"github.com/dont-be-evil-company/nvpm-client/internal/lib/local_packages_parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinAndUnpinInstalledPackage(t *testing.T) {
	t.Setenv("NVPM_HOME", t.TempDir())
	require.NoError(t, local_packages_parser.AddLocalPackage("npm:eslint", "8.0.0"))

	pinCmd.Run(pinCmd, []string{"npm:eslint"})
	item := local_packages_parser.GetBySourceId("npm:eslint")
	require.NotNil(t, item.Extras)
	assert.True(t, item.Extras.Pinned)
	assert.Equal(t, "8.0.0", item.Version)

	unpinCmd.Run(unpinCmd, []string{"npm:eslint"})
	item = local_packages_parser.GetBySourceId("npm:eslint")
	assert.Equal(t, "8.0.0", item.Version)
	assert.False(t, packageIsPinned(item))
}

func TestRejectPinnedVersionChange(t *testing.T) {
	t.Setenv("NVPM_HOME", t.TempDir())
	require.NoError(t, local_packages_parser.AddLocalPackage("npm:eslint", "8.0.0"))
	require.NoError(t, local_packages_parser.MergePackagePinned("npm:eslint", true))

	prev := installPin
	installPin = false
	t.Cleanup(func() { installPin = prev })

	err := rejectIfPinnedVersionChange("npm:eslint", "9.0.0", "9.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nvpm unpin npm:eslint")
	assert.Contains(t, err.Error(), "nvpm pin npm:eslint@<version>")

	assert.NoError(t, rejectIfPinnedVersionChange("npm:eslint", "8.0.0", "8.0.0"))
	assert.NoError(t, rejectIfPinnedVersionChange("npm:eslint", "", "v8.0.0"))

	installPin = true
	assert.NoError(t, rejectIfPinnedVersionChange("npm:eslint", "9.0.0", "9.0.0"))
}
