package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// testDB wraps a temporary Chrome History SQLite database for integration tests.
// It provides helpers to populate realistic data and cleans up automatically
// when the test completes.
type testDB struct {
	DB   *sql.DB
	Path string
	t    *testing.T
}

// newTestDB creates a new temporary Chrome History database with the full
// Chrome schema (urls and visits tables) in a temp directory. The database
// and directory are automatically removed when the test finishes.
func newTestDB(t *testing.T) *testDB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("newTestDB: failed to open database: %v", err)
	}

	// Create the Chrome History schema.
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
		CREATE INDEX IF NOT EXISTS urls_url_index ON urls (url);
		CREATE INDEX IF NOT EXISTS visits_url_index ON visits (url);
		CREATE INDEX IF NOT EXISTS visits_time_index ON visits (visit_time);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("newTestDB: failed to create schema: %v", err)
	}

	tdb := &testDB{DB: db, Path: dbPath, t: t}
	t.Cleanup(func() {
		db.Close()
	})
	return tdb
}

// Close closes the database connection. Typically not needed since cleanup
// is registered automatically, but useful if you need to close before
// passing the path to functions that open it themselves.
func (tdb *testDB) Close() {
	tdb.DB.Close()
}

// timeToChrome converts a time.Time to Chrome timestamp format
// (microseconds since 1601-01-01 00:00:00 UTC).
func timeToChrome(t time.Time) int64 {
	return t.UnixMicro() + 11644473600*1000000
}

// urlEntry represents a URL row to insert into the test database.
type urlEntry struct {
	URL           string
	Title         string
	VisitCount    int
	LastVisitTime int64 // Chrome timestamp format
	Hidden        int
}

// visitEntry represents a visit row to insert into the test database.
type visitEntry struct {
	URLID     int64 // references urls.id
	VisitTime int64 // Chrome timestamp format
}

// InsertURL inserts a single URL entry and returns its auto-generated ID.
func (tdb *testDB) InsertURL(e urlEntry) int64 {
	tdb.t.Helper()
	result, err := tdb.DB.Exec(
		"INSERT INTO urls (url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, ?)",
		e.URL, e.Title, e.VisitCount, e.LastVisitTime, e.Hidden,
	)
	if err != nil {
		tdb.t.Fatalf("InsertURL: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		tdb.t.Fatalf("InsertURL LastInsertId: %v", err)
	}
	return id
}

// InsertVisit inserts a single visit entry and returns its auto-generated ID.
func (tdb *testDB) InsertVisit(v visitEntry) int64 {
	tdb.t.Helper()
	result, err := tdb.DB.Exec(
		"INSERT INTO visits (url, visit_time) VALUES (?, ?)",
		v.URLID, v.VisitTime,
	)
	if err != nil {
		tdb.t.Fatalf("InsertVisit: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		tdb.t.Fatalf("InsertVisit LastInsertId: %v", err)
	}
	return id
}

// InsertURLWithVisits inserts a URL entry and the specified number of visit
// records at evenly-spaced intervals ending at the given last visit time.
// Returns the URL's auto-generated ID.
func (tdb *testDB) InsertURLWithVisits(e urlEntry, visitCount int) int64 {
	tdb.t.Helper()

	if visitCount < 1 {
		visitCount = 1
	}
	e.VisitCount = visitCount

	urlID := tdb.InsertURL(e)

	// Space visits 1 hour apart, ending at LastVisitTime.
	baseTime := e.LastVisitTime
	if baseTime == 0 {
		baseTime = timeToChrome(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))
	}
	hourInMicro := int64(3600 * 1000000)

	for i := 0; i < visitCount; i++ {
		visitTime := baseTime - int64(visitCount-1-i)*hourInMicro
		tdb.InsertVisit(visitEntry{URLID: urlID, VisitTime: visitTime})
	}

	return urlID
}

// SeedRealisticData populates the database with a realistic set of Chrome
// history entries spanning different domains, visit patterns, and edge cases.
// Returns the number of non-hidden URL entries inserted.
func (tdb *testDB) SeedRealisticData() int {
	tdb.t.Helper()

	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	nowChrome := timeToChrome(now)
	dayAgo := timeToChrome(now.Add(-24 * time.Hour))
	weekAgo := timeToChrome(now.Add(-7 * 24 * time.Hour))
	monthAgo := timeToChrome(now.Add(-30 * 24 * time.Hour))

	// Frequently visited sites (multiple visits).
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search?q=golang+tutorial", Title: "golang tutorial - Google Search",
		LastVisitTime: nowChrome,
	}, 15)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "user/repo: My Project",
		LastVisitTime: nowChrome,
	}, 8)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", Title: "Rick Astley - Never Gonna Give You Up",
		LastVisitTime: dayAgo,
	}, 5)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://stackoverflow.com/questions/12345/how-to-parse-json", Title: "How to parse JSON in Go - Stack Overflow",
		LastVisitTime: dayAgo,
	}, 12)

	// Single-visit sites.
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com/page1", Title: "Example Page 1",
		LastVisitTime: weekAgo,
	}, 1)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://news.ycombinator.com/", Title: "Hacker News",
		LastVisitTime: weekAgo,
	}, 1)

	// Banking/sensitive site (useful for testing protect filters).
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://bank.example.com/accounts", Title: "My Bank - Account Summary",
		LastVisitTime: monthAgo,
	}, 3)

	// Ad/tracking URLs (useful for testing match-and-delete flows).
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ads.doubleclick.net/track?id=abc", Title: "",
		LastVisitTime: dayAgo,
	}, 2)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://tracker.example.com/pixel.gif", Title: "",
		LastVisitTime: weekAgo,
	}, 1)

	// Unicode content.
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ko.wikipedia.org/wiki/한국어", Title: "한국어 - 위키백과",
		LastVisitTime: monthAgo,
	}, 2)

	// URL with special characters in query string.
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://search.example.com/q?term=a%20b&lang=en&sort=date", Title: "Search Results: a b",
		LastVisitTime: dayAgo,
	}, 1)

	// Hidden entry (should be excluded by getAllURLs).
	tdb.InsertURL(urlEntry{
		URL: "https://hidden.example.com/internal", Title: "Hidden Page",
		VisitCount: 1, LastVisitTime: dayAgo, Hidden: 1,
	})

	// Very long URL and title.
	longURL := "https://example.com/path?" + repeatString("param=value&", 50)
	longTitle := "A Very Long Title " + repeatString("With Repeated Words ", 20)
	tdb.InsertURLWithVisits(urlEntry{
		URL: longURL, Title: longTitle,
		LastVisitTime: weekAgo,
	}, 1)

	// Entry with empty title (common for redirects/tracking).
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://redirect.example.com/go?to=somewhere", Title: "",
		LastVisitTime: monthAgo,
	}, 1)

	// Scholar entry (useful for protect filter tests).
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://scholar.google.com/scholar?q=machine+learning", Title: "Google Scholar - machine learning",
		LastVisitTime: dayAgo,
	}, 4)

	return 14 // number of non-hidden entries
}

// CountURLs returns the total number of URL rows in the database.
func (tdb *testDB) CountURLs() int {
	tdb.t.Helper()
	var count int
	if err := tdb.DB.QueryRow("SELECT COUNT(*) FROM urls").Scan(&count); err != nil {
		tdb.t.Fatalf("CountURLs: %v", err)
	}
	return count
}

// CountVisibleURLs returns the number of non-hidden URL rows.
func (tdb *testDB) CountVisibleURLs() int {
	tdb.t.Helper()
	var count int
	if err := tdb.DB.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count); err != nil {
		tdb.t.Fatalf("CountVisibleURLs: %v", err)
	}
	return count
}

// CountVisits returns the total number of visit rows in the database.
func (tdb *testDB) CountVisits() int {
	tdb.t.Helper()
	var count int
	if err := tdb.DB.QueryRow("SELECT COUNT(*) FROM visits").Scan(&count); err != nil {
		tdb.t.Fatalf("CountVisits: %v", err)
	}
	return count
}

// CountVisitsForURL returns the number of visit rows for a given URL ID.
func (tdb *testDB) CountVisitsForURL(urlID int64) int {
	tdb.t.Helper()
	var count int
	if err := tdb.DB.QueryRow("SELECT COUNT(*) FROM visits WHERE url = ?", urlID).Scan(&count); err != nil {
		tdb.t.Fatalf("CountVisitsForURL: %v", err)
	}
	return count
}

// CopyDBFile creates a copy of the test database at a new path (to simulate
// the pattern used by copyDB). Returns the path to the copy.
func (tdb *testDB) CopyDBFile(destDir string) string {
	tdb.t.Helper()
	destPath := filepath.Join(destDir, "History")
	data, err := os.ReadFile(tdb.Path)
	if err != nil {
		tdb.t.Fatalf("CopyDBFile read: %v", err)
	}
	if err := os.WriteFile(destPath, data, 0600); err != nil {
		tdb.t.Fatalf("CopyDBFile write: %v", err)
	}
	return destPath
}

// repeatString repeats a string n times. Simpler than importing strings
// for test helpers.
func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// createTestChromeDB creates a minimal SQLite database with the Chrome History
// schema (urls and visits tables) at the given path. Unlike newTestDB, the
// path is caller-supplied, which is useful for testing path-resolution helpers
// such as resolveDBPath. Cleanup of the file is the caller's responsibility.
func createTestChromeDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("createTestChromeDB: failed to open database at %s: %v", path, err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS urls (
			id INTEGER PRIMARY KEY,
			url TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			visit_count INTEGER NOT NULL DEFAULT 0,
			last_visit_time INTEGER NOT NULL DEFAULT 0,
			hidden INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS visits (
			id INTEGER PRIMARY KEY,
			url INTEGER NOT NULL,
			visit_time INTEGER NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("createTestChromeDB: failed to create schema: %v", err)
	}
}

// SeedMinimalData populates the database with a small, predictable set of
// Chrome history entries suitable for lightweight integration tests that do
// not require the full realistic data set. The seeded entries cover:
//   - a frequently-visited search engine (useful for match filter tests)
//   - a single-visit code repository (useful as a non-matching control)
//   - a financial site (useful for protect filter tests)
//
// Returns the number of visible (non-hidden) entries inserted, which is
// always 3.
func (tdb *testDB) SeedMinimalData() int {
	tdb.t.Helper()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search?q=test", Title: "test - Google Search",
		LastVisitTime: now,
	}, 3)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "user/repo: My Project",
		LastVisitTime: now,
	}, 1)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://bank.example.com/accounts", Title: "My Bank - Accounts",
		LastVisitTime: now,
	}, 2)

	return 3
}

// --- Self-tests for test infrastructure ---

func TestNewTestDB_CreatesValidDB(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	// The database should pass Chrome DB validation.
	if err := validateChromeDB(tdb.Path); err != nil {
		t.Fatalf("newTestDB did not create a valid Chrome DB: %v", err)
	}
}

func TestNewTestDB_EmptyByDefault(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	if count := tdb.CountURLs(); count != 0 {
		t.Fatalf("expected 0 URLs in fresh DB, got %d", count)
	}
	if count := tdb.CountVisits(); count != 0 {
		t.Fatalf("expected 0 visits in fresh DB, got %d", count)
	}
}

func TestInsertURL_ReturnsIncrementingIDs(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	id1 := tdb.InsertURL(urlEntry{URL: "https://a.com", Title: "A", VisitCount: 1})
	id2 := tdb.InsertURL(urlEntry{URL: "https://b.com", Title: "B", VisitCount: 1})

	if id1 >= id2 {
		t.Fatalf("expected incrementing IDs, got %d and %d", id1, id2)
	}
	if count := tdb.CountURLs(); count != 2 {
		t.Fatalf("expected 2 URLs, got %d", count)
	}
}

func TestInsertURLWithVisits_CreatesCorrectVisitCount(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	urlID := tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example", LastVisitTime: now,
	}, 5)

	if count := tdb.CountVisitsForURL(urlID); count != 5 {
		t.Fatalf("expected 5 visits, got %d", count)
	}
}

func TestSeedRealisticData_PopulatesDB(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	expectedVisible := tdb.SeedRealisticData()

	totalURLs := tdb.CountURLs()
	visibleURLs := tdb.CountVisibleURLs()
	totalVisits := tdb.CountVisits()

	if visibleURLs != expectedVisible {
		t.Fatalf("expected %d visible URLs, got %d", expectedVisible, visibleURLs)
	}

	// There's 1 hidden entry, so total should be visible + 1.
	if totalURLs != expectedVisible+1 {
		t.Fatalf("expected %d total URLs (including hidden), got %d", expectedVisible+1, totalURLs)
	}

	// Visits should be > URLs since some URLs have multiple visits.
	if totalVisits <= totalURLs {
		t.Fatalf("expected more visits than URLs, got %d visits for %d URLs", totalVisits, totalURLs)
	}
}

func TestSeedRealisticData_CompatibleWithGetAllURLs(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	expectedVisible := tdb.SeedRealisticData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed: %v", err)
	}

	if len(entries) != expectedVisible {
		t.Fatalf("getAllURLs returned %d entries, expected %d", len(entries), expectedVisible)
	}

	// Verify no hidden entries are returned.
	for _, e := range entries {
		if e.URL == "https://hidden.example.com/internal" {
			t.Fatal("getAllURLs should not return hidden entries")
		}
	}
}

func TestSeedRealisticData_FilteringWorks(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	tdb.SeedRealisticData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed: %v", err)
	}

	// Match google entries.
	googleMatches := filterEntries(entries, []string{"google"}, nil)
	if len(googleMatches) < 2 {
		t.Fatalf("expected at least 2 google matches, got %d", len(googleMatches))
	}

	// Match all, protect bank.
	nonBank := filterEntries(entries, []string{"*"}, []string{"bank"})
	allCount := len(entries)
	if len(nonBank) >= allCount {
		t.Fatalf("protect filter should reduce results: got %d out of %d", len(nonBank), allCount)
	}

	// Match ads/tracking.
	adMatches := filterEntries(entries, []string{"doubleclick", "tracker"}, nil)
	if len(adMatches) != 2 {
		t.Fatalf("expected 2 ad/tracker matches, got %d", len(adMatches))
	}
}

func TestTimeToChrome_RoundTrip(t *testing.T) {
	original := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	chromeTS := timeToChrome(original)
	roundTrip := chromeTimeToTime(chromeTS)

	if !roundTrip.UTC().Equal(original) {
		t.Fatalf("round-trip failed: %v -> %d -> %v", original, chromeTS, roundTrip.UTC())
	}
}

func TestCopyDBFile_CreatesUsableCopy(t *testing.T) {
	tdb := newTestDB(t)
	tdb.InsertURL(urlEntry{URL: "https://example.com", Title: "Test", VisitCount: 1})
	tdb.Close()

	destDir := t.TempDir()
	copyPath := tdb.CopyDBFile(destDir)

	// Verify the copy is a valid Chrome DB.
	if err := validateChromeDB(copyPath); err != nil {
		t.Fatalf("copy failed validation: %v", err)
	}

	// Verify data is present in the copy.
	db, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatalf("failed to open copy: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM urls").Scan(&count); err != nil {
		t.Fatalf("query copy failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 URL in copy, got %d", count)
	}
}

func TestCreateTestChromeDB_CreatesValidDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "History")

	createTestChromeDB(t, path)

	// The created file must pass Chrome DB validation.
	if err := validateChromeDB(path); err != nil {
		t.Fatalf("createTestChromeDB did not produce a valid Chrome DB: %v", err)
	}

	// Verify the file actually exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected DB file to exist at %s: %v", path, err)
	}
}

func TestCreateTestChromeDB_StartsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "History")

	createTestChromeDB(t, path)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open created DB: %v", err)
	}
	defer db.Close()

	var urlCount, visitCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM urls").Scan(&urlCount); err != nil {
		t.Fatalf("count urls failed: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM visits").Scan(&visitCount); err != nil {
		t.Fatalf("count visits failed: %v", err)
	}
	if urlCount != 0 || visitCount != 0 {
		t.Fatalf("expected empty DB, got %d urls and %d visits", urlCount, visitCount)
	}
}

func TestSeedMinimalData_PopulatesThreeEntries(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	n := tdb.SeedMinimalData()

	if n != 3 {
		t.Fatalf("SeedMinimalData returned %d, expected 3", n)
	}
	if got := tdb.CountURLs(); got != 3 {
		t.Fatalf("expected 3 URL rows, got %d", got)
	}
	if got := tdb.CountVisibleURLs(); got != 3 {
		t.Fatalf("expected 3 visible URL rows, got %d", got)
	}
}

func TestSeedMinimalData_CompatibleWithGetAllURLs(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	expected := tdb.SeedMinimalData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed: %v", err)
	}
	if len(entries) != expected {
		t.Fatalf("getAllURLs returned %d entries, expected %d", len(entries), expected)
	}
}

func TestSeedMinimalData_MatchAndProtectFilters(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	tdb.SeedMinimalData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs failed: %v", err)
	}

	// Match google; expect 1 result.
	googleMatches := filterEntries(entries, []string{"google"}, nil)
	if len(googleMatches) != 1 {
		t.Fatalf("expected 1 google match, got %d", len(googleMatches))
	}

	// Wildcard with bank protected; expect 2 results (google + github).
	nonBank := filterEntries(entries, []string{"*"}, []string{"bank"})
	if len(nonBank) != 2 {
		t.Fatalf("expected 2 entries after protecting bank, got %d", len(nonBank))
	}
}
