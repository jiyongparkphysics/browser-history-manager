package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// seedTestDB creates a temporary Chrome History database with test data
// and returns its path. Caller is responsible for cleanup via t.TempDir().
func seedTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

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
			t.Fatalf("failed to create schema: %v", err)
		}
	}

	entries := []struct {
		url, title string
		visits     int
		time       int64
	}{
		{"https://google.com", "Google", 2, 13350000000000000},
		{"https://github.com", "GitHub", 1, 13349000000000000},
		{"https://example.com", "Example", 1, 13348000000000000},
		{"https://reddit.com", "Reddit", 1, 13347000000000000},
		{"https://news.ycombinator.com", "Hacker News", 1, 13346000000000000},
	}

	for _, e := range entries {
		res, err := db.Exec(
			"INSERT INTO urls (url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, 0)",
			e.url, e.title, e.visits, e.time,
		)
		if err != nil {
			t.Fatalf("failed to insert url: %v", err)
		}
		urlID, _ := res.LastInsertId()
		for i := 0; i < e.visits; i++ {
			if _, err := db.Exec(
				"INSERT INTO visits (url, visit_time) VALUES (?, ?)",
				urlID, e.time-int64(i)*1000000,
			); err != nil {
				t.Fatalf("failed to insert visit: %v", err)
			}
		}
	}

	return dbPath
}

func TestNewDeletionQueue(t *testing.T) {
	q := NewDeletionQueue("/tmp/History")
	if q == nil {
		t.Fatal("NewDeletionQueue returned nil")
	}
	if q.QueueSize() != 0 {
		t.Errorf("new queue should be empty, got size %d", q.QueueSize())
	}
}

func TestQueueForDeletion(t *testing.T) {
	q := NewDeletionQueue("/tmp/History")

	q.QueueForDeletion(1, 2, 3)
	if q.QueueSize() != 3 {
		t.Errorf("expected 3 queued items, got %d", q.QueueSize())
	}

	// Duplicate IDs should be ignored.
	q.QueueForDeletion(2, 3, 4)
	if q.QueueSize() != 4 {
		t.Errorf("expected 4 queued items after dedup, got %d", q.QueueSize())
	}
}

func TestRemoveFromQueue(t *testing.T) {
	q := NewDeletionQueue("/tmp/History")

	q.QueueForDeletion(1, 2, 3)
	q.RemoveFromQueue(2)
	if q.QueueSize() != 2 {
		t.Errorf("expected 2 queued items after remove, got %d", q.QueueSize())
	}
	if q.IsQueued(2) {
		t.Error("id 2 should not be queued after removal")
	}

	// Removing non-existent ID should not error.
	q.RemoveFromQueue(99)
	if q.QueueSize() != 2 {
		t.Errorf("expected 2 queued items, got %d", q.QueueSize())
	}
}

func TestIsQueued(t *testing.T) {
	q := NewDeletionQueue("/tmp/History")

	q.QueueForDeletion(42)
	if !q.IsQueued(42) {
		t.Error("id 42 should be queued")
	}
	if q.IsQueued(99) {
		t.Error("id 99 should not be queued")
	}
}

func TestQueuedIDs(t *testing.T) {
	q := NewDeletionQueue("/tmp/History")
	q.QueueForDeletion(3, 1, 2)

	ids := q.QueuedIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}

	// Check all IDs are present (order not guaranteed).
	found := make(map[int64]bool)
	for _, id := range ids {
		found[id] = true
	}
	for _, expected := range []int64{1, 2, 3} {
		if !found[expected] {
			t.Errorf("expected id %d in QueuedIDs", expected)
		}
	}
}

func TestClearQueue(t *testing.T) {
	q := NewDeletionQueue("/tmp/History")
	q.QueueForDeletion(1, 2, 3)
	q.ClearQueue()
	if q.QueueSize() != 0 {
		t.Errorf("expected empty queue after clear, got %d", q.QueueSize())
	}
}

func TestCommitDeletions_EmptyQueue(t *testing.T) {
	q := NewDeletionQueue("/tmp/nonexistent")

	deleted, backupPath, err := q.CommitDeletions()
	if err != nil {
		t.Fatalf("empty commit should not error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
	if backupPath != "" {
		t.Errorf("expected empty backup path, got %q", backupPath)
	}
}

func TestCommitDeletions_SingleBackup(t *testing.T) {
	dbPath := seedTestDB(t)
	q := NewDeletionQueue(dbPath)

	// Queue IDs 1 and 2 for deletion.
	q.QueueForDeletion(1, 2)

	deleted, backupPath, err := q.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}
	if backupPath == "" {
		t.Error("expected non-empty backup path")
	}

	// Verify exactly one backup was created.
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "History_backup_*"))
	if len(matches) != 1 {
		t.Errorf("expected exactly 1 backup file, got %d", len(matches))
	}

	// Verify the backup file exists and is non-empty.
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup file is empty")
	}

	// Verify entries were actually deleted from the database.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 remaining entries, got %d", count)
	}

	// Queue should be cleared after successful commit.
	if q.QueueSize() != 0 {
		t.Errorf("queue should be empty after commit, got %d", q.QueueSize())
	}
}

func TestCommitDeletions_BatchNotMultipleBackups(t *testing.T) {
	dbPath := seedTestDB(t)
	q := NewDeletionQueue(dbPath)

	// Queue all 5 entries for deletion.
	q.QueueForDeletion(1, 2, 3, 4, 5)

	deleted, _, err := q.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}
	if deleted != 5 {
		t.Errorf("expected 5 deleted, got %d", deleted)
	}

	// Verify exactly one backup was created (not one per entry).
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "History_backup_*"))
	if len(matches) != 1 {
		t.Errorf("expected exactly 1 backup file for batch of 5, got %d", len(matches))
	}

	// Verify all entries deleted.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 remaining entries, got %d", count)
	}
}

func TestCommitDeletions_BackupPreservesData(t *testing.T) {
	dbPath := seedTestDB(t)
	q := NewDeletionQueue(dbPath)

	// Queue 2 entries for deletion.
	q.QueueForDeletion(1, 2)

	_, backupPath, err := q.CommitDeletions()
	if err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}

	// The backup should contain all 5 original entries.
	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatalf("failed to open backup: %v", err)
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

func TestCommitDeletions_QueueClearedOnSuccess(t *testing.T) {
	dbPath := seedTestDB(t)
	q := NewDeletionQueue(dbPath)

	q.QueueForDeletion(1)
	if _, _, err := q.CommitDeletions(); err != nil {
		t.Fatalf("CommitDeletions failed: %v", err)
	}

	// Second commit should be a no-op.
	deleted, backupPath, err := q.CommitDeletions()
	if err != nil {
		t.Fatalf("second CommitDeletions should not error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted on second commit, got %d", deleted)
	}
	if backupPath != "" {
		t.Errorf("expected empty backup on second commit, got %q", backupPath)
	}
}
