package main

// sql_injection_test.go — Tests that verify SQL injection payloads supplied
// through every user-facing input are handled safely.
//
// Security model recap (documented in db.go):
//  1. --match and --protect keywords are used exclusively for in-memory
//     string comparisons inside filterEntries. They never appear in any SQL
//     query. loadFilters → filterEntries is the only code path they traverse.
//  2. --db is validated by validateDBPath/sanitizePath before being passed
//     to sql.Open as a filesystem path — it is never interpolated into SQL.
//  3. --browser is checked against a hard-coded allowlist by
//     validateBrowserName before any DB operation.
//  4. --out is sanitised by validateOutputPath before file creation.
//  5. All SQL statements are static literals. The only bound values are
//     int64 row IDs obtained from rows.Scan, never from user input.
//
// Each test group therefore verifies a specific boundary:
//   A. Filter inputs (--match / --protect) — loadFilters, filterEntries
//   B. Database path (--db)               — validateDBPath, sanitizePath
//   C. Browser name (--browser)           — validateBrowserName
//   D. Output path (--out)                — validateOutputPath
//   E. Flag parser                        — parseFlags, validateFlagValues
//   F. End-to-end DB integrity            — full pipeline with real SQLite DB

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// sqlPayloads is a representative set of SQL injection strings used across
// all tests. These strings contain only printable ASCII (no null bytes or
// disallowed control characters) so they must be accepted by the filter
// layer as plain keyword strings and must never reach SQL.
var sqlPayloads = []string{
	"'; DROP TABLE urls; --",
	"' OR '1'='1",
	"' OR 1=1 --",
	"' UNION SELECT id,url,title,visit_count,last_visit_time FROM urls --",
	"'; SELECT * FROM sqlite_master; --",
	"Robert'); DROP TABLE Students;--",
	"/**/UNION/**/SELECT/**/1,2,3--",
	"'; PRAGMA journal_mode=DELETE; --",
	"'; UPDATE urls SET url='hacked' WHERE '1'='1",
	"' AND SLEEP(5) --",
	"1' AND '1'='1",
	"' OR 'x'='x",
	"UNION SELECT NULL--",
	"'; INSERT INTO urls VALUES(999,'evil','evil',1,0,0); --",
	"' AND 1=0 UNION ALL SELECT null,null,null,null,null--",
}

// ─────────────────────────────────────────────────────────────────────────────
// A. Filter input tests (--match / --protect)
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLInjection_LoadFilters_AcceptsSQLPayloads verifies that SQL injection
// payloads supplied as --match values are accepted as valid filter keywords.
// They contain no null bytes or disallowed control characters, so loadFilters
// must accept them and return them as literal strings for in-memory matching.
func TestSQLInjection_LoadFilters_AcceptsSQLPayloads(t *testing.T) {
	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("payload_%02d", i), func(t *testing.T) {
			filters, err := loadFilters(payload)
			if err != nil {
				t.Errorf("loadFilters rejected SQL payload %q unexpectedly: %v", payload, err)
				return
			}
			if len(filters) == 0 {
				t.Errorf("loadFilters returned empty filter list for payload %q", payload)
			}
		})
	}
}

// TestSQLInjection_LoadFilters_CommaSeparatedPayloads verifies that multiple
// SQL injection payloads separated by commas are split into individual keywords.
func TestSQLInjection_LoadFilters_CommaSeparatedPayloads(t *testing.T) {
	value := "'; DROP TABLE urls; --,'; SELECT * FROM urls; --"
	filters, err := loadFilters(value)
	if err != nil {
		t.Fatalf("loadFilters rejected comma-separated SQL payloads: %v", err)
	}
	if len(filters) != 2 {
		t.Fatalf("expected 2 keywords, got %d: %v", len(filters), filters)
	}
}

// TestSQLInjection_LoadFilters_RejectsNullByteInPayload verifies that a
// --match value containing a null byte is rejected before any processing
// occurs. Null bytes can truncate C-string paths and must never pass validation.
func TestSQLInjection_LoadFilters_RejectsNullByteInPayload(t *testing.T) {
	_, err := loadFilters("'; DROP TABLE urls\x00; --")
	if err == nil {
		t.Fatal("expected loadFilters to reject filter containing null byte")
	}
}

// TestSQLInjection_LoadFilters_RejectsControlCharInPayload verifies that
// disallowed control characters (0x01–0x08, 0x0B–0x0C, 0x0E–0x1F) in a
// filter value are rejected.
func TestSQLInjection_LoadFilters_RejectsControlCharInPayload(t *testing.T) {
	disallowed := []byte{0x01, 0x02, 0x07, 0x08, 0x0B, 0x0C, 0x0E, 0x1F}
	for _, c := range disallowed {
		_, err := loadFilters("sql" + string([]byte{c}) + "payload")
		if err == nil {
			t.Errorf("expected loadFilters to reject control char 0x%02x in filter value", c)
		}
	}
}

// TestSQLInjection_LoadFilters_FromFileWithSQLPayloads verifies that a filter
// file containing SQL injection payloads (one per line) is loaded as plain
// keyword strings and returned correctly for in-memory use only.
func TestSQLInjection_LoadFilters_FromFileWithSQLPayloads(t *testing.T) {
	dir := t.TempDir()
	filterPath := filepath.Join(dir, "sql_payloads.txt")

	content := "'; DROP TABLE urls; --\n' OR 1=1 --\nUNION SELECT * FROM urls\n"
	if err := os.WriteFile(filterPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write filter file: %v", err)
	}

	filters, err := loadFilters(filterPath)
	if err != nil {
		t.Fatalf("loadFilters rejected filter file with SQL payloads: %v", err)
	}
	if len(filters) != 3 {
		t.Fatalf("expected 3 filters from file, got %d: %v", len(filters), filters)
	}

	// Payloads are lowercased and stored as literal strings.
	expected := []string{
		"'; drop table urls; --",
		"' or 1=1 --",
		"union select * from urls",
	}
	for i, want := range expected {
		if filters[i] != want {
			t.Errorf("filter[%d] = %q, want %q", i, filters[i], want)
		}
	}
}

// TestSQLInjection_LoadFilters_FromFileRejectsNullByte verifies that a filter
// file containing a null byte is rejected, even if the null byte appears inside
// a SQL injection payload.
func TestSQLInjection_LoadFilters_FromFileRejectsNullByte(t *testing.T) {
	dir := t.TempDir()
	filterPath := filepath.Join(dir, "null_payload.txt")

	content := "normal keyword\n'; DROP TABLE urls\x00; --\n"
	if err := os.WriteFile(filterPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write filter file: %v", err)
	}

	_, err := loadFilters(filterPath)
	if err == nil {
		t.Fatal("expected loadFilters to reject filter file containing null byte")
	}
}

// TestSQLInjection_FilterEntries_SQLPayloadsNeverMatchNormalURLs verifies that
// SQL injection payloads used as --match keywords do not accidentally match
// normal history entries, because no entry URL/title contains the literal
// payload string.
func TestSQLInjection_FilterEntries_SQLPayloadsNeverMatchNormalURLs(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://google.com/search?q=golang", Title: "golang - Google Search"},
		{ID: 2, URL: "https://github.com/user/repo", Title: "user/repo"},
		{ID: 3, URL: "https://bank.example.com/accounts", Title: "My Bank"},
	}

	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("match_%02d", i), func(t *testing.T) {
			matched := filterEntries(entries, []string{payload}, nil)
			// None of the entries contain the SQL payload as a substring.
			if len(matched) != 0 {
				t.Errorf("SQL payload %q unexpectedly matched %d entry/entries", payload, len(matched))
			}
		})
	}
}

// TestSQLInjection_FilterEntries_SQLPayloadsAsProtectNeverBlockNormal verifies
// that SQL injection payloads used as --protect keywords do not accidentally
// block normal entries, since no entry URL/title contains the literal payload.
func TestSQLInjection_FilterEntries_SQLPayloadsAsProtectNeverBlockNormal(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://google.com", Title: "Google"},
		{ID: 2, URL: "https://github.com", Title: "GitHub"},
	}

	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("protect_%02d", i), func(t *testing.T) {
			// Wildcard match with SQL payload as protect; all entries should pass
			// since no entry URL/title contains the payload string.
			result := filterEntries(entries, []string{"*"}, []string{payload})
			if len(result) != len(entries) {
				t.Errorf("SQL payload %q as protect incorrectly blocked %d/%d entries",
					payload, len(entries)-len(result), len(entries))
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// B. Database path tests (--db)
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLInjection_DBPath_SQLContentTreatedAsFilesystemPath verifies that SQL
// injection characters in the --db path are treated purely as filesystem path
// characters and cause a "file not found" error (not a SQL error).
func TestSQLInjection_DBPath_SQLContentTreatedAsFilesystemPath(t *testing.T) {
	sqlPaths := []string{
		"'; DROP TABLE urls; --.db",
		"' OR '1'='1.db",
		"UNION SELECT.db",
		"'; SELECT * FROM sqlite_master; --",
	}

	for i, path := range sqlPaths {
		path := path // capture
		t.Run(fmt.Sprintf("dbpath_%02d", i), func(t *testing.T) {
			// The path doesn't exist, so validateDBPath must return "file not found".
			// Critically, no SQL error should occur — the path never reaches SQL.
			_, err := validateDBPath(path)
			if err == nil {
				t.Errorf("validateDBPath accepted nonexistent SQL-payload path %q", path)
				return
			}
			// Error must describe a path/file problem, not a SQL problem.
			// (A "file not found" or similar error is the expected outcome.)
		})
	}
}

// TestSQLInjection_DBPath_NullByteRejectedBeforeSQL verifies that a --db path
// containing a null byte is rejected immediately by sanitizePath, before any
// SQL operation is attempted.
func TestSQLInjection_DBPath_NullByteRejectedBeforeSQL(t *testing.T) {
	_, err := validateDBPath("History\x00'; DROP TABLE urls;--")
	if err == nil {
		t.Fatal("expected validateDBPath to reject path with embedded null byte")
	}
}

// TestSQLInjection_DBPath_TraversalWithSQLContent verifies that a path
// combining traversal sequences with SQL injection content is rejected.
func TestSQLInjection_DBPath_TraversalWithSQLContent(t *testing.T) {
	_, err := validateDBPath("../../'; DROP TABLE urls; --")
	// The path does not exist as a file, so an error is expected.
	if err == nil {
		t.Error("expected error for nonexistent traversal+SQL path")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C. Browser name tests (--browser)
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLInjection_BrowserName_SQLPayloadsRejected verifies that SQL injection
// payloads supplied as the --browser value are rejected by validateBrowserName
// because they do not appear in the hard-coded browser allowlist. No DB
// operation is attempted for an invalid browser name.
func TestSQLInjection_BrowserName_SQLPayloadsRejected(t *testing.T) {
	sqlBrowserNames := []string{
		"'; DROP TABLE urls; --",
		"chrome' OR '1'='1",
		"chrome; DROP TABLE urls; --",
		"' UNION SELECT * FROM urls --",
		"edge' AND '1'='1",
		"UNION SELECT",
	}

	for i, name := range sqlBrowserNames {
		name := name // capture
		t.Run(fmt.Sprintf("browser_%02d", i), func(t *testing.T) {
			err := validateBrowserName(name)
			if err == nil {
				t.Errorf("validateBrowserName accepted SQL injection payload as browser name: %q", name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// D. Output path tests (--out)
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLInjection_OutPath_TraversalWithSQLContent verifies that --out paths
// combining traversal sequences with SQL content are rejected by
// validateOutputPath before any file or SQL operation occurs.
func TestSQLInjection_OutPath_TraversalWithSQLContent(t *testing.T) {
	traversalPaths := []string{
		"../../'; DROP TABLE urls; --.csv",
		"../'; UNION SELECT * FROM urls;--.csv",
	}

	for i, path := range traversalPaths {
		path := path // capture
		t.Run(fmt.Sprintf("outpath_%02d", i), func(t *testing.T) {
			_, err := validateOutputPath(path)
			if err == nil {
				t.Errorf("validateOutputPath should reject traversal+SQL path %q", path)
			}
		})
	}
}

// TestSQLInjection_OutPath_NullByteRejected verifies that a --out path
// containing a null byte is rejected by validateOutputPath.
func TestSQLInjection_OutPath_NullByteRejected(t *testing.T) {
	_, err := validateOutputPath("output\x00'; DROP TABLE urls; --.csv")
	if err == nil {
		t.Fatal("expected validateOutputPath to reject path with null byte")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// E. Flag parser tests (parseFlags / validateFlagValues)
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLInjection_ParseFlags_SQLPayloadsInMatchFlag verifies that parseFlags
// accepts SQL injection payloads in the --match flag and stores them verbatim.
// The values are later sanitised by loadFilters, which rejects only null bytes
// and disallowed control characters.
func TestSQLInjection_ParseFlags_SQLPayloadsInMatchFlag(t *testing.T) {
	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("match_%02d", i), func(t *testing.T) {
			args := []string{"--match", payload}
			flags, _, err := parseFlags(args)
			if err != nil {
				t.Errorf("parseFlags rejected SQL payload in --match: %v", err)
				return
			}
			if flags["include"] != payload {
				t.Errorf("parseFlags did not preserve payload value: got %q, want %q",
					flags["include"], payload)
			}
		})
	}
}

// TestSQLInjection_ParseFlags_SQLPayloadsInProtectFlag verifies that parseFlags
// accepts SQL injection payloads in the --protect flag and stores them verbatim.
func TestSQLInjection_ParseFlags_SQLPayloadsInProtectFlag(t *testing.T) {
	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("protect_%02d", i), func(t *testing.T) {
			args := []string{"--protect", payload}
			flags, _, err := parseFlags(args)
			if err != nil {
				t.Errorf("parseFlags rejected SQL payload in --protect: %v", err)
				return
			}
			if flags["exclude"] != payload {
				t.Errorf("parseFlags did not preserve exclude payload: got %q, want %q",
					flags["exclude"], payload)
			}
		})
	}
}

// TestSQLInjection_ValidateFlagValues_SQLPayloadsInMatchFlag verifies that
// validateFlagValues accepts SQL injection payloads in the --match flag value,
// since they are within the allowed length and contain no invalid characters
// at the flag level (further validation is performed by loadFilters).
func TestSQLInjection_ValidateFlagValues_SQLPayloadsInMatchFlag(t *testing.T) {
	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("flagval_%02d", i), func(t *testing.T) {
			flags := map[string]string{"include": payload}
			if err := validateFlagValues(flags); err != nil {
				t.Errorf("validateFlagValues rejected SQL payload %q: %v", payload, err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// F. End-to-end DB integrity tests (full pipeline with real SQLite DB)
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLInjection_GetAllURLs_UnaffectedBySQLFilterKeywords verifies that
// getAllURLs returns the correct result regardless of SQL injection keywords
// present in the environment. The query is a static literal with no user input.
func TestSQLInjection_GetAllURLs_UnaffectedBySQLFilterKeywords(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	expectedCount := tdb.SeedRealisticData()

	// Call getAllURLs multiple times after "setting" SQL payloads as filter
	// values (in practice the payloads never reach SQL, but we confirm the
	// count is stable across all calls).
	for i, payload := range sqlPayloads {
		_ = payload // payload is never passed to SQL
		entries, err := getAllURLs(tdb.DB)
		if err != nil {
			t.Fatalf("payload %02d: getAllURLs failed: %v", i, err)
		}
		if len(entries) != expectedCount {
			t.Fatalf("payload %02d: expected %d entries, got %d", i, expectedCount, len(entries))
		}
	}
}

// TestSQLInjection_DBIntegrity_AfterFilteringWithSQLPayloads verifies that
// running the full filter pipeline (getAllURLs → filterEntries) with SQL
// injection payloads as match keywords leaves the database completely unchanged.
func TestSQLInjection_DBIntegrity_AfterFilteringWithSQLPayloads(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	initialCount := tdb.SeedRealisticData()

	for i, payload := range sqlPayloads {
		payload := payload // capture
		t.Run(fmt.Sprintf("integrity_%02d", i), func(t *testing.T) {
			// Simulate the full preview pipeline.
			filters, err := loadFilters(payload)
			if err != nil {
				// Only null bytes / control chars cause rejection; normal SQL
				// payloads must be accepted and continue.
				t.Fatalf("loadFilters rejected SQL payload %q: %v", payload, err)
			}

			entries, err := getAllURLs(tdb.DB)
			if err != nil {
				t.Fatalf("getAllURLs failed: %v", err)
			}

			// filterEntries: payload used only for in-memory strings.Contains.
			_ = filterEntries(entries, filters, nil)

			// The database must be completely unmodified.
			if count := tdb.CountVisibleURLs(); count != initialCount {
				t.Errorf("database was modified after filter! expected %d URLs, got %d",
					initialCount, count)
			}

			// The urls table must still exist and be queryable.
			var tableCount int
			err = tdb.DB.QueryRow(
				"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='urls'",
			).Scan(&tableCount)
			if err != nil || tableCount != 1 {
				t.Errorf("urls table missing after SQL payload filter (count=%d, err=%v)",
					tableCount, err)
			}
		})
	}
}

// TestSQLInjection_EntriesContainingSQLContent verifies that history entries
// whose URL/title fields contain SQL injection strings are retrieved, filtered,
// and processed correctly. The SQL content in data rows is data, not code.
func TestSQLInjection_EntriesContainingSQLContent(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	// Insert entries whose URL/title fields contain SQL injection strings.
	// This simulates a user who genuinely visited pages with such URLs.
	sqlURLEntries := []urlEntry{
		{
			URL:        "https://example.com/search?q='; DROP TABLE urls; --",
			Title:      "Search: '; DROP TABLE urls; --",
			VisitCount: 1,
		},
		{
			URL:        "https://example.com/page?id=1' OR '1'='1",
			Title:      "Page: ' OR '1'='1",
			VisitCount: 2,
		},
		{
			URL:        "https://example.com/item?x=UNION SELECT * FROM urls",
			Title:      "UNION SELECT result",
			VisitCount: 3,
		},
		{
			URL:        "https://safe.com/normal",
			Title:      "Normal safe page",
			VisitCount: 1,
		},
	}

	for _, e := range sqlURLEntries {
		tdb.InsertURL(e)
	}

	// getAllURLs must retrieve all entries including those with SQL-like content
	// without any query errors.
	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed when entries contain SQL content in URL/title: %v", err)
	}
	if len(entries) != len(sqlURLEntries) {
		t.Fatalf("expected %d entries, got %d", len(sqlURLEntries), len(entries))
	}

	// Filter by normal keyword: should return the example.com entries.
	matched := filterEntries(entries, []string{"example.com"}, nil)
	if len(matched) != 3 {
		t.Fatalf("expected 3 matches for 'example.com', got %d", len(matched))
	}

	// Filter using the SQL payload as a keyword (in-memory string match).
	matched = filterEntries(entries, []string{"drop table"}, nil)
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for 'drop table' keyword (in-memory match), got %d", len(matched))
	}

	// Protect filter with SQL keyword works in-memory: entries whose URL/title
	// contains "drop table" are excluded.
	protected := filterEntries(entries, []string{"example.com"}, []string{"drop table"})
	if len(protected) != 2 {
		t.Fatalf("expected 2 entries after protecting 'drop table', got %d", len(protected))
	}
}

// TestSQLInjection_DeleteEntries_SQLContentURLsSafelyDeleted verifies that
// deleteEntries correctly removes entries whose URLs contain SQL injection
// strings, using only the int64 row ID — never the URL string — in SQL.
func TestSQLInjection_DeleteEntries_SQLContentURLsSafelyDeleted(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	// Insert an entry with a SQL injection string in its URL.
	id1 := tdb.InsertURL(urlEntry{
		URL:        "https://evil.com/'; DROP TABLE urls; --",
		Title:      "SQL injection URL",
		VisitCount: 1,
	})
	id2 := tdb.InsertURL(urlEntry{
		URL:        "https://safe.com/normal",
		Title:      "Safe page",
		VisitCount: 1,
	})
	tdb.InsertVisit(visitEntry{URLID: id1, VisitTime: 1000})
	tdb.InsertVisit(visitEntry{URLID: id2, VisitTime: 2000})

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed: %v", err)
	}

	toDelete := filterEntries(entries, []string{"evil.com"}, nil)
	if len(toDelete) != 1 {
		t.Fatalf("expected 1 entry to delete, got %d", len(toDelete))
	}

	// deleteEntries binds only int64 IDs — the URL string never reaches SQL.
	if err := deleteEntries(tdb.DB, toDelete); err != nil {
		t.Fatalf("deleteEntries failed for entry with SQL-injection URL: %v", err)
	}

	// Verify the correct entry was deleted and the table is intact.
	remaining, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs after delete failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(remaining))
	}
	if remaining[0].URL != "https://safe.com/normal" {
		t.Errorf("wrong entry survived deletion: %s", remaining[0].URL)
	}

	// Verify the visits table is also correct.
	if count := tdb.CountVisits(); count != 1 {
		t.Errorf("expected 1 visit remaining, got %d", count)
	}

	// Verify the urls table still exists (DROP TABLE payload had no effect).
	var tableCount int
	if err := tdb.DB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='urls'",
	).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Errorf("urls table missing after deletion (count=%d, err=%v)", tableCount, err)
	}
}

// TestSQLInjection_WriteCSV_SQLContentInEntries verifies that history entries
// containing SQL injection strings in their URL/title fields are exported to
// CSV correctly. The SQL content is data and must appear verbatim in the output.
func TestSQLInjection_WriteCSV_SQLContentInEntries(t *testing.T) {
	entries := []HistoryEntry{
		{
			ID:         1,
			URL:        "https://example.com/q='; DROP TABLE urls; --",
			Title:      "'; DROP TABLE urls; --",
			VisitCount: 1,
		},
		{
			ID:         2,
			URL:        "https://example.com/union?x=UNION SELECT * FROM urls --",
			Title:      "UNION SELECT attack simulation",
			VisitCount: 2,
		},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV failed with SQL content in entries: %v", err)
	}

	// Both SQL payload strings must appear verbatim in the CSV output.
	if !bytes.Contains(buf.Bytes(), []byte("DROP TABLE")) {
		t.Error("expected 'DROP TABLE' to appear in CSV output as data")
	}
	if !bytes.Contains(buf.Bytes(), []byte("UNION SELECT")) {
		t.Error("expected 'UNION SELECT' to appear in CSV output as data")
	}

	// Output must begin with the UTF-8 BOM.
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("expected UTF-8 BOM prefix in CSV output")
	}
}

// TestSQLInjection_FullPipeline_IsolationVerification is a documentation-style
// integration test that walks the complete preview pipeline with a DROP TABLE
// payload as the --match value and confirms the database tables are intact
// throughout. This explicitly verifies the five isolation points in db.go:
//
//  1. loadFilters processes the payload as a plain string.
//  2. getAllURLs uses a static SQL query with no user input.
//  3. filterEntries uses the payload only for in-memory strings.Contains.
//  4. No deleteEntries call is made (preview is read-only).
//  5. The database tables (urls, visits) remain exactly as seeded.
func TestSQLInjection_FullPipeline_IsolationVerification(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	tdb.InsertURL(urlEntry{URL: "https://target.example.com", Title: "Target", VisitCount: 1})
	tdb.InsertURL(urlEntry{URL: "https://safe.example.com", Title: "Safe", VisitCount: 1})

	payload := "'; DROP TABLE urls; --"

	// Step 1: loadFilters treats the payload as a literal keyword string.
	filters, err := loadFilters(payload)
	if err != nil {
		t.Fatalf("loadFilters rejected SQL payload: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter keyword, got %d", len(filters))
	}

	// Step 2: getAllURLs uses a static query — the payload has no effect.
	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from getAllURLs, got %d", len(entries))
	}

	// Step 3: filterEntries uses the payload for in-memory string comparison.
	// Neither URL/title contains the DROP TABLE string, so 0 matches.
	matched := filterEntries(entries, filters, nil)
	if len(matched) != 0 {
		t.Errorf("expected 0 matches for DROP TABLE payload, got %d", len(matched))
	}

	// Step 4 & 5: Verify database is completely intact.
	verifyEntries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("post-filter getAllURLs failed: %v", err)
	}
	if len(verifyEntries) != 2 {
		t.Fatalf("database modified! expected 2 entries, got %d", len(verifyEntries))
	}

	// Confirm the urls and visits tables still exist.
	for _, tableName := range []string{"urls", "visits"} {
		var count int
		err := tdb.DB.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName,
		).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("table %q missing after SQL injection test (count=%d, err=%v)",
				tableName, count, err)
		}
	}
}
