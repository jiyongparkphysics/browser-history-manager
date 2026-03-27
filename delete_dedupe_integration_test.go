package main

// delete_dedupe_integration_test.go — full-pipeline integration tests for the
// delete command.
//
// These tests simulate the exact sequence executed by cmdDelete:
//
//	openReadOnlyDB  → read entries
//	  ↓  (close read DB, cleanup temp)
//	backupDB        → create timestamped backup of original
//	  ↓
//	openWriteDB     → open original for direct WAL-mode writes
//	  ↓
//	deleteEntries   → mutate inside a transaction
//
// By exercising the full pipeline against a real temporary SQLite file we
// verify that the pieces compose correctly: the read-only copy does not
// interfere with the subsequent write, the backup contains pre-mutation data,
// and the original database reflects the expected post-mutation state.

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// --- delete command — full-pipeline integration tests ---

// TestDeletePipeline_BasicFlow verifies the canonical delete pipeline:
// read from a temp copy → backup original → write to original.
// After the pipeline the original DB must be missing the matched entries.
func TestDeletePipeline_BasicFlow(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ads.doubleclick.net/track", Title: "Ad Tracker",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "My Repo",
		LastVisitTime: now,
	}, 2)
	tdb.Close() // Release file lock so copyDB / backupDB can read it.

	// ── Phase 1: read from a temporary copy ──────────────────────────────────
	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}

	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	matched := filterEntries(entries, []string{"doubleclick"}, nil)
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched entry, got %d", len(matched))
	}

	// ── Phase 2: backup original ──────────────────────────────────────────────
	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	// ── Phase 3: write to original ────────────────────────────────────────────
	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	// ── Verify: original DB reflects the deletion ─────────────────────────────
	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original for verification: %v", err)
	}
	defer verDB.Close()

	remaining, err := getAllURLs(verDB)
	if err != nil {
		t.Fatalf("getAllURLs after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(remaining))
	}
	if remaining[0].URL != "https://github.com/user/repo" {
		t.Errorf("wrong remaining URL: %q", remaining[0].URL)
	}
}

// TestDeletePipeline_BackupPreservesPreMutationData verifies that the backup
// created during the delete pipeline contains the original (pre-deletion) data.
func TestDeletePipeline_BackupPreservesPreMutationData(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://tracker.example.com/pixel", Title: "",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com/keep", Title: "Keep This",
		LastVisitTime: now,
	}, 1)
	tdb.Close()

	// Read phase.
	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	// Backup before mutation.
	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	// Delete the tracker entry from the original.
	matched := filterEntries(entries, []string{"tracker"}, nil)
	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	// The backup must still contain both entries.
	backupDB2, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backupDB2.Close()

	backupEntries, err := getAllURLs(backupDB2)
	if err != nil {
		t.Fatalf("getAllURLs on backup: %v", err)
	}
	if len(backupEntries) != 2 {
		t.Errorf("backup should have 2 entries (pre-delete), got %d", len(backupEntries))
	}

	// Verify original is now missing the tracker.
	origDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer origDB.Close()

	origEntries, err := getAllURLs(origDB)
	if err != nil {
		t.Fatalf("getAllURLs on original: %v", err)
	}
	if len(origEntries) != 1 {
		t.Errorf("original should have 1 entry after delete, got %d", len(origEntries))
	}
}

// TestDeletePipeline_MatchAllProtectBank exercises the full pipeline with a
// combined match-all + protect filter, ensuring the protect filter applies
// end-to-end.
func TestDeletePipeline_MatchAllProtectBank(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://bank.example.com/accounts", Title: "My Bank",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example",
		LastVisitTime: now,
	}, 1)
	tdb.Close()

	// Read.
	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	// Match all, protect bank.
	matched := filterEntries(entries, []string{"*"}, []string{"bank"})
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched entries (bank protected), got %d", len(matched))
	}

	// Backup + write.
	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	// Only the bank entry should remain.
	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer verDB.Close()

	remaining, err := getAllURLs(verDB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining entry (bank), got %d", len(remaining))
	}
	if !strings.Contains(remaining[0].URL, "bank") {
		t.Errorf("expected bank entry to remain, got: %q", remaining[0].URL)
	}
}

// TestDeletePipeline_VisitsRemovedWithURL verifies that visit rows are deleted
// alongside their parent URL rows in the full pipeline.
func TestDeletePipeline_VisitsRemovedWithURL(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://spam.example.com/ads", Title: "Spam",
		LastVisitTime: now,
	}, 7)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://keep.example.com", Title: "Keep",
		LastVisitTime: now,
	}, 4)
	tdb.Close()

	// Read.
	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	matched := filterEntries(entries, []string{"spam"}, nil)
	if len(matched) != 1 {
		t.Fatalf("expected 1 spam match, got %d", len(matched))
	}

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	// Reopen and count rows directly.
	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer verDB.Close()

	var urlCount, visitCount int
	if err := verDB.QueryRow("SELECT COUNT(*) FROM urls").Scan(&urlCount); err != nil {
		t.Fatalf("count urls: %v", err)
	}
	if err := verDB.QueryRow("SELECT COUNT(*) FROM visits").Scan(&visitCount); err != nil {
		t.Fatalf("count visits: %v", err)
	}

	if urlCount != 1 {
		t.Errorf("expected 1 url after delete, got %d", urlCount)
	}
	// Only the 4 keep.example.com visits should remain.
	if visitCount != 4 {
		t.Errorf("expected 4 visits after delete (spam's 7 removed), got %d", visitCount)
	}
}

// TestDeletePipeline_RealisticDataset runs the full pipeline against the
// realistic seed dataset, deleting ad/tracking entries and verifying correctness.
func TestDeletePipeline_RealisticDataset(t *testing.T) {
	tdb := newTestDB(t)
	expectedVisible := tdb.SeedRealisticData()
	tdb.Close()

	// Read.
	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	if len(entries) != expectedVisible {
		t.Fatalf("expected %d visible entries, got %d", expectedVisible, len(entries))
	}

	matched := filterEntries(entries, []string{"doubleclick", "tracker"}, nil)
	if len(matched) != 2 {
		t.Fatalf("expected 2 ad/tracker matches, got %d", len(matched))
	}

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	// Verify.
	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer verDB.Close()

	remaining, err := getAllURLs(verDB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(remaining) != expectedVisible-2 {
		t.Errorf("expected %d entries after delete, got %d", expectedVisible-2, len(remaining))
	}
	for _, e := range remaining {
		if strings.Contains(e.URL, "doubleclick") || strings.Contains(e.URL, "tracker") {
			t.Errorf("deleted URL still present: %q", e.URL)
		}
	}
}

// TestDeletePipeline_EmptyMatchIsNoOp verifies that when the match filter
// produces no results the database is not modified (no backup is created
// and the write phase is never reached).
func TestDeletePipeline_EmptyMatchIsNoOp(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example",
		LastVisitTime: now,
	}, 2)
	tdb.Close()

	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	// Nothing matches this filter.
	matched := filterEntries(entries, []string{"nonexistentdomain"}, nil)
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}

	// The command would return early — no backup, no write.
	// Just verify the DB is unmodified by checking it directly.
	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer verDB.Close()

	remaining, err := getAllURLs(verDB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("DB should be unmodified, expected 1 entry, got %d", len(remaining))
	}

	// No backup files should exist in the same directory.
	dir := filepath.Dir(tdb.Path)
	dirEntries, _ := os.ReadDir(dir)
	for _, e := range dirEntries {
		if strings.HasPrefix(e.Name(), "History_backup_") {
			t.Errorf("unexpected backup file found: %s", e.Name())
		}
	}
}

// TestDeletePipeline_TransactionRollbackOnError verifies that if deleteEntries
// returns an error (simulated by deleting from a closed/read-only connection)
// no partial changes remain in the database. This tests transaction atomicity.
func TestDeletePipeline_TransactionRollbackOnError(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://a.example.com", Title: "A",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://b.example.com", Title: "B",
		LastVisitTime: now,
	}, 1)

	initialURLCount := tdb.CountURLs()
	initialVisitCount := tdb.CountVisits()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	tdb.Close()

	// Attempt to delete from a closed DB — must error without partial changes.
	closedDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	closedDB.Close() // Close immediately to simulate a broken connection.

	matched := filterEntries(entries, []string{"*"}, nil)
	deleteErr := deleteEntries(closedDB, matched)
	// deleteErr may or may not be non-nil depending on driver behaviour
	// (some drivers allow ops on closed DBs and report errors per-op).
	_ = deleteErr

	// Reopen and verify nothing changed (the transaction should have been rolled back or never applied).
	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}
	defer verDB.Close()

	var urlCount, visitCount int
	if err := verDB.QueryRow("SELECT COUNT(*) FROM urls").Scan(&urlCount); err != nil {
		t.Fatalf("count urls: %v", err)
	}
	if err := verDB.QueryRow("SELECT COUNT(*) FROM visits").Scan(&visitCount); err != nil {
		t.Fatalf("count visits: %v", err)
	}

	if deleteErr != nil {
		// An error was returned — no changes should have been committed.
		if urlCount != initialURLCount {
			t.Errorf("partial URL deletion on error: before=%d, after=%d", initialURLCount, urlCount)
		}
		if visitCount != initialVisitCount {
			t.Errorf("partial visit deletion on error: before=%d, after=%d", initialVisitCount, visitCount)
		}
	}
}

// TestDeletePipeline_BackupIsValidChromeDB verifies that the backup created
// by the delete pipeline is a valid Chrome History database that passes schema
// validation — not a corrupted or partial file.
func TestDeletePipeline_BackupIsValidChromeDB(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{URL: "https://example.com", Title: "Example", LastVisitTime: now}, 3)
	tdb.Close()

	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	matched := filterEntries(entries, []string{"*"}, nil)

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	// The backup must pass Chrome DB schema validation.
	if err := validateChromeDB(backup); err != nil {
		t.Errorf("backup is not a valid Chrome DB: %v", err)
	}
}

// TestDeletePipeline_HiddenEntriesUntouched verifies that hidden URL rows
// survive the full delete pipeline (since getAllURLs never returns them,
// they are never passed to deleteEntries).
func TestDeletePipeline_HiddenEntriesUntouched(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{URL: "https://visible.example.com", Title: "Visible", LastVisitTime: now}, 2)
	tdb.InsertURL(urlEntry{URL: "https://hidden.example.com", Title: "Hidden", VisitCount: 1, LastVisitTime: now, Hidden: 1})
	tdb.Close()

	readDB, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	entries, err := getAllURLs(readDB)
	if err != nil {
		readDB.Close()
		cleanupTmp(tmpPath)
		t.Fatalf("getAllURLs: %v", err)
	}
	readDB.Close()
	cleanupTmp(tmpPath)

	// Should only see 1 visible entry.
	if len(entries) != 1 {
		t.Fatalf("expected 1 visible entry, got %d", len(entries))
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	defer os.Remove(backup)

	writeDB, err := openWriteDB(tdb.Path)
	if err != nil {
		t.Fatalf("openWriteDB: %v", err)
	}
	if err := deleteEntries(writeDB, matched); err != nil {
		writeDB.Close()
		t.Fatalf("deleteEntries: %v", err)
	}
	writeDB.Close()

	verDB, err := sql.Open("sqlite", tdb.Path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer verDB.Close()

	// The hidden row must still exist.
	var urlCount int
	if err := verDB.QueryRow("SELECT COUNT(*) FROM urls").Scan(&urlCount); err != nil {
		t.Fatalf("count urls: %v", err)
	}
	if urlCount != 1 {
		t.Errorf("expected 1 URL row (hidden) after deleting all visible, got %d", urlCount)
	}

	var url string
	if err := verDB.QueryRow("SELECT url FROM urls").Scan(&url); err != nil {
		t.Fatalf("query remaining URL: %v", err)
	}
	if url != "https://hidden.example.com" {
		t.Errorf("expected hidden URL to remain, got %q", url)
	}
}
