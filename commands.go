// Package main — commands.go: CLI flag definitions, flag parsing, validation,
// command dispatch helpers, and command handler functions for the Browser History
// Manager.
//
// # Responsibilities
//
// This file owns:
//   - knownValueFlags / knownBoolFlags / validCommands: flag and command registries.
//   - parseFlags: parses raw os.Args slices into typed flag maps.
//   - validateFlagValues: validates all parsed flag values before any I/O.
//   - confirm: prompts the user for a yes/no answer.
//   - cmdPreview / cmdDelete / cmdExport / cmdBrowsers: one function
//     per CLI subcommand; each function contains the complete logic for that
//     command and is called directly from main().
//   - writePreview / writeCSV: output-formatting helpers used by the above.
//   - printUsage: prints the help text shown for --help or unknown commands.
//   - exitErr: writes a formatted error to stderr and exits with code 1.
//
// # Design Notes
//
// All flag validation is intentionally front-loaded in validateFlagValues so
// that command handlers (cmdXxx) can assume their inputs are clean. I/O,
// database access, and business logic are delegated to the internal/history
// package, which is imported directly by the command handlers.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chrome-history-manager/internal/history"
)

// exitErr prints an error message to stderr and exits with code 1.
func exitErr(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
	os.Exit(1)
}

// knownValueFlags lists all valid flags that take a value argument.
var knownValueFlags = map[string]bool{
	"include": true,
	"exclude": true,
	"match":   true, // alias for --include (backward compat)
	"protect": true, // alias for --exclude (backward compat)
	"browser": true,
	"db":      true,
	"out":     true,
	"limit":   true,
	"profile": true,
	"since":   true,
	"until":   true,
}

// flagAliases maps deprecated flag names to their canonical replacements.
var flagAliases = map[string]string{
	"match":   "include",
	"protect": "exclude",
}

// knownBoolFlags lists all valid boolean flags.
var knownBoolFlags = map[string]bool{
	"yes": true,
}

// maxFlagValueLen is the maximum allowed length for string flag values.
const maxFlagValueLen = 4096

// validCommands lists all accepted subcommands.
var validCommands = map[string]bool{
	"preview":  true,
	"delete":   true,
	"export":   true,
	"browsers": true,
}

// parseFlags parses command-line arguments into value flags and boolean flags,
// validating flag names and detecting missing values.
func parseFlags(args []string) (map[string]string, map[string]bool, error) {
	flags := make(map[string]string)
	boolFlags := make(map[string]bool)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--yes" || arg == "-y" {
			boolFlags["yes"] = true
			continue
		}

		// -n is a short form of --limit (e.g. -n 25).
		if arg == "-n" {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag -n requires a value")
			}
			flags["limit"] = args[i+1]
			i++
			continue
		}

		if strings.HasPrefix(arg, "--") {
			name := arg[2:]
			if name == "" {
				return nil, nil, fmt.Errorf("invalid empty flag: --")
			}

			if knownBoolFlags[name] {
				boolFlags[name] = true
				continue
			}

			if !knownValueFlags[name] {
				return nil, nil, fmt.Errorf("unknown flag: --%s", name)
			}

			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag --%s requires a value", name)
			}

			value := args[i+1]
			if strings.HasPrefix(value, "--") && knownValueFlags[value[2:]] {
				return nil, nil, fmt.Errorf("flag --%s appears to be missing its value (got '%s')", name, value)
			}

			// Normalise deprecated aliases to canonical names.
			if canonical, ok := flagAliases[name]; ok {
				name = canonical
			}
			flags[name] = value
			i++
			continue
		}

		return nil, nil, fmt.Errorf("unexpected argument: %s", arg)
	}

	return flags, boolFlags, nil
}

// validateFlagValues validates the semantic content of parsed flag values.
func validateFlagValues(flags map[string]string) error {
	for name, value := range flags {
		if len(value) > maxFlagValueLen {
			return fmt.Errorf("--%s value too long (%d chars, max %d)", name, len(value), maxFlagValueLen)
		}
	}

	// Validate search query inputs for null bytes and shell-injection-prone
	// control characters. These values are used only for in-memory substring
	// matching and never reach SQL or a shell, but early rejection at flag
	// validation time provides consistent, defence-in-depth protection.
	for _, flag := range []string{"include", "exclude"} {
		if v, ok := flags[flag]; ok && v != "" {
			if err := history.ValidateSearchInput(v); err != nil {
				return fmt.Errorf("--%s: %w", flag, err)
			}
		}
	}

	if browser, ok := flags["browser"]; ok && browser != "" {
		if err := history.ValidateBrowserName(browser); err != nil {
			return err
		}
	}

	// Early path syntax check for --db: catch null bytes and control characters
	// before the full validateDBPath check runs later in resolveDBPath.
	// Existence and regular-file checks are deferred to resolveDBPath, since the
	// default DB path is resolved by browser detection when --db is absent.
	if db, ok := flags["db"]; ok && db != "" {
		if _, err := history.SanitizePath(db); err != nil {
			return fmt.Errorf("--db: %w", err)
		}
	}

	if out, ok := flags["out"]; ok && out != "" {
		ext := strings.ToLower(filepath.Ext(out))
		if ext != "" && ext != ".csv" {
			return fmt.Errorf("--out file must have .csv extension (got '%s')", ext)
		}
		// Early path validation for --out: catch null bytes, control characters,
		// and traversal sequences before cmdExport runs validateOutputPath.
		// validateOutputPath also enforces that the path stays within the current
		// working directory, preventing writes to arbitrary filesystem locations.
		if _, err := history.ValidateOutputPath(out); err != nil {
			return fmt.Errorf("--out: %w", err)
		}
	}

	if limitStr, ok := flags["limit"]; ok && limitStr != "" {
		if _, err := validateLimit(limitStr); err != nil {
			return err
		}
	}

	if profile, ok := flags["profile"]; ok && profile != "" {
		if err := history.ValidateProfileName(profile); err != nil {
			return err
		}
	}

	// Validate --since and --until date formats and logical ordering.
	sinceTime, err := history.ValidateDateFlag(flags["since"])
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	untilTime, err := history.ValidateDateFlag(flags["until"])
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}
	if !sinceTime.IsZero() && !untilTime.IsZero() && sinceTime.After(untilTime) {
		return fmt.Errorf("--since (%s) must not be later than --until (%s)",
			flags["since"], flags["until"])
	}

	return nil
}

// confirm prompts the user for a yes/no confirmation and returns true if accepted.
func confirm(msg string) bool {
	fmt.Printf("\n%s (y/N): ", msg)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
}

func cmdPreview(dbPath, includeVal, excludeVal string, limit int, since, until time.Time) {
	matchList, err := history.LoadFilters(includeVal)
	if err != nil {
		exitErr("invalid --include value: %v", err)
	}
	protectList, err := history.LoadFilters(excludeVal)
	if err != nil {
		exitErr("invalid --exclude value: %v", err)
	}

	if len(matchList) == 0 {
		exitErr("--include is required.")
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		exitErr("%v", err)
	}
	defer history.CleanupTmp(tmpPath)
	defer db.Close()

	entries, err := history.GetAllURLs(db)
	if err != nil {
		exitErr("%v", err)
	}

	matched := history.FilterEntries(entries, matchList, protectList)
	matched = history.FilterByDateRange(matched, since, until)
	history.SortEntries(matched)

	writePreview(os.Stdout, matched, len(entries), limit)
}

// writePreview formats and writes preview output to w, showing matched entries
// with their visit counts, titles, and URLs. At most limit entries are displayed;
// when limit is 0 or negative, defaultPreviewLimit is used.
func writePreview(w io.Writer, matched []history.HistoryEntry, totalCount int, limit int) {
	if limit <= 0 {
		limit = defaultPreviewLimit
	}
	fmt.Fprintf(w, "%d matched out of %d total\n\n", len(matched), totalCount)
	display := limit
	if len(matched) < display {
		display = len(matched)
	}
	for _, e := range matched[:display] {
		fmt.Fprintf(w, "  [%d] %-40s  %s\n", e.VisitCount, history.Truncate(e.Title, 40), history.Truncate(e.URL, 80))
	}
	if len(matched) > limit {
		fmt.Fprintf(w, "\n  ... and %d more\n", len(matched)-limit)
	}
}

func cmdDelete(dbPath, includeVal, excludeVal string, yes bool, since, until time.Time) {
	matchList, err := history.LoadFilters(includeVal)
	if err != nil {
		exitErr("invalid --include value: %v", err)
	}
	protectList, err := history.LoadFilters(excludeVal)
	if err != nil {
		exitErr("invalid --exclude value: %v", err)
	}

	if len(matchList) == 0 {
		exitErr("--include is required.")
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		exitErr("%v", err)
	}

	entries, err := history.GetAllURLs(db)
	if err != nil {
		db.Close()
		history.CleanupTmp(tmpPath)
		exitErr("%v", err)
	}
	db.Close()
	history.CleanupTmp(tmpPath)

	matched := history.FilterEntries(entries, matchList, protectList)
	matched = history.FilterByDateRange(matched, since, until)

	fmt.Printf("%d matched out of %d total\n", len(matched), len(entries))
	if len(matched) == 0 {
		fmt.Println("Nothing to delete.")
		return
	}

	if !yes && !confirm(fmt.Sprintf("Delete %d items? This cannot be undone.", len(matched))) {
		fmt.Println("Cancelled.")
		return
	}

	// Queue-then-apply: add all matched entries to the shared DeleteQueue,
	// then flush with a single backup. This uses the same queue abstraction
	// available to the GUI, ensuring consistent delete semantics.
	queue := history.NewDeleteQueue()
	queue.AddMany(matched)

	backupPath, deleted, err := queue.FlushWithBackup(dbPath)
	if err != nil {
		exitErr("%v", err)
	}
	fmt.Printf("Backup: %s\n", backupPath)
	fmt.Printf("Deleted %d items.\n", deleted)
}

func cmdExport(dbPath, includeVal, excludeVal, outFile string, since, until time.Time) {
	matchList, err := history.LoadFilters(includeVal)
	if err != nil {
		exitErr("invalid --include value: %v", err)
	}
	protectList, err := history.LoadFilters(excludeVal)
	if err != nil {
		exitErr("invalid --exclude value: %v", err)
	}

	if len(matchList) == 0 {
		matchList = []string{"*"}
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		exitErr("%v", err)
	}
	defer history.CleanupTmp(tmpPath)
	defer db.Close()

	entries, err := history.GetAllURLs(db)
	if err != nil {
		exitErr("%v", err)
	}

	matched := history.FilterEntries(entries, matchList, protectList)
	matched = history.FilterByDateRange(matched, since, until)
	history.SortEntries(matched)

	if outFile == "" {
		outFile = "history_export.csv"
	}

	safeOut, err := history.ValidateOutputPath(outFile)
	if err != nil {
		exitErr("%v", err)
	}

	if err := history.WriteCSVFile(safeOut, matched); err != nil {
		exitErr("failed to write CSV: %v", err)
	}

	fmt.Printf("%d items exported to %s\n", len(matched), safeOut)
}

// cmdBrowsers lists all detected Chromium-based browsers and their
// History DB paths.
func cmdBrowsers() {
	browsers := history.DetectBrowserPaths()
	if len(browsers) == 0 {
		fmt.Println("No Chromium-based browser found.")
		return
	}
	fmt.Println("Detected browsers:")
	for name, path := range browsers {
		fmt.Printf("  %-10s %s\n", name, path)
	}
}

// writeCSV writes history entries to w in CSV format with a UTF-8 BOM prefix
// for Excel compatibility, delegating to the shared history.WriteCSV function.
func writeCSV(w io.Writer, entries []history.HistoryEntry) error {
	return history.WriteCSV(w, entries)
}

func printUsage() {
	fmt.Printf(`History Manager %s

Usage:
  browser-history-manager preview  --include FILTER [--exclude FILTER] [--limit N] [--since YYYY-MM-DD] [--until YYYY-MM-DD] [--browser NAME] [--profile NAME] [--db PATH]
  browser-history-manager delete   --include FILTER [--exclude FILTER] [--since YYYY-MM-DD] [--until YYYY-MM-DD] [--yes] [--browser NAME] [--profile NAME] [--db PATH]
  browser-history-manager export   [--include FILTER] [--exclude FILTER] [--out FILE] [--since YYYY-MM-DD] [--until YYYY-MM-DD] [--browser NAME] [--profile NAME] [--db PATH]
  browser-history-manager browsers
  browser-history-manager --version

Filters:
  --include   Include filter — show entries matching these keywords (file path, comma-separated, or * = all)
  --exclude   Exclude filter — hide entries matching these keywords (overrides --include)
              Aliases: --match (= --include), --protect (= --exclude)

Options:
  --browser   Target browser (chrome, chromium, edge, opera)
  --profile   Browser profile directory name (default: Default, e.g. "Profile 1")
  --db        History DB path (direct)
  --limit -n  Maximum number of entries to display in preview (1-%d, default %d)
  --out       CSV output filename
  --since     Lower date bound for last visit (inclusive, YYYY-MM-DD)
  --until     Upper date bound for last visit (inclusive, YYYY-MM-DD)
  --yes       Skip confirmation prompts (for scripting)

Examples:
  browser-history-manager preview --include "google.com,youtube.com"
  browser-history-manager preview --include include.txt --exclude exclude.txt
  browser-history-manager preview --include "*" --limit 10
  browser-history-manager preview --include "*" --since 2026-01-01 --until 2026-01-31
  browser-history-manager preview --include "github.com" --profile "Profile 1"
  browser-history-manager delete --include "*" --exclude "scholar"
  browser-history-manager delete --include "ads.com" --yes
  browser-history-manager export --include "*" --out all.csv
  browser-history-manager export --include "*" --since 2026-01-01
  browser-history-manager export --browser edge --include "github.com"
  browser-history-manager browsers
`, version, maxPreviewLimit, defaultPreviewLimit)
}
