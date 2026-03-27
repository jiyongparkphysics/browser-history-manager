// Package main — sanitize.go: thin wrappers around the internal/history package.
//
// All core sanitization and validation logic has been extracted to
// internal/history for reuse by both the CLI and GUI frontends. This file
// provides package-level aliases and wrapper functions so that the existing
// CLI code and tests continue to work without modification.
package main

import (
	"fmt"
	"strconv"
	"time"

	"chrome-history-manager/internal/history"
)

// defaultPreviewLimit is the number of entries shown by preview when --limit is not specified.
const defaultPreviewLimit = 50

// maxPreviewLimit is the maximum allowed value for the --limit flag.
const maxPreviewLimit = 10000

// validateLimit parses a string as a positive integer for the --limit flag.
// An empty string returns defaultPreviewLimit. Non-integer, zero, negative,
// or out-of-range values return a clear error message.
func validateLimit(s string) (int, error) {
	if s == "" {
		return defaultPreviewLimit, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --limit value %q: must be a positive integer", s)
	}

	if n < 1 {
		return 0, fmt.Errorf("invalid --limit value %d: must be at least 1", n)
	}

	if n > maxPreviewLimit {
		return 0, fmt.Errorf("invalid --limit value %d: must be at most %d", n, maxPreviewLimit)
	}

	return n, nil
}

// maxFilterKeywords is the maximum number of keywords allowed in a single
// comma-separated --match or --protect value.
const maxFilterKeywords = history.MaxFilterKeywords

// maxFilterKeywordLen is the maximum length (in bytes) of a single filter keyword.
const maxFilterKeywordLen = history.MaxFilterKeywordLen

// maxFilterFileBytes is the maximum file size that loadFilters will read into memory.
const maxFilterFileBytes = history.MaxFilterFileBytes

// maxSQLWildcardCount is the maximum number of SQL wildcard characters permitted.
const maxSQLWildcardCount = history.MaxSQLWildcardCount

// validateSearchInput validates a raw --match or --protect string value.
func validateSearchInput(value string) error {
	return history.ValidateSearchInput(value)
}

// validateKeyword validates a single parsed search keyword.
func validateKeyword(kw string) error {
	return history.ValidateKeyword(kw)
}

// validateFilterKeywords checks that parsed keywords meet count limits.
func validateFilterKeywords(keywords []string) error {
	return history.ValidateFilterKeywords(keywords)
}

// sanitizePath resolves a user-supplied path to its clean, absolute form.
func sanitizePath(rawPath string) (string, error) {
	return history.SanitizePath(rawPath)
}

// validateDBPath sanitizes a user-supplied database path and ensures it
// points to an existing regular file.
func validateDBPath(rawPath string) (string, error) {
	return history.ValidateDBPath(rawPath)
}

// validateOutputPath sanitizes a user-supplied output file path.
func validateOutputPath(rawPath string) (string, error) {
	return history.ValidateOutputPath(rawPath)
}

// sanitizeEnvPath sanitizes a path obtained from an environment variable.
func sanitizeEnvPath(raw string) string {
	return history.SanitizeEnvPath(raw)
}

// maxProfileNameLen is the maximum allowed length for a Chrome profile directory name.
const maxProfileNameLen = history.MaxProfileNameLen

// validateProfileName validates a Chrome profile directory name.
func validateProfileName(name string) error {
	return history.ValidateProfileName(name)
}

// dateLayout is the expected format for --since and --until flag values.
const dateLayout = history.DateLayout

// validateDateFlag parses a --since or --until flag value as a YYYY-MM-DD date.
func validateDateFlag(s string) (time.Time, error) {
	return history.ValidateDateFlag(s)
}

// validateFilterPath sanitizes a user-supplied filter file path.
func validateFilterPath(rawPath string) (string, error) {
	return history.ValidateFilterPath(rawPath)
}
