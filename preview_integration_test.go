package main

// preview_integration_test.go — additional full-pipeline integration tests for
// the preview command.
//
// The core pipeline tests (ViaTempCopy, SortedByLastVisitDescending,
// CustomLimit, ProtectFilter, RealisticData, HiddenEntriesNotInTempCopy) live
// in commands_test.go. This file adds complementary scenarios that are not
// yet covered there:
//
//   - Limit equal to match count (no overflow message expected)
//   - Empty database (zero entries, zero matches)
//   - Wildcard "*" match returns every visible entry
//   - Multi-keyword match (comma-separated terms each matching independently)
//   - Visit count displayed in output matches the value stored in the DB
//   - Output structure: header, blank separator, indented entry lines
//   - Entry line field completeness: visit count, title, URL all present
//   - Limit=1 shows only the most-recent entry and correct overflow message
//   - Long title and URL are truncated to 40 and 80 runes when loaded from DB

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestPreviewPipeline_LimitEqualToMatchCount verifies that no overflow message
// appears when the limit exactly equals the number of matched entries, and that
// all matched entries are displayed.
func TestPreviewPipeline_LimitEqualToMatchCount(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	const count = 5
	for i := 0; i < count; i++ {
		tdb.InsertURL(urlEntry{
			URL:           fmt.Sprintf("https://page%d.example.com", i),
			Title:         fmt.Sprintf("Page %d", i),
			VisitCount:    1,
			LastVisitTime: now,
		})
	}
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), count) // limit == match count
	output := buf.String()

	// No overflow line when limit equals the number of matched entries.
	if strings.Contains(output, "... and") {
		t.Errorf("no overflow message expected when limit == match count: %q", output)
	}

	// All count entries must be rendered.
	entryCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryCount++
		}
	}
	if entryCount != count {
		t.Errorf("expected %d entry lines, got %d", count, entryCount)
	}
}

// TestPreviewPipeline_EmptyDatabase verifies output when the database contains
// no visible entries: the header shows "0 matched out of 0 total" and no entry
// lines are produced.
func TestPreviewPipeline_EmptyDatabase(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries from empty DB, got %d", len(entries))
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "0 matched out of 0 total") {
		t.Errorf("unexpected header for empty DB: %q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			t.Errorf("no entry lines expected for empty DB, found: %q", line)
		}
	}
}

// TestPreviewPipeline_WildcardMatchAll verifies that the wildcard keyword "*"
// matches all visible entries and each seeded domain appears in the output.
func TestPreviewPipeline_WildcardMatchAll(t *testing.T) {
	tdb := newTestDB(t)
	n := tdb.SeedMinimalData() // google, github, bank entries
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("expected %d entries, got %d", n, len(entries))
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	wantHeader := fmt.Sprintf("%d matched out of %d total", n, n)
	if !strings.Contains(output, wantHeader) {
		t.Errorf("expected header %q in output: %q", wantHeader, output)
	}

	for _, domain := range []string{"google.com", "github.com", "bank.example.com"} {
		if !strings.Contains(output, domain) {
			t.Errorf("expected domain %q in wildcard output: %q", domain, output)
		}
	}
}

// TestPreviewPipeline_MultiKeywordMatch verifies that multiple match keywords
// each independently match their respective entries and non-matching entries
// are absent from the output.
func TestPreviewPipeline_MultiKeywordMatch(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com", Title: "Google", LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.github.com", Title: "GitHub", LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.youtube.com", Title: "YouTube", LastVisitTime: now,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.example.com", Title: "Example", LastVisitTime: now,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match google OR github simultaneously (two-keyword list).
	matched := filterEntries(entries, []string{"google", "github"}, nil)
	sortEntries(matched)

	if len(matched) != 2 {
		t.Fatalf("expected 2 matched entries (google + github), got %d", len(matched))
	}

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "2 matched out of 4 total") {
		t.Errorf("unexpected header: %q", output)
	}
	for _, domain := range []string{"google.com", "github.com"} {
		if !strings.Contains(output, domain) {
			t.Errorf("%q should appear in output: %q", domain, output)
		}
	}
	for _, domain := range []string{"youtube.com", "example.com"} {
		if strings.Contains(output, domain) {
			t.Errorf("%q should not appear in output: %q", domain, output)
		}
	}
}

// TestPreviewPipeline_VisitCountsMatchDB verifies that the visit count shown
// in each entry line in the output matches the value stored in the database.
func TestPreviewPipeline_VisitCountsMatchDB(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://frequent.example.com", Title: "Frequent", LastVisitTime: now,
	}, 42)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://rare.example.com", Title: "Rare", LastVisitTime: now,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "[42]") {
		t.Errorf("expected [42] visit count for frequent entry: %q", output)
	}
	if !strings.Contains(output, "[1]") {
		t.Errorf("expected [1] visit count for rare entry: %q", output)
	}
}

// TestPreviewPipeline_OutputStructure verifies the structural layout of preview
// output:
//   - Line 0: summary header ("N matched out of M total")
//   - Line 1: blank separator (the \n\n after the header)
//   - Line 2+: indented entry lines beginning with "  ["
func TestPreviewPipeline_OutputStructure(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURL(urlEntry{
		URL: "https://example.com", Title: "Example", VisitCount: 3, LastVisitTime: now,
	})
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	lines := strings.Split(output, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %q", len(lines), output)
	}

	// Line 0: summary header.
	if !strings.Contains(lines[0], "matched out of") {
		t.Errorf("line 0 should be the summary header, got: %q", lines[0])
	}

	// Line 1: blank separator (the double-newline after the header).
	if lines[1] != "" {
		t.Errorf("line 1 should be a blank separator, got: %q", lines[1])
	}

	// Line 2: first entry, indented.
	if !strings.HasPrefix(lines[2], "  [") {
		t.Errorf("line 2 should be an indented entry line (\"  [\"), got: %q", lines[2])
	}
}

// TestPreviewPipeline_EntryLineContainsAllFields verifies that each entry line
// contains all expected fields: two-space indentation, visit count in brackets,
// the entry title, and the entry URL.
func TestPreviewPipeline_EntryLineContainsAllFields(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL:           "https://format-test.example.com/path",
		Title:         "Format Test Page",
		LastVisitTime: now,
	}, 7)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	// Locate the single entry line.
	var entryLine string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryLine = line
			break
		}
	}
	if entryLine == "" {
		t.Fatalf("no entry lines found in output: %q", output)
	}

	// Two-space indentation.
	if !strings.HasPrefix(entryLine, "  ") {
		t.Errorf("entry line should start with two spaces: %q", entryLine)
	}
	// Visit count in brackets.
	if !strings.Contains(entryLine, "[7]") {
		t.Errorf("entry line should contain [7]: %q", entryLine)
	}
	// Title.
	if !strings.Contains(entryLine, "Format Test Page") {
		t.Errorf("entry line should contain the title: %q", entryLine)
	}
	// URL.
	if !strings.Contains(entryLine, "format-test.example.com") {
		t.Errorf("entry line should contain the URL: %q", entryLine)
	}
}

// TestPreviewPipeline_LimitOne verifies that limit=1 shows only the single
// most-recently-visited entry and produces a "... and 2 more" overflow message
// for a three-entry database.
func TestPreviewPipeline_LimitOne(t *testing.T) {
	tdb := newTestDB(t)

	base := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	tdb.InsertURL(urlEntry{
		URL:           "https://first.example.com",
		Title:         "First",
		VisitCount:    1,
		LastVisitTime: timeToChrome(base),
	})
	tdb.InsertURL(urlEntry{
		URL:           "https://second.example.com",
		Title:         "Second",
		VisitCount:    1,
		LastVisitTime: timeToChrome(base.Add(-1 * time.Hour)),
	})
	tdb.InsertURL(urlEntry{
		URL:           "https://third.example.com",
		Title:         "Third",
		VisitCount:    1,
		LastVisitTime: timeToChrome(base.Add(-2 * time.Hour)),
	})
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), 1)
	output := buf.String()

	// Only one entry line should be rendered.
	entryCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryCount++
		}
	}
	if entryCount != 1 {
		t.Errorf("expected 1 entry line with limit=1, got %d", entryCount)
	}

	// The most-recent entry (base time) must be the one shown.
	if !strings.Contains(output, "first.example.com") {
		t.Errorf("the most-recent entry (first.example.com) should be shown: %q", output)
	}

	// Overflow: 3 - 1 = 2 more.
	if !strings.Contains(output, "... and 2 more") {
		t.Errorf("expected '... and 2 more' overflow message: %q", output)
	}
}

// TestPreviewPipeline_TitleAndURLTruncationViaDB verifies that long titles
// (> 40 runes) and long URLs (> 80 runes) loaded from a real database are
// truncated correctly in preview output.
func TestPreviewPipeline_TitleAndURLTruncationViaDB(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	longTitle := "This Is A Very Long Title That Definitely Exceeds Forty Characters By Quite A Lot"
	longURL := "https://example.com/" + strings.Repeat("very/long/path/segment/", 10)

	tdb.InsertURL(urlEntry{
		URL:           longURL,
		Title:         longTitle,
		VisitCount:    1,
		LastVisitTime: now,
	})
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	// Full (untrimmed) title must NOT appear.
	if strings.Contains(output, longTitle) {
		t.Error("full long title should be truncated in preview output")
	}
	// Full (untrimmed) URL must NOT appear.
	if strings.Contains(output, longURL) {
		t.Error("full long URL should be truncated in preview output")
	}

	// Truncated prefix of the title must appear.
	wantTitle := truncate(longTitle, 40)
	if !strings.Contains(output, wantTitle) {
		t.Errorf("truncated title %q should appear in output: %q", wantTitle, output)
	}
	// Truncated prefix of the URL must appear.
	wantURL := truncate(longURL, 80)
	if !strings.Contains(output, wantURL) {
		t.Errorf("truncated URL %q should appear in output: %q", wantURL, output)
	}
}

// TestPreviewPipeline_NoMatchReturnsZero verifies that when no entries match
// the filter, the output header shows "0 matched out of N total" and no entry
// lines are produced.
func TestPreviewPipeline_NoMatchReturnsZero(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example", LastVisitTime: now,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com", Title: "GitHub", LastVisitTime: now,
	}, 2)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Filter for a domain that does not exist in the DB.
	matched := filterEntries(entries, []string{"nonexistentdomain99999"}, nil)
	sortEntries(matched)

	if len(matched) != 0 {
		t.Fatalf("expected 0 matches for unknown domain, got %d", len(matched))
	}

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "0 matched out of 2 total") {
		t.Errorf("unexpected header for zero-match result: %q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			t.Errorf("no entry lines expected for zero-match result, found: %q", line)
		}
	}
}
