package nvpm

import (
	"fmt"
	"strings"

	"github.com/dont-be-evil-company/nvpm-client/internal/lib/local_packages_parser"
	"github.com/dont-be-evil-company/nvpm-client/internal/lib/registry_parser"
	"github.com/dont-be-evil-company/nvpm-client/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Browse and manage packages in a terminal UI",
	Long: `Open a terminal UI for installed and registry packages.

Search, filter, and sort the list, then install, update, remove, pin, unpin,
diff, or sync. Pinned packages are skipped by update and update-all until they
are unpinned. Install, update, and remove suspend the UI so the same prompts
as the CLI can run.

  nvpm tui`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := tui.Run(cliTUIBackend{}); err != nil {
			fmt.Printf("%s %v\n", IconClose(), err)
			osExit(1)
		}
	},
}

type cliTUIBackend struct{}

func (cliTUIBackend) Load() ([]tui.Package, error) {
	_, refreshErr := downloadAndUnzipRegistryQuietFn()
	svc := NewListService()
	svc.registry.GetData(true)
	local := local_packages_parser.GetData(true)

	rows := make([]tui.Package, 0, len(local.Packages))
	seen := map[string]struct{}{}
	for _, pkg := range local.Packages {
		id := strings.TrimSpace(pkg.SourceID)
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
		item := newRegistryParser().GetBySourceId(id)
		_, outdated := svc.checkUpdateAvailability(id, pkg.Version, pkg.Commit)
		stable, prerelease, _ := resolveUpdateCandidatesOffline(svc.registry, id)
		rows = append(rows, tui.Package{
			ID:          id,
			Name:        packageDisplayName(item, id),
			Provider:    getProviderFromSourceID(id),
			Description: item.Description,
			Installed:   true,
			Version:     pkg.Version,
			Available:   chooseBestRemoteVersion(pkg.Version, stable, prerelease),
			Pinned:      packageIsPinned(pkg),
			Outdated:    outdated,
		})
	}
	for _, item := range svc.registry.GetData(false) {
		id := strings.TrimSpace(item.Source.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		rows = append(rows, tui.Package{
			ID:          id,
			Name:        packageDisplayName(item, id),
			Provider:    getProviderFromSourceID(id),
			Description: item.Description,
			Available:   item.Version,
		})
	}
	if refreshErr != nil && len(rows) == 0 {
		return nil, refreshErr
	}
	return rows, nil
}

func packageDisplayName(item registry_parser.RegistryItem, id string) string {
	if name := strings.TrimSpace(item.Name); name != "" {
		return name
	}
	return getPackageNameFromSourceID(id)
}

func (cliTUIBackend) Details(id string) (string, error) {
	item := newRegistryParser().GetBySourceId(id)
	lock := local_packages_parser.GetBySourceId(id)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", id)
	if item.Name != "" {
		fmt.Fprintf(&b, "Name: %s\n", item.Name)
	}
	if lock.SourceID != "" {
		fmt.Fprintf(&b, "Installed: %s\n", lock.Version)
		if lock.Commit != "" {
			fmt.Fprintf(&b, "Commit: %s\n", lock.Commit)
		}
		if packageIsPinned(lock) {
			b.WriteString("Pinned: yes (nvpm up will not move this package until nvpm unpin)\n")
		}
	} else {
		b.WriteString("Installed: no\n")
	}
	if item.Version != "" {
		fmt.Fprintf(&b, "Registry: %s\n", item.Version)
	}
	if item.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", item.Description)
	}
	if item.Homepage != "" {
		fmt.Fprintf(&b, "\nHomepage: %s\n", item.Homepage)
	}
	if len(item.Categories) > 0 {
		fmt.Fprintf(&b, "Categories: %s\n", strings.Join(item.Categories, ", "))
	}
	if len(item.Licenses) > 0 {
		fmt.Fprintf(&b, "Licenses: %s\n", strings.Join(item.Licenses, ", "))
	}
	return b.String(), nil
}

func (cliTUIBackend) Diff(id string) (string, error) {
	_, _ = downloadAndUnzipRegistryQuietFn()
	result := diffPackage(diffSpec{sourceID: id}, false)
	text := formatDiffResult(result)
	if result.Error != "" {
		return text, fmt.Errorf("%s", result.Error)
	}
	return text, nil
}

func (cliTUIBackend) Install(id string) error {
	prev := installPin
	installPin = false
	defer func() { installPin = prev }()
	addCmd.Run(addCmd, []string{id})
	if lastAddFailureCount > 0 || lastAddSuccessCount == 0 {
		return fmt.Errorf("failed to install %s", id)
	}
	return nil
}

func (cliTUIBackend) Update(id string) error {
	upCmd.Run(upCmd, []string{id})
	return nil
}

func (cliTUIBackend) UpdateAll() error {
	if err := upCmd.Flags().Set("all", "true"); err != nil {
		return err
	}
	defer func() { _ = upCmd.Flags().Set("all", "false") }()
	upCmd.Run(upCmd, nil)
	return nil
}

func (cliTUIBackend) Remove(id string) error {
	rmCmd.Run(rmCmd, []string{id})
	return nil
}

func (cliTUIBackend) Pin(id string) error {
	return pinInstalled(id)
}

func (cliTUIBackend) Unpin(id string) error {
	item := local_packages_parser.GetBySourceId(id)
	if item.SourceID == "" {
		return fmt.Errorf("%s is not installed", id)
	}
	if !packageIsPinned(item) {
		return nil
	}
	return local_packages_parser.MergePackagePinned(id, false)
}

func (cliTUIBackend) Sync() error {
	syncPackagesCmd.Run(syncPackagesCmd, nil)
	return nil
}
