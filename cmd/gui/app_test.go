package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"chrome-history-manager/internal/history"

	_ "modernc.org/sqlite"
)

// testDB creates a temporary Chrome History database seeded with test data
// and returns the path. The database is cleaned up when the test finishes.
func testDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create Chrome History schema.
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

	// Seed test data.
	entries := []struct {
		url, title string
		visits     int
		time       int64
	}{
		{"https://google.com", "Google", 5, 13350000000000000},
		{"https://github.com", "GitHub", 3, 13349000000000000},
		{"https://example.com", "Example", 1, 13348000000000000},
		{"https://reddit.com", "Reddit", 2, 13347000000000000},
		{"https://news.ycombinator.com", "Hacker News", 4, 13346000000000000},
	}

	for _, e := range entries {
		res, err := db.Exec(
			"INSERT INTO urls (url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, 0)",
			e.url, e.title, e.visits, e.time,
		)
		if err != nil {
			db.Close()
			t.Fatalf("failed to insert url: %v", err)
		}
		urlID, _ := res.LastInsertId()
		for i := 0; i < e.visits; i++ {
			if _, err := db.Exec(
				"INSERT INTO visits (url, visit_time) VALUES (?, ?)",
				urlID, e.time-int64(i)*1000000,
			); err != nil {
				db.Close()
				t.Fatalf("failed to insert visit: %v", err)
			}
		}
	}

	db.Close()
	return dbPath
}

func createNamedBackupFromDB(t *testing.T, dbPath string, name string, modTime time.Time) string {
	t.Helper()
	backupPath := filepath.Join(filepath.Dir(dbPath), name)
	if err := history.CopyFile(dbPath, backupPath); err != nil {
		t.Fatalf("failed to create named backup %q: %v", name, err)
	}
	if err := os.Chtimes(backupPath, modTime, modTime); err != nil {
		t.Fatalf("failed to set backup mod time: %v", err)
	}
	return backupPath
}

func TestNewApp(t *testing.T) {
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
}

func TestAppStartup(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	if app.ctx != ctx {
		t.Error("startup did not save context")
	}
}

func TestSearchHistory_NoBrowser(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Search all entries.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("expected 5 total entries, got %d", result.Total)
	}
	if len(result.Entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(result.Entries))
	}
	if result.Page != 1 {
		t.Errorf("expected page 1, got %d", result.Page)
	}
}

func TestSearchHistory_WithQuery(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	result, err := app.SearchHistory(dbPath, "google", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 matching entry, got %d", result.Total)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].URL != "https://google.com" {
		t.Errorf("expected google.com, got %s", result.Entries[0].URL)
	}
}

func TestSearchHistory_Pagination(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Page size 2, page 1.
	result, err := app.SearchHistory(dbPath, "", 1, 2)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("expected 5 total entries, got %d", result.Total)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries on page 1, got %d", len(result.Entries))
	}
	if result.TotalPages != 3 {
		t.Errorf("expected 3 total pages, got %d", result.TotalPages)
	}

	// Page 3 should have 1 entry.
	result, err = app.SearchHistory(dbPath, "", 3, 2)
	if err != nil {
		t.Fatalf("SearchHistory page 3 failed: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry on page 3, got %d", len(result.Entries))
	}
}

func TestSearchHistory_ParameterBinding(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// SQL injection attempt ??should be safely bound as a literal string.
	result, err := app.SearchHistory(dbPath, "'; DROP TABLE urls; --", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory with injection attempt failed: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 matches for injection string, got %d", result.Total)
	}

	// Verify database is intact.
	result, err = app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory after injection attempt failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("database should still have 5 entries, got %d", result.Total)
	}
}

func TestSearchHistoryAll_ReturnsAllEntriesBeyondPageLimit(t *testing.T) {
	dbPath := largeTestDB(t, 650)
	app := NewApp()

	entries, err := app.SearchHistoryAll(dbPath, "")
	if err != nil {
		t.Fatalf("SearchHistoryAll failed: %v", err)
	}
	if len(entries) != 650 {
		t.Errorf("expected 650 entries, got %d", len(entries))
	}
}

func TestSearchHistoryAll_WithQuery(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	entries, err := app.SearchHistoryAll(dbPath, "git")
	if err != nil {
		t.Fatalf("SearchHistoryAll failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].URL != "https://github.com" {
		t.Errorf("expected github.com, got %s", entries[0].URL)
	}
}

func TestDeleteEntries(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Get entries first.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}

	// Delete the first entry.
	ids := []int64{result.Entries[0].ID}
	if err := app.DeleteEntries(dbPath, ids); err != nil {
		t.Fatalf("DeleteEntries failed: %v", err)
	}

	// Verify deletion.
	result, err = app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory after delete failed: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 entries after delete, got %d", result.Total)
	}

	// Verify backup was created.
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "History_backup_*"))
	if len(matches) == 0 {
		t.Error("no backup file found after delete")
	}
}

func TestExportCSV(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "export.csv")
	count, err := app.ExportCSV(dbPath, []string{"*"}, nil, outPath)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 exported entries, got %d", count)
	}

	// Verify file exists and has content.
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("export file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("export file is empty")
	}

	// Verify UTF-8 BOM.
	data, _ := os.ReadFile(outPath)
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("export file missing UTF-8 BOM")
	}
}

func TestExportCSV_WithFilter(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "filtered.csv")
	count, err := app.ExportCSV(dbPath, []string{"github"}, nil, outPath)
	if err != nil {
		t.Fatalf("ExportCSV with filter failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 filtered entry, got %d", count)
	}
}

func TestExportCSV_WithProtect(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "protected.csv")
	count, err := app.ExportCSV(dbPath, []string{"*"}, []string{"google"}, outPath)
	if err != nil {
		t.Fatalf("ExportCSV with protect failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 entries (excluding google), got %d", count)
	}
}

func TestExportSelectedCSV(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "selected.csv")

	// Export entries with IDs 1 and 3 (Google and Example from test data).
	count, err := app.ExportSelectedCSV(dbPath, []int64{1, 3}, outPath)
	if err != nil {
		t.Fatalf("ExportSelectedCSV failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 exported entries, got %d", count)
	}

	// Verify file exists and has content.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	// Check UTF-8 BOM.
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("export file missing UTF-8 BOM")
	}
	content := string(data[3:])
	if !contains(content, "google.com") {
		t.Error("exported CSV should contain google.com")
	}
	if !contains(content, "example.com") {
		t.Error("exported CSV should contain example.com")
	}
	// Should NOT contain entries we didn't select.
	if contains(content, "reddit.com") {
		t.Error("exported CSV should not contain reddit.com (not selected)")
	}
}

func TestExportSelectedCSV_EmptyIDs(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "empty.csv")
	_, err := app.ExportSelectedCSV(dbPath, []int64{}, outPath)
	if err == nil {
		t.Error("expected error for empty IDs")
	}
}

func TestExportFilteredCSV_NoQuery(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "all.csv")
	count, err := app.ExportFilteredCSV(dbPath, "", outPath)
	if err != nil {
		t.Fatalf("ExportFilteredCSV failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 entries (all), got %d", count)
	}
}

func TestExportFilteredCSV_WithQuery(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "filtered.csv")
	count, err := app.ExportFilteredCSV(dbPath, "google", outPath)
	if err != nil {
		t.Fatalf("ExportFilteredCSV failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 filtered entry, got %d", count)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	content := string(data[3:]) // Skip BOM.
	if !contains(content, "google.com") {
		t.Error("filtered CSV should contain google.com")
	}
	if contains(content, "github.com") {
		t.Error("filtered CSV should not contain github.com")
	}
}

func TestExportFilteredCSV_SQLInjection(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	outPath := filepath.Join(t.TempDir(), "injection.csv")
	count, err := app.ExportFilteredCSV(dbPath, "'; DROP TABLE urls; --", outPath)
	if err != nil {
		t.Fatalf("ExportFilteredCSV with injection attempt failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 entries for injection string, got %d", count)
	}

	// Verify database is intact by searching all entries.
	result, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory after injection attempt failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("database should still have 5 entries, got %d", result.Total)
	}
}

func TestListBrowsersWithProfiles(t *testing.T) {
	app := NewApp()
	// Should not panic or error ??may return empty on CI.
	result := app.ListBrowsersWithProfiles()
	if result == nil {
		t.Error("ListBrowsersWithProfiles returned nil instead of empty slice")
	}
	// Verify structure: each browser must have at least one profile.
	for _, browser := range result {
		if browser.Name == "" {
			t.Error("browser entry has empty name")
		}
		if len(browser.Profiles) == 0 {
			t.Errorf("browser %q has no profiles ??should have been omitted", browser.Name)
		}
		for _, profile := range browser.Profiles {
			if profile.Name == "" {
				t.Errorf("browser %q has a profile with empty name", browser.Name)
			}
			if profile.DBPath == "" {
				t.Errorf("browser %q profile %q has empty dbPath", browser.Name, profile.Name)
			}
		}
	}
}

func TestDetectBrowsers(t *testing.T) {
	app := NewApp()
	// This should not panic or error ??it may return empty on CI.
	browsers := app.DetectBrowsers()
	if browsers == nil {
		t.Error("DetectBrowsers returned nil instead of empty slice")
	}
}

func TestAutoDetectBrowser_DoesNotPanic(t *testing.T) {
	app := NewApp()
	result, err := app.AutoDetectBrowser()
	if err != nil {
		// Expected on CI where no browser is installed.
		if result != nil {
			t.Error("expected nil result when error is returned")
		}
		return
	}
	if result.Name == "" {
		t.Error("non-error result should have a browser name")
	}
	if result.DBPath == "" {
		t.Error("non-error result should have a DBPath")
	}
}

func TestFolderPathForDB_UsesParentDirectory(t *testing.T) {
	dbPath := testDB(t)

	folder, err := folderPathForDB(dbPath)
	if err != nil {
		t.Fatalf("folderPathForDB failed: %v", err)
	}
	if folder != filepath.Dir(dbPath) {
		t.Errorf("expected %q, got %q", filepath.Dir(dbPath), folder)
	}
}

func TestFolderPathForDB_RejectsInvalidDBPath(t *testing.T) {
	_, err := folderPathForDB(filepath.Join(t.TempDir(), "missing-History"))
	if err == nil {
		t.Fatal("expected error for missing db path")
	}
}

func TestOpenFolderCommandForOS(t *testing.T) {
	tests := []struct {
		goos     string
		folder   string
		wantName string
		wantArgs []string
	}{
		{goos: "windows", folder: `C:\Users\test\AppData\Local\Google\Chrome\User Data\Default`, wantName: "explorer.exe", wantArgs: []string{`C:\Users\test\AppData\Local\Google\Chrome\User Data\Default`}},
		{goos: "darwin", folder: "/Users/test/Library/Application Support/Google/Chrome/Default", wantName: "open", wantArgs: []string{"/Users/test/Library/Application Support/Google/Chrome/Default"}},
		{goos: "linux", folder: "/home/test/.config/google-chrome/Default", wantName: "xdg-open", wantArgs: []string{"/home/test/.config/google-chrome/Default"}},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			gotName, gotArgs, err := openFolderCommandForOS(tt.goos, tt.folder)
			if err != nil {
				t.Fatalf("openFolderCommandForOS failed: %v", err)
			}
			if gotName != tt.wantName {
				t.Errorf("expected command %q, got %q", tt.wantName, gotName)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("expected args %v, got %v", tt.wantArgs, gotArgs)
			}
		})
	}
}

func TestOpenFolderCommandForOS_UnsupportedOS(t *testing.T) {
	_, _, err := openFolderCommandForOS("freebsd", "/tmp")
	if err == nil {
		t.Fatal("expected error for unsupported OS")
	}
}

func TestListBackups_ReturnsNewestFirstWithMetadata(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	olderTime := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	newerTime := time.Date(2026, 3, 27, 12, 30, 0, 0, time.UTC)
	olderPath := createNamedBackupFromDB(t, dbPath, "History_backup_20260326_100000", olderTime)
	newerPath := createNamedBackupFromDB(t, dbPath, "History_backup_20260327_123000", newerTime)

	if err := os.WriteFile(newerPath+"-wal", []byte("wal"), 0600); err != nil {
		t.Fatalf("failed to create sidecar: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dbPath), "not-a-backup.txt"), []byte("ignore me"), 0600); err != nil {
		t.Fatalf("failed to create unrelated file: %v", err)
	}

	backups, err := app.ListBackups(dbPath)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}

	if backups[0].Path != newerPath {
		t.Errorf("expected newest backup first, got %q", backups[0].Path)
	}
	if backups[1].Path != olderPath {
		t.Errorf("expected older backup second, got %q", backups[1].Path)
	}
	if backups[0].FileName != "History_backup_20260327_123000" {
		t.Errorf("unexpected fileName %q", backups[0].FileName)
	}
	if backups[0].SizeBytes <= 0 {
		t.Errorf("expected positive size, got %d", backups[0].SizeBytes)
	}
	if backups[0].ItemCount != 5 {
		t.Errorf("expected item count 5, got %d", backups[0].ItemCount)
	}
	if backups[0].CreatedUnix <= backups[1].CreatedUnix {
		t.Errorf("expected newer backup createdUnix %d to be > older backup %d", backups[0].CreatedUnix, backups[1].CreatedUnix)
	}
}

func TestListBackups_RejectsInvalidDBPath(t *testing.T) {
	app := NewApp()

	_, err := app.ListBackups(filepath.Join(t.TempDir(), "missing-History"))
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
}

func TestDeleteBackups_RemovesSelectedBackupsAndSidecars(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	selected := createNamedBackupFromDB(t, dbPath, "History_backup_20260327_123000", time.Date(2026, 3, 27, 12, 30, 0, 0, time.UTC))
	other := createNamedBackupFromDB(t, dbPath, "History_backup_20260327_101500", time.Date(2026, 3, 27, 10, 15, 0, 0, time.UTC))
	if err := os.WriteFile(selected+"-wal", []byte("wal"), 0600); err != nil {
		t.Fatalf("failed to create wal sidecar: %v", err)
	}
	if err := os.WriteFile(selected+"-shm", []byte("shm"), 0600); err != nil {
		t.Fatalf("failed to create shm sidecar: %v", err)
	}

	deleted, err := app.DeleteBackups(dbPath, []string{selected})
	if err != nil {
		t.Fatalf("DeleteBackups failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted backup, got %d", deleted)
	}

	for _, path := range []string{selected, selected + "-wal", selected + "-shm"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %q to be removed", path)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("expected unselected backup %q to remain: %v", other, err)
	}
}

func TestDeleteBackups_RejectsEmptySelection(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	_, err := app.DeleteBackups(dbPath, nil)
	if err == nil {
		t.Fatal("expected error for empty backup selection")
	}
}

func TestRestoreBackup_ReplacesCurrentHistoryAndCreatesSafetyBackup(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	selectedBackup := createNamedBackupFromDB(t, dbPath, "History_backup_20260327_090000", time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC))

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db for mutation: %v", err)
	}
	if _, err := db.Exec("DELETE FROM visits WHERE url = 1"); err != nil {
		db.Close()
		t.Fatalf("failed to delete visits: %v", err)
	}
	if _, err := db.Exec("DELETE FROM urls WHERE id = 1"); err != nil {
		db.Close()
		t.Fatalf("failed to delete url: %v", err)
	}
	db.Close()

	beforeRestore, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory before restore failed: %v", err)
	}
	if beforeRestore.Total != 4 {
		t.Fatalf("expected mutated db to have 4 entries, got %d", beforeRestore.Total)
	}

	safetyBackupPath, err := app.RestoreBackup(dbPath, selectedBackup)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if safetyBackupPath == "" {
		t.Fatal("expected non-empty safety backup path")
	}

	afterRestore, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory after restore failed: %v", err)
	}
	if afterRestore.Total != 5 {
		t.Fatalf("expected restored db to have 5 entries, got %d", afterRestore.Total)
	}

	safetyDB, err := sql.Open("sqlite", safetyBackupPath)
	if err != nil {
		t.Fatalf("failed to open safety backup: %v", err)
	}
	defer safetyDB.Close()

	var safetyCount int
	if err := safetyDB.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&safetyCount); err != nil {
		t.Fatalf("failed to count safety backup entries: %v", err)
	}
	if safetyCount != 4 {
		t.Fatalf("expected safety backup to preserve 4-entry state, got %d", safetyCount)
	}
}

func TestRestoreBackup_RejectsInvalidBackupPath(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	_, err := app.RestoreBackup(dbPath, filepath.Join(t.TempDir(), "missing-backup"))
	if err == nil {
		t.Fatal("expected error for invalid backup path")
	}
}

func TestWriteCSVFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "test.csv")
	entries := []history.HistoryEntry{
		{ID: 1, URL: "https://test.com", Title: "Test", VisitCount: 1, LastVisitTime: 13350000000000000},
	}

	err := history.WriteCSVFile(outPath, entries)
	if err != nil {
		t.Fatalf("WriteCSVFile failed: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	// Check BOM.
	if data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("missing UTF-8 BOM")
	}
	// Check header present.
	content := string(data[3:])
	if !contains(content, "URL") || !contains(content, "Title") {
		t.Error("CSV missing header columns")
	}
	if !contains(content, "https://test.com") {
		t.Error("CSV missing test entry URL")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Batch Deletion Queue Tests ---

func TestQueueDeletions(t *testing.T) {
	app := NewApp()
	dbPath := testDB(t)

	size := app.QueueDeletions(dbPath, []int64{1, 2, 3})
	if size != 3 {
		t.Errorf("expected queue size 3, got %d", size)
	}

	// Adding more to the same DB should accumulate.
	size = app.QueueDeletions(dbPath, []int64{4})
	if size != 4 {
		t.Errorf("expected queue size 4, got %d", size)
	}

	// Duplicates should be ignored.
	size = app.QueueDeletions(dbPath, []int64{1, 2})
	if size != 4 {
		t.Errorf("expected queue size 4 after dupes, got %d", size)
	}
}

func TestQueueDeletions_DBPathChange(t *testing.T) {
	app := NewApp()
	dbPath1 := testDB(t)
	dbPath2 := testDB(t)

	app.QueueDeletions(dbPath1, []int64{1, 2})
	// Switching DB should reset the queue.
	size := app.QueueDeletions(dbPath2, []int64{3})
	if size != 1 {
		t.Errorf("expected queue size 1 after DB change, got %d", size)
	}
}

func TestUnqueueDeletions(t *testing.T) {
	app := NewApp()
	dbPath := testDB(t)

	app.QueueDeletions(dbPath, []int64{1, 2, 3})
	size := app.UnqueueDeletions([]int64{2})
	if size != 2 {
		t.Errorf("expected queue size 2 after unqueue, got %d", size)
	}

	// Unqueue on nil queue should return 0.
	app2 := NewApp()
	size = app2.UnqueueDeletions([]int64{1})
	if size != 0 {
		t.Errorf("expected 0 for nil queue, got %d", size)
	}
}

func TestGetDeletionQueueSize(t *testing.T) {
	app := NewApp()
	if app.GetDeletionQueueSize() != 0 {
		t.Error("new app should have queue size 0")
	}

	dbPath := testDB(t)
	app.QueueDeletions(dbPath, []int64{1})
	if app.GetDeletionQueueSize() != 1 {
		t.Errorf("expected 1, got %d", app.GetDeletionQueueSize())
	}
}

func TestIsDeletionQueued(t *testing.T) {
	app := NewApp()
	if app.IsDeletionQueued(1) {
		t.Error("nil queue should return false")
	}

	dbPath := testDB(t)
	app.QueueDeletions(dbPath, []int64{42})
	if !app.IsDeletionQueued(42) {
		t.Error("id 42 should be queued")
	}
	if app.IsDeletionQueued(99) {
		t.Error("id 99 should not be queued")
	}
}

func TestClearDeletionQueue(t *testing.T) {
	app := NewApp()
	// Should not panic on nil queue.
	app.ClearDeletionQueue()

	dbPath := testDB(t)
	app.QueueDeletions(dbPath, []int64{1, 2, 3})
	app.ClearDeletionQueue()
	if app.GetDeletionQueueSize() != 0 {
		t.Errorf("expected 0 after clear, got %d", app.GetDeletionQueueSize())
	}
}

func TestCommitDeletions_EmptyQueue(t *testing.T) {
	app := NewApp()
	result, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("empty commit should not error: %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", result.Deleted)
	}
	if result.BackupPath != "" {
		t.Errorf("expected empty backup path, got %q", result.BackupPath)
	}
}

func TestCommitDeletions_SingleBackup(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	// Queue multiple entries for deletion.
	app.QueueDeletions(dbPath, []int64{1, 2, 3})

	result, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}
	if result.Deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", result.Deleted)
	}
	if result.BackupPath == "" {
		t.Error("expected non-empty backup path")
	}

	// Verify exactly one backup was created.
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "History_backup_*"))
	if len(matches) != 1 {
		t.Errorf("expected exactly 1 backup for batch delete, got %d", len(matches))
	}

	// Verify entries were deleted.
	searchResult, err := app.SearchHistory(dbPath, "", 1, 100)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}
	if searchResult.Total != 2 {
		t.Errorf("expected 2 remaining entries, got %d", searchResult.Total)
	}

	// Queue should be empty after commit.
	if app.GetDeletionQueueSize() != 0 {
		t.Errorf("queue should be empty after commit, got %d", app.GetDeletionQueueSize())
	}
}

func TestCommitDeletions_BackupPreservesOriginal(t *testing.T) {
	dbPath := testDB(t)
	app := NewApp()

	app.QueueDeletions(dbPath, []int64{1, 2})

	result, err := app.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}

	// Backup should contain all 5 original entries.
	backupDB, dbErr := sql.Open("sqlite", result.BackupPath)
	if dbErr != nil {
		t.Fatalf("failed to open backup: %v", dbErr)
	}
	defer backupDB.Close()

	var count int
	if err := backupDB.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count); err != nil {
		t.Fatalf("backup count query failed: %v", err)
	}
	if count != 5 {
		t.Errorf("backup should have all 5 original entries, got %d", count)
	}
}
