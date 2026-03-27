package history

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// seedExportTestDB creates a temporary Chrome History database for export tests.
func seedExportTestDB(t *testing.T) string {
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
		{"https://google.com", "Google", 5, 13350000000000000},
		{"https://github.com", "GitHub", 3, 13349000000000000},
		{"https://example.com", "Example", 1, 13348000000000000},
	}

	for _, e := range entries {
		if _, err := db.Exec(
			"INSERT INTO urls (url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, 0)",
			e.url, e.title, e.visits, e.time,
		); err != nil {
			t.Fatalf("failed to insert url: %v", err)
		}
	}

	return dbPath
}

func TestGetEntriesByIDs(t *testing.T) {
	dbPath := seedExportTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	entries, err := GetEntriesByIDs(db, []int64{1, 3})
	if err != nil {
		t.Fatalf("GetEntriesByIDs failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Results should be ordered by last_visit_time DESC.
	if entries[0].URL != "https://google.com" {
		t.Errorf("first entry should be google.com (most recent), got %s", entries[0].URL)
	}
	if entries[1].URL != "https://example.com" {
		t.Errorf("second entry should be example.com, got %s", entries[1].URL)
	}
}

func TestGetEntriesByIDs_Empty(t *testing.T) {
	dbPath := seedExportTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	entries, err := GetEntriesByIDs(db, []int64{})
	if err != nil {
		t.Fatalf("GetEntriesByIDs with empty IDs failed: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty IDs, got %v", entries)
	}
}

func TestGetEntriesByIDs_NonExistent(t *testing.T) {
	dbPath := seedExportTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	entries, err := GetEntriesByIDs(db, []int64{999, 1000})
	if err != nil {
		t.Fatalf("GetEntriesByIDs with non-existent IDs failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for non-existent IDs, got %d", len(entries))
	}
}
