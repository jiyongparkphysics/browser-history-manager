// Package main — cli_integration_test.go: end-to-end integration tests for the
// CLI binary. These tests build the actual binary, run it via os/exec against
// seeded test databases, and verify stdout, stderr, and exit codes.
//
// This file complements the existing unit/integration tests (commands_test.go,
// preview_integration_test.go, etc.) by exercising the full binary including
// main(), flag parsing, command dispatch, and output formatting.
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// testBinary holds the path to the compiled CLI binary. It is built once
// per test run by TestMain or the first test that needs it.
var (
	testBinaryPath string
	buildOnce      sync.Once
	buildErr       error
)

// ensureBinary compiles the CLI binary into a temporary directory.
// The result is cached so subsequent tests reuse the same binary.
func ensureBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir := os.TempDir()
		name := "browser-history-manager-test"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		testBinaryPath = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", testBinaryPath, ".")
		cmd.Dir = projectRoot()
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = err
			t.Logf("go build output: %s", string(out))
		}
	})
	if buildErr != nil {
		t.Fatalf("failed to build CLI binary: %v", buildErr)
	}
	t.Cleanup(func() {
		// Don't remove — other tests may still need it.
		// The OS temp dir will clean it up eventually.
	})
	return testBinaryPath
}

// projectRoot returns the project root directory.
func projectRoot() string {
	// This file lives in the project root, so just use the working directory.
	wd, err := os.Getwd()
	if err != nil {
		panic("cannot determine working directory: " + err.Error())
	}
	return wd
}

// runCLI runs the CLI binary with the given arguments and returns stdout,
// stderr, and any error. A non-zero exit code is returned as an *exec.ExitError.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	bin := ensureBinary(t)
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// runCLIInDir runs the CLI binary in a specific working directory.
func runCLIInDir(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	bin := ensureBinary(t)
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// requireSuccess asserts the command exited with code 0.
func requireSuccess(t *testing.T, stderr string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstderr: %s", err, stderr)
	}
}

// requireFailure asserts the command exited with a non-zero code.
func requireFailure(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected non-zero exit code, but command succeeded")
	}
}

// seedTestDB creates a test database in a temp directory, seeds it, and
// returns the path to the History file. The caller should close the returned
// testDB when they no longer need the SQL connection (before passing the
// path to the CLI binary, which opens its own connection).
func seedTestDB(t *testing.T) (*testDB, string) {
	t.Helper()
	tdb := newTestDB(t)
	tdb.SeedRealisticData()
	return tdb, tdb.Path
}

// --- Preview Command Tests ---

func TestCLI_Preview_BasicSearch(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "google", "--db", dbPath)
	requireSuccess(t, stderr, err)

	// Should contain the "matched out of" header.
	if !strings.Contains(stdout, "matched out of") {
		t.Errorf("expected 'matched out of' header, got:\n%s", stdout)
	}

	// Google entries should appear in the output.
	if !strings.Contains(stdout, "google") {
		t.Errorf("expected 'google' in output, got:\n%s", stdout)
	}
}

func TestCLI_Preview_WildcardMatchAll(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "*", "--db", dbPath)
	requireSuccess(t, stderr, err)

	// Should report 14 matched out of 14 total (realistic data has 14 visible entries).
	if !strings.Contains(stdout, "14 matched out of 14 total") {
		t.Errorf("expected '14 matched out of 14 total', got:\n%s", stdout)
	}
}

func TestCLI_Preview_WithProtect(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "google", "--protect", "scholar", "--db", dbPath)
	requireSuccess(t, stderr, err)

	// Scholar entries should be excluded.
	if strings.Contains(stdout, "scholar") {
		t.Errorf("protected 'scholar' entry should not appear in output, got:\n%s", stdout)
	}

	// Other google entries should still match.
	if !strings.Contains(stdout, "google") {
		t.Errorf("expected 'google' entries in output, got:\n%s", stdout)
	}
}

func TestCLI_Preview_WithLimit(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "*", "--db", dbPath, "--limit", "3")
	requireSuccess(t, stderr, err)

	// Count entry lines (lines starting with "  [" are entries).
	lines := strings.Split(stdout, "\n")
	entryCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			entryCount++
		}
	}
	if entryCount != 3 {
		t.Errorf("expected 3 entry lines with --limit 3, got %d\noutput:\n%s", entryCount, stdout)
	}

	// Should mention remaining entries.
	if !strings.Contains(stdout, "... and") {
		t.Errorf("expected '... and N more' overflow indicator, got:\n%s", stdout)
	}
}

func TestCLI_Preview_ShortLimitFlag(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "*", "--db", dbPath, "-n", "2")
	requireSuccess(t, stderr, err)

	lines := strings.Split(stdout, "\n")
	entryCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			entryCount++
		}
	}
	if entryCount != 2 {
		t.Errorf("expected 2 entry lines with -n 2, got %d", entryCount)
	}
}

func TestCLI_Preview_NoMatch_RequiresFlag(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	_, stderr, err := runCLI(t, "preview", "--db", dbPath)
	requireFailure(t, err)

	if !strings.Contains(stderr, "--include is required") {
		t.Errorf("expected '--match is required' error, got stderr:\n%s", stderr)
	}
}

func TestCLI_Preview_EmptyResults(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "nonexistent-domain-xyz", "--db", dbPath)
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "0 matched out of") {
		t.Errorf("expected '0 matched out of' header, got:\n%s", stdout)
	}
}

func TestCLI_Preview_DateRangeFilter(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	// Use a date range that should capture some but not all entries.
	// Realistic data spans from ~30 days ago to "now" (2024-06-15).
	stdout, stderr, err := runCLI(t, "preview", "--match", "*", "--db", dbPath,
		"--since", "2024-06-14", "--until", "2024-06-15")
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "matched out of") {
		t.Errorf("expected 'matched out of' header, got:\n%s", stdout)
	}

	// Should match fewer than all 14 entries.
	if strings.Contains(stdout, "14 matched") {
		t.Errorf("date range filter should reduce results from 14, got:\n%s", stdout)
	}
}

// --- Export Command Tests ---

func TestCLI_Export_BasicCSV(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	outDir := t.TempDir()
	outFile := filepath.Join(outDir, "export.csv")

	stdout, stderr, err := runCLIInDir(t, outDir, "export", "--match", "*", "--db", dbPath, "--out", "export.csv")
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "items exported to") {
		t.Errorf("expected 'items exported to' message, got:\n%s", stdout)
	}

	// Verify the CSV file was created.
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read exported CSV: %v", err)
	}

	// Check UTF-8 BOM.
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("exported CSV should start with UTF-8 BOM")
	}

	// Parse as CSV (skip BOM).
	reader := csv.NewReader(strings.NewReader(string(data[3:])))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Header + 14 data rows.
	if len(records) < 2 {
		t.Fatalf("expected at least header + data rows, got %d rows", len(records))
	}

	// Verify header.
	header := records[0]
	expectedHeader := []string{"Title", "URL", "Visit Count", "Last Visit"}
	for i, h := range expectedHeader {
		if header[i] != h {
			t.Errorf("header[%d]: expected %q, got %q", i, h, header[i])
		}
	}

	// Should have 14 data rows (all visible entries).
	if len(records)-1 != 14 {
		t.Errorf("expected 14 data rows, got %d", len(records)-1)
	}
}

func TestCLI_Export_WithMatchFilter(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	outDir := t.TempDir()

	stdout, stderr, err := runCLIInDir(t, outDir, "export", "--match", "google", "--db", dbPath, "--out", "filtered.csv")
	requireSuccess(t, stderr, err)

	// Should report fewer than 14 items.
	if strings.Contains(stdout, "14 items") {
		t.Errorf("filtered export should have fewer than 14 items, got:\n%s", stdout)
	}

	outFile := filepath.Join(outDir, "filtered.csv")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data[3:])))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// All data rows should contain "google" in URL or title.
	for _, row := range records[1:] {
		combined := strings.ToLower(row[0] + " " + row[1])
		if !strings.Contains(combined, "google") {
			t.Errorf("row should match 'google', got: %v", row)
		}
	}
}

func TestCLI_Export_WithProtectFilter(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	outDir := t.TempDir()

	_, stderr, err := runCLIInDir(t, outDir, "export", "--match", "*", "--protect", "bank", "--db", dbPath, "--out", "protected.csv")
	requireSuccess(t, stderr, err)

	outFile := filepath.Join(outDir, "protected.csv")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data[3:])))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// No row should contain "bank".
	for _, row := range records[1:] {
		combined := strings.ToLower(row[0] + " " + row[1])
		if strings.Contains(combined, "bank") {
			t.Errorf("protected 'bank' entry should not appear in CSV, got: %v", row)
		}
	}

	// Should have 13 data rows (14 - 1 bank entry).
	if len(records)-1 != 13 {
		t.Errorf("expected 13 data rows after protecting bank, got %d", len(records)-1)
	}
}

func TestCLI_Export_DefaultFilename(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	outDir := t.TempDir()

	stdout, stderr, err := runCLIInDir(t, outDir, "export", "--match", "google", "--db", dbPath)
	requireSuccess(t, stderr, err)

	// Should use default filename "history_export.csv".
	if !strings.Contains(stdout, "history_export.csv") {
		t.Errorf("expected default filename 'history_export.csv', got:\n%s", stdout)
	}

	defaultFile := filepath.Join(outDir, "history_export.csv")
	if _, err := os.Stat(defaultFile); os.IsNotExist(err) {
		t.Error("default export file history_export.csv was not created")
	}
}

func TestCLI_Export_CSVColumnsConsistent(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	outDir := t.TempDir()

	_, stderr, err := runCLIInDir(t, outDir, "export", "--match", "*", "--db", dbPath, "--out", "cols.csv")
	requireSuccess(t, stderr, err)

	outFile := filepath.Join(outDir, "cols.csv")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data[3:])))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Every row should have exactly 4 columns.
	for i, row := range records {
		if len(row) != 4 {
			t.Errorf("row %d: expected 4 columns, got %d: %v", i, len(row), row)
		}
	}
}

func TestCLI_Export_UnicodeData(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	outDir := t.TempDir()

	_, stderr, err := runCLIInDir(t, outDir, "export", "--match", "위키백과", "--db", dbPath, "--out", "unicode.csv")
	requireSuccess(t, stderr, err)

	outFile := filepath.Join(outDir, "unicode.csv")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "한국어") {
		t.Error("exported CSV should contain Korean characters")
	}
}

func TestCLI_Export_EmptyDB(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	outDir := t.TempDir()

	stdout, stderr, err := runCLIInDir(t, outDir, "export", "--match", "*", "--db", tdb.Path, "--out", "empty.csv")
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "0 items exported") {
		t.Errorf("expected '0 items exported', got:\n%s", stdout)
	}
}

// --- Delete Command Tests ---

func TestCLI_Delete_WithYesFlag(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "delete", "--match", "doubleclick", "--db", dbPath, "--yes")
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "Deleted") {
		t.Errorf("expected 'Deleted' confirmation, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Backup:") {
		t.Errorf("expected 'Backup:' path in output, got:\n%s", stdout)
	}
}

func TestCLI_Delete_NoMatch(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "delete", "--match", "nonexistent-xyz", "--db", dbPath, "--yes")
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "Nothing to delete") {
		t.Errorf("expected 'Nothing to delete', got:\n%s", stdout)
	}
}

func TestCLI_Delete_RequiresMatch(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	_, stderr, err := runCLI(t, "delete", "--db", dbPath, "--yes")
	requireFailure(t, err)

	if !strings.Contains(stderr, "--include is required") {
		t.Errorf("expected '--match is required' error, got stderr:\n%s", stderr)
	}
}

func TestCLI_Delete_WithProtect(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	// Delete all google entries but protect scholar.
	stdout, stderr, err := runCLI(t, "delete", "--match", "google", "--protect", "scholar", "--db", dbPath, "--yes")
	requireSuccess(t, stderr, err)

	// Should delete some entries.
	if !strings.Contains(stdout, "Deleted") {
		t.Errorf("expected deletion confirmation, got:\n%s", stdout)
	}

	// Verify scholar entries are preserved by re-reading the DB.
	stdout2, stderr2, err2 := runCLI(t, "preview", "--match", "scholar", "--db", dbPath)
	requireSuccess(t, stderr2, err2)

	if !strings.Contains(stdout2, "scholar") {
		t.Error("scholar entries should be preserved after delete with --protect")
	}
}

// --- Error Handling Tests ---

func TestCLI_NoArgs_ShowsUsage(t *testing.T) {
	stdout, _, err := runCLI(t)
	// No args should show usage and exit 0.
	if err != nil {
		t.Logf("exit code: %v (may be 0 on some platforms)", err)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("expected usage text, got:\n%s", stdout)
	}
}

func TestCLI_Help_ShowsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "--help")
	if err != nil {
		t.Logf("exit code: %v", err)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("expected usage text, got:\n%s", stdout)
	}
}

func TestCLI_Version_ShowsVersion(t *testing.T) {
	stdout, _, err := runCLI(t, "--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		t.Error("--version should output a version string")
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	_, stderr, err := runCLI(t, "foobar")
	requireFailure(t, err)

	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' error, got stderr:\n%s", stderr)
	}
}

func TestCLI_UnknownFlag(t *testing.T) {
	_, stderr, err := runCLI(t, "preview", "--nonexistent", "value")
	requireFailure(t, err)

	if !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected 'unknown flag' error, got stderr:\n%s", stderr)
	}
}

func TestCLI_InvalidBrowser(t *testing.T) {
	_, stderr, err := runCLI(t, "preview", "--match", "test", "--browser", "firefox")
	requireFailure(t, err)

	// Should reject non-allowlisted browser names.
	if !strings.Contains(strings.ToLower(stderr), "browser") {
		t.Errorf("expected browser validation error, got stderr:\n%s", stderr)
	}
}

func TestCLI_InvalidLimit(t *testing.T) {
	_, stderr, err := runCLI(t, "preview", "--match", "test", "--limit", "0")
	requireFailure(t, err)

	if !strings.Contains(stderr, "limit") {
		t.Errorf("expected limit validation error, got stderr:\n%s", stderr)
	}
}

func TestCLI_InvalidDateRange(t *testing.T) {
	_, stderr, err := runCLI(t, "preview", "--match", "test", "--since", "2024-12-31", "--until", "2024-01-01")
	requireFailure(t, err)

	if !strings.Contains(stderr, "since") || !strings.Contains(stderr, "until") {
		t.Errorf("expected date range validation error, got stderr:\n%s", stderr)
	}
}

func TestCLI_InvalidDBPath(t *testing.T) {
	_, stderr, err := runCLI(t, "preview", "--match", "test", "--db", "/nonexistent/path/History")
	requireFailure(t, err)

	if stderr == "" {
		t.Error("expected error message for invalid DB path")
	}
}

func TestCLI_InvalidOutExtension(t *testing.T) {
	_, stderr, err := runCLI(t, "export", "--match", "test", "--out", "output.json")
	requireFailure(t, err)

	if !strings.Contains(stderr, ".csv") {
		t.Errorf("expected .csv extension error, got stderr:\n%s", stderr)
	}
}

// --- Browsers Command Tests ---

func TestCLI_Browsers_Runs(t *testing.T) {
	// The browsers command should always succeed (even if no browsers found).
	stdout, stderr, err := runCLI(t, "browsers")
	requireSuccess(t, stderr, err)

	// Should contain either browser listings or "No Chromium-based browser found."
	if !strings.Contains(stdout, "Detected browsers:") && !strings.Contains(stdout, "No Chromium-based browser found") {
		t.Errorf("expected browser detection output, got:\n%s", stdout)
	}
}

// --- End-to-End Pipeline Tests ---

func TestCLI_Pipeline_PreviewThenExport_ConsistentResults(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	// Preview with match filter.
	previewOut, stderr, err := runCLI(t, "preview", "--match", "google", "--db", dbPath)
	requireSuccess(t, stderr, err)

	// Export with same filter.
	outDir := t.TempDir()
	_, stderr, err = runCLIInDir(t, outDir, "export", "--match", "google", "--db", dbPath, "--out", "pipeline.csv")
	requireSuccess(t, stderr, err)

	outFile := filepath.Join(outDir, "pipeline.csv")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data[3:])))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Extract the count from preview output ("N matched out of M total").
	// The number of CSV data rows should match the preview count.
	csvDataRows := len(records) - 1 // subtract header

	// Preview shows "N matched" — extract N.
	parts := strings.Fields(previewOut)
	if len(parts) > 0 {
		previewCount := parts[0]
		expectedCount := strings.TrimSpace(previewCount)
		actualCount := fmt.Sprintf("%d", csvDataRows)
		if expectedCount != actualCount {
			t.Errorf("preview count (%s) != export CSV rows (%s)", expectedCount, actualCount)
		}
	}
}

func TestCLI_Pipeline_DeleteThenPreview_VerifiesRemoval(t *testing.T) {
	tdb, dbPath := seedTestDB(t)
	tdb.Close()

	// Delete doubleclick entries.
	_, stderr, err := runCLI(t, "delete", "--match", "doubleclick", "--db", dbPath, "--yes")
	requireSuccess(t, stderr, err)

	// Preview should no longer find doubleclick entries.
	stdout, stderr, err := runCLI(t, "preview", "--match", "doubleclick", "--db", dbPath)
	requireSuccess(t, stderr, err)

	if !strings.Contains(stdout, "0 matched") {
		t.Errorf("expected '0 matched' after deletion, got:\n%s", stdout)
	}
}

// --- Security Tests ---

func TestCLI_ControlCharInMatch_Rejected(t *testing.T) {
	// Null bytes are stripped by the OS arg passing, so test with control chars
	// that survive command-line argument passing (e.g., \x01).
	_, stderr, err := runCLI(t, "preview", "--match", "test\x01injection", "--db", "/tmp/fake")
	requireFailure(t, err)

	if !strings.Contains(strings.ToLower(stderr), "control") && !strings.Contains(strings.ToLower(stderr), "invalid") {
		t.Errorf("expected control char rejection error, got stderr:\n%s", stderr)
	}
}

func TestCLI_PathTraversal_Rejected(t *testing.T) {
	_, stderr, err := runCLI(t, "export", "--match", "test", "--out", "../../../etc/passwd.csv")
	requireFailure(t, err)

	if stderr == "" {
		t.Error("expected error for path traversal attempt")
	}
}

func TestCLI_ShellMetachars_InBrowser_Rejected(t *testing.T) {
	_, stderr, err := runCLI(t, "preview", "--match", "test", "--browser", "chrome;rm -rf /")
	requireFailure(t, err)

	if stderr == "" {
		t.Error("expected error for shell metacharacters in browser name")
	}
}

// --- Sorted Output Tests ---

func TestCLI_Preview_SortedByLastVisitDescending(t *testing.T) {
	tdb := newTestDB(t)

	// Insert entries with known visit times.
	oldest := timeToChrome(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	middle := timeToChrome(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC))
	newest := timeToChrome(time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://oldest.example.com", Title: "Oldest Entry", LastVisitTime: oldest,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://newest.example.com", Title: "Newest Entry", LastVisitTime: newest,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://middle.example.com", Title: "Middle Entry", LastVisitTime: middle,
	}, 1)
	tdb.Close()

	stdout, stderr, err := runCLI(t, "preview", "--match", "*", "--db", tdb.Path)
	requireSuccess(t, stderr, err)

	newestIdx := strings.Index(stdout, "newest.example.com")
	middleIdx := strings.Index(stdout, "middle.example.com")
	oldestIdx := strings.Index(stdout, "oldest.example.com")

	if newestIdx < 0 || middleIdx < 0 || oldestIdx < 0 {
		t.Fatalf("expected all three entries in output, got:\n%s", stdout)
	}

	if !(newestIdx < middleIdx && middleIdx < oldestIdx) {
		t.Errorf("expected newest before middle before oldest (descending order), got:\n%s", stdout)
	}
}

func TestCLI_Export_SortedByLastVisitDescending(t *testing.T) {
	tdb := newTestDB(t)

	oldest := timeToChrome(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	middle := timeToChrome(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC))
	newest := timeToChrome(time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://oldest.example.com", Title: "Oldest", LastVisitTime: oldest,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://newest.example.com", Title: "Newest", LastVisitTime: newest,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://middle.example.com", Title: "Middle", LastVisitTime: middle,
	}, 1)
	tdb.Close()

	outDir := t.TempDir()
	_, stderr, err := runCLIInDir(t, outDir, "export", "--match", "*", "--db", tdb.Path, "--out", "sorted.csv")
	requireSuccess(t, stderr, err)

	outFile := filepath.Join(outDir, "sorted.csv")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data[3:])))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 4 { // header + 3 rows
		t.Fatalf("expected 4 rows (header + 3 data), got %d", len(records))
	}

	// Rows should be in descending order: newest, middle, oldest.
	if !strings.Contains(records[1][1], "newest") {
		t.Errorf("first data row should be newest, got: %s", records[1][1])
	}
	if !strings.Contains(records[2][1], "middle") {
		t.Errorf("second data row should be middle, got: %s", records[2][1])
	}
	if !strings.Contains(records[3][1], "oldest") {
		t.Errorf("third data row should be oldest, got: %s", records[3][1])
	}
}

