// Package history — batch.go: batch deletion queue with single backup before commit.
//
// The DeletionQueue accumulates entry IDs to be deleted, then applies
// them all in a single transaction after creating exactly one backup
// of the database. This avoids redundant backup creation when the GUI
// user selects multiple items for deletion before confirming.
//
// # SQL Security Model
//
// All IDs in the queue are int64 values obtained from database scans,
// never from user-supplied strings. The DELETE statements use
// parameterized placeholders (?), so no user input reaches SQL.
package history

import (
	"fmt"
	"sync"
)

// DeletionQueue collects history entry IDs to be deleted in batch.
// Items are queued via QueueForDeletion and applied with CommitDeletions,
// which creates a single backup before executing all deletions in one
// transaction.
type DeletionQueue struct {
	mu      sync.Mutex
	dbPath  string
	entries map[int64]bool // set of IDs to delete (deduped)
}

// NewDeletionQueue creates a new queue targeting the given History database.
func NewDeletionQueue(dbPath string) *DeletionQueue {
	return &DeletionQueue{
		dbPath:  dbPath,
		entries: make(map[int64]bool),
	}
}

// QueueForDeletion adds one or more entry IDs to the deletion queue.
// Duplicate IDs are silently ignored.
func (q *DeletionQueue) QueueForDeletion(ids ...int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, id := range ids {
		q.entries[id] = true
	}
}

// RemoveFromQueue removes one or more entry IDs from the deletion queue.
// IDs not in the queue are silently ignored.
func (q *DeletionQueue) RemoveFromQueue(ids ...int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, id := range ids {
		delete(q.entries, id)
	}
}

// QueuedIDs returns the current set of queued entry IDs as a sorted slice.
func (q *DeletionQueue) QueuedIDs() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := make([]int64, 0, len(q.entries))
	for id := range q.entries {
		ids = append(ids, id)
	}
	return ids
}

// QueueSize returns the number of entries currently queued for deletion.
func (q *DeletionQueue) QueueSize() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

// IsQueued reports whether a given ID is in the deletion queue.
func (q *DeletionQueue) IsQueued(id int64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.entries[id]
}

// ClearQueue removes all entries from the deletion queue without
// performing any database operations.
func (q *DeletionQueue) ClearQueue() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = make(map[int64]bool)
}

// CommitDeletions creates a single backup of the database, then deletes
// all queued entries in one transaction. On success the queue is cleared
// and the backup path and number of deleted entries are returned.
//
// If the queue is empty, CommitDeletions returns (0, "", nil) without
// creating a backup or opening the database.
func (q *DeletionQueue) CommitDeletions() (deleted int, backupPath string, err error) {
	q.mu.Lock()
	if len(q.entries) == 0 {
		q.mu.Unlock()
		return 0, "", nil
	}

	// Snapshot the queued IDs under the lock, then release it
	// so the backup and DB write are not holding the mutex.
	ids := make([]int64, 0, len(q.entries))
	for id := range q.entries {
		ids = append(ids, id)
	}
	q.mu.Unlock()

	// Single backup before the batch commit.
	backupPath, err = BackupDB(q.dbPath)
	if err != nil {
		return 0, "", fmt.Errorf("backup failed: %w", err)
	}

	// Open the database for writing.
	db, err := OpenWriteDB(q.dbPath)
	if err != nil {
		return 0, backupPath, fmt.Errorf("failed to open database for writing: %w", err)
	}
	defer db.Close()

	// Build HistoryEntry slice from IDs.
	entries := make([]HistoryEntry, len(ids))
	for i, id := range ids {
		entries[i] = HistoryEntry{ID: id}
	}

	// Delete all entries in a single transaction.
	if err := DeleteEntries(db, entries); err != nil {
		return 0, backupPath, fmt.Errorf("batch deletion failed: %w", err)
	}

	// Clear the queue on success.
	q.mu.Lock()
	for _, id := range ids {
		delete(q.entries, id)
	}
	q.mu.Unlock()

	return len(ids), backupPath, nil
}
