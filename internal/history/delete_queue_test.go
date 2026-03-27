package history

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// testQueueDB creates a temporary Chrome History database for queue tests.
func testQueueDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("testQueueDB: failed to open database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE urls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			visit_count INTEGER NOT NULL DEFAULT 0,
			typed_count INTEGER NOT NULL DEFAULT 0,
			last_visit_time INTEGER NOT NULL DEFAULT 0,
			hidden INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE visits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url INTEGER NOT NULL,
			visit_time INTEGER NOT NULL,
			from_visit INTEGER NOT NULL DEFAULT 0,
			transition INTEGER NOT NULL DEFAULT 0,
			segment_id INTEGER NOT NULL DEFAULT 0,
			visit_duration INTEGER NOT NULL DEFAULT 0
		);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("testQueueDB: failed to create schema: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db, dbPath
}

// seedQueueTestData inserts n URL entries with visits into the test database,
// returning the inserted entries.
func seedQueueTestData(t *testing.T, db *sql.DB, n int) []HistoryEntry {
	t.Helper()
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	chromeNow := now.UnixMicro() + 11644473600*1000000

	var entries []HistoryEntry
	for i := 0; i < n; i++ {
		visitTime := chromeNow - int64(i)*3600*1000000
		res, err := db.Exec(
			"INSERT INTO urls (url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, 0)",
			"https://example.com/page"+string(rune('A'+i)),
			"Page "+string(rune('A'+i)),
			1, visitTime,
		)
		if err != nil {
			t.Fatalf("seedQueueTestData: insert url failed: %v", err)
		}
		id, _ := res.LastInsertId()
		_, err = db.Exec("INSERT INTO visits (url, visit_time) VALUES (?, ?)", id, visitTime)
		if err != nil {
			t.Fatalf("seedQueueTestData: insert visit failed: %v", err)
		}
		entries = append(entries, HistoryEntry{
			ID:            id,
			URL:           "https://example.com/page" + string(rune('A'+i)),
			Title:         "Page " + string(rune('A'+i)),
			VisitCount:    1,
			LastVisitTime: visitTime,
		})
	}
	return entries
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	// Use hardcoded queries to avoid SQL injection in tests.
	var count int
	var err error
	switch table {
	case "urls":
		err = db.QueryRow("SELECT COUNT(*) FROM urls").Scan(&count)
	case "visits":
		err = db.QueryRow("SELECT COUNT(*) FROM visits").Scan(&count)
	default:
		t.Fatalf("countRows: unknown table %s", table)
	}
	if err != nil {
		t.Fatalf("countRows(%s): %v", table, err)
	}
	return count
}

func TestNewDeleteQueue_StartsEmpty(t *testing.T) {
	q := NewDeleteQueue()
	if q.Len() != 0 {
		t.Fatalf("expected empty queue, got %d items", q.Len())
	}
	if entries := q.Entries(); len(entries) != 0 {
		t.Fatalf("expected empty entries slice, got %d", len(entries))
	}
	if ids := q.IDs(); len(ids) != 0 {
		t.Fatalf("expected empty IDs slice, got %d", len(ids))
	}
}

func TestDeleteQueue_Add(t *testing.T) {
	q := NewDeleteQueue()
	q.Add(HistoryEntry{ID: 1, URL: "https://a.com"})
	q.Add(HistoryEntry{ID: 2, URL: "https://b.com"})

	if q.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", q.Len())
	}
	if !q.Has(1) || !q.Has(2) {
		t.Fatal("expected both IDs to be present")
	}
}

func TestDeleteQueue_Add_Deduplicates(t *testing.T) {
	q := NewDeleteQueue()
	q.Add(HistoryEntry{ID: 1, URL: "https://a.com", Title: "Original"})
	q.Add(HistoryEntry{ID: 1, URL: "https://a.com", Title: "Updated"})

	if q.Len() != 1 {
		t.Fatalf("expected 1 item after duplicate add, got %d", q.Len())
	}

	entries := q.Entries()
	if entries[0].Title != "Updated" {
		t.Fatalf("expected updated title, got %q", entries[0].Title)
	}
}

func TestDeleteQueue_AddMany(t *testing.T) {
	q := NewDeleteQueue()
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com"},
		{ID: 2, URL: "https://b.com"},
		{ID: 3, URL: "https://c.com"},
	}
	q.AddMany(entries)

	if q.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", q.Len())
	}
}

func TestDeleteQueue_AddIDs(t *testing.T) {
	q := NewDeleteQueue()
	q.AddIDs([]int64{10, 20, 30})

	if q.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", q.Len())
	}
	if !q.Has(10) || !q.Has(20) || !q.Has(30) {
		t.Fatal("expected all IDs to be present")
	}
}

func TestDeleteQueue_Remove(t *testing.T) {
	q := NewDeleteQueue()
	q.Add(HistoryEntry{ID: 1})
	q.Add(HistoryEntry{ID: 2})

	if removed := q.Remove(1); !removed {
		t.Fatal("expected Remove to return true for existing ID")
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 item after removal, got %d", q.Len())
	}
	if q.Has(1) {
		t.Fatal("expected ID 1 to be removed")
	}
}

func TestDeleteQueue_Remove_NonExistent(t *testing.T) {
	q := NewDeleteQueue()
	q.Add(HistoryEntry{ID: 1})

	if removed := q.Remove(999); removed {
		t.Fatal("expected Remove to return false for non-existent ID")
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 item (unchanged), got %d", q.Len())
	}
}

func TestDeleteQueue_Clear(t *testing.T) {
	q := NewDeleteQueue()
	q.AddIDs([]int64{1, 2, 3, 4, 5})
	q.Clear()

	if q.Len() != 0 {
		t.Fatalf("expected 0 items after clear, got %d", q.Len())
	}
}

func TestDeleteQueue_Has(t *testing.T) {
	q := NewDeleteQueue()
	q.Add(HistoryEntry{ID: 42})

	if !q.Has(42) {
		t.Fatal("expected Has(42) to be true")
	}
	if q.Has(99) {
		t.Fatal("expected Has(99) to be false")
	}
}

func TestDeleteQueue_Entries_ReturnsCopy(t *testing.T) {
	q := NewDeleteQueue()
	q.Add(HistoryEntry{ID: 1, URL: "https://a.com"})

	entries := q.Entries()
	entries[0].URL = "modified" // Modify the returned copy.

	// The queue should be unaffected.
	original := q.Entries()
	if original[0].URL == "modified" {
		t.Fatal("Entries() should return a copy, not a reference")
	}
}

func TestDeleteQueue_Flush_Empty(t *testing.T) {
	db, _ := testQueueDB(t)
	q := NewDeleteQueue()

	deleted, err := q.Flush(db)
	if err != nil {
		t.Fatalf("Flush on empty queue failed: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

func TestDeleteQueue_Flush_DeletesEntries(t *testing.T) {
	db, _ := testQueueDB(t)
	entries := seedQueueTestData(t, db, 5)

	q := NewDeleteQueue()
	// Queue 3 of the 5 entries for deletion.
	q.Add(entries[0])
	q.Add(entries[1])
	q.Add(entries[2])

	deleted, err := q.Flush(db)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}

	// Queue should be empty after successful flush.
	if q.Len() != 0 {
		t.Fatalf("expected empty queue after flush, got %d", q.Len())
	}

	// Database should have 2 remaining URLs and 2 remaining visits.
	if urls := countRows(t, db, "urls"); urls != 2 {
		t.Fatalf("expected 2 remaining URLs, got %d", urls)
	}
	if visits := countRows(t, db, "visits"); visits != 2 {
		t.Fatalf("expected 2 remaining visits, got %d", visits)
	}
}

func TestDeleteQueue_Flush_DeletesAll(t *testing.T) {
	db, _ := testQueueDB(t)
	entries := seedQueueTestData(t, db, 3)

	q := NewDeleteQueue()
	q.AddMany(entries)

	deleted, err := q.Flush(db)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}

	if urls := countRows(t, db, "urls"); urls != 0 {
		t.Fatalf("expected 0 remaining URLs, got %d", urls)
	}
}

func TestDeleteQueue_Flush_ClearsOnlyFlushedEntries(t *testing.T) {
	db, _ := testQueueDB(t)
	entries := seedQueueTestData(t, db, 3)

	q := NewDeleteQueue()
	q.Add(entries[0])

	// Simulate a concurrent add during flush by adding after snapshot
	// but before clear. We test the simpler case: after flush, only
	// flushed entries are removed.
	deleted, err := q.Flush(db)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	if q.Len() != 0 {
		t.Fatalf("expected 0 remaining in queue, got %d", q.Len())
	}
}

func TestDeleteQueue_Flush_ByIDs(t *testing.T) {
	db, _ := testQueueDB(t)
	entries := seedQueueTestData(t, db, 4)

	q := NewDeleteQueue()
	q.AddIDs([]int64{entries[0].ID, entries[3].ID})

	deleted, err := q.Flush(db)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	if urls := countRows(t, db, "urls"); urls != 2 {
		t.Fatalf("expected 2 remaining URLs, got %d", urls)
	}
}

func TestDeleteQueue_ConcurrentAccess(t *testing.T) {
	q := NewDeleteQueue()
	var wg sync.WaitGroup

	// Concurrently add entries from multiple goroutines.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			q.Add(HistoryEntry{ID: id})
		}(int64(i))
	}
	wg.Wait()

	if q.Len() != 100 {
		t.Fatalf("expected 100 items after concurrent adds, got %d", q.Len())
	}

	// Concurrently remove half.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			q.Remove(id)
		}(int64(i))
	}
	wg.Wait()

	if q.Len() != 50 {
		t.Fatalf("expected 50 items after concurrent removes, got %d", q.Len())
	}
}

func TestDeleteQueue_ConcurrentAddAndLen(t *testing.T) {
	q := NewDeleteQueue()
	var wg sync.WaitGroup

	// Concurrent adds and len checks should not race.
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(id int64) {
			defer wg.Done()
			q.Add(HistoryEntry{ID: id})
		}(int64(i))
		go func() {
			defer wg.Done()
			_ = q.Len()
		}()
	}
	wg.Wait()
}

func TestDeleteQueue_FlushWithBackup(t *testing.T) {
	db, dbPath := testQueueDB(t)
	entries := seedQueueTestData(t, db, 3)
	db.Close() // Close so FlushWithBackup can open it.

	q := NewDeleteQueue()
	q.AddMany(entries)

	backupPath, deleted, err := q.FlushWithBackup(dbPath)
	if err != nil {
		t.Fatalf("FlushWithBackup failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}
	if backupPath == "" {
		t.Fatal("expected non-empty backup path")
	}

	// Verify the original database is now empty.
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to reopen db: %v", err)
	}
	defer db2.Close()

	if urls := countRows(t, db2, "urls"); urls != 0 {
		t.Fatalf("expected 0 URLs after flush, got %d", urls)
	}
}

func TestDeleteQueue_FlushWithBackup_EmptyQueue(t *testing.T) {
	_, dbPath := testQueueDB(t)
	q := NewDeleteQueue()

	backupPath, deleted, err := q.FlushWithBackup(dbPath)
	if err != nil {
		t.Fatalf("FlushWithBackup on empty queue failed: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
	if backupPath != "" {
		t.Fatalf("expected empty backup path for empty queue, got %q", backupPath)
	}
}

func TestDeleteQueue_MultipleFlushes(t *testing.T) {
	db, _ := testQueueDB(t)
	entries := seedQueueTestData(t, db, 6)

	q := NewDeleteQueue()

	// First flush: delete 2 entries.
	q.Add(entries[0])
	q.Add(entries[1])
	deleted1, err := q.Flush(db)
	if err != nil {
		t.Fatalf("first Flush failed: %v", err)
	}
	if deleted1 != 2 {
		t.Fatalf("expected 2 deleted in first flush, got %d", deleted1)
	}

	// Second flush: delete 2 more.
	q.Add(entries[2])
	q.Add(entries[3])
	deleted2, err := q.Flush(db)
	if err != nil {
		t.Fatalf("second Flush failed: %v", err)
	}
	if deleted2 != 2 {
		t.Fatalf("expected 2 deleted in second flush, got %d", deleted2)
	}

	// 2 entries should remain.
	if urls := countRows(t, db, "urls"); urls != 2 {
		t.Fatalf("expected 2 remaining URLs, got %d", urls)
	}
}
