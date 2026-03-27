package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// --- validateChromeDB tests ---

// TestValidateChromeDB_ValidSchema verifies that a database with the full
// Chrome History schema passes validation without error.
func TestValidateChromeDB_ValidSchema(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	if err := validateChromeDB(tdb.Path); err != nil {
		t.Fatalf("expected valid Chrome DB to pass, got error: %v", err)
	}
}

// TestValidateChromeDB_EmptyFile verifies that an empty file (not a valid
// SQLite database) is rejected.
func TestValidateChromeDB_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

// TestValidateChromeDB_NotSQLite verifies that a plain text file is rejected
// as not a valid SQLite database.
func TestValidateChromeDB_NotSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notadb")
	if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0600); err != nil {
		t.Fatalf("failed to create text file: %v", err)
	}

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error for non-SQLite file, got nil")
	}
}

// TestValidateChromeDB_MissingURLsTable verifies that a SQLite file without
// the required 'urls' table is rejected.
func TestValidateChromeDB_MissingURLsTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_urls")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	// Only create visits, not urls.
	_, err = db.Exec(`CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER)`)
	db.Close()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error when urls table is missing, got nil")
	} else if !strings.Contains(err.Error(), "urls") {
		t.Fatalf("error should mention 'urls' table, got: %v", err)
	}
}

// TestValidateChromeDB_MissingVisitsTable verifies that a SQLite file without
// the required 'visits' table is rejected.
func TestValidateChromeDB_MissingVisitsTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_visits")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	// Only create urls, not visits.
	_, err = db.Exec(`CREATE TABLE urls (
		id INTEGER PRIMARY KEY, url TEXT, title TEXT,
		visit_count INTEGER, last_visit_time INTEGER, hidden INTEGER
	)`)
	db.Close()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error when visits table is missing, got nil")
	} else if !strings.Contains(err.Error(), "visits") {
		t.Fatalf("error should mention 'visits' table, got: %v", err)
	}
}

// TestValidateChromeDB_EmptyDatabase verifies that a valid SQLite database
// with no tables is rejected (missing required tables).
func TestValidateChromeDB_EmptyDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	// Ping to initialize the database file without creating any tables.
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("failed to ping DB: %v", err)
	}
	db.Close()

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error for empty database (no tables), got nil")
	}
}

// TestValidateChromeDB_URLsMissingRequiredColumn verifies that a SQLite file
// whose 'urls' table is missing a required column (e.g. 'visit_count') is rejected.
func TestValidateChromeDB_URLsMissingRequiredColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_col")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	// Create urls without visit_count column.
	_, err = db.Exec(`
		CREATE TABLE urls (
			id INTEGER PRIMARY KEY, url TEXT, title TEXT,
			last_visit_time INTEGER, hidden INTEGER
		);
		CREATE TABLE visits (
			id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER
		);
	`)
	db.Close()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error for missing visit_count column, got nil")
	} else if !strings.Contains(err.Error(), "visit_count") {
		t.Fatalf("error should mention missing column 'visit_count', got: %v", err)
	}
}

// TestValidateChromeDB_VisitsMissingRequiredColumn verifies that a SQLite file
// whose 'visits' table is missing a required column (e.g. 'visit_time') is rejected.
func TestValidateChromeDB_VisitsMissingRequiredColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visits_missing_col")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	// Create visits without visit_time column.
	_, err = db.Exec(`
		CREATE TABLE urls (
			id INTEGER PRIMARY KEY, url TEXT, title TEXT,
			visit_count INTEGER, last_visit_time INTEGER, hidden INTEGER
		);
		CREATE TABLE visits (
			id INTEGER PRIMARY KEY, url INTEGER
		);
	`)
	db.Close()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if err := validateChromeDB(path); err == nil {
		t.Fatal("expected error for missing visit_time column, got nil")
	} else if !strings.Contains(err.Error(), "visit_time") {
		t.Fatalf("error should mention missing column 'visit_time', got: %v", err)
	}
}

// TestValidateChromeDB_AllRequiredURLColumns verifies that all individually
// required 'urls' columns are checked.
func TestValidateChromeDB_AllRequiredURLColumns(t *testing.T) {
	requiredCols := []string{"id", "url", "title", "visit_count", "last_visit_time", "hidden"}

	for _, missingCol := range requiredCols {
		missingCol := missingCol // capture range var
		t.Run("missing_"+missingCol, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "History")

			// Build CREATE TABLE with the specified column omitted.
			colDefs := buildColDefs(
				[]string{"id INTEGER PRIMARY KEY", "url TEXT", "title TEXT",
					"visit_count INTEGER", "last_visit_time INTEGER", "hidden INTEGER"},
				missingCol,
			)

			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("failed to open DB: %v", err)
			}
			_, err = db.Exec("CREATE TABLE urls (" + colDefs + "); CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER);")
			db.Close()
			if err != nil {
				// If the column was part of a compound definition, the test may not apply cleanly.
				t.Skipf("could not create table without %s: %v", missingCol, err)
			}

			if err := validateChromeDB(path); err == nil {
				t.Fatalf("expected error when urls.%s is missing, got nil", missingCol)
			}
		})
	}
}

// TestValidateChromeDB_AllRequiredVisitsColumns verifies that all individually
// required 'visits' columns are checked.
func TestValidateChromeDB_AllRequiredVisitsColumns(t *testing.T) {
	requiredCols := []string{"id", "url", "visit_time"}

	for _, missingCol := range requiredCols {
		missingCol := missingCol // capture range var
		t.Run("missing_"+missingCol, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "History")

			// Build CREATE TABLE with the specified column omitted.
			colDefs := buildColDefs(
				[]string{"id INTEGER PRIMARY KEY", "url INTEGER", "visit_time INTEGER"},
				missingCol,
			)

			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("failed to open DB: %v", err)
			}
			_, err = db.Exec(`CREATE TABLE urls (
				id INTEGER PRIMARY KEY, url TEXT, title TEXT,
				visit_count INTEGER, last_visit_time INTEGER, hidden INTEGER
			); CREATE TABLE visits (` + colDefs + `);`)
			db.Close()
			if err != nil {
				t.Skipf("could not create table without %s: %v", missingCol, err)
			}

			if err := validateChromeDB(path); err == nil {
				t.Fatalf("expected error when visits.%s is missing, got nil", missingCol)
			}
		})
	}
}

// TestValidateChromeDB_ExtraColumnsAllowed verifies that a database with the
// required columns plus extra ones still passes validation.
func TestValidateChromeDB_ExtraColumnsAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "extra_cols")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	// Add extra columns beyond what's required.
	_, err = db.Exec(`
		CREATE TABLE urls (
			id INTEGER PRIMARY KEY,
			url TEXT, title TEXT,
			visit_count INTEGER, last_visit_time INTEGER, hidden INTEGER,
			favicon_id INTEGER, typed_count INTEGER
		);
		CREATE TABLE visits (
			id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER,
			from_visit INTEGER, transition INTEGER, segment_id INTEGER
		);
	`)
	db.Close()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if err := validateChromeDB(path); err != nil {
		t.Fatalf("extra columns should be allowed, got error: %v", err)
	}
}

// TestValidateChromeDB_NonexistentFile verifies that an error is returned for
// a path that does not exist on disk.
func TestValidateChromeDB_NonexistentFile(t *testing.T) {
	if err := validateChromeDB("/nonexistent/path/History"); err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// TestResolveDBPath_CustomPathSchemaValidation verifies that resolveDBPath
// returns an error when the --db file is a valid SQLite file but lacks
// the Chrome History schema.
func TestResolveDBPath_CustomPathSchemaValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WrongSchema")

	// Create a SQLite file that has no Chrome tables.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE random_table (id INTEGER PRIMARY KEY, data TEXT)`)
	db.Close()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	_, resolveErr := resolveDBPath("", path, "")
	if resolveErr == nil {
		t.Fatal("expected error when --db file lacks Chrome schema, got nil")
	}
	if !strings.Contains(resolveErr.Error(), "invalid --db file") {
		t.Fatalf("error should mention 'invalid --db file', got: %v", resolveErr)
	}
}

// TestResolveDBPath_CustomPathNonSQLite verifies that resolveDBPath returns
// an error when the --db file is not a SQLite database at all.
func TestResolveDBPath_CustomPathNonSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notadb")
	if err := os.WriteFile(path, []byte("not a database"), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	_, err := resolveDBPath("", path, "")
	if err == nil {
		t.Fatal("expected error for non-SQLite --db file, got nil")
	}
}

// TestResolveDBPath_CustomPathValidChromeLikeDB verifies that resolveDBPath
// accepts a valid custom DB with the Chrome schema.
func TestResolveDBPath_CustomPathValidChromeLikeDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CustomHistory")
	createTestChromeDB(t, path)

	result, err := resolveDBPath("", path, "")
	if err != nil {
		t.Fatalf("expected success for valid Chrome-like DB, got error: %v", err)
	}
	if result != path {
		t.Fatalf("expected returned path %q, got %q", path, result)
	}
}

// buildColDefs returns a comma-separated list of column definitions with the
// named column omitted. columnPrefix should be the base name (e.g. "id"),
// not including the type. Definitions are matched by prefix.
func buildColDefs(defs []string, omitColName string) string {
	var kept []string
	for _, def := range defs {
		// Each def starts with the column name; skip the omitted one.
		if !strings.HasPrefix(def, omitColName+" ") && def != omitColName {
			kept = append(kept, def)
		}
	}
	return strings.Join(kept, ", ")
}

// --- copyDB / WAL and SHM sidecar handling tests ---

// TestCopyDB_NoSidecars verifies that copyDB succeeds when no WAL/SHM
// sidecar files exist alongside the source database.
func TestCopyDB_NoSidecars(t *testing.T) {
	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	// Confirm no sidecar files exist next to the source.
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(tdb.Path + suffix); !os.IsNotExist(err) {
			t.Skipf("sidecar %s unexpectedly exists — skipping", suffix)
		}
	}

	tmpPath, err := copyDB(tdb.Path)
	if err != nil {
		t.Fatalf("copyDB failed with no sidecars: %v", err)
	}
	defer cleanupTmp(tmpPath)

	// The temp copy should be a valid Chrome DB.
	if err := validateChromeDB(tmpPath); err != nil {
		t.Fatalf("temp copy failed validation: %v", err)
	}
}

// TestCopyDB_WithWALSidecar verifies that copyDB copies a WAL sidecar file
// when one exists alongside the source database.
func TestCopyDB_WithWALSidecar(t *testing.T) {
	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	// Write a fake WAL sidecar file next to the source database.
	walSrc := tdb.Path + "-wal"
	if err := os.WriteFile(walSrc, []byte("fake wal data"), 0600); err != nil {
		t.Fatalf("failed to write fake WAL: %v", err)
	}

	tmpPath, err := copyDB(tdb.Path)
	if err != nil {
		t.Fatalf("copyDB failed with WAL sidecar: %v", err)
	}
	defer cleanupTmp(tmpPath)

	// The corresponding WAL sidecar should have been copied.
	walDst := tmpPath + "-wal"
	data, err := os.ReadFile(walDst)
	if err != nil {
		t.Fatalf("expected WAL copy at %s: %v", walDst, err)
	}
	if string(data) != "fake wal data" {
		t.Errorf("WAL copy content mismatch: got %q", string(data))
	}
}

// TestCopyDB_WALCopyFailure verifies that copyDB returns an error and cleans
// up all temporary files when the WAL sidecar cannot be copied (simulated by
// making the sidecar a directory, which os.ReadFile cannot read).
func TestCopyDB_WALCopyFailure(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	// Create a directory at the WAL path to make reading it fail.
	walSrc := tdb.Path + "-wal"
	if err := os.Mkdir(walSrc, 0755); err != nil {
		t.Fatalf("failed to create fake WAL directory: %v", err)
	}
	defer os.Remove(walSrc)

	tmpPath, err := copyDB(tdb.Path)
	if err == nil {
		cleanupTmp(tmpPath)
		t.Fatal("copyDB should have returned an error when WAL copy fails")
	}
	if !strings.Contains(err.Error(), "wal") && !strings.Contains(err.Error(), "sidecar") {
		t.Errorf("error should mention WAL or sidecar, got: %v", err)
	}

	// All temporary files must have been cleaned up.
	if tmpPath != "" {
		for _, path := range []string{tmpPath, tmpPath + "-wal", tmpPath + "-shm"} {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("expected temp file %s to be cleaned up after error", path)
			}
		}
	}
}

// TestCopyDB_SHMCopyFailure verifies that copyDB returns an error and cleans
// up all temporary files when the SHM sidecar cannot be copied.
func TestCopyDB_SHMCopyFailure(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	// Write a valid WAL sidecar so the loop progresses past -wal.
	walSrc := tdb.Path + "-wal"
	if err := os.WriteFile(walSrc, []byte("fake wal"), 0600); err != nil {
		t.Fatalf("failed to write fake WAL: %v", err)
	}

	// Create a directory at the SHM path to make reading it fail.
	shmSrc := tdb.Path + "-shm"
	if err := os.Mkdir(shmSrc, 0755); err != nil {
		t.Fatalf("failed to create fake SHM directory: %v", err)
	}
	defer os.Remove(shmSrc)

	tmpPath, err := copyDB(tdb.Path)
	if err == nil {
		cleanupTmp(tmpPath)
		t.Fatal("copyDB should have returned an error when SHM copy fails")
	}
	if !strings.Contains(err.Error(), "shm") && !strings.Contains(err.Error(), "sidecar") {
		t.Errorf("error should mention SHM or sidecar, got: %v", err)
	}

	// All temporary files must have been cleaned up.
	if tmpPath != "" {
		for _, path := range []string{tmpPath, tmpPath + "-wal", tmpPath + "-shm"} {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("expected temp file %s to be cleaned up after error", path)
			}
		}
	}
}

// --- backupDB / WAL and SHM sidecar handling tests ---

// TestBackupDB_NoSidecars verifies that backupDB succeeds when no WAL/SHM
// sidecar files exist alongside the source database.
func TestBackupDB_NoSidecars(t *testing.T) {
	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB failed with no sidecars: %v", err)
	}
	defer os.Remove(backup)

	// Backup must be a valid Chrome DB.
	if err := validateChromeDB(backup); err != nil {
		t.Fatalf("backup failed validation: %v", err)
	}
}

// TestBackupDB_WALCopyFailure verifies that backupDB returns an error and
// removes all partial backup files when the WAL sidecar cannot be copied.
func TestBackupDB_WALCopyFailure(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	// Create a directory at the WAL path to make reading it fail.
	walSrc := tdb.Path + "-wal"
	if err := os.Mkdir(walSrc, 0755); err != nil {
		t.Fatalf("failed to create fake WAL directory: %v", err)
	}
	defer os.Remove(walSrc)

	backup, err := backupDB(tdb.Path)
	if err == nil {
		// Clean up if the function unexpectedly succeeded.
		os.Remove(backup)
		os.Remove(backup + "-wal")
		os.Remove(backup + "-shm")
		t.Fatal("backupDB should have returned an error when WAL copy fails")
	}

	// The partial backup file itself must have been removed.
	// We can't predict the exact backup name, but we verify no
	// History_backup_* files exist in the source directory.
	dir := filepath.Dir(tdb.Path)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "History_backup_") {
			t.Errorf("partial backup file %s was not cleaned up", e.Name())
		}
	}
}

// TestBackupDB_SHMCopyFailure verifies that backupDB returns an error and
// removes all partial backup files when the SHM sidecar cannot be copied.
func TestBackupDB_SHMCopyFailure(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	// Write a valid WAL sidecar so the loop progresses past -wal.
	walSrc := tdb.Path + "-wal"
	if err := os.WriteFile(walSrc, []byte("fake wal"), 0600); err != nil {
		t.Fatalf("failed to write fake WAL: %v", err)
	}

	// Create a directory at the SHM path to make reading it fail.
	shmSrc := tdb.Path + "-shm"
	if err := os.Mkdir(shmSrc, 0755); err != nil {
		t.Fatalf("failed to create fake SHM directory: %v", err)
	}
	defer os.Remove(shmSrc)

	backup, err := backupDB(tdb.Path)
	if err == nil {
		os.Remove(backup)
		os.Remove(backup + "-wal")
		os.Remove(backup + "-shm")
		t.Fatal("backupDB should have returned an error when SHM copy fails")
	}
	if !strings.Contains(err.Error(), "shm") && !strings.Contains(err.Error(), "sidecar") {
		t.Errorf("error should mention SHM or sidecar, got: %v", err)
	}

	// All partial backup files must have been cleaned up.
	dir := filepath.Dir(tdb.Path)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "History_backup_") {
			t.Errorf("partial backup file %s was not cleaned up", e.Name())
		}
	}
}

// TestCopyDB_SidecarStatError verifies that copyDB returns an error (rather
// than silently skipping) when os.Stat on a sidecar file returns an error
// that is not os.IsNotExist (e.g., permission denied on the parent directory).
// This test is skipped on Windows and on systems where chmod cannot restrict
// directory traversal for the current user.
func TestCopyDB_SidecarStatError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot reliably trigger non-IsNotExist stat errors on Windows")
	}

	tdb := newTestDB(t)
	tdb.Close()

	// Remove execute permission on the source directory so that os.Stat
	// of sidecar paths within it fails with a permission error.
	srcDir := filepath.Dir(tdb.Path)
	if err := os.Chmod(srcDir, 0000); err != nil {
		t.Skipf("cannot chmod parent directory: %v", err)
	}
	// Restore permissions so t.TempDir() cleanup can remove the directory.
	defer os.Chmod(srcDir, 0700)

	tmpPath, err := copyDB(tdb.Path)
	if err == nil {
		cleanupTmp(tmpPath)
		t.Fatal("copyDB should have returned an error when sidecar stat fails")
	}
	if !strings.Contains(err.Error(), "sidecar") && !strings.Contains(err.Error(), "stat") {
		t.Errorf("error should mention sidecar or stat, got: %v", err)
	}
}

// TestBackupDB_WithSidecars verifies that backupDB copies both WAL and SHM
// sidecar files when they exist alongside the source database.
func TestBackupDB_WithSidecars(t *testing.T) {
	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	// Write fake sidecar files.
	for suffix, content := range map[string]string{
		"-wal": "fake wal content",
		"-shm": "fake shm content",
	} {
		if err := os.WriteFile(tdb.Path+suffix, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write fake %s: %v", suffix, err)
		}
	}

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB failed with sidecars: %v", err)
	}
	defer func() {
		os.Remove(backup)
		os.Remove(backup + "-wal")
		os.Remove(backup + "-shm")
	}()

	// Verify each sidecar was copied with the correct content.
	for suffix, want := range map[string]string{
		"-wal": "fake wal content",
		"-shm": "fake shm content",
	} {
		data, err := os.ReadFile(backup + suffix)
		if err != nil {
			t.Errorf("expected backup sidecar %s to exist: %v", suffix, err)
			continue
		}
		if string(data) != want {
			t.Errorf("backup sidecar %s content = %q, want %q", suffix, string(data), want)
		}
	}
}

// --- File permission tests ---

// TestCopyFile_SetsSecurePermissions verifies that copyFile creates the
// destination file with owner-only read/write permissions (0600).
// This test is skipped on Windows where NTFS does not map Unix permission bits.
func TestCopyFile_SetsSecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style permission bits are not enforced on Windows/NTFS")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("test data"), 0600); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("failed to stat destination file: %v", err)
	}

	// 0600: owner read+write, no group or other access.
	const want = os.FileMode(0600)
	if got := info.Mode().Perm(); got != want {
		t.Errorf("copyFile created file with permissions %04o, want %04o", got, want)
	}
}

// TestBackupDB_FilePermissions verifies that the main backup file created by
// backupDB has owner-only read/write permissions (0600).
// This test is skipped on Windows where NTFS does not map Unix permission bits.
func TestBackupDB_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style permission bits are not enforced on Windows/NTFS")
	}

	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB failed: %v", err)
	}
	defer os.Remove(backup)

	info, err := os.Stat(backup)
	if err != nil {
		t.Fatalf("failed to stat backup file: %v", err)
	}

	// 0600: owner read+write, no group or other access.
	const want = os.FileMode(0600)
	if got := info.Mode().Perm(); got != want {
		t.Errorf("backup file has permissions %04o, want %04o", got, want)
	}
}

// TestBackupDB_SidecarFilePermissions verifies that sidecar backup files
// (-wal, -shm) created by backupDB also have owner-only permissions (0600).
// This test is skipped on Windows where NTFS does not map Unix permission bits.
func TestBackupDB_SidecarFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style permission bits are not enforced on Windows/NTFS")
	}

	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	// Write fake sidecar files alongside the source.
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(tdb.Path+suffix, []byte("fake data"), 0600); err != nil {
			t.Fatalf("failed to write fake sidecar %s: %v", suffix, err)
		}
	}

	backup, err := backupDB(tdb.Path)
	if err != nil {
		t.Fatalf("backupDB failed: %v", err)
	}
	defer func() {
		os.Remove(backup)
		os.Remove(backup + "-wal")
		os.Remove(backup + "-shm")
	}()

	const want = os.FileMode(0600)
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(backup + suffix)
		if err != nil {
			t.Errorf("failed to stat backup sidecar %s: %v", suffix, err)
			continue
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("backup sidecar %s has permissions %04o, want %04o", suffix, got, want)
		}
	}
}

// TestCopyDB_TempFilePermissions verifies that the temporary database copy
// created by copyDB has owner-only read/write permissions (0600).
// This test is skipped on Windows where NTFS does not map Unix permission bits.
func TestCopyDB_TempFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style permission bits are not enforced on Windows/NTFS")
	}

	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	tmpPath, err := copyDB(tdb.Path)
	if err != nil {
		t.Fatalf("copyDB failed: %v", err)
	}
	defer cleanupTmp(tmpPath)

	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("failed to stat temp file: %v", err)
	}

	// 0600: owner read+write, no group or other access.
	const want = os.FileMode(0600)
	if got := info.Mode().Perm(); got != want {
		t.Errorf("temp DB copy has permissions %04o, want %04o", got, want)
	}
}
