// Package history provides the core database access layer for Chrome History
// management. It is designed to be shared by both the CLI and GUI frontends.
//
// # SQL Security Model
//
// Every SQL statement in this file is safe from injection by construction:
//
//  1. Static queries -- GetAllURLs and the PRAGMA journal_mode statement
//     contain no user-controlled values and are compiled as plain string
//     literals.
//
//  2. Parameterized queries -- DeleteEntries passes only int64 row IDs
//     (obtained from rows.Scan into typed variables, never from user input)
//     via the database/sql placeholder mechanism (?). The driver substitutes
//     bound values after parsing, so no user string can alter the query
//     structure.
//
//  3. Schema validation -- ValidateChromeDB queries only hardcoded table
//     names ("urls", "visits") that come from a static slice in source code.
//     The primary form uses pragma_table_info(?) with those hardcoded names
//     as bound parameters; the fallback uses precomputed static string
//     literals, never fmt.Sprintf or string concatenation.
//
//  4. Filter isolation -- user-supplied --match and --protect keywords are
//     used exclusively for in-memory string comparisons inside filterEntries
//     (filter.go) and never appear in any SQL query.
//
//  5. Path isolation -- the user-supplied --db path is passed to sql.Open()
//     as a file-system path, not interpolated into any SQL string. Path
//     traversal is prevented before this point by validateDBPath (sanitize.go).
package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// HistoryEntry represents a single URL record from the Chrome History database.
type HistoryEntry struct {
	ID            int64  `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	VisitCount    int    `json:"visitCount"`
	LastVisitTime int64  `json:"lastVisitTime"`
}

// CopyFile copies a file from src to dst by reading the entire contents into memory.
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// CopyDB creates a temporary copy of the Chrome History database,
// including WAL and SHM sidecar files if present. This allows safe
// read-only access while the browser may still be running.
func CopyDB(src string) (string, error) {
	f, err := os.CreateTemp("", "history_manager_*.db")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	f.Close()

	if err := CopyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("close the browser and try again: %w", err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		srcSide := src + suffix
		if _, statErr := os.Stat(srcSide); statErr != nil {
			if !os.IsNotExist(statErr) {
				// A stat error other than "not found" (e.g. permission
				// denied) means we cannot determine whether the sidecar
				// exists. Treat this as a fatal error to avoid opening
				// an inconsistent database copy.
				CleanupTmp(tmp)
				return "", fmt.Errorf("failed to stat sidecar %s: %w", filepath.Base(srcSide), statErr)
			}
			// Sidecar does not exist -- skip it.
			continue
		}
		if err := CopyFile(srcSide, tmp+suffix); err != nil {
			// WAL/SHM files contain uncommitted data; an incomplete
			// copy would leave the temp database in an inconsistent
			// state, so treat this as a fatal error.
			CleanupTmp(tmp)
			return "", fmt.Errorf("failed to copy sidecar %s: %w (close the browser and try again)", filepath.Base(srcSide), err)
		}
	}

	return tmp, nil
}

// CleanupTmp removes the temporary database copy and its sidecar files.
func CleanupTmp(tmp string) {
	os.Remove(tmp)
	os.Remove(tmp + "-wal")
	os.Remove(tmp + "-shm")
}

// BackupDB creates a timestamped backup of the History database in the same
// directory as the original, including WAL and SHM sidecar files.
func BackupDB(src string) (string, error) {
	dir := filepath.Dir(src)
	backup := filepath.Join(dir, fmt.Sprintf("History_backup_%s", time.Now().Format("20060102_150405")))

	if err := CopyFile(src, backup); err != nil {
		return "", fmt.Errorf("close the browser and try again: %w", err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		srcSide := src + suffix
		if _, statErr := os.Stat(srcSide); statErr != nil {
			if !os.IsNotExist(statErr) {
				// A stat error other than "not found" (e.g. permission
				// denied) means we cannot determine whether the sidecar
				// exists. Clean up partial backup files and return an
				// error to avoid an incomplete backup.
				os.Remove(backup)
				os.Remove(backup + "-wal")
				os.Remove(backup + "-shm")
				return "", fmt.Errorf("failed to stat sidecar %s: %w", filepath.Base(srcSide), statErr)
			}
			// Sidecar does not exist -- skip it.
			continue
		}
		if err := CopyFile(srcSide, backup+suffix); err != nil {
			// A backup missing WAL/SHM data is incomplete and
			// unreliable. Clean up partial files and return an error.
			os.Remove(backup)
			os.Remove(backup + "-wal")
			os.Remove(backup + "-shm")
			return "", fmt.Errorf("failed to copy sidecar %s: %w (close the browser and try again)", filepath.Base(srcSide), err)
		}
	}

	return backup, nil
}

// ValidateChromeDB opens the given SQLite file and verifies it contains the
// expected Chrome History schema (urls and visits tables with required columns).
// This is used to validate user-supplied --db paths before proceeding.
func ValidateChromeDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("cannot open database: %w", err)
	}
	defer db.Close()

	// Verify the file is a valid SQLite database by running a simple query.
	if err := db.Ping(); err != nil {
		return fmt.Errorf("not a valid SQLite database: %w", err)
	}

	// Required tables and their minimum expected columns.
	type tableSchema struct {
		name    string
		columns []string
	}
	required := []tableSchema{
		{"urls", []string{"id", "url", "title", "visit_count", "last_visit_time", "hidden"}},
		{"visits", []string{"id", "url", "visit_time"}},
	}

	// Precomputed static PRAGMA queries for each required table name.
	// These are hardcoded strings (never built from user input) to avoid
	// any fmt.Sprintf usage in SQL construction.
	pragmaQueries := map[string]string{
		"urls":   "PRAGMA table_info(urls)",
		"visits": "PRAGMA table_info(visits)",
	}

	for _, ts := range required {
		rows, err := db.Query("SELECT name FROM pragma_table_info(?) WHERE 1", ts.name)
		if err != nil {
			// Fallback for drivers that don't support pragma_table_info as a table-valued function.
			// Use the precomputed static query rather than fmt.Sprintf to avoid SQL string formatting.
			pragma, ok := pragmaQueries[ts.name]
			if !ok {
				return fmt.Errorf("no pragma query defined for table %s", ts.name)
			}
			rows, err = db.Query(pragma)
			if err != nil {
				return fmt.Errorf("failed to query schema for table %s: %w", ts.name, err)
			}
		}

		foundCols := make(map[string]bool)

		// Detect which form of result we got. pragma_table_info() returns
		// (cid, name, type, notnull, dflt_value, pk) while the SELECT name
		// form returns a single column.
		colTypes, _ := rows.ColumnTypes()
		if len(colTypes) == 1 {
			// Single-column result from SELECT name FROM pragma_table_info(?).
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					rows.Close()
					return fmt.Errorf("failed to read schema for table %s: %w", ts.name, err)
				}
				foundCols[name] = true
			}
		} else {
			// Full PRAGMA table_info result.
			for rows.Next() {
				var cid int
				var name, colType string
				var notNull, pk int
				var dfltValue sql.NullString
				if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
					rows.Close()
					return fmt.Errorf("failed to read schema for table %s: %w", ts.name, err)
				}
				foundCols[name] = true
			}
		}
		rows.Close()

		if len(foundCols) == 0 {
			return fmt.Errorf("missing required table: %s (not a Chrome History database)", ts.name)
		}

		for _, col := range ts.columns {
			if !foundCols[col] {
				return fmt.Errorf("table %s missing required column: %s (not a Chrome History database)", ts.name, col)
			}
		}
	}

	return nil
}

// OpenReadOnlyDB opens a temporary copy of the database for read-only access.
// The caller must call CleanupTmp on the returned tmpPath when done.
func OpenReadOnlyDB(dbPath string) (db *sql.DB, tmpPath string, err error) {
	tmpPath, err = CopyDB(dbPath)
	if err != nil {
		return nil, "", err
	}

	db, err = sql.Open("sqlite", tmpPath)
	if err != nil {
		CleanupTmp(tmpPath)
		return nil, "", err
	}

	return db, tmpPath, nil
}

// OpenWriteDB opens the original database for direct write access
// and enables WAL journal mode for safer concurrent writes.
//
// The PRAGMA statement is a static literal; dbPath is passed to sql.Open()
// as a file-system argument, not interpolated into any SQL string.
func OpenWriteDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("PRAGMA journal_mode=wal"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	return db, nil
}

// GetAllURLs retrieves all non-hidden URL entries from the database.
// The query is a static literal with no user-supplied values; it is safe
// from SQL injection by construction.
func GetAllURLs(db *sql.DB) ([]HistoryEntry, error) {
	rows, err := db.Query("SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.URL, &e.Title, &e.VisitCount, &e.LastVisitTime); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetEntriesByIDs retrieves history entries matching the given IDs.
// The query uses parameterized placeholders for each ID (all int64 values
// sourced from prior rows.Scan calls, never from user strings).
func GetEntriesByIDs(db *sql.DB, ids []int64) ([]HistoryEntry, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build a parameterized IN clause: "?,?,?" with one placeholder per ID.
	placeholders := make([]byte, 0, len(ids)*2-1)
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}

	query := "SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE id IN (" + string(placeholders) + ") ORDER BY last_visit_time DESC"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.URL, &e.Title, &e.VisitCount, &e.LastVisitTime); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// DeleteEntries removes the given history entries (both visits and urls rows)
// from the database in a single transaction.
//
// Both DELETE statements use parameterized placeholders (?). The only bound
// value is e.ID, an int64 obtained from rows.Scan when the entries were first
// loaded from the database. No user-supplied string ever reaches these queries.
func DeleteEntries(db *sql.DB, entries []HistoryEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for _, e := range entries {
		if _, err := tx.Exec("DELETE FROM visits WHERE url = ?", e.ID); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete visits for url id %d: %w", e.ID, err)
		}
		if _, err := tx.Exec("DELETE FROM urls WHERE id = ?", e.ID); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete url id %d: %w", e.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

