package nvpm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dont-be-evil-company/nvpm-client/internal/lib/local_packages_parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffArgs(t *testing.T) {
	specs, err := parseDiffArgs([]string{"github:owner/repo"})
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "github:owner/repo", specs[0].sourceID)
	assert.Empty(t, specs[0].base)

	specs, err = parseDiffArgs([]string{"github:owner/repo", "v1.2.3"})
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "github:owner/repo", specs[0].sourceID)
	assert.Equal(t, "v1.2.3", specs[0].base)

	specs, err = parseDiffArgs([]string{"github:owner/repo@v0.9.0"})
	require.NoError(t, err)
	assert.Equal(t, "v0.9.0", specs[0].base)

	specs, err = parseDiffArgs([]string{"github:a/b", "gitlab:c/d"})
	require.NoError(t, err)
	require.Len(t, specs, 2)
	assert.Equal(t, "github:a/b", specs[0].sourceID)
	assert.Equal(t, "gitlab:c/d", specs[1].sourceID)
}

func TestSelectDiffRefs(t *testing.T) {
	item := local_packages_parser.LocalPackageItem{Version: "v1.0.0", Commit: "aaa"}
	base, head := selectDiffRefs(item, "", "v2.0.0", "bbb")
	assert.Equal(t, "aaa", base)
	assert.Equal(t, "bbb", head)

	base, head = selectDiffRefs(item, "v0.9.0", "v2.0.0", "bbb")
	assert.Equal(t, "v0.9.0", base)
	assert.Equal(t, "bbb", head)
}

func TestDiffPackageInstalledAndExplicitBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	sha0 := commitFile(t, repo, "file.txt", "base-zero\n", "zero")
	sha1 := commitFile(t, repo, "file.txt", "installed-one\n", "one")
	sha2 := commitFile(t, repo, "file.txt", "latest-two\n", "two")

	t.Setenv("NVPM_HOME", t.TempDir())
	require.NoError(t, local_packages_parser.AddLocalPackageWithCommit("github:owner/repo", "v1.0.0", sha1))
	require.NoError(t, local_packages_parser.AddLocalPackage("npm:eslint", "8.0.0"))

	oldRepo, oldFetch, oldLatest := diffRepoPathFn, diffFetchFn, diffLatestFn
	diffRepoPathFn = func(string) (string, bool) { return repo, true }
	diffFetchFn = func(string) error { return nil }
	diffLatestFn = func(id, installed string) (string, string) {
		if id == "npm:eslint" {
			return "9.0.0", ""
		}
		return "v2.0.0", sha2
	}
	t.Cleanup(func() {
		diffRepoPathFn = oldRepo
		diffFetchFn = oldFetch
		diffLatestFn = oldLatest
	})

	got := diffPackage(diffSpec{sourceID: "github:owner/repo"}, false)
	require.Empty(t, got.Error, got.Diff)
	assert.Equal(t, sha1, got.Base)
	assert.Equal(t, sha2, got.Head)
	assert.Contains(t, got.Diff, "latest-two")
	assert.NotContains(t, got.Diff, "base-zero")
	assert.Greater(t, got.Commits, 0)

	explicit := diffPackage(diffSpec{sourceID: "github:owner/repo", base: sha0}, false)
	require.Empty(t, explicit.Error, explicit.Diff)
	assert.Equal(t, sha0, explicit.Base)
	assert.Contains(t, explicit.Diff, "base-zero")
	assert.Contains(t, explicit.Diff, "latest-two")

	nonGit := diffPackage(diffSpec{sourceID: "npm:eslint"}, false)
	require.Empty(t, nonGit.Error)
	assert.Contains(t, nonGit.Diff, "installed: 8.0.0")
	assert.Contains(t, nonGit.Diff, "latest: 9.0.0")
	assert.Contains(t, nonGit.Diff, "File diffs are only available")
}

func commitFile(t *testing.T, repo, name, contents, message string) string {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=nvpm",
			"GIT_AUTHOR_EMAIL=nvpm@example.com",
			"GIT_COMMITTER_NAME=nvpm",
			"GIT_COMMITTER_EMAIL=nvpm@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s: %s", strings.Join(args, " "), out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		git("init")
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(contents), 0o644))
	git("add", name)
	git("-c", "user.name=nvpm", "-c", "user.email=nvpm@example.com", "-c", "commit.gpgsign=false", "commit", "-m", message)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
