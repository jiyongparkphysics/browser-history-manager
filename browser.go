// Package main — browser.go: thin wrappers around the internal/history package.
//
// All core browser detection and path resolution logic has been extracted to
// internal/history for reuse by both the CLI and GUI frontends. This file
// provides package-level aliases and wrapper functions so that the existing
// CLI code and tests continue to work without modification.
package main

import (
	"chrome-history-manager/internal/history"
)

// validBrowserNames lists all accepted --browser flag values.
var validBrowserNames = history.ValidBrowserNames

// validateBrowserName checks that the given browser name is a recognised
// --browser value.
func validateBrowserName(browser string) error {
	return history.ValidateBrowserName(browser)
}

// buildBrowserCandidates returns candidate browser paths for the given OS.
func buildBrowserCandidates(goos, home, localAppData, appData, profile string) map[string]string {
	return history.BuildBrowserCandidates(goos, home, localAppData, appData, profile)
}

// filterExistingPaths returns only the entries whose paths exist on disk.
func filterExistingPaths(candidates map[string]string) map[string]string {
	return history.FilterExistingPaths(candidates)
}

// validateProfileDir checks that the given Chrome profile directory exists
// under at least one candidate browser installation on the current OS.
func validateProfileDir(goos, home, localAppData, appData, browser, profile string) error {
	return history.ValidateProfileDir(goos, home, localAppData, appData, browser, profile)
}

// detectBrowserPaths returns a map of browser name to History DB path
// for all Chromium-based browsers found on the current platform.
func detectBrowserPaths() map[string]string {
	return history.DetectBrowserPaths()
}

// resolveDBPath determines the History DB path based on the browser name,
// profile name, or a custom path provided by the user.
func resolveDBPath(browser string, customPath string, profile string) (string, error) {
	return history.ResolveDBPath(browser, customPath, profile)
}
