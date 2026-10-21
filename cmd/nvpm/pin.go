package nvpm

import (
	"fmt"
	"strings"

	"github.com/dont-be-evil-company/nvpm-client/internal/lib/local_packages_parser"
	"github.com/spf13/cobra"
)

var (
	installPin          bool
	lastAddSuccessCount int
	lastAddFailureCount int
)

func packageIsPinned(pkg local_packages_parser.LocalPackageItem) bool {
	return pkg.Extras != nil && pkg.Extras.Pinned
}

func withPinnedMarker(pkg local_packages_parser.LocalPackageItem, installedText string) string {
	if packageIsPinned(pkg) {
		return installedText + " (pinned)"
	}
	return installedText
}

func pinnedOutdatedCount(pkgs []local_packages_parser.LocalPackageItem) int {
	n := 0
	for _, pkg := range pkgs {
		if packageIsPinned(pkg) {
			n++
		}
	}
	return n
}

func samePinnedVersion(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(a, b) {
		return true
	}
	return strings.EqualFold(strings.TrimPrefix(strings.ToLower(a), "v"), strings.TrimPrefix(strings.ToLower(b), "v"))
}

// rejectIfPinnedVersionChange blocks add from moving a pinned package unless --pin is set.
func rejectIfPinnedVersionChange(sourceID, requested, resolved string) error {
	if installPin {
		return nil
	}
	item := local_packages_parser.GetBySourceId(sourceID)
	if item.SourceID == "" || !packageIsPinned(item) {
		return nil
	}
	if samePinnedVersion(item.Version, requested) || (strings.TrimSpace(requested) == "" && samePinnedVersion(item.Version, resolved)) {
		return nil
	}
	return fmt.Errorf("%s is pinned at %s. Unpin it with `nvpm unpin %s`, or move the pin with `nvpm pin %s@<version>`", sourceID, item.Version, sourceID, sourceID)
}

func persistPinAfterInstall(sourceID string) {
	if !installPin {
		return
	}
	if err := local_packages_parser.MergePackagePinned(sourceID, true); err != nil {
		fmt.Printf("%s Failed to pin %s: %v\n", IconClose(), sourceID, err)
	}
}

var pinCmd = &cobra.Command{
	Use:   "pin <pkgId[@version]>...",
	Short: "Pin packages to their installed or given version",
	Long: `Pin packages so later updates keep the locked version.

nvpm pin npm:eslint
  holds the version currently recorded in nvpm-lock.json.

nvpm pin npm:eslint@8.0.0
  installs that version, then holds it.

Pinned packages stay listed as installed and, when a newer candidate exists, as outdated.
nvpm up, including nvpm up -A --force, will not move them until nvpm unpin.

nvpm add pkg@version --pin installs and pins in one step.`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: installedPackageIDCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		failures := 0
		for _, arg := range args {
			baseID, version := parsePackageIDAndVersion(arg)
			ids, err := resolvePinPackageIDs(baseID, version != "")
			if err != nil {
				fmt.Printf("%s %v\n", IconClose(), err)
				failures++
				continue
			}
			for _, id := range ids {
				if version == "" {
					if err := pinInstalled(id); err != nil {
						fmt.Printf("%s %v\n", IconClose(), err)
						failures++
						continue
					}
					item := local_packages_parser.GetBySourceId(id)
					fmt.Printf("%s Pinned %s at %s\n", IconCheck(), id, item.Version)
					continue
				}
				if !installAndPin(id, version) {
					failures++
				}
			}
		}
		if failures > 0 {
			osExit(1)
		}
	},
}

var unpinCmd = &cobra.Command{
	Use:   "unpin <pkgId>...",
	Short: "Remove a version pin so the package can be updated",
	Long: `Clear extras.pinned for one or more installed packages.

The installed version stays as it is. Later nvpm up commands can move the package again.`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: installedPackageIDCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		failures := 0
		for _, arg := range args {
			baseID, _ := parsePackageIDAndVersion(arg)
			ids, err := resolvePinPackageIDs(baseID, false)
			if err != nil {
				fmt.Printf("%s %v\n", IconClose(), err)
				failures++
				continue
			}
			for _, id := range ids {
				item := local_packages_parser.GetBySourceId(id)
				if item.SourceID == "" {
					fmt.Printf("%s %s is not installed\n", IconClose(), id)
					failures++
					continue
				}
				if !packageIsPinned(item) {
					fmt.Printf("%s %s is not pinned\n", IconCheck(), id)
					continue
				}
				if err := local_packages_parser.MergePackagePinned(id, false); err != nil {
					fmt.Printf("%s %v\n", IconClose(), err)
					failures++
					continue
				}
				fmt.Printf("%s Unpinned %s (still at %s)\n", IconCheck(), id, item.Version)
			}
		}
		if failures > 0 {
			osExit(1)
		}
	},
}

func pinInstalled(sourceID string) error {
	item := local_packages_parser.GetBySourceId(sourceID)
	if item.SourceID == "" {
		return fmt.Errorf("%s is not installed. Use `nvpm pin %s@<version>` to install and pin a version", sourceID, sourceID)
	}
	return local_packages_parser.MergePackagePinned(sourceID, true)
}

func installAndPin(sourceID, version string) bool {
	prev := installPin
	installPin = true
	defer func() { installPin = prev }()

	addCmd.Run(addCmd, []string{sourceID + "@" + version})
	if lastAddFailureCount > 0 || lastAddSuccessCount == 0 {
		return false
	}
	item := local_packages_parser.GetBySourceId(sourceID)
	if !packageIsPinned(item) {
		fmt.Printf("%s Installed %s but failed to record the pin\n", IconClose(), sourceID)
		return false
	}
	fmt.Printf("%s Pinned %s at %s\n", IconCheck(), sourceID, item.Version)
	return true
}

func resolvePinPackageIDs(baseID string, allowRegistry bool) ([]string, error) {
	baseID = strings.TrimSpace(baseID)
	if baseID == "" {
		return nil, fmt.Errorf("empty package id")
	}
	if strings.Contains(baseID, ":") || strings.HasPrefix(baseID, "pkg:") {
		provider, pkgName, err := parseUserPackageID(baseID)
		if err != nil {
			return nil, err
		}
		return []string{toInternalPackageID(provider, pkgName)}, nil
	}

	matches := findInstalledPackagesByName(baseID)
	if len(matches) == 0 && allowRegistry {
		matches = findPackagesByName(baseID)
	}
	if len(matches) == 0 {
		if allowRegistry {
			return nil, fmt.Errorf("no packages found matching '%s'", baseID)
		}
		return nil, fmt.Errorf("no installed packages found matching '%s'", baseID)
	}
	if len(matches) == 1 {
		return []string{matches[0].SourceID}, nil
	}
	selected, err := promptForProviderSelection(baseID, matches, "pin")
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no package selected for '%s'", baseID)
	}
	return selected, nil
}
