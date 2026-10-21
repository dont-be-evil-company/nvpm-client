package providers

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveInstalledGitRepoPath returns the on-disk path for a git package.
// It prefers preferred (kind-derived: plugins/ vs packages/). If that path is
// missing but alternate exists - e.g. a neovim-plugin locked under packages/,
// or a legacy install - it returns alternate. When neither exists, preferred
// is returned so new installs land in the correct directory.
func resolveInstalledGitRepoPath(preferredDir, alternateDir, safeRepo string, stat func(string) (os.FileInfo, error)) string {
	preferred := filepath.Join(preferredDir, safeRepo)
	if _, err := stat(preferred); err == nil {
		return preferred
	}
	alternate := filepath.Join(alternateDir, safeRepo)
	if _, err := stat(alternate); err == nil {
		return alternate
	}
	return preferred
}

// InstalledGitRepoPath returns the local clone for a git-hosted package.
// The boolean is true when that directory contains a .git entry.
func InstalledGitRepoPath(sourceID string) (string, bool) {
	if !IsGitHostedSourceID(sourceID) {
		return "", false
	}
	var path string
	switch detectProvider(sourceID) {
	case ProviderGitHub:
		p := NewProviderGitHub()
		repo := p.getRepo(sourceID)
		if repo == "" {
			return "", false
		}
		path = p.getRepoPath(sourceID, repo)
	case ProviderGitLab:
		p := NewProviderGitLab()
		repo := p.getRepo(sourceID)
		if repo == "" {
			return "", false
		}
		path = p.getRepoPath(sourceID, repo)
	case ProviderCodeberg:
		p := NewProviderCodeberg()
		repo := p.getRepo(sourceID)
		if repo == "" {
			return "", false
		}
		path = p.getRepoPath(sourceID, repo)
	default:
		return "", false
	}
	gitDir := filepath.Join(path, ".git")
	if info, err := os.Stat(gitDir); err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
		return path, false
	}
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	return path, true
}
