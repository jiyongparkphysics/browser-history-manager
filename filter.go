// Package main — filter.go: thin wrappers around the internal/history package.
//
// All core filtering logic has been extracted to internal/history for reuse
// by both the CLI and GUI frontends. This file provides package-level
// aliases and wrapper functions so that the existing CLI code and tests
// continue to work without modification.
//
// # Design Notes
//
// All filtering is performed in-memory on slices returned by getAllURLs (db.go).
// No user-supplied keyword ever reaches a SQL query; see the SQL Security Model
// comment in internal/history/db.go for the full injection-safety argument.
package main

import (
	"time"

	"chrome-history-manager/internal/history"
)

// chromeTimeToTime converts a Chrome timestamp (microseconds since 1601-01-01) to time.Time.
func chromeTimeToTime(ct int64) time.Time {
	return history.ChromeTimeToTime(ct)
}

// formatTime formats a Chrome timestamp as a human-readable string.
func formatTime(ct int64) string {
	return history.FormatTime(ct)
}

// truncate performs UTF-8 safe string truncation to the given rune count.
func truncate(s string, maxRunes int) string {
	return history.Truncate(s, maxRunes)
}

// loadFilters parses a filter value into a list of lowercase keywords.
func loadFilters(value string) ([]string, error) {
	return history.LoadFilters(value)
}

// matches returns true if the given URL/title matches the matchList
// and is not protected by the protectList.
func matches(url, title string, matchList, protectList []string) bool {
	return history.Matches(url, title, matchList, protectList)
}

// filterEntries returns entries that match the given filters.
func filterEntries(entries []HistoryEntry, matchList, protectList []string) []HistoryEntry {
	return history.FilterEntries(entries, matchList, protectList)
}

// filterByDateRange returns entries whose LastVisitTime falls within the
// inclusive range [after, before].
func filterByDateRange(entries []HistoryEntry, after, before time.Time) []HistoryEntry {
	return history.FilterByDateRange(entries, after, before)
}

// sortEntries sorts a slice of HistoryEntry in-place by LastVisitTime descending.
func sortEntries(entries []HistoryEntry) {
	history.SortEntries(entries)
}

// uniqueByURL returns a new slice containing only the first occurrence of each URL.
func uniqueByURL(entries []HistoryEntry) []HistoryEntry {
	return history.UniqueByURL(entries)
}
