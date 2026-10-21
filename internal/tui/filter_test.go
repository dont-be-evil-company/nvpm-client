package tui

import "testing"

func TestVisibleFilterSearchSort(t *testing.T) {
	packages := []Package{
		{ID: "npm:eslint", Name: "eslint", Provider: "npm", Installed: true, Version: "8.0.0", Available: "9.0.0", Outdated: true, Pinned: true},
		{ID: "npm:prettier", Name: "prettier", Provider: "npm", Installed: true, Version: "3.0.0", Available: "3.0.0"},
		{ID: "pypi:black", Name: "black", Provider: "pypi", Installed: false, Available: "24.0.0", Description: "formatter"},
	}

	installed := Visible(packages, FilterInstalled, "", SortName)
	if len(installed) != 2 {
		t.Fatalf("installed len=%d", len(installed))
	}

	outdated := Visible(packages, FilterOutdated, "", SortName)
	if len(outdated) != 1 || outdated[0].ID != "npm:eslint" {
		t.Fatalf("outdated=%v", outdated)
	}

	pinned := Visible(packages, FilterPinned, "es", SortName)
	if len(pinned) != 1 || pinned[0].ID != "npm:eslint" {
		t.Fatalf("pinned search=%v", pinned)
	}

	all := Visible(packages, FilterAll, "format", SortProvider)
	if len(all) != 1 || all[0].ID != "pypi:black" {
		t.Fatalf("description search=%v", all)
	}

	byStatus := Visible(packages, FilterAll, "", SortStatus)
	if byStatus[0].ID != "npm:eslint" {
		t.Fatalf("status sort first=%s", byStatus[0].ID)
	}
	if byStatus[len(byStatus)-1].ID != "pypi:black" {
		t.Fatalf("status sort last=%s", byStatus[len(byStatus)-1].ID)
	}
}
