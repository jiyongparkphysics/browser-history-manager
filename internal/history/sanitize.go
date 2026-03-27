// Package history — sanitize.go: input sanitization and path validation
// utilities shared by filter.go and browser.go in the internal/history package.
//
// These functions were extracted from the root sanitize.go to resolve cross-file
// dependencies when filter.go and browser.go were moved into internal/history.
// The root sanitize.go retains the original functions as thin wrappers for
// backward compatibility with the CLI and its tests.
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// MaxFilterKeywords is the maximum number of keywords allowed in a single
// comma-separated --match or --protect value, to prevent excessive memory use.
const MaxFilterKeywords = 1000

// MaxFilterKeywordLen is the maximum length (in bytes) of a single filter keyword.
const MaxFilterKeywordLen = 2048

// MaxFilterFileBytes is the maximum file size that LoadFilters will read into
// memory for a filter file supplied via --match or --protect.
const MaxFilterFileBytes = 4 * 1024 * 1024

// MaxSQLWildcardCount is the maximum number of SQL wildcard characters
// (percent sign % and underscore _) permitted in a single search keyword.
const MaxSQLWildcardCount = 20

// MaxProfileNameLen is the maximum allowed length for a Chrome profile directory name.
const MaxProfileNameLen = 128

// DateLayout is the expected format for --since and --until flag values.
const DateLayout = "2006-01-02"

// windowsReservedNames lists device names that are reserved on Windows and
// cannot safely be used as file names regardless of extension.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM0": true, "COM1": true, "COM2": true, "COM3": true,
	"COM4": true, "COM5": true, "COM6": true, "COM7": true,
	"COM8": true, "COM9": true,
	"LPT0": true, "LPT1": true, "LPT2": true, "LPT3": true,
	"LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true,
	"LPT8": true, "LPT9": true,
}

// ValidateSearchInput validates a raw --match or --protect string value,
// rejecting null bytes, control characters, and excessively long inputs.
func ValidateSearchInput(value string) error {
	if value == "" {
		return nil
	}

	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("filter value contains null byte")
	}

	for _, r := range value {
		if r > 0 && r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return fmt.Errorf("filter value contains control character (0x%02x)", r)
		}
	}

	return nil
}

// ValidateKeyword validates a single parsed search keyword for null bytes,
// disallowed control characters, byte-length limits, and SQL wildcard
// character density.
func ValidateKeyword(kw string) error {
	if err := ValidateSearchInput(kw); err != nil {
		return err
	}

	if len(kw) > MaxFilterKeywordLen {
		return fmt.Errorf("search keyword too long (%d bytes, max %d)", len(kw), MaxFilterKeywordLen)
	}

	wildcards := strings.Count(kw, "%") + strings.Count(kw, "_")
	if wildcards > MaxSQLWildcardCount {
		return fmt.Errorf(
			"keyword contains too many SQL wildcard characters (%% and _): %d found, max %d",
			wildcards, MaxSQLWildcardCount)
	}

	return nil
}

// ValidateFilterKeywords checks that parsed keywords meet count limits and
// that each individual keyword satisfies ValidateKeyword.
func ValidateFilterKeywords(keywords []string) error {
	if len(keywords) > MaxFilterKeywords {
		return fmt.Errorf("too many filter keywords (%d, max %d)", len(keywords), MaxFilterKeywords)
	}
	for _, k := range keywords {
		if err := ValidateKeyword(k); err != nil {
			return err
		}
	}
	return nil
}

// SanitizePath resolves a user-supplied path to its clean, absolute form
// and rejects paths containing null bytes, control characters, traversal
// sequences, or Windows reserved device names.
func SanitizePath(rawPath string) (string, error) {
	if rawPath == "" {
		return "", fmt.Errorf("empty path")
	}

	if strings.ContainsRune(rawPath, 0) {
		return "", fmt.Errorf("path contains null byte")
	}

	for _, r := range rawPath {
		if r > 0 && r < 0x20 {
			return "", fmt.Errorf("path contains control character (0x%02x)", r)
		}
	}

	cleaned := filepath.Clean(rawPath)

	if runtime.GOOS == "windows" {
		base := filepath.Base(cleaned)
		nameOnly := strings.TrimSuffix(base, filepath.Ext(base))
		if windowsReservedNames[strings.ToUpper(nameOnly)] {
			return "", fmt.Errorf("path uses reserved device name: %s", base)
		}
	}

	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(abs, `\\.\`) || strings.HasPrefix(abs, `\\?\`) {
			return "", fmt.Errorf("path resolves to a Windows device path: %s", abs)
		}
	}

	return abs, nil
}

// ValidateDBPath sanitizes a user-supplied database path and ensures it
// points to an existing regular file (not a directory or other special file).
func ValidateDBPath(rawPath string) (string, error) {
	abs, err := SanitizePath(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid database path: %w", err)
	}

	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("file not found: %s", abs)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", abs)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symbolic links are not allowed: %s", abs)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file: %s", abs)
	}

	return abs, nil
}

// ValidateOutputPath sanitizes a user-supplied output file path and
// ensures it does not escape the current working directory via traversal.
func ValidateOutputPath(rawPath string) (string, error) {
	abs, err := SanitizePath(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid output path: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine working directory: %w", err)
	}

	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return "", fmt.Errorf("invalid output path: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("output path escapes working directory: %s", rawPath)
	}

	if info, err := os.Lstat(abs); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symbolic links are not allowed for output: %s", abs)
		}
		if info.IsDir() {
			return "", fmt.Errorf("output path is a directory: %s", abs)
		}
	}

	return abs, nil
}

// SanitizeEnvPath sanitizes a path obtained from an environment variable.
// On failure it returns an empty string so the caller can silently treat
// the variable as unset.
func SanitizeEnvPath(raw string) string {
	if raw == "" {
		return ""
	}
	abs, err := SanitizePath(raw)
	if err != nil {
		return ""
	}
	return abs
}

// ValidateProfileName validates a Chrome profile directory name.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if len(name) > MaxProfileNameLen {
		return fmt.Errorf("profile name too long (%d bytes, max %d)", len(name), MaxProfileNameLen)
	}
	for i, r := range name {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			// always allowed
		case r == ' ' || r == '_' || r == '-':
			if i == 0 {
				return fmt.Errorf("invalid --profile value %q: must start with a letter or digit", name)
			}
		default:
			return fmt.Errorf("invalid --profile value %q: contains invalid character %q"+
				" (only ASCII letters, digits, spaces, hyphens, and underscores are allowed)", name, r)
		}
	}
	return nil
}

// ValidateDateFlag parses a --since or --until flag value as a YYYY-MM-DD date.
func ValidateDateFlag(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected format YYYY-MM-DD", s)
	}
	return t, nil
}

// ValidateFilterPath sanitizes a user-supplied filter file path and
// ensures it points to an existing regular file.
func ValidateFilterPath(rawPath string) (string, error) {
	abs, err := SanitizePath(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid filter file path: %w", err)
	}

	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return "", fmt.Errorf("filter path is a directory, not a file: %s", abs)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symbolic links are not allowed for filter files: %s", abs)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("filter path is not a regular file: %s", abs)
	}

	return abs, nil
}
