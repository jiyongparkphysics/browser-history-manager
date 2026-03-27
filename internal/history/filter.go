// Package history — filter.go: history entry filtering, searching, and sorting
// logic for the Chrome History manager.
//
// # Responsibilities
//
// This file owns all logic that operates on []HistoryEntry values after they
// have been loaded from the database:
//
//   - ChromeTimeToTime / FormatTime: Chrome microsecond timestamp conversion
//     and human-readable formatting.
//   - Truncate: UTF-8-safe string truncation used when rendering preview output.
//   - LoadFilters: parses a --match or --protect flag value (inline keywords,
//     file path, or single keyword) into a validated []string filter list.
//   - Matches / FilterEntries: in-memory keyword filtering against URL and
//     title fields; protectList takes precedence over matchList.
//   - FilterByDateRange: retains only entries whose LastVisitTime falls within
//     an inclusive [after, before] date range.
//   - SortEntries: stable descending sort by LastVisitTime (most recent first).
//   - UniqueByURL: deduplication that keeps the first occurrence of each URL
//     (most recent when the slice has been pre-sorted by SortEntries).
//
// # Design Notes
//
// All filtering is performed in-memory on slices returned by GetAllURLs (db.go).
// No user-supplied keyword ever reaches a SQL query; see the SQL Security Model
// comment in db.go for the full injection-safety argument.
//
// LoadFilters delegates input validation to ValidateSearchInput,
// ValidateFilterKeywords, and ValidateKeyword in sanitize.go, and path
// validation to ValidateFilterPath (also sanitize.go). This file contains no
// validation logic of its own.
package history

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ChromeTimeToTime converts a Chrome timestamp (microseconds since 1601-01-01) to time.Time.
func ChromeTimeToTime(ct int64) time.Time {
	if ct == 0 {
		return time.Time{}
	}
	unixMicro := ct - 11644473600*1000000
	return time.UnixMicro(unixMicro)
}

// FormatTime formats a Chrome timestamp as a human-readable string.
func FormatTime(ct int64) string {
	t := ChromeTimeToTime(ct)
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// Truncate performs UTF-8 safe string truncation to the given rune count.
func Truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// LoadFilters parses a filter value into a list of lowercase keywords.
// If value contains a comma or is "*", it is treated as inline keywords.
// Otherwise it is tried as a file path, falling back to a single keyword.
// Returns an error if the input contains invalid characters or exceeds limits.
func LoadFilters(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}

	// Validate input for null bytes and control characters.
	if err := ValidateSearchInput(value); err != nil {
		return nil, err
	}

	// Inline: contains comma or is wildcard
	if strings.Contains(value, ",") || value == "*" {
		var filters []string
		for _, k := range strings.Split(value, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				filters = append(filters, strings.ToLower(k))
			}
		}
		if err := ValidateFilterKeywords(filters); err != nil {
			return nil, err
		}
		return filters, nil
	}

	// Try as file — validate path to prevent traversal attacks.
	if safePath, err := ValidateFilterPath(value); err == nil {
		// Reject oversized files before reading into memory.
		// MaxFilterFileBytes is set well above the maximum valid content
		// (MaxFilterKeywords * MaxFilterKeywordLen ~ 2 MiB) to prevent memory
		// exhaustion from a very large file being supplied as a filter path.
		if info, statErr := os.Lstat(safePath); statErr == nil {
			if info.Size() > MaxFilterFileBytes {
				return nil, fmt.Errorf(
					"filter file too large (%d bytes, max %d bytes): %s",
					info.Size(), MaxFilterFileBytes, safePath)
			}
		}
		data, err := os.ReadFile(safePath)
		if err == nil {
			// Validate file contents for control characters.
			if err := ValidateSearchInput(string(data)); err != nil {
				return nil, fmt.Errorf("filter file contains invalid content: %w", err)
			}
			var filters []string
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					filters = append(filters, strings.ToLower(line))
				}
			}
			if len(filters) > 0 {
				if err := ValidateFilterKeywords(filters); err != nil {
					return nil, err
				}
				return filters, nil
			}
		}
	}

	// Single keyword — validate length, control characters, and SQL wildcard
	// density before use.
	keyword := strings.ToLower(strings.TrimSpace(value))
	if err := ValidateKeyword(keyword); err != nil {
		return nil, err
	}
	return []string{keyword}, nil
}

// Matches returns true if the given URL/title matches the matchList
// and is not protected by the protectList.
func Matches(url, title string, matchList, protectList []string) bool {
	urlL := strings.ToLower(url)
	titleL := strings.ToLower(title)

	for _, p := range protectList {
		if strings.Contains(urlL, p) || strings.Contains(titleL, p) {
			return false
		}
	}

	for _, m := range matchList {
		if m == "*" {
			return true
		}
		if strings.Contains(urlL, m) || strings.Contains(titleL, m) {
			return true
		}
	}

	return false
}

// FilterEntries returns entries that match the given filters.
func FilterEntries(entries []HistoryEntry, matchList, protectList []string) []HistoryEntry {
	var matched []HistoryEntry
	for _, e := range entries {
		if Matches(e.URL, e.Title, matchList, protectList) {
			matched = append(matched, e)
		}
	}
	return matched
}

// FilterByDateRange returns entries whose LastVisitTime falls within the
// inclusive range [after, before]. A zero after value means no lower bound;
// a zero before value means no upper bound. Entries with a zero LastVisitTime
// (unknown visit date) are excluded when either bound is non-zero.
func FilterByDateRange(entries []HistoryEntry, after, before time.Time) []HistoryEntry {
	var result []HistoryEntry
	for _, e := range entries {
		t := ChromeTimeToTime(e.LastVisitTime)
		// Exclude entries with an unknown visit time when any bound is active.
		if t.IsZero() && (!after.IsZero() || !before.IsZero()) {
			continue
		}
		if !after.IsZero() && t.Before(after) {
			continue
		}
		if !before.IsZero() && t.After(before) {
			continue
		}
		result = append(result, e)
	}
	return result
}

// SortEntries sorts a slice of HistoryEntry in-place by LastVisitTime descending
// (most recently visited first). Entries with equal timestamps retain their
// relative order (stable sort).
func SortEntries(entries []HistoryEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].LastVisitTime > entries[j].LastVisitTime
	})
}

// UniqueByURL returns a new slice containing only the first occurrence of each
// URL, preserving the input order. When the input has already been sorted by
// LastVisitTime descending via SortEntries, this keeps the most recently
// visited entry for each URL.
func UniqueByURL(entries []HistoryEntry) []HistoryEntry {
	seen := make(map[string]bool, len(entries))
	result := make([]HistoryEntry, 0, len(entries))
	for _, e := range entries {
		if !seen[e.URL] {
			seen[e.URL] = true
			result = append(result, e)
		}
	}
	return result
}
