package tui

import (
	"sort"
	"strings"
)

// Package is one row in the package browser.
type Package struct {
	ID          string
	Name        string
	Provider    string
	Description string
	Installed   bool
	Version     string
	Available   string
	Pinned      bool
	Outdated    bool
}

// Filter selects which packages are shown.
type Filter int

const (
	FilterInstalled Filter = iota
	FilterOutdated
	FilterPinned
	FilterAll
)

// Sort selects the row order.
type Sort int

const (
	SortName Sort = iota
	SortProvider
	SortStatus
)

func (f Filter) Next() Filter {
	switch f {
	case FilterInstalled:
		return FilterOutdated
	case FilterOutdated:
		return FilterPinned
	case FilterPinned:
		return FilterAll
	default:
		return FilterInstalled
	}
}

func (f Filter) String() string {
	switch f {
	case FilterOutdated:
		return "outdated"
	case FilterPinned:
		return "pinned"
	case FilterAll:
		return "all"
	default:
		return "installed"
	}
}

func (s Sort) Next() Sort {
	switch s {
	case SortName:
		return SortProvider
	case SortProvider:
		return SortStatus
	default:
		return SortName
	}
}

func (s Sort) String() string {
	switch s {
	case SortProvider:
		return "provider"
	case SortStatus:
		return "status"
	default:
		return "name"
	}
}

// Visible filters, searches, and sorts packages.
func Visible(packages []Package, filter Filter, query string, order Sort) []Package {
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]Package, 0, len(packages))
	for _, pkg := range packages {
		if !matchesFilter(pkg, filter) || !matchesQuery(pkg, query) {
			continue
		}
		out = append(out, pkg)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return lessPackage(out[i], out[j], order)
	})
	return out
}

func matchesFilter(pkg Package, filter Filter) bool {
	switch filter {
	case FilterOutdated:
		return pkg.Installed && pkg.Outdated
	case FilterPinned:
		return pkg.Installed && pkg.Pinned
	case FilterAll:
		return true
	default:
		return pkg.Installed
	}
}

func matchesQuery(pkg Package, query string) bool {
	if query == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{pkg.ID, pkg.Name, pkg.Provider, pkg.Description}, " "))
	return strings.Contains(hay, query)
}

func lessPackage(a, b Package, order Sort) bool {
	switch order {
	case SortProvider:
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
	case SortStatus:
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	return a.Version < b.Version
}

func rank(pkg Package) int {
	if pkg.Outdated && pkg.Pinned {
		return 0
	}
	if pkg.Outdated {
		return 1
	}
	if pkg.Pinned {
		return 2
	}
	if pkg.Installed {
		return 3
	}
	return 4
}
