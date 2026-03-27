package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- filterEntries tests ---

func TestFilterEntries_EmptyInput(t *testing.T) {
	// Empty entries slice returns empty result.
	result := filterEntries(nil, []string{"foo"}, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result))
	}

	result = filterEntries([]HistoryEntry{}, []string{"foo"}, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result))
	}
}

func TestFilterEntries_EmptyMatchList(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example"},
	}
	result := filterEntries(entries, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 results with nil matchList, got %d", len(result))
	}

	result = filterEntries(entries, []string{}, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 results with empty matchList, got %d", len(result))
	}
}

func TestFilterEntries_MatchByURL(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com/page", Title: "Page"},
		{ID: 2, URL: "https://other.org/stuff", Title: "Stuff"},
	}
	result := filterEntries(entries, []string{"example"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected entry 1, got %v", result)
	}
}

func TestFilterEntries_MatchByTitle(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com", Title: "Go Programming"},
		{ID: 2, URL: "https://b.com", Title: "Rust Programming"},
	}
	result := filterEntries(entries, []string{"go programming"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected entry 1, got %v", result)
	}
}

func TestFilterEntries_CaseInsensitive(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://GitHub.com/Repo", Title: "My REPO"},
	}
	// Match list is lowercase, URL/title have mixed case.
	result := filterEntries(entries, []string{"github"}, nil)
	if len(result) != 1 {
		t.Fatalf("expected case-insensitive match on URL, got %d", len(result))
	}

	result = filterEntries(entries, []string{"my repo"}, nil)
	if len(result) != 1 {
		t.Fatalf("expected case-insensitive match on title, got %d", len(result))
	}

	// Upper-case in match list should still work (matches() lowercases URL/title).
	result = filterEntries(entries, []string{"GITHUB"}, nil)
	// matches() does NOT lowercase the matchList keywords — they come from
	// loadFilters which lowercases them. So "GITHUB" won't match "github.com".
	if len(result) != 0 {
		t.Fatalf("expected no match for upper-case keyword, got %d", len(result))
	}
}

func TestFilterEntries_WildcardMatchesAll(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com", Title: "A"},
		{ID: 2, URL: "https://b.com", Title: "B"},
	}
	result := filterEntries(entries, []string{"*"}, nil)
	if len(result) != 2 {
		t.Fatalf("expected wildcard to match all, got %d", len(result))
	}
}

func TestFilterEntries_ProtectListExcludes(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example"},
		{ID: 2, URL: "https://bank.example.com", Title: "My Bank"},
	}
	result := filterEntries(entries, []string{"example"}, []string{"bank"})
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected only entry 1 (bank protected), got %v", result)
	}
}

func TestFilterEntries_ProtectListByTitle(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://news.com", Title: "Important Banking News"},
		{ID: 2, URL: "https://news.com", Title: "Sports News"},
	}
	result := filterEntries(entries, []string{"news"}, []string{"banking"})
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("expected only entry 2, got %v", result)
	}
}

func TestFilterEntries_ProtectOverridesWildcard(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://safe.com", Title: "Safe"},
		{ID: 2, URL: "https://secret.com", Title: "Secret"},
	}
	result := filterEntries(entries, []string{"*"}, []string{"secret"})
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected protect to override wildcard, got %v", result)
	}
}

func TestFilterEntries_NoMatch(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example"},
	}
	result := filterEntries(entries, []string{"nonexistent"}, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result))
	}
}

func TestFilterEntries_MultipleMatchKeywords(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://foo.com", Title: "Foo"},
		{ID: 2, URL: "https://bar.com", Title: "Bar"},
		{ID: 3, URL: "https://baz.com", Title: "Baz"},
	}
	result := filterEntries(entries, []string{"foo", "bar"}, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
}

func TestFilterEntries_PreservesOrder(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 3, URL: "https://c.com", Title: "C"},
		{ID: 1, URL: "https://a.com", Title: "A"},
		{ID: 2, URL: "https://b.com", Title: "B"},
	}
	result := filterEntries(entries, []string{"*"}, nil)
	if result[0].ID != 3 || result[1].ID != 1 || result[2].ID != 2 {
		t.Fatalf("expected original order preserved, got IDs %d,%d,%d",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

// --- matches tests ---

func TestMatches_EmptyLists(t *testing.T) {
	if matches("https://x.com", "X", nil, nil) {
		t.Fatal("expected false with nil matchList")
	}
}

func TestMatches_ProtectTakesPrecedence(t *testing.T) {
	// Even though "example" matches, "example" is also in protect list.
	if matches("https://example.com", "Example", []string{"example"}, []string{"example"}) {
		t.Fatal("expected protect to take precedence")
	}
}

// --- chromeTimeToTime tests ---

func TestChromeTimeToTime_Zero(t *testing.T) {
	result := chromeTimeToTime(0)
	if !result.IsZero() {
		t.Fatalf("expected zero time, got %v", result)
	}
}

func TestChromeTimeToTime_KnownValue(t *testing.T) {
	// Chrome epoch: 1601-01-01 00:00:00 UTC
	// Verify by computing the expected Unix microseconds directly.
	ct := int64(13228684800000000)
	result := chromeTimeToTime(ct)
	// The underlying calculation: unixMicro = ct - 11644473600*1e6
	expectedUnixMicro := ct - 11644473600*1000000
	expected := time.UnixMicro(expectedUnixMicro)
	if !result.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
	// Sanity-check the year.
	if result.UTC().Year() != 2020 {
		t.Fatalf("expected year 2020, got %d", result.UTC().Year())
	}
}

func TestChromeTimeToTime_ChromeEpochStart(t *testing.T) {
	// Chrome timestamp 1 microsecond should map to 1601-01-01 00:00:00.000001 UTC.
	result := chromeTimeToTime(1)
	if result.UTC().Year() != 1601 || result.UTC().Month() != time.January || result.UTC().Day() != 1 {
		t.Fatalf("expected 1601-01-01, got %v", result.UTC())
	}
}

func TestChromeTimeToTime_UnixEpoch(t *testing.T) {
	// The Unix epoch (1970-01-01 00:00:00 UTC) in Chrome time is exactly
	// 11644473600 seconds = 11644473600000000 microseconds.
	ct := int64(11644473600 * 1000000)
	result := chromeTimeToTime(ct)
	expected := time.Unix(0, 0).UTC()
	if !result.UTC().Equal(expected) {
		t.Fatalf("expected Unix epoch %v, got %v", expected, result.UTC())
	}
}

func TestChromeTimeToTime_RecentTimestamp(t *testing.T) {
	// 2024-01-01 00:00:00 UTC
	target := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Convert to Chrome timestamp: add Chrome-to-Unix offset in microseconds.
	ct := target.UnixMicro() + 11644473600*1000000
	result := chromeTimeToTime(ct)
	if !result.UTC().Equal(target) {
		t.Fatalf("expected %v, got %v", target, result.UTC())
	}
}

func TestChromeTimeToTime_NegativeValue(t *testing.T) {
	// Negative Chrome timestamps are technically invalid but should not panic.
	// Just verify the function doesn't crash.
	result := chromeTimeToTime(-1)
	if result.IsZero() {
		t.Fatal("negative value should not return zero time (only 0 does)")
	}
}

func TestChromeTimeToTime_LargeValue(t *testing.T) {
	// A far-future timestamp: year ~2100.
	target := time.Date(2100, 6, 15, 12, 0, 0, 0, time.UTC)
	ct := target.UnixMicro() + 11644473600*1000000
	result := chromeTimeToTime(ct)
	if result.UTC().Year() != 2100 {
		t.Fatalf("expected year 2100, got %d", result.UTC().Year())
	}
}

func TestChromeTimeToTime_RoundTrip(t *testing.T) {
	// Verify multiple timestamps round-trip correctly through the conversion.
	dates := []time.Time{
		time.Date(2000, 3, 15, 10, 30, 0, 0, time.UTC),
		time.Date(2010, 7, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
	}
	for _, d := range dates {
		ct := d.UnixMicro() + 11644473600*1000000
		result := chromeTimeToTime(ct).UTC().Truncate(time.Second)
		expected := d.Truncate(time.Second)
		if !result.Equal(expected) {
			t.Errorf("round-trip failed for %v: got %v", d, result)
		}
	}
}

func TestChromeTimeToTime_MicrosecondPrecision(t *testing.T) {
	// Verify that sub-second microsecond precision is preserved through
	// the conversion. Use a timestamp with a known fractional second.
	base := time.Date(2023, 5, 17, 8, 30, 0, 0, time.UTC)
	extraMicros := int64(123456) // 0.123456 s
	ct := base.UnixMicro() + extraMicros + 11644473600*1000000

	result := chromeTimeToTime(ct).UTC()

	// The result must agree with base to full microsecond resolution.
	expectedMicro := base.UnixMicro() + extraMicros
	if result.UnixMicro() != expectedMicro {
		t.Fatalf("microsecond precision lost: expected UnixMicro %d, got %d",
			expectedMicro, result.UnixMicro())
	}
}

func TestChromeTimeToTime_LeapYearDate(t *testing.T) {
	// 2024-02-29 is a valid leap-year date; verify it survives the round-trip.
	target := time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC)
	ct := target.UnixMicro() + 11644473600*1000000

	result := chromeTimeToTime(ct).UTC()

	if result.Year() != 2024 || result.Month() != time.February || result.Day() != 29 {
		t.Fatalf("expected 2024-02-29, got %v", result)
	}
}

func TestChromeTimeToTime_YearBoundary(t *testing.T) {
	// Test the last microsecond of a year and the first microsecond of the next.
	lastMoment := time.Date(2022, 12, 31, 23, 59, 59, 999999000, time.UTC) // last μs of 2022
	firstMoment := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)            // start of 2023

	for _, tc := range []struct {
		name string
		t    time.Time
	}{
		{"last_moment_of_2022", lastMoment},
		{"first_moment_of_2023", firstMoment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ct := tc.t.UnixMicro() + 11644473600*1000000
			result := chromeTimeToTime(ct).UTC()
			if !result.Equal(tc.t) {
				t.Fatalf("expected %v, got %v", tc.t, result)
			}
		})
	}
}

func TestChromeTimeToTime_MaxInt64NoPanic(t *testing.T) {
	// math.MaxInt64 is an invalid Chrome timestamp but must not panic;
	// the function should return some time.Time without crashing.
	const maxInt64 = int64(^uint64(0) >> 1)
	// Just ensure no panic — we do not assert a specific result.
	_ = chromeTimeToTime(maxInt64)
}

func TestChromeTimeToTime_ReturnsUTCCompatible(t *testing.T) {
	// time.UnixMicro returns local time; calling .UTC() must produce the
	// canonical UTC representation with the same instant.
	ct := int64(13228684800000000) // ~2020
	result := chromeTimeToTime(ct)
	resultUTC := result.UTC()

	// The two representations must be the same instant.
	if !result.Equal(resultUTC) {
		t.Fatalf("result and result.UTC() represent different instants: %v vs %v", result, resultUTC)
	}
}

// --- formatTime tests ---

func TestFormatTime_Zero(t *testing.T) {
	if s := formatTime(0); s != "" {
		t.Fatalf("expected empty string for zero, got %q", s)
	}
}

func TestFormatTime_NonZero(t *testing.T) {
	ct := int64(13228684800000000) // 2020-01-01 00:00:00 UTC
	s := formatTime(ct)
	if s == "" {
		t.Fatal("expected non-empty formatted time")
	}
	// Should contain the date portion at minimum.
	if len(s) < 10 {
		t.Fatalf("formatted time too short: %q", s)
	}
}

func TestFormatTime_OutputFormat(t *testing.T) {
	// 2024-03-15 12:30:45 UTC expressed as a Chrome timestamp.
	target := time.Date(2024, 3, 15, 12, 30, 45, 0, time.UTC)
	ct := target.UnixMicro() + 11644473600*1000000

	// formatTime uses the local timezone; convert expected to local for comparison.
	expected := target.In(time.Local).Format("2006-01-02 15:04:05")
	got := formatTime(ct)
	if got != expected {
		t.Fatalf("formatTime output = %q, want %q", got, expected)
	}
}

func TestFormatTime_NegativeReturnsNonEmpty(t *testing.T) {
	// Negative Chrome timestamps are invalid but formatTime must not return ""
	// (empty is reserved for the zero sentinel). The exact value is unimportant.
	s := formatTime(-1)
	if s == "" {
		t.Fatal("negative timestamp should produce a non-empty string (not the zero sentinel)")
	}
}

// --- truncate tests ---

func TestTruncate_ShortString(t *testing.T) {
	if s := truncate("hello", 10); s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	if s := truncate("hello", 5); s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
}

func TestTruncate_Truncated(t *testing.T) {
	if s := truncate("hello world", 5); s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
}

func TestTruncate_Unicode(t *testing.T) {
	// Multi-byte (Korean) input of 5 runes truncated to 3 should return the first 3 runes.
	if s := truncate("가나다라마", 3); s != "가나다" {
		t.Fatalf("expected '가나다', got %q", s)
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	if s := truncate("", 5); s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
}

// --- loadFilters tests ---

func TestLoadFilters_Empty(t *testing.T) {
	f, err := loadFilters("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil {
		t.Fatalf("expected nil, got %v", f)
	}
}

func TestLoadFilters_Wildcard(t *testing.T) {
	f, err := loadFilters("*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 1 || f[0] != "*" {
		t.Fatalf("expected [*], got %v", f)
	}
}

func TestLoadFilters_CommaSeparated(t *testing.T) {
	f, err := loadFilters("foo, BAR ,baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"foo", "bar", "baz"}
	if len(f) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, f)
	}
	for i := range expected {
		if f[i] != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], f[i])
		}
	}
}

func TestLoadFilters_SingleKeyword(t *testing.T) {
	f, err := loadFilters("FooBar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 1 || f[0] != "foobar" {
		t.Fatalf("expected [foobar], got %v", f)
	}
}

func TestLoadFilters_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "filters.txt")
	err := os.WriteFile(path, []byte("Alpha\nBeta\n\nGamma\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	f, err := loadFilters(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"alpha", "beta", "gamma"}
	if len(f) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, f)
	}
	for i := range expected {
		if f[i] != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], f[i])
		}
	}
}

func TestLoadFilters_RejectsNullByte(t *testing.T) {
	_, err := loadFilters("foo\x00bar")
	if err == nil {
		t.Fatal("expected error for null byte in filter value")
	}
}

func TestLoadFilters_RejectsControlChar(t *testing.T) {
	_, err := loadFilters("foo\x07bar")
	if err == nil {
		t.Fatal("expected error for control character in filter value")
	}
}

func TestLoadFilters_SpecialCharsInKeywords(t *testing.T) {
	// SQL-like special characters should be handled safely since
	// filtering is done in-memory, not via SQL.
	f, err := loadFilters("'; DROP TABLE urls;--,Robert'); DROP TABLE--")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 2 {
		t.Fatalf("expected 2 keywords, got %d: %v", len(f), f)
	}
}

func TestLoadFilters_UnicodeKeywords(t *testing.T) {
	f, err := loadFilters("카페,café,日本語")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 3 {
		t.Fatalf("expected 3 keywords, got %d: %v", len(f), f)
	}
}

// --- sortEntries tests ---

func TestSortEntries_Empty(t *testing.T) {
	// Nil and empty slices must not panic.
	sortEntries(nil)
	sortEntries([]HistoryEntry{})
}

func TestSortEntries_SingleEntry(t *testing.T) {
	entries := []HistoryEntry{{ID: 1, LastVisitTime: 100}}
	sortEntries(entries)
	if entries[0].ID != 1 {
		t.Fatalf("single entry should remain, got id=%d", entries[0].ID)
	}
}

func TestSortEntries_Descending(t *testing.T) {
	// Entries arrive in ascending order; sort must reorder to descending.
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 100},
		{ID: 2, LastVisitTime: 300},
		{ID: 3, LastVisitTime: 200},
	}
	sortEntries(entries)
	if entries[0].ID != 2 || entries[1].ID != 3 || entries[2].ID != 1 {
		t.Fatalf("expected order 2,3,1 (by descending LastVisitTime), got %d,%d,%d",
			entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSortEntries_AlreadySorted(t *testing.T) {
	// Entries already in descending order; result must be unchanged.
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 300},
		{ID: 2, LastVisitTime: 200},
		{ID: 3, LastVisitTime: 100},
	}
	sortEntries(entries)
	if entries[0].ID != 1 || entries[1].ID != 2 || entries[2].ID != 3 {
		t.Fatalf("expected order preserved 1,2,3, got %d,%d,%d",
			entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSortEntries_StableOnEqualTimestamps(t *testing.T) {
	// Entries with the same timestamp retain their original relative order
	// because we use a stable sort.
	entries := []HistoryEntry{
		{ID: 10, LastVisitTime: 500},
		{ID: 20, LastVisitTime: 500},
		{ID: 30, LastVisitTime: 500},
	}
	sortEntries(entries)
	if entries[0].ID != 10 || entries[1].ID != 20 || entries[2].ID != 30 {
		t.Fatalf("stable sort: expected 10,20,30 for equal timestamps, got %d,%d,%d",
			entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSortEntries_ZeroTimestampLast(t *testing.T) {
	// A zero LastVisitTime (unknown date) should sort after real timestamps.
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 0},
		{ID: 2, LastVisitTime: 999},
		{ID: 3, LastVisitTime: 500},
	}
	sortEntries(entries)
	// Descending: 999 > 500 > 0 => IDs 2, 3, 1
	if entries[0].ID != 2 || entries[1].ID != 3 || entries[2].ID != 1 {
		t.Fatalf("expected zero timestamp last (order 2,3,1), got %d,%d,%d",
			entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSortEntries_DoesNotModifySliceLength(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 100},
		{ID: 2, LastVisitTime: 200},
	}
	sortEntries(entries)
	if len(entries) != 2 {
		t.Fatalf("sort must not change slice length, got %d", len(entries))
	}
}

func TestSortEntries_ChromeTimestamps(t *testing.T) {
	// Use realistic Chrome timestamps to ensure correct ordering.
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMicro() + 11644473600*1000000
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).UnixMicro() + 11644473600*1000000
	t3 := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC).UnixMicro() + 11644473600*1000000

	entries := []HistoryEntry{
		{ID: 1, URL: "https://jan.example.com", LastVisitTime: t1},
		{ID: 2, URL: "https://jun.example.com", LastVisitTime: t2},
		{ID: 3, URL: "https://dec.example.com", LastVisitTime: t3},
	}
	sortEntries(entries)
	// Most recent first: June (t2) > January (t1) > December-2023 (t3)
	if entries[0].ID != 2 || entries[1].ID != 1 || entries[2].ID != 3 {
		t.Fatalf("expected most-recent-first ordering (2,1,3), got %d,%d,%d",
			entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

// --- uniqueByURL tests ---

func TestUniqueByURL_Empty(t *testing.T) {
	// Nil and empty slices must not panic and return empty.
	if r := uniqueByURL(nil); len(r) != 0 {
		t.Fatalf("expected empty result for nil input, got %d", len(r))
	}
	if r := uniqueByURL([]HistoryEntry{}); len(r) != 0 {
		t.Fatalf("expected empty result for empty input, got %d", len(r))
	}
}

func TestUniqueByURL_AllUnique(t *testing.T) {
	// No duplicates: all entries must be retained.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com"},
		{ID: 2, URL: "https://b.com"},
		{ID: 3, URL: "https://c.com"},
	}
	result := uniqueByURL(entries)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique entries, got %d", len(result))
	}
}

func TestUniqueByURL_AllSameURL(t *testing.T) {
	// All entries share a URL; only the first should survive.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://same.com", LastVisitTime: 100},
		{ID: 2, URL: "https://same.com", LastVisitTime: 300},
		{ID: 3, URL: "https://same.com", LastVisitTime: 200},
	}
	result := uniqueByURL(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry for all-same URL, got %d", len(result))
	}
	if result[0].ID != 1 {
		t.Fatalf("expected first entry (ID=1) to be kept, got ID=%d", result[0].ID)
	}
}

func TestUniqueByURL_KeepsFirstOccurrence(t *testing.T) {
	// uniqueByURL keeps the first occurrence; when combined with sortEntries
	// (newest first), this means the most recent entry is kept.
	entries := []HistoryEntry{
		{ID: 10, URL: "https://dup.com", LastVisitTime: 999},
		{ID: 20, URL: "https://other.com", LastVisitTime: 500},
		{ID: 30, URL: "https://dup.com", LastVisitTime: 111},
	}
	result := uniqueByURL(entries)
	if len(result) != 2 {
		t.Fatalf("expected 2 unique entries, got %d", len(result))
	}
	// First occurrence of dup.com is ID=10 (highest LastVisitTime after sort).
	if result[0].ID != 10 {
		t.Fatalf("expected ID=10 (first occurrence), got %d", result[0].ID)
	}
	if result[1].ID != 20 {
		t.Fatalf("expected ID=20 second, got %d", result[1].ID)
	}
}

func TestUniqueByURL_PreservesOrder(t *testing.T) {
	// Order of first occurrences must be preserved.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://b.com"},
		{ID: 2, URL: "https://a.com"},
		{ID: 3, URL: "https://b.com"}, // duplicate
		{ID: 4, URL: "https://c.com"},
	}
	result := uniqueByURL(entries)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique entries, got %d", len(result))
	}
	ids := []int64{result[0].ID, result[1].ID, result[2].ID}
	expected := []int64{1, 2, 4}
	for i, id := range ids {
		if id != expected[i] {
			t.Fatalf("position %d: expected ID=%d, got ID=%d", i, expected[i], id)
		}
	}
}

func TestUniqueByURL_DoesNotMutateInput(t *testing.T) {
	// The function must not modify the input slice.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com"},
		{ID: 2, URL: "https://a.com"},
	}
	original := make([]HistoryEntry, len(entries))
	copy(original, entries)

	uniqueByURL(entries)

	for i := range original {
		if entries[i].ID != original[i].ID || entries[i].URL != original[i].URL {
			t.Fatalf("input slice was mutated at index %d", i)
		}
	}
}

func TestSortThenUnique_MostRecentKept(t *testing.T) {
	// Combining sortEntries + uniqueByURL should keep the most recent entry
	// for each URL (since sort puts newest first and unique keeps first).
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMicro() + 11644473600*1000000
	recent := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).UnixMicro() + 11644473600*1000000

	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", LastVisitTime: old},
		{ID: 2, URL: "https://other.com", LastVisitTime: recent},
		{ID: 3, URL: "https://example.com", LastVisitTime: recent},
	}

	sortEntries(entries)
	result := uniqueByURL(entries)

	if len(result) != 2 {
		t.Fatalf("expected 2 unique entries, got %d", len(result))
	}
	// example.com should be kept with the recent entry (ID=3), not the old one (ID=1).
	var exampleEntry HistoryEntry
	for _, e := range result {
		if e.URL == "https://example.com" {
			exampleEntry = e
			break
		}
	}
	if exampleEntry.ID != 3 {
		t.Fatalf("expected most-recent example.com entry (ID=3), got ID=%d", exampleEntry.ID)
	}
}

// --- Additional filterEntries keyword filtering tests ---

func TestFilterEntries_URLWithQueryParams(t *testing.T) {
	// Keywords should match against query parameters in the URL.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://search.com/q?query=golang+tutorial", Title: "Search"},
		{ID: 2, URL: "https://search.com/q?query=rust+lang", Title: "Search"},
	}
	result := filterEntries(entries, []string{"golang"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected match on URL query param, got %v", result)
	}
}

func TestFilterEntries_SubstringMatch(t *testing.T) {
	// Keyword must match as a substring anywhere in URL or title.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://docs.golang.org/pkg/fmt/", Title: "fmt package - Go"},
		{ID: 2, URL: "https://pkg.go.dev/net/http", Title: "http package"},
	}
	// "golang" appears only in entry 1
	result := filterEntries(entries, []string{"golang"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected substring match in URL, got %v", result)
	}
	// "package" appears in the title of both entries
	result = filterEntries(entries, []string{"package"}, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 substring matches via title, got %d", len(result))
	}
}

func TestFilterEntries_MultipleProtectKeywords(t *testing.T) {
	// Multiple protect keywords should each be able to exclude an entry.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://bank.example.com", Title: "My Bank"},
		{ID: 2, URL: "https://pay.example.com", Title: "Payment Portal"},
		{ID: 3, URL: "https://blog.example.com", Title: "Tech Blog"},
	}
	result := filterEntries(entries, []string{"example"}, []string{"bank", "pay"})
	if len(result) != 1 || result[0].ID != 3 {
		t.Fatalf("expected only blog entry after multiple protect keywords, got %v", result)
	}
}

func TestFilterEntries_EmptyURLAndTitle(t *testing.T) {
	// An entry with empty URL and title should not match any keyword except wildcard.
	entries := []HistoryEntry{
		{ID: 1, URL: "", Title: ""},
		{ID: 2, URL: "https://example.com", Title: "Example"},
	}
	result := filterEntries(entries, []string{"example"}, nil)
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("empty URL/title should not match keyword 'example', got %v", result)
	}

	// Wildcard should match even empty URL/title entries.
	result = filterEntries(entries, []string{"*"}, nil)
	if len(result) != 2 {
		t.Fatalf("wildcard should match entry with empty URL/title, got %d", len(result))
	}
}

func TestFilterEntries_MatchedByURLNotTitle(t *testing.T) {
	// Keyword present only in URL (not in title) must still match.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://developer.mozilla.org/docs/Web/CSS", Title: "CSS Documentation"},
		{ID: 2, URL: "https://example.com", Title: "Homepage"},
	}
	result := filterEntries(entries, []string{"mozilla"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected URL-only match, got %v", result)
	}
}

func TestFilterEntries_MatchedByTitleNotURL(t *testing.T) {
	// Keyword present only in title (not in URL) must still match.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com/12345", Title: "How to Learn Go Programming"},
		{ID: 2, URL: "https://b.com/67890", Title: "Daily News"},
	}
	result := filterEntries(entries, []string{"go programming"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected title-only match, got %v", result)
	}
}

func TestFilterEntries_ProtectKeywordOnlyInURL(t *testing.T) {
	// Protect keyword present only in the URL (title has no match) still excludes.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://ads.tracking.com/pixel", Title: "Image"},
		{ID: 2, URL: "https://clean.com/page", Title: "Clean Page"},
	}
	result := filterEntries(entries, []string{"*"}, []string{"tracking"})
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("tracking URL should be excluded via protect keyword in URL, got %v", result)
	}
}

func TestFilterEntries_AllProtected(t *testing.T) {
	// When all entries match the protect list, result should be empty.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://ads.com", Title: "Ads"},
		{ID: 2, URL: "https://tracker.com", Title: "Tracker Ads"},
	}
	result := filterEntries(entries, []string{"*"}, []string{"ads"})
	if len(result) != 0 {
		t.Fatalf("expected all entries protected, got %d", len(result))
	}
}

func TestFilterEntries_KeywordMatchesProtectAlso(t *testing.T) {
	// An entry that matches both matchList and protectList must be excluded
	// (protect takes precedence).
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example"},
	}
	result := filterEntries(entries, []string{"example"}, []string{"example"})
	if len(result) != 0 {
		t.Fatalf("protect must override match for the same keyword, got %d", len(result))
	}
}

func TestFilterEntries_LargeDataset(t *testing.T) {
	// filterEntries must remain correct with a large number of entries.
	const N = 10000
	entries := make([]HistoryEntry, N)
	for i := range entries {
		if i%2 == 0 {
			entries[i] = HistoryEntry{ID: int64(i), URL: "https://even.com", Title: "Even"}
		} else {
			entries[i] = HistoryEntry{ID: int64(i), URL: "https://odd.com", Title: "Odd"}
		}
	}
	result := filterEntries(entries, []string{"even"}, nil)
	if len(result) != N/2 {
		t.Fatalf("expected %d matches for large dataset, got %d", N/2, len(result))
	}
}

func TestFilterEntries_UnicodeURLAndTitle(t *testing.T) {
	// Unicode characters in URL/title should be matched correctly.
	entries := []HistoryEntry{
		{ID: 1, URL: "https://例え.jp/page", Title: "日本語サイト"},
		{ID: 2, URL: "https://example.com", Title: "English Site"},
	}
	result := filterEntries(entries, []string{"日本語"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected Unicode title match, got %v", result)
	}
}

// --- filterByDateRange tests ---

// chromeTS is a helper that converts a time.Time to a Chrome microsecond timestamp.
func chromeTS(t time.Time) int64 {
	return t.UnixMicro() + 11644473600*1000000
}

func TestFilterByDateRange_Empty(t *testing.T) {
	// Nil and empty slices must return empty without panic.
	result := filterByDateRange(nil, time.Time{}, time.Time{})
	if len(result) != 0 {
		t.Fatalf("nil input: expected 0 results, got %d", len(result))
	}
	result = filterByDateRange([]HistoryEntry{}, time.Time{}, time.Time{})
	if len(result) != 0 {
		t.Fatalf("empty input: expected 0 results, got %d", len(result))
	}
}

func TestFilterByDateRange_NoConstraints(t *testing.T) {
	// Both bounds zero: all entries pass through.
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))},
		{ID: 3, LastVisitTime: 0}, // zero timestamp (unknown date)
	}
	result := filterByDateRange(entries, time.Time{}, time.Time{})
	if len(result) != 3 {
		t.Fatalf("no-constraint filter must pass all entries, got %d", len(result))
	}
}

func TestFilterByDateRange_AfterOnly(t *testing.T) {
	// Only lower bound set: entries visited strictly before the bound are excluded.
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},  // boundary
		{ID: 3, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))}, // within
	}
	result := filterByDateRange(entries, after, time.Time{})
	// Boundary (ID=2) is included; before boundary (ID=1) is excluded.
	if len(result) != 2 {
		t.Fatalf("expected 2 entries with after-only bound, got %d", len(result))
	}
	if result[0].ID != 2 || result[1].ID != 3 {
		t.Fatalf("expected IDs 2,3 — got %d,%d", result[0].ID, result[1].ID)
	}
}

func TestFilterByDateRange_BeforeOnly(t *testing.T) {
	// Only upper bound set: entries visited strictly after the bound are excluded.
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))},  // within
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},  // boundary
		{ID: 3, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))}, // after bound
	}
	result := filterByDateRange(entries, time.Time{}, before)
	// Boundary (ID=2) is included; after boundary (ID=3) is excluded.
	if len(result) != 2 {
		t.Fatalf("expected 2 entries with before-only bound, got %d", len(result))
	}
	if result[0].ID != 1 || result[1].ID != 2 {
		t.Fatalf("expected IDs 1,2 — got %d,%d", result[0].ID, result[1].ID)
	}
}

func TestFilterByDateRange_BothBounds(t *testing.T) {
	// Entries outside [after, before] are excluded; those inside are retained.
	after := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 9, 30, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))}, // before range
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))},  // on after boundary
		{ID: 3, LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},  // mid range
		{ID: 4, LastVisitTime: chromeTS(time.Date(2024, 9, 30, 0, 0, 0, 0, time.UTC))}, // on before boundary
		{ID: 5, LastVisitTime: chromeTS(time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC))}, // after range
	}
	result := filterByDateRange(entries, after, before)
	if len(result) != 3 {
		t.Fatalf("expected 3 entries within range, got %d", len(result))
	}
	ids := []int64{result[0].ID, result[1].ID, result[2].ID}
	for i, want := range []int64{2, 3, 4} {
		if ids[i] != want {
			t.Fatalf("position %d: expected ID=%d, got ID=%d", i, want, ids[i])
		}
	}
}

func TestFilterByDateRange_AllExcluded(t *testing.T) {
	// All entries fall outside the specified range.
	after := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, after, before)
	if len(result) != 0 {
		t.Fatalf("expected 0 results when all entries outside range, got %d", len(result))
	}
}

func TestFilterByDateRange_AllIncluded(t *testing.T) {
	// All entries fall within the date range.
	after := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 3, 10, 0, 0, 0, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC))},
		{ID: 3, LastVisitTime: chromeTS(time.Date(2025, 11, 5, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, after, before)
	if len(result) != 3 {
		t.Fatalf("expected all 3 entries within wide range, got %d", len(result))
	}
}

func TestFilterByDateRange_ZeroTimestampExcludedWhenBoundSet(t *testing.T) {
	// Entries with zero LastVisitTime (unknown date) are excluded when either
	// bound is active, since we cannot verify they fall within the range.
	after := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 0},                                                       // zero = unknown
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))}, // valid
	}
	result := filterByDateRange(entries, after, time.Time{})
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("zero-timestamp entry should be excluded when bound is set, got %v", result)
	}
}

func TestFilterByDateRange_ZeroTimestampIncludedWhenNoBounds(t *testing.T) {
	// Zero LastVisitTime passes through when both bounds are zero (no constraint).
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 0},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, time.Time{}, time.Time{})
	if len(result) != 2 {
		t.Fatalf("zero-timestamp entry should pass when no bounds set, got %d", len(result))
	}
}

func TestFilterByDateRange_PreservesInputOrder(t *testing.T) {
	// Output order must match the input order.
	after := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 3, LastVisitTime: chromeTS(time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, after, time.Time{})
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if result[0].ID != 3 || result[1].ID != 1 || result[2].ID != 2 {
		t.Fatalf("expected original order 3,1,2 — got %d,%d,%d",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestFilterByDateRange_DoesNotMutateInput(t *testing.T) {
	// filterByDateRange must not modify the input slice.
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
	}
	original := make([]HistoryEntry, len(entries))
	copy(original, entries)

	filterByDateRange(entries, after, time.Time{})

	for i := range original {
		if entries[i].ID != original[i].ID || entries[i].LastVisitTime != original[i].LastVisitTime {
			t.Fatalf("input slice was mutated at index %d", i)
		}
	}
}

func TestFilterByDateRange_SameDayBoundaries(t *testing.T) {
	// Verify that after and before set to the same day returns only that day's entries.
	day := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	dayEnd := time.Date(2024, 6, 15, 23, 59, 59, 999999000, time.UTC)

	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2024, 6, 14, 12, 0, 0, 0, time.UTC))}, // day before
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 8, 0, 0, 0, time.UTC))},  // within day
		{ID: 3, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 18, 30, 0, 0, time.UTC))}, // within day
		{ID: 4, LastVisitTime: chromeTS(time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC))},  // day after
	}
	result := filterByDateRange(entries, day, dayEnd)
	if len(result) != 2 {
		t.Fatalf("expected 2 same-day entries, got %d", len(result))
	}
	if result[0].ID != 2 || result[1].ID != 3 {
		t.Fatalf("expected IDs 2,3 — got %d,%d", result[0].ID, result[1].ID)
	}
}

// --- Combined filter scenarios (keyword + date range) ---

func TestCombined_KeywordFilterThenDateRange(t *testing.T) {
	// Apply keyword filter first, then date range filter on the result.
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, URL: "https://golang.org", Title: "Go", LastVisitTime: chromeTS(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, URL: "https://golang.org/doc", Title: "Go Docs", LastVisitTime: chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 3, URL: "https://rust-lang.org", Title: "Rust", LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
	}
	// Step 1: filter by keyword "golang" → IDs 1, 2
	matched := filterEntries(entries, []string{"golang"}, nil)
	// Step 2: filter by date range (after 2024-01-01) → ID 2 only
	dated := filterByDateRange(matched, after, time.Time{})
	if len(dated) != 1 || dated[0].ID != 2 {
		t.Fatalf("expected only ID=2 after combined keyword+date filter, got %v", dated)
	}
}

func TestCombined_DateRangeThenKeywordFilter(t *testing.T) {
	// Apply date range filter first, then keyword filter on the result.
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, URL: "https://golang.org", Title: "Go", LastVisitTime: chromeTS(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, URL: "https://golang.org/doc", Title: "Go Docs", LastVisitTime: chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 3, URL: "https://rust-lang.org", Title: "Rust", LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
	}
	// Step 1: filter by date (after 2024-01-01) → IDs 2, 3
	dated := filterByDateRange(entries, after, time.Time{})
	// Step 2: filter by keyword "golang" → ID 2 only
	matched := filterEntries(dated, []string{"golang"}, nil)
	if len(matched) != 1 || matched[0].ID != 2 {
		t.Fatalf("expected only ID=2 after combined date+keyword filter, got %v", matched)
	}
}

func TestCombined_WildcardMatchWithDateRange(t *testing.T) {
	// Wildcard match scoped to a date range returns all entries in that range.
	after := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com", Title: "A", LastVisitTime: chromeTS(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))}, // before range
		{ID: 2, URL: "https://b.com", Title: "B", LastVisitTime: chromeTS(time.Date(2024, 7, 10, 0, 0, 0, 0, time.UTC))}, // in range
		{ID: 3, URL: "https://c.com", Title: "C", LastVisitTime: chromeTS(time.Date(2024, 9, 20, 0, 0, 0, 0, time.UTC))}, // in range
	}
	// All keywords match via wildcard, then narrow by date range.
	allMatched := filterEntries(entries, []string{"*"}, nil)
	inRange := filterByDateRange(allMatched, after, before)
	if len(inRange) != 2 {
		t.Fatalf("expected 2 entries in range after wildcard+date, got %d", len(inRange))
	}
}

func TestCombined_ProtectKeywordWithDateRange(t *testing.T) {
	// Protect keywords are applied during keyword filter stage; date range
	// is independent and applied to the already-filtered result.
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, URL: "https://news.com", Title: "Breaking News", LastVisitTime: chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, URL: "https://ads.news.com", Title: "Ad News", LastVisitTime: chromeTS(time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 3, URL: "https://news.com/tech", Title: "Tech News", LastVisitTime: chromeTS(time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC))},
	}
	// Keyword filter: match "news", protect "ads" → IDs 1, 3
	matched := filterEntries(entries, []string{"news"}, []string{"ads"})
	// Date range: after 2024-01-01 → ID 1 only (ID 3 is 2023)
	result := filterByDateRange(matched, after, time.Time{})
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected only ID=1 after protect+date filter, got %v", result)
	}
}

func TestCombined_NoResultAfterBothFilters(t *testing.T) {
	// Keyword filter matches some entries, but date range filter excludes them all.
	before := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, URL: "https://golang.org", Title: "Go", LastVisitTime: chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, URL: "https://golang.org/doc", Title: "Go Docs", LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
	}
	matched := filterEntries(entries, []string{"golang"}, nil)
	result := filterByDateRange(matched, time.Time{}, before)
	if len(result) != 0 {
		t.Fatalf("expected 0 results when date range excludes all keyword-matched entries, got %d", len(result))
	}
}

func TestCombined_SortThenDateRangeThenKeyword(t *testing.T) {
	// Full pipeline: sort by recent → date range → keyword filter.
	cutoff := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t1 := chromeTS(time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC))
	t2 := chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	t3 := chromeTS(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))

	entries := []HistoryEntry{
		{ID: 1, URL: "https://github.com/user/repo", Title: "Repository", LastVisitTime: t1},
		{ID: 2, URL: "https://github.com/org/proj", Title: "Project", LastVisitTime: t2},
		{ID: 3, URL: "https://gitlab.com/user/repo", Title: "GitLab Repo", LastVisitTime: t1},
		{ID: 4, URL: "https://github.com/old/repo", Title: "Old Repo", LastVisitTime: t3},
	}

	// Sort newest first.
	sortEntries(entries)
	// Filter by date: only entries from 2024 onward.
	inRange := filterByDateRange(entries, cutoff, time.Time{})
	// Filter by keyword: only GitHub entries.
	result := filterEntries(inRange, []string{"github"}, nil)

	// Expected: IDs 1 and 2 (both github.com, both in 2024), in sort order.
	if len(result) != 2 {
		t.Fatalf("expected 2 results from full pipeline, got %d", len(result))
	}
	// After sortEntries: t1 > t2, so ID=1 comes before ID=2.
	if result[0].ID != 1 || result[1].ID != 2 {
		t.Fatalf("expected IDs 1,2 (sort order) — got %d,%d", result[0].ID, result[1].ID)
	}
}

func TestCombined_MultipleKeywordsAndDateRange(t *testing.T) {
	// Multiple match keywords combined with a date range.
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, URL: "https://golang.org", Title: "Go", LastVisitTime: chromeTS(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, URL: "https://rust-lang.org", Title: "Rust", LastVisitTime: chromeTS(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 3, URL: "https://python.org", Title: "Python", LastVisitTime: chromeTS(time.Date(2023, 8, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 4, URL: "https://golang.org/blog", Title: "Go Blog", LastVisitTime: chromeTS(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))},
	}
	// Match golang or rust (IDs 1, 2, 4), then narrow to after 2024 (IDs 1, 2 only).
	matched := filterEntries(entries, []string{"golang", "rust"}, nil)
	result := filterByDateRange(matched, after, time.Time{})
	if len(result) != 2 {
		t.Fatalf("expected 2 results for multi-keyword + date, got %d", len(result))
	}
}

// --- Additional edge-case tests ---

// TestFilterEntries_DoesNotMutateInput verifies that filterEntries returns a new
// slice and never modifies the caller's input slice.
func TestFilterEntries_DoesNotMutateInput(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example"},
		{ID: 2, URL: "https://other.org", Title: "Other"},
	}
	original := make([]HistoryEntry, len(entries))
	copy(original, entries)

	_ = filterEntries(entries, []string{"example"}, nil)

	for i := range original {
		if entries[i].ID != original[i].ID ||
			entries[i].URL != original[i].URL ||
			entries[i].Title != original[i].Title {
			t.Fatalf("filterEntries mutated input at index %d", i)
		}
	}
}

// TestFilterEntries_ZeroValueEntry confirms that a zero-value HistoryEntry
// (empty URL and title) is excluded by specific keywords but matched by wildcard.
func TestFilterEntries_ZeroValueEntry(t *testing.T) {
	entries := []HistoryEntry{
		{}, // zero value: ID=0, URL="", Title=""
		{ID: 1, URL: "https://example.com", Title: "Example"},
	}
	// Specific keyword should not match the zero-value entry.
	result := filterEntries(entries, []string{"example"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("zero-value entry should not match keyword 'example', got %v", result)
	}
	// Wildcard matches everything, including zero-value entries.
	result = filterEntries(entries, []string{"*"}, nil)
	if len(result) != 2 {
		t.Fatalf("wildcard should match zero-value entry too, got %d", len(result))
	}
}

// TestFilterEntries_WildcardWithKeyword verifies that when matchList contains both
// a wildcard and a specific keyword, the wildcard causes all entries to match
// (the keyword is redundant).
func TestFilterEntries_WildcardWithKeyword(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://golang.org", Title: "Go"},
		{ID: 2, URL: "https://unrelated.com", Title: "Unrelated"},
	}
	// With wildcard present, every entry matches regardless of the other keyword.
	result := filterEntries(entries, []string{"golang", "*"}, nil)
	if len(result) != 2 {
		t.Fatalf("wildcard alongside keyword should match all entries, got %d", len(result))
	}
}

// TestFilterEntries_CaseSensitivity_UpperKeyword explicitly documents that
// filterEntries does NOT lowercase the matchList keywords — callers (loadFilters)
// are responsible for normalizing to lowercase before passing to filterEntries.
// Passing an uppercase keyword therefore does NOT match a lowercase URL.
func TestFilterEntries_CaseSensitivity_UpperKeyword(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://github.com/repo", Title: "Repository"},
	}
	// Upper-case keyword is not lowercased inside filterEntries; the URL is
	// lowercased but "GITHUB" != "github", so there is no match.
	result := filterEntries(entries, []string{"GITHUB"}, nil)
	if len(result) != 0 {
		t.Fatalf("upper-case keyword must not match lower-case URL (callers normalize to lower): got %d results", len(result))
	}
	// Lowercase keyword does match.
	result = filterEntries(entries, []string{"github"}, nil)
	if len(result) != 1 {
		t.Fatalf("lowercase keyword must match lower-cased URL, got %d results", len(result))
	}
}

// TestFilterEntries_SingleEntry_Matched confirms that a single-element input
// slice is handled correctly when the entry matches.
func TestFilterEntries_SingleEntry_Matched(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 42, URL: "https://golang.org", Title: "Go Programming Language"},
	}
	result := filterEntries(entries, []string{"golang"}, nil)
	if len(result) != 1 || result[0].ID != 42 {
		t.Fatalf("expected single matched entry with ID=42, got %v", result)
	}
}

// TestFilterEntries_SingleEntry_NotMatched confirms that a single-element input
// slice is handled correctly when the entry does not match.
func TestFilterEntries_SingleEntry_NotMatched(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 42, URL: "https://golang.org", Title: "Go Programming Language"},
	}
	result := filterEntries(entries, []string{"python"}, nil)
	if len(result) != 0 {
		t.Fatalf("expected no matches, got %v", result)
	}
}

// TestFilterEntries_ProtectByBothURLAndTitle confirms that when a protect keyword
// appears in both the URL and the title of the same entry, the entry is still
// excluded (protect check short-circuits on first match).
func TestFilterEntries_ProtectByBothURLAndTitle(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://ads.tracking.com/ads-page", Title: "Ads Portal"},
		{ID: 2, URL: "https://clean.com", Title: "Safe Site"},
	}
	result := filterEntries(entries, []string{"*"}, []string{"ads"})
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("entry matching protect keyword in both URL and title must be excluded, got %v", result)
	}
}

// TestFilterByDateRange_InvertedBounds documents behavior when the caller
// supplies after > before (logically impossible range). Every entry falls
// outside the inconsistent window, so the result must be empty.
func TestFilterByDateRange_InvertedBounds(t *testing.T) {
	// after is later than before — no entry can satisfy both constraints.
	after := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // before < after
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
		{ID: 2, LastVisitTime: chromeTS(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, after, before)
	if len(result) != 0 {
		t.Fatalf("inverted bounds should produce empty result, got %d entries", len(result))
	}
}

// TestFilterByDateRange_SingleEntry_Included verifies that a one-element slice
// passes through when the entry is within the date range.
func TestFilterByDateRange_SingleEntry_Included(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 7, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, after, time.Time{})
	if len(result) != 1 || result[0].ID != 7 {
		t.Fatalf("expected single entry to be included, got %v", result)
	}
}

// TestFilterByDateRange_SingleEntry_Excluded verifies that a one-element slice
// returns empty when the entry falls outside the date range.
func TestFilterByDateRange_SingleEntry_Excluded(t *testing.T) {
	after := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 7, LastVisitTime: chromeTS(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, after, time.Time{})
	if len(result) != 0 {
		t.Fatalf("expected single entry to be excluded, got %v", result)
	}
}

// TestFilterByDateRange_ZeroTimestampBeforeBound verifies that zero-timestamp
// entries are excluded when only the "before" bound is set (not just "after").
func TestFilterByDateRange_ZeroTimestampBeforeBound(t *testing.T) {
	before := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{ID: 1, LastVisitTime: 0}, // unknown date — must be excluded
		{ID: 2, LastVisitTime: chromeTS(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
	}
	result := filterByDateRange(entries, time.Time{}, before)
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("zero-timestamp entry must be excluded when before-only bound is set, got %v", result)
	}
}

// TestFilterEntries_KeywordInURLFragmentAndHash confirms matching against URL
// fragment identifiers (the "#anchor" part) and hash-based paths.
func TestFilterEntries_KeywordInURLFragmentAndHash(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://docs.example.com/guide#installation", Title: "Guide"},
		{ID: 2, URL: "https://app.com/#/dashboard", Title: "App"},
		{ID: 3, URL: "https://other.com/page", Title: "Page"},
	}
	// "installation" appears only in the URL fragment of entry 1.
	result := filterEntries(entries, []string{"installation"}, nil)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("expected fragment match on entry 1, got %v", result)
	}
	// "dashboard" appears in the hash-based path of entry 2.
	result = filterEntries(entries, []string{"dashboard"}, nil)
	if len(result) != 1 || result[0].ID != 2 {
		t.Fatalf("expected hash-path match on entry 2, got %v", result)
	}
}

// TestFilterEntries_MatchListORLogic confirms that entries are matched with OR
// semantics across the matchList: an entry matching ANY keyword is included.
func TestFilterEntries_MatchListORLogic(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://golang.org", Title: "Go"},       // matches keyword[0]
		{ID: 2, URL: "https://rust-lang.org", Title: "Rust"},  // matches keyword[1]
		{ID: 3, URL: "https://python.org", Title: "Python"},   // matches keyword[2]
		{ID: 4, URL: "https://unrelated.com", Title: "Other"}, // matches nothing
	}
	result := filterEntries(entries, []string{"golang", "rust", "python"}, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 entries via OR-logic across keywords, got %d", len(result))
	}
	// The unmatched entry must not appear.
	for _, e := range result {
		if e.ID == 4 {
			t.Fatal("entry 4 should not have matched any keyword")
		}
	}
}

// TestFilterEntries_MatchListANDProtectLogic confirms that the protect list
// applies with AND-NOT logic: an entry must match the matchList AND must NOT
// match any protect keyword.
func TestFilterEntries_MatchListANDProtectLogic(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://news.com/technology", Title: "Tech News"},     // match + not protected
		{ID: 2, URL: "https://news.com/advertising", Title: "Ad News"},      // match + protected by "ad"
		{ID: 3, URL: "https://news.com/finance", Title: "Finance News"},     // match + not protected
		{ID: 4, URL: "https://tracking.com/pixel", Title: "Tracking Pixel"}, // match + protected by "tracking"
	}
	result := filterEntries(entries, []string{"news", "tracking"}, []string{"ad", "tracking"})
	// IDs 2 (has "ad") and 4 (has "tracking") are protected; IDs 1 and 3 survive.
	if len(result) != 2 {
		t.Fatalf("expected 2 results after AND-NOT protect logic, got %d", len(result))
	}
	for _, e := range result {
		if e.ID == 2 || e.ID == 4 {
			t.Fatalf("entry %d should have been excluded by protect list", e.ID)
		}
	}
}
