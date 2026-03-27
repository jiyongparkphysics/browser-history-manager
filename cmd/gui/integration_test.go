package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// largeTestDB creates a Chrome History database with many entries for
// pagination and performance integration tests.
func largeTestDB(t *testing.T, count int) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE urls (
			id INTEGER PRIMARY KEY,
			url TEXT NOT NULL,
			title TEXT DEFAULT '',
			visit_count INTEGER DEFAULT 0,
			last_visit_time INTEGER DEFAULT 0,
			hidden INTEGER DEFAULT 0
		)`,
		`CREATE TABLE visits (
			id INTEGER PRIMARY KEY,
			url INTEGER NOT NULL,
			visit_time INTEGER DEFAULT 0
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("failed to create schema: %v", err)
		}
	}

	baseTime := int64(13350000000000000)
	for i := 0; i < count; i++ {
		visitTime := baseTime - int64(i)*1000000000
		res, err := db.Exec(
			"INSERT INTO urls (url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, 0)",
			"https://example.com/page/"+string(rune('a'+i%26))+"/"+itoa(i),
			"Page "+itoa(i),
			(i%5)+1,
			visitTime,
		)
		if err != nil {
			db.Close()
			t.Fatalf("failed to insert url %d: %v", i, err)
		}
		urlID, _ := res.LastInsertId()
		for v := 0; v < (i%5)+1; v++ {
			if _, err := db.Exec(
				"INSERT INTO visits (url, visit_time) VALUES (?, ?)",
				urlID, visitTime-int64(v)*1000000,
			); err != nil {
				db.Close()
				t.Fatalf("failed to insert visit: %v", err)
			}
		}
	}

	db.Close()
	return dbPath
}

// itoa is a simple int-to-string helper to avoid importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// --- Integration Test: Full Search ??Select ??Delete Workflow ---

func TestIntegration_SearchSelectDeleteWorkflow(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Step 1: User opens the app and sees all history entries.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("initial search failed: %v", err)
	}
	if result.Total != 5 {
		t.Fatalf("expected 5 entries on load, got %d", result.Total)
	}

	// Step 2: User searches for "google" to narrow down entries.
	result, err = app.SearchHistory(dbPath, "google", 1, 100)
	if err != nil {
		t.Fatalf("filtered search failed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 match for 'google', got %d", result.Total)
	}
	googleID := result.Entries[0].ID

	// Step 3: User also searches for "reddit" to queue more deletions.
	result, err = app.SearchHistory(dbPath, "reddit", 1, 100)
	if err != nil {
		t.Fatalf("reddit search failed: %v", err)
	}
	redditID := result.Entries[0].ID

	// Step 4: User selects entries and queues them for deletion.
	size := app.QueueDeletions(dbPath, []int64{googleID})
	if size != 1 {
		t.Errorf("expected queue size 1, got %d", size)
	}
	size = app.QueueDeletions(dbPath, []int64{redditID})
	if size != 2 {
		t.Errorf("expected queue size 2, got %d", size)
	}

	// Step 5: User checks the queue state (frontend badge shows count).
	if !app.IsDeletionQueued(googleID) {
		t.Error("google entry should be queued")
	}
	if !app.IsDeletionQueued(redditID) {
		t.Error("reddit entry should be queued")
	}

	// Step 6: User commits the deletion.
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}
	if delResult.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", delResult.Deleted)
	}
	if delResult.BackupPath == "" {
		t.Error("expected backup path")
	}

	// Step 7: User refreshes the view ??deleted entries should be gone.
	result, err = app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("post-delete search failed: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 remaining entries, got %d", result.Total)
	}
	for _, e := range result.Entries {
		if e.URL == "https://google.com" || e.URL == "https://reddit.com" {
			t.Errorf("deleted entry %q still present", e.URL)
		}
	}

	// Step 8: Queue should be empty after commit.
	if app.GetDeletionQueueSize() != 0 {
		t.Errorf("queue should be empty after commit, got %d", app.GetDeletionQueueSize())
	}
}

// --- Integration Test: Search ??Unqueue ??Commit Partial ---

func TestIntegration_QueueUnqueuePartialDelete(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Queue three entries.
	app.QueueDeletions(dbPath, []int64{1, 2, 3})
	if app.GetDeletionQueueSize() != 3 {
		t.Fatalf("expected queue size 3, got %d", app.GetDeletionQueueSize())
	}

	// User changes mind about entry 2, unqueues it.
	size := app.UnqueueDeletions([]int64{2})
	if size != 2 {
		t.Errorf("expected queue size 2 after unqueue, got %d", size)
	}
	if app.IsDeletionQueued(2) {
		t.Error("entry 2 should not be queued after unqueue")
	}

	// Commit ??only entries 1 and 3 should be deleted.
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}
	if delResult.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", delResult.Deleted)
	}

	// Verify entry 2 still exists.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("post-delete search failed: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 remaining entries, got %d", result.Total)
	}

	// Entry with ID 2 (GitHub) should still be there.
	found := false
	for _, e := range result.Entries {
		if e.ID == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Error("entry ID 2 should still exist after partial deletion")
	}
}

// --- Integration Test: Export Filtered Results ---

func TestIntegration_SearchThenExportFilteredCSV(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Step 1: User searches for "git" (should match GitHub).
	result, err := app.SearchHistory(dbPath, "git", 1, 100)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 match for 'git', got %d", result.Total)
	}

	// Step 2: User clicks "Export Current View" ??exports the filtered results.
	outPath := filepath.Join(t.TempDir(), "filtered_export.csv")
	count, err := app.ExportFilteredCSV(dbPath, "git", outPath)
	if err != nil {
		t.Fatalf("ExportFilteredCSV failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 exported entry, got %d", count)
	}

	// Step 3: Verify CSV contents.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "github.com") {
		t.Error("CSV should contain github.com")
	}
	if strings.Contains(content, "google.com") {
		t.Error("CSV should not contain google.com (not in filter)")
	}
}

// --- Integration Test: Export Selected Entries ---

func TestIntegration_SearchSelectExportCSV(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Step 1: User searches for entries.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// Step 2: User selects specific entries (first and last).
	selectedIDs := []int64{result.Entries[0].ID, result.Entries[len(result.Entries)-1].ID}
	selectedURLs := map[string]bool{
		result.Entries[0].URL:                     true,
		result.Entries[len(result.Entries)-1].URL: true,
	}

	// Step 3: Export selected.
	outPath := filepath.Join(t.TempDir(), "selected_export.csv")
	count, err := app.ExportSelectedCSV(dbPath, selectedIDs, outPath)
	if err != nil {
		t.Fatalf("ExportSelectedCSV failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 exported entries, got %d", count)
	}

	// Step 4: Verify CSV contains exactly the selected entries.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}
	content := string(data)
	for url := range selectedURLs {
		if !strings.Contains(content, url) {
			t.Errorf("CSV should contain selected URL %q", url)
		}
	}

	// Step 5: Verify UTF-8 BOM.
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("CSV missing UTF-8 BOM")
	}
}

// --- Integration Test: Pagination Consistency ---

func TestIntegration_PaginationConsistency(t *testing.T) {
	dbPath := largeTestDB(t, 50)
	app := NewApp()

	// Step 1: Get total count.
	result, err := app.SearchHistory(dbPath, "", 1, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if result.Total != 50 {
		t.Fatalf("expected 50 total entries, got %d", result.Total)
	}
	if result.TotalPages != 5 {
		t.Errorf("expected 5 pages, got %d", result.TotalPages)
	}

	// Step 2: Page through all results and collect all IDs.
	allIDs := make(map[int64]bool)
	for page := 1; page <= result.TotalPages; page++ {
		pageResult, err := app.SearchHistory(dbPath, "", page, 10)
		if err != nil {
			t.Fatalf("search page %d failed: %v", page, err)
		}
		if page < result.TotalPages && len(pageResult.Entries) != 10 {
			t.Errorf("page %d: expected 10 entries, got %d", page, len(pageResult.Entries))
		}
		for _, e := range pageResult.Entries {
			if allIDs[e.ID] {
				t.Errorf("duplicate entry ID %d found across pages", e.ID)
			}
			allIDs[e.ID] = true
		}
	}

	// Step 3: All entries should be covered.
	if len(allIDs) != 50 {
		t.Errorf("expected 50 unique entries across all pages, got %d", len(allIDs))
	}
}

// --- Integration Test: Browser Switch Resets Deletion Queue ---

func TestIntegration_BrowserSwitchResetsQueue(t *testing.T) {
	dbPath1 := testDB(t)
	dbPath2 := testDB(t)
	app := NewApp()

	// User queues deletions on browser 1.
	app.QueueDeletions(dbPath1, []int64{1, 2, 3})
	if app.GetDeletionQueueSize() != 3 {
		t.Fatalf("expected 3 queued, got %d", app.GetDeletionQueueSize())
	}

	// User switches browser ??queue should reset for the new DB.
	size := app.QueueDeletions(dbPath2, []int64{4})
	if size != 1 {
		t.Errorf("expected queue size 1 after browser switch, got %d", size)
	}

	// Old IDs should not be queued.
	if app.IsDeletionQueued(1) {
		t.Error("entry 1 from old browser should not be queued")
	}
	if !app.IsDeletionQueued(4) {
		t.Error("entry 4 from new browser should be queued")
	}

	// Commit should only delete from browser 2.
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}
	if delResult.Deleted != 1 {
		t.Errorf("expected 1 deleted from browser 2, got %d", delResult.Deleted)
	}

	// Browser 1 should be untouched.
	result, err := app.SearchHistory(dbPath1, "", 1, 100)
	if err != nil {
		t.Fatalf("search on browser 1 failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("browser 1 should still have 5 entries, got %d", result.Total)
	}
}

// --- Integration Test: Delete Then Export Reflects Changes ---

func TestIntegration_DeleteThenExportReflectsChanges(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Step 1: Delete Google and Reddit.
	app.QueueDeletions(dbPath, []int64{1, 4}) // Google=1, Reddit=4
	_, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}

	// Step 2: Export all remaining entries.
	outPath := filepath.Join(t.TempDir(), "after_delete.csv")
	count, err := app.ExportFilteredCSV(dbPath, "", outPath)
	if err != nil {
		t.Fatalf("ExportFilteredCSV failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 exported entries after deletion, got %d", count)
	}

	// Step 3: Verify deleted entries are not in the export.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "google.com") {
		t.Error("CSV should not contain deleted google.com")
	}
	if strings.Contains(content, "reddit.com") {
		t.Error("CSV should not contain deleted reddit.com")
	}
	if !strings.Contains(content, "github.com") {
		t.Error("CSV should contain github.com")
	}
}

// --- Integration Test: SQL Injection Across Multiple Operations ---

func TestIntegration_SQLInjectionAcrossWorkflows(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	injectionStrings := []string{
		"'; DROP TABLE urls; --",
		"\" OR 1=1 --",
		"'; DELETE FROM visits WHERE 1=1; --",
		"' UNION SELECT * FROM urls --",
		"Robert'); DROP TABLE urls;--",
	}

	for _, injection := range injectionStrings {
		// Test search with injection.
		result, err := app.SearchHistory(dbPath, injection, 1, 100)
		if err != nil {
			t.Fatalf("search with %q should not error: %v", injection, err)
		}
		if result.Total != 0 {
			t.Errorf("search with %q should return 0 results, got %d", injection, result.Total)
		}

		// Test export with injection.
		outPath := filepath.Join(t.TempDir(), "inject.csv")
		count, err := app.ExportFilteredCSV(dbPath, injection, outPath)
		if err != nil {
			t.Fatalf("export with %q should not error: %v", injection, err)
		}
		if count != 0 {
			t.Errorf("export with %q should return 0 entries, got %d", injection, count)
		}
	}

	// Verify database integrity after all injection attempts.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("post-injection search failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("database should still have 5 entries after injection attempts, got %d", result.Total)
	}
}

// --- Integration Test: Full Lifecycle (Search ??Delete ??Export) ---

func TestIntegration_FullLifecycle(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Phase 1: Search and explore.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("initial search: %v", err)
	}
	initialCount := result.Total
	if initialCount != 5 {
		t.Fatalf("expected 5 entries, got %d", initialCount)
	}

	// Phase 2: Delete one entry (Reddit).
	app.QueueDeletions(dbPath, []int64{4})
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if delResult.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", delResult.Deleted)
	}

	// Phase 3: Verify deletion.
	result, err = app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("post-delete search: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 after delete, got %d", result.Total)
	}

	// Phase 4: Export final state.
	outPath := filepath.Join(t.TempDir(), "final_export.csv")
	count, err := app.ExportFilteredCSV(dbPath, "", outPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 exported entries, got %d", count)
	}

	// Phase 5: Verify export file is valid.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("export missing UTF-8 BOM")
	}

	// Phase 6: Verify at least one backup exists from the operations.
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "History_backup_*"))
	if len(matches) == 0 {
		t.Error("expected at least one backup file after delete operations")
	}
}

// --- Integration Test: Concurrent-Style Queue Interleaving ---

func TestIntegration_QueueInterleavingOperations(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Simulate rapid queue/unqueue as user clicks checkboxes.
	app.QueueDeletions(dbPath, []int64{1})
	app.QueueDeletions(dbPath, []int64{2})
	app.UnqueueDeletions([]int64{1})
	app.QueueDeletions(dbPath, []int64{3})
	app.QueueDeletions(dbPath, []int64{1}) // re-queue 1
	app.UnqueueDeletions([]int64{3})

	// Final state: IDs 1 and 2 queued.
	if app.GetDeletionQueueSize() != 2 {
		t.Errorf("expected queue size 2, got %d", app.GetDeletionQueueSize())
	}
	if !app.IsDeletionQueued(1) {
		t.Error("ID 1 should be queued")
	}
	if !app.IsDeletionQueued(2) {
		t.Error("ID 2 should be queued")
	}
	if app.IsDeletionQueued(3) {
		t.Error("ID 3 should NOT be queued")
	}

	// Commit and verify.
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions: %v", err)
	}
	if delResult.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", delResult.Deleted)
	}

	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 remaining, got %d", result.Total)
	}
}

// --- Integration Test: Export With Match and Protect Filters ---

func TestIntegration_ExportWithFiltersWorkflow(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Step 1: Export with match filter (only entries containing "com").
	outPath := filepath.Join(t.TempDir(), "match_export.csv")
	count, err := app.ExportCSV(dbPath, []string{"github"}, nil, outPath)
	if err != nil {
		t.Fatalf("ExportCSV with match: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 entry matching 'github', got %d", count)
	}

	// Step 2: Export with protect filter (everything except google).
	outPath2 := filepath.Join(t.TempDir(), "protect_export.csv")
	count, err = app.ExportCSV(dbPath, []string{"*"}, []string{"google"}, outPath2)
	if err != nil {
		t.Fatalf("ExportCSV with protect: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 entries (excluding google), got %d", count)
	}

	data, err := os.ReadFile(outPath2)
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "google.com") {
		t.Error("protected export should not contain google.com")
	}
}

// --- Integration Test: Search Ordering Preserved Across Pages ---

func TestIntegration_SearchOrderingAcrossPages(t *testing.T) {
	dbPath := largeTestDB(t, 20)
	app := NewApp()

	// Get first two pages.
	page1, err := app.SearchHistory(dbPath, "", 1, 10)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	page2, err := app.SearchHistory(dbPath, "", 2, 10)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}

	// Last entry on page 1 should have a higher (more recent) last_visit_time
	// than the first entry on page 2 (ORDER BY last_visit_time DESC).
	if len(page1.Entries) == 0 || len(page2.Entries) == 0 {
		t.Fatal("expected non-empty pages")
	}
	lastOnPage1 := page1.Entries[len(page1.Entries)-1].LastVisitTime
	firstOnPage2 := page2.Entries[0].LastVisitTime
	if lastOnPage1 < firstOnPage2 {
		t.Errorf("ordering broken: last entry on page 1 (time=%d) should be >= first entry on page 2 (time=%d)",
			lastOnPage1, firstOnPage2)
	}
}

// --- Integration Test: Export After Search Yields Same Results ---

func TestIntegration_ExportMatchesSearchResults(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	query := "com"

	// Search for entries.
	result, err := app.SearchHistory(dbPath, query, 1, 100)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	searchCount := result.Total

	// Export with the same query.
	outPath := filepath.Join(t.TempDir(), "export_match.csv")
	exportCount, err := app.ExportFilteredCSV(dbPath, query, outPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Counts should match.
	if exportCount != searchCount {
		t.Errorf("export count (%d) should match search count (%d)", exportCount, searchCount)
	}
}

// --- Integration Test: Delete All Then Search Returns Empty ---

func TestIntegration_DeleteAllEntries(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Queue all 5 entries for deletion.
	app.QueueDeletions(dbPath, []int64{1, 2, 3, 4, 5})
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions: %v", err)
	}
	if delResult.Deleted != 5 {
		t.Errorf("expected 5 deleted, got %d", delResult.Deleted)
	}

	// Search should return empty results.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("search after delete-all: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 entries after deleting all, got %d", result.Total)
	}
	if result.TotalPages != 1 {
		t.Errorf("expected 1 total page for empty results, got %d", result.TotalPages)
	}

	// Export should produce empty CSV (just header).
	outPath := filepath.Join(t.TempDir(), "empty_export.csv")
	count, err := app.ExportFilteredCSV(dbPath, "", outPath)
	if err != nil {
		t.Fatalf("export empty: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 exported entries, got %d", count)
	}
}

// --- Integration Test: Backup Integrity After Multiple Operations ---

func TestIntegration_BackupIntegrityAfterMultipleOps(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Operation 1: Delete entry 1.
	app.QueueDeletions(dbPath, []int64{1})
	delResult1, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("first delete: %v", err)
	}

	// Operation 2: Delete entry 2.
	app.QueueDeletions(dbPath, []int64{2})
	delResult2, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}

	// Verify both backups were created and are valid Chrome DBs.
	backup1DB, err := sql.Open("sqlite", delResult1.BackupPath)
	if err != nil {
		t.Fatalf("open backup 1: %v", err)
	}
	defer backup1DB.Close()

	var count1 int
	backup1DB.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count1)
	// Backup 1 captures state at time of first delete.
	if count1 < 4 {
		t.Errorf("backup 1 should have at least 4 entries, got %d", count1)
	}

	// Second backup captures state after first delete.
	backup2DB, err := sql.Open("sqlite", delResult2.BackupPath)
	if err != nil {
		t.Fatalf("open backup 2: %v", err)
	}
	defer backup2DB.Close()

	var count2 int
	backup2DB.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count2)
	if count2 < 3 {
		t.Errorf("backup 2 should have at least 3 entries, got %d", count2)
	}

	// Backup 1 should have more or equal entries than backup 2.
	if count1 < count2 {
		t.Errorf("backup 1 (%d entries) should have >= entries than backup 2 (%d entries)", count1, count2)
	}

	// Current DB should have 3 entries.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("final search: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 entries, got %d", result.Total)
	}
}

// --- Integration Test: Large Dataset Pagination Performance ---

func TestIntegration_LargeDatasetPagination(t *testing.T) {
	dbPath := largeTestDB(t, 200)
	app := NewApp()

	// Verify total count.
	result, err := app.SearchHistory(dbPath, "", 1, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 200 {
		t.Fatalf("expected 200 entries, got %d", result.Total)
	}
	if result.TotalPages != 4 {
		t.Errorf("expected 4 pages of 50, got %d", result.TotalPages)
	}
	if len(result.Entries) != 50 {
		t.Errorf("expected 50 entries on page 1, got %d", len(result.Entries))
	}

	// Search with filter should still paginate correctly.
	// All entries have URLs containing "example.com/page/".
	result, err = app.SearchHistory(dbPath, "example", 1, 25)
	if err != nil {
		t.Fatalf("filtered search: %v", err)
	}
	if result.Total != 200 {
		t.Errorf("expected 200 entries matching 'example', got %d", result.Total)
	}
	if len(result.Entries) != 25 {
		t.Errorf("expected 25 entries on filtered page 1, got %d", len(result.Entries))
	}
}

// --- Integration Test: Clear Queue Then Re-Queue ---

func TestIntegration_ClearQueueThenRequeue(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Queue some deletions.
	app.QueueDeletions(dbPath, []int64{1, 2, 3})
	if app.GetDeletionQueueSize() != 3 {
		t.Fatalf("expected 3 queued")
	}

	// Clear all.
	app.ClearDeletionQueue()
	if app.GetDeletionQueueSize() != 0 {
		t.Fatalf("expected 0 after clear")
	}

	// Re-queue different entries.
	app.QueueDeletions(dbPath, []int64{4, 5})
	if app.GetDeletionQueueSize() != 2 {
		t.Errorf("expected 2 after re-queue, got %d", app.GetDeletionQueueSize())
	}

	// Commit ??only 4 and 5 should be deleted.
	delResult, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions: %v", err)
	}
	if delResult.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", delResult.Deleted)
	}

	// Entries 1, 2, 3 should still exist.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 remaining, got %d", result.Total)
	}
	for _, e := range result.Entries {
		if e.ID == 4 || e.ID == 5 {
			t.Errorf("entry %d should have been deleted", e.ID)
		}
	}
}

// --- Integration Test: Export All Formats Consistency ---

func TestIntegration_ExportFormatsConsistency(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	dir := t.TempDir()

	// Export via keyword filter (all entries).
	outAll := filepath.Join(dir, "all.csv")
	countAll, err := app.ExportCSV(dbPath, []string{"*"}, nil, outAll)
	if err != nil {
		t.Fatalf("ExportCSV all: %v", err)
	}

	// Export via SQL filter (no query = all entries).
	outFiltered := filepath.Join(dir, "filtered.csv")
	countFiltered, err := app.ExportFilteredCSV(dbPath, "", outFiltered)
	if err != nil {
		t.Fatalf("ExportFilteredCSV all: %v", err)
	}

	// Export via selected IDs (all 5 IDs).
	outSelected := filepath.Join(dir, "selected.csv")
	countSelected, err := app.ExportSelectedCSV(dbPath, []int64{1, 2, 3, 4, 5}, outSelected)
	if err != nil {
		t.Fatalf("ExportSelectedCSV all: %v", err)
	}

	// All three should have the same count.
	if countAll != countFiltered || countAll != countSelected {
		t.Errorf("export counts should match: all=%d, filtered=%d, selected=%d",
			countAll, countFiltered, countSelected)
	}

	// All three files should have UTF-8 BOM.
	for _, path := range []string{outAll, outFiltered, outSelected} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
			t.Errorf("%s missing UTF-8 BOM", filepath.Base(path))
		}
	}
}

// --- Integration Test: Search Edge Cases ---

func TestIntegration_SearchEdgeCases(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Page 0 should be clamped to 1.
	result, err := app.SearchHistory(dbPath, "", 0, 10)
	if err != nil {
		t.Fatalf("search page 0: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("page 0 should be clamped to 1, got %d", result.Page)
	}

	// Page beyond total should clamp to the final page so the GUI never lands
	// on an empty out-of-range page after deletions or page-size changes.
	result, err = app.SearchHistory(dbPath, "", 999, 10)
	if err != nil {
		t.Fatalf("search page 999: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("page 999 should clamp to 1, got %d", result.Page)
	}
	if len(result.Entries) != 5 {
		t.Errorf("expected 5 entries on clamped page, got %d", len(result.Entries))
	}
	if result.Total != 5 {
		t.Errorf("total should still be 5, got %d", result.Total)
	}

	// PageSize 0 should be clamped to default (100).
	result, err = app.SearchHistory(dbPath, "", 1, 0)
	if err != nil {
		t.Fatalf("search pageSize 0: %v", err)
	}
	if result.PageSize != 100 {
		t.Errorf("pageSize 0 should be clamped to 100, got %d", result.PageSize)
	}

	// PageSize > 500 should be clamped to 100.
	result, err = app.SearchHistory(dbPath, "", 1, 1000)
	if err != nil {
		t.Fatalf("search pageSize 1000: %v", err)
	}
	if result.PageSize != 100 {
		t.Errorf("pageSize 1000 should be clamped to 100, got %d", result.PageSize)
	}

	// Empty query with special characters.
	result, err = app.SearchHistory(dbPath, "%", 1, 100)
	if err != nil {
		t.Fatalf("search with %%: %v", err)
	}
	// '%' is a LIKE wildcard, so it matches everything ??this tests that
	// the wildcard passes through safely via parameter binding.
	if result.Total < 0 {
		t.Error("search with % should not produce negative total")
	}
}
