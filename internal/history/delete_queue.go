// Package history — delete_queue.go: batch delete queue that collects multiple
// delete operations and executes them as a single database transaction.
//
// # Design
//
// The DeleteQueue is designed for the GUI frontend where users may select
// multiple history entries for deletion across different pages or search
// results before committing the operation. Rather than executing individual
// DELETE statements, the queue collects entry IDs and flushes them all in
// one transaction for efficiency and atomicity.
//
// # Thread Safety
//
// All methods are safe for concurrent use. The queue uses a sync.Mutex to
// protect its internal state, allowing the GUI frontend to add/remove items
// from any goroutine.
//
// # SQL Security
//
// The Flush method delegates to DeleteEntries, which uses parameterized
// queries with int64 IDs obtained from rows.Scan. No user-supplied string
// ever reaches any SQL query.
package history

import (
	"database/sql"
	"fmt"
	"sync"
)

// DeleteQueue collects history entries for batch deletion. Entries are
// added individually or in bulk and then flushed to the database in a
// single transaction via DeleteEntries.
type DeleteQueue struct {
	mu      sync.Mutex
	pending map[int64]HistoryEntry // keyed by ID for O(1) add/remove/lookup
}

// NewDeleteQueue creates an empty delete queue ready for use.
func NewDeleteQueue() *DeleteQueue {
	return &DeleteQueue{
		pending: make(map[int64]HistoryEntry),
	}
}

// Add enqueues a single history entry for deletion. If an entry with
// the same ID is already queued, it is silently replaced.
func (q *DeleteQueue) Add(entry HistoryEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending[entry.ID] = entry
}

// AddMany enqueues multiple history entries for deletion.
func (q *DeleteQueue) AddMany(entries []HistoryEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range entries {
		q.pending[e.ID] = e
	}
}

// AddIDs enqueues entries by ID only (sufficient for deletion since
// DeleteEntries only uses the ID field). This is convenient when the
// GUI frontend passes a list of selected row IDs.
func (q *DeleteQueue) AddIDs(ids []int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, id := range ids {
		q.pending[id] = HistoryEntry{ID: id}
	}
}

// Remove dequeues a single entry by ID. Returns true if the entry was
// present and removed, false if it was not in the queue.
func (q *DeleteQueue) Remove(id int64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.pending[id]; ok {
		delete(q.pending, id)
		return true
	}
	return false
}

// Clear removes all entries from the queue without executing any deletes.
func (q *DeleteQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = make(map[int64]HistoryEntry)
}

// Len returns the number of entries currently in the queue.
func (q *DeleteQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// Entries returns a copy of all currently queued entries. The returned
// slice is safe to use without holding the queue lock.
func (q *DeleteQueue) Entries() []HistoryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := make([]HistoryEntry, 0, len(q.pending))
	for _, e := range q.pending {
		entries = append(entries, e)
	}
	return entries
}

// IDs returns the IDs of all currently queued entries.
func (q *DeleteQueue) IDs() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := make([]int64, 0, len(q.pending))
	for id := range q.pending {
		ids = append(ids, id)
	}
	return ids
}

// Has returns true if the given ID is currently in the queue.
func (q *DeleteQueue) Has(id int64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.pending[id]
	return ok
}

// Flush executes all queued delete operations in a single transaction,
// then clears the queue. If the queue is empty, Flush is a no-op and
// returns (0, nil).
//
// The caller must provide an open write-mode database handle. Flush
// delegates to DeleteEntries which uses parameterized queries; no
// user-supplied string reaches SQL.
//
// On success the queue is cleared and the number of deleted entries is
// returned. On error the queue is NOT cleared so the caller may retry
// or inspect the pending items.
func (q *DeleteQueue) Flush(db *sql.DB) (int, error) {
	q.mu.Lock()
	entries := make([]HistoryEntry, 0, len(q.pending))
	for _, e := range q.pending {
		entries = append(entries, e)
	}
	q.mu.Unlock()

	if len(entries) == 0 {
		return 0, nil
	}

	if err := DeleteEntries(db, entries); err != nil {
		return 0, fmt.Errorf("batch delete failed: %w", err)
	}

	// Clear queue only on success.
	q.mu.Lock()
	// Remove only the entries we successfully deleted (others may have
	// been added concurrently between the snapshot and now).
	for _, e := range entries {
		delete(q.pending, e.ID)
	}
	q.mu.Unlock()

	return len(entries), nil
}

// FlushWithBackup creates a backup of the database, then flushes the
// queue. This is the recommended method for GUI use, matching the CLI's
// backup-before-delete pattern.
//
// Returns the backup path and the number of deleted entries.
func (q *DeleteQueue) FlushWithBackup(dbPath string) (backupPath string, deleted int, err error) {
	if q.Len() == 0 {
		return "", 0, nil
	}

	backupPath, err = BackupDB(dbPath)
	if err != nil {
		return "", 0, fmt.Errorf("backup failed: %w", err)
	}

	db, err := OpenWriteDB(dbPath)
	if err != nil {
		return backupPath, 0, fmt.Errorf("failed to open database for writing: %w", err)
	}
	defer db.Close()

	deleted, err = q.Flush(db)
	if err != nil {
		return backupPath, 0, err
	}

	return backupPath, deleted, nil
}
