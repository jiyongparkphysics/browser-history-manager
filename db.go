// Package main -- db.go: thin wrappers around the internal/history package.
//
// All core database logic has been extracted to internal/history for reuse
// by both the CLI and GUI frontends. This file provides package-level
// aliases and wrapper functions so that the existing CLI code and tests
// continue to work without modification.
//
// # SQL Security Model
//
// Every SQL statement is safe from injection by construction. See the full
// security model documentation in internal/history/db.go.
package main

import (
	"database/sql"

	"chrome-history-manager/internal/history"
)

// HistoryEntry is a type alias for history.HistoryEntry, ensuring all
// existing code that references HistoryEntry continues to compile and
// work identically.
type HistoryEntry = history.HistoryEntry

// copyFile copies a file from src to dst by reading the entire contents into memory.
func copyFile(src, dst string) error {
	return history.CopyFile(src, dst)
}

// copyDB creates a temporary copy of the Chrome History database,
// including WAL and SHM sidecar files if present.
func copyDB(src string) (string, error) {
	return history.CopyDB(src)
}

// cleanupTmp removes the temporary database copy and its sidecar files.
func cleanupTmp(tmp string) {
	history.CleanupTmp(tmp)
}

// backupDB creates a timestamped backup of the History database in the same
// directory as the original, including WAL and SHM sidecar files.
func backupDB(src string) (string, error) {
	return history.BackupDB(src)
}

// validateChromeDB opens the given SQLite file and verifies it contains the
// expected Chrome History schema (urls and visits tables with required columns).
func validateChromeDB(path string) error {
	return history.ValidateChromeDB(path)
}

// openReadOnlyDB opens a temporary copy of the database for read-only access.
// The caller must call cleanupTmp on the returned tmpPath when done.
func openReadOnlyDB(dbPath string) (db *sql.DB, tmpPath string, err error) {
	return history.OpenReadOnlyDB(dbPath)
}

// openWriteDB opens the original database for direct write access
// and enables WAL journal mode for safer concurrent writes.
func openWriteDB(dbPath string) (*sql.DB, error) {
	return history.OpenWriteDB(dbPath)
}

// getAllURLs retrieves all non-hidden URL entries from the database.
func getAllURLs(db *sql.DB) ([]HistoryEntry, error) {
	return history.GetAllURLs(db)
}

// deleteEntries removes the given history entries from the database.
func deleteEntries(db *sql.DB, entries []HistoryEntry) error {
	return history.DeleteEntries(db, entries)
}
