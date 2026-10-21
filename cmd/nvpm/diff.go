package nvpm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dont-be-evil-company/nvpm-client/internal/lib/local_packages_parser"
	"github.com/dont-be-evil-company/nvpm-client/internal/lib/providers"
	"github.com/dont-be-evil-company/nvpm-client/internal/lib/shell_out"
	"github.com/spf13/cobra"
)

type diffSpec struct {
	sourceID string
	base     string
}

type diffResult struct {
	SourceID string `json:"source_id"`
	Base     string `json:"base,omitempty"`
	Head     string `json:"head,omitempty"`
	Commits  int    `json:"commits,omitempty"`
	Diff     string `json:"diff,omitempty"`
	Error    string `json:"error,omitempty"`
}

var (
	diffRepoPathFn = providers.InstalledGitRepoPath
	diffFetchFn    = fetchDiffTags
	diffLatestFn   = latestDiffHead
)

var diffCmd = &cobra.Command{
	Use:   "diff <pkg[@base]>...",
	Short: "Show changes in packages since a version",
	Long: `Show file changes in one or more packages since a base version.

The base defaults to the installed commit (git) or version. The other side is the
latest candidate nvpm up would select, including update-resolution. A pin does not
hide the diff.

  nvpm diff github:owner/repo
  nvpm diff github:owner/repo v1.2.3
  nvpm diff github:owner/repo@v1.2.3
  nvpm diff npm:eslint github:owner/repo

-s / --stat prints a summary instead of the full patch.
File diffs require a local git checkout (github, gitlab, codeberg). Other providers
print the installed and latest versions.`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: installedPackageIDCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		statOnly, _ := cmd.Flags().GetBool("stat")
		specs, err := parseDiffArgs(args)
		if err != nil {
			fmt.Printf("%s %v\n", IconClose(), err)
			osExit(1)
			return
		}
		_, _ = downloadAndUnzipRegistryFn()

		results := make([]diffResult, 0, len(specs))
		failed := false
		for _, spec := range specs {
			result := diffPackage(spec, statOnly)
			if result.Error != "" {
				failed = true
			}
			results = append(results, result)
		}
		if ShouldUseJSONOutput() {
			PrintJSON(map[string]any{"packages": results})
		} else {
			for i, result := range results {
				if i > 0 {
					fmt.Println()
				}
				fmt.Print(formatDiffResult(result))
			}
		}
		if failed {
			osExit(1)
		}
	},
}

func init() {
	diffCmd.Flags().BoolP("stat", "s", false, "show a diffstat instead of the full patch")
}

func parseDiffArgs(args []string) ([]diffSpec, error) {
	if len(args) == 2 && !argLooksLikePackageID(args[1]) {
		id, ver := parsePackageIDAndVersion(args[0])
		if ver != "" {
			return nil, fmt.Errorf("pass either %s@version or a separate version, not both", id)
		}
		resolved, err := resolveDiffPackageID(id)
		if err != nil {
			return nil, err
		}
		return []diffSpec{{sourceID: resolved, base: strings.TrimSpace(args[1])}}, nil
	}
	specs := make([]diffSpec, 0, len(args))
	for _, arg := range args {
		id, ver := parsePackageIDAndVersion(arg)
		resolved, err := resolveDiffPackageID(id)
		if err != nil {
			return nil, err
		}
		specs = append(specs, diffSpec{sourceID: resolved, base: ver})
	}
	return specs, nil
}

func argLooksLikePackageID(arg string) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}
	if strings.Contains(arg, ":") || strings.HasPrefix(arg, "pkg:") {
		return true
	}
	for _, match := range findInstalledPackagesByName(arg) {
		if strings.EqualFold(match.PackageName, arg) || strings.EqualFold(match.SourceID, arg) || strings.EqualFold(match.Name, arg) {
			return true
		}
	}
	return false
}

func resolveDiffPackageID(baseID string) (string, error) {
	baseID = strings.TrimSpace(baseID)
	if strings.Contains(baseID, ":") || strings.HasPrefix(baseID, "pkg:") {
		provider, pkgName, err := parseUserPackageID(baseID)
		if err != nil {
			return "", err
		}
		return toInternalPackageID(provider, pkgName), nil
	}
	matches := findInstalledPackagesByName(baseID)
	if len(matches) == 0 {
		return "", fmt.Errorf("no installed packages found matching '%s'", baseID)
	}
	if len(matches) == 1 {
		return matches[0].SourceID, nil
	}
	selected, err := promptForProviderSelection(baseID, matches, "diff")
	if err != nil {
		return "", err
	}
	if len(selected) != 1 {
		return "", fmt.Errorf("diff one package at a time when several match '%s'", baseID)
	}
	return selected[0], nil
}

func latestDiffHead(sourceID, installedVersion string) (string, string) {
	stable, prerelease, commit := resolveUpdateCandidates(&defaultRegistryProvider{}, sourceID)
	version := chooseBestRemoteVersion(installedVersion, stable, prerelease)
	if strings.TrimSpace(version) == "" {
		version = installedVersion
	}
	return version, commit
}

func selectDiffRefs(item local_packages_parser.LocalPackageItem, baseOverride, headVersion, headCommit string) (string, string) {
	base := strings.TrimSpace(baseOverride)
	if base == "" {
		if commit := strings.TrimSpace(item.Commit); commit != "" {
			base = commit
		} else {
			base = strings.TrimSpace(item.Version)
		}
	}
	head := strings.TrimSpace(headCommit)
	if head == "" {
		head = strings.TrimSpace(headVersion)
	}
	if head == "" {
		head = base
	}
	return base, head
}

func diffPackage(spec diffSpec, statOnly bool) diffResult {
	item := local_packages_parser.GetBySourceId(spec.sourceID)
	if item.SourceID == "" {
		return diffResult{SourceID: spec.sourceID, Error: spec.sourceID + " is not installed"}
	}
	headVersion, headCommit := diffLatestFn(spec.sourceID, item.Version)
	base, head := selectDiffRefs(item, spec.base, headVersion, headCommit)
	result := diffResult{SourceID: spec.sourceID, Base: base, Head: head}

	if !providers.IsGitHostedSourceID(spec.sourceID) {
		result.Diff = formatNonGitDiff(spec.sourceID, item, base, headVersion)
		return result
	}

	repo, ok := diffRepoPathFn(spec.sourceID)
	if !ok {
		result.Error = fmt.Sprintf("no local git checkout for %s", spec.sourceID)
		result.Diff = formatNonGitDiff(spec.sourceID, item, base, headVersion)
		return result
	}
	if diffFetchFn != nil {
		if err := diffFetchFn(repo); err != nil {
			result.Diff = fmt.Sprintf("warning: git fetch failed: %v\n", err)
		}
	}
	baseSHA, baseErr := gitRev(repo, base)
	headSHA, headErr := gitRev(repo, head)
	if baseErr != nil {
		result.Error = fmt.Sprintf("cannot resolve %s in %s: %v", base, spec.sourceID, baseErr)
		return result
	}
	if headErr != nil {
		result.Error = fmt.Sprintf("cannot resolve %s in %s: %v", head, spec.sourceID, headErr)
		return result
	}
	if baseSHA == headSHA {
		result.Commits = 0
		result.Diff += fmt.Sprintf("%s: no changes since %s\n", spec.sourceID, base)
		return result
	}
	count, countErr := gitCommitCount(repo, baseSHA, headSHA)
	if countErr == nil {
		result.Commits = count
	}
	patch, err := gitDiffRange(repo, baseSHA, headSHA, statOnly)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	header := fmt.Sprintf("%s\n%s..%s", spec.sourceID, base, head)
	if result.Commits > 0 {
		header += fmt.Sprintf(" (%d commits)", result.Commits)
	}
	result.Diff += header + "\n\n" + patch
	if !strings.HasSuffix(result.Diff, "\n") {
		result.Diff += "\n"
	}
	return result
}

func formatNonGitDiff(sourceID string, item local_packages_parser.LocalPackageItem, base, headVersion string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\ninstalled: %s\n", sourceID, item.Version)
	if strings.TrimSpace(headVersion) != "" {
		fmt.Fprintf(&b, "latest: %s\n", headVersion)
	}
	if strings.TrimSpace(base) != "" && base != item.Version && base != item.Commit {
		fmt.Fprintf(&b, "compare from: %s\n", base)
	}
	b.WriteString("File diffs are only available for git-hosted packages (github, gitlab, codeberg).\n")
	return b.String()
}

func formatDiffResult(result diffResult) string {
	if result.Error != "" {
		text := fmt.Sprintf("%s %s: %s\n", IconClose(), result.SourceID, result.Error)
		if result.Diff != "" {
			text += result.Diff
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
		}
		return text
	}
	if result.Diff == "" {
		return result.SourceID + "\n"
	}
	if strings.HasSuffix(result.Diff, "\n") {
		return result.Diff
	}
	return result.Diff + "\n"
}

func fetchDiffTags(repo string) error {
	_, err := runGit(repo, false, "fetch", "--tags", "origin")
	return err
}

func gitRev(repo, ref string) (string, error) {
	out, err := runGit(repo, false, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitCommitCount(repo, base, head string) (int, error) {
	out, err := runGit(repo, false, "rev-list", "--count", base+".."+head)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, err
	}
	return n, nil
}

func gitDiffRange(repo, base, head string, statOnly bool) (string, error) {
	color := "never"
	if shouldUseColors() && !ShouldUsePlainOutput() && !ShouldUseJSONOutput() {
		color = "always"
	}
	args := []string{"-c", "color.ui=" + color, "diff"}
	if statOnly {
		args = append(args, "--stat")
	}
	args = append(args, base, head)
	return runGit(repo, true, args...)
}

// runGit runs git in repo. diffAllowed treats exit status 1 as success (git diff found changes).
func runGit(repo string, diffAllowed bool, args ...string) (string, error) {
	code, out, err := shell_out.ShellOutCapture("git", args, repo, nil)
	if diffAllowed && code == 1 {
		return out, nil
	}
	if err != nil || code != 0 {
		msg := strings.TrimSpace(out)
		if msg == "" && err != nil {
			msg = err.Error()
		}
		return out, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}
