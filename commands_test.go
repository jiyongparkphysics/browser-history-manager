package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"chrome-history-manager/internal/history"
)

// utf8BOM is the byte sequence for a UTF-8 Byte Order Mark.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func TestWriteCSV_Header(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, nil); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	data := buf.Bytes()

	// Verify BOM prefix.
	if !bytes.HasPrefix(data, utf8BOM) {
		t.Error("output missing UTF-8 BOM prefix")
	}

	// Strip BOM and parse CSV.
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record (header only), got %d", len(records))
	}

	want := []string{"Title", "URL", "Visit Count", "Last Visit"}
	for i, col := range want {
		if records[0][i] != col {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], col)
		}
	}
}

func TestWriteCSV_BasicEntry(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", VisitCount: 5, LastVisitTime: 0},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records (header + 1 row), got %d", len(records))
	}

	row := records[1]
	if row[0] != "Example" {
		t.Errorf("title = %q, want %q", row[0], "Example")
	}
	if row[1] != "https://example.com" {
		t.Errorf("url = %q, want %q", row[1], "https://example.com")
	}
	if row[2] != "5" {
		t.Errorf("visit count = %q, want %q", row[2], "5")
	}
}

func TestWriteCSV_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name  string
		entry HistoryEntry
		// Expected values after CSV round-trip parse.
		wantTitle string
		wantURL   string
	}{
		{
			name:      "commas in title",
			entry:     HistoryEntry{Title: "Buy shoes, shirts, pants", URL: "https://shop.example.com", VisitCount: 1},
			wantTitle: "Buy shoes, shirts, pants",
			wantURL:   "https://shop.example.com",
		},
		{
			name:      "double quotes in title",
			entry:     HistoryEntry{Title: `He said "hello"`, URL: "https://example.com", VisitCount: 2},
			wantTitle: `He said "hello"`,
			wantURL:   "https://example.com",
		},
		{
			name:      "newline in title",
			entry:     HistoryEntry{Title: "Line1\nLine2", URL: "https://example.com/nl", VisitCount: 1},
			wantTitle: "Line1\nLine2",
			wantURL:   "https://example.com/nl",
		},
		{
			name:      "unicode characters",
			entry:     HistoryEntry{Title: "日本語のページ — ñoño", URL: "https://example.com/日本語", VisitCount: 3},
			wantTitle: "日本語のページ — ñoño",
			wantURL:   "https://example.com/日本語",
		},
		{
			name:      "empty title",
			entry:     HistoryEntry{Title: "", URL: "https://example.com/empty", VisitCount: 1},
			wantTitle: "",
			wantURL:   "https://example.com/empty",
		},
		{
			name:      "URL with query params",
			entry:     HistoryEntry{Title: "Search", URL: "https://example.com/search?q=a&b=c,d", VisitCount: 1},
			wantTitle: "Search",
			wantURL:   "https://example.com/search?q=a&b=c,d",
		},
		{
			name:  "tab and carriage return",
			entry: HistoryEntry{Title: "Tab\there\r\nCRLF", URL: "https://example.com/ws", VisitCount: 1},
			// CSV readers normalize \r\n to \n, so the round-trip strips \r.
			wantTitle: "Tab\there\nCRLF",
			wantURL:   "https://example.com/ws",
		},
		{
			name:      "backslash and semicolons",
			entry:     HistoryEntry{Title: `Path: C:\Users\test; data=1`, URL: "https://example.com", VisitCount: 1},
			wantTitle: `Path: C:\Users\test; data=1`,
			wantURL:   "https://example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeCSV(&buf, []HistoryEntry{tc.entry}); err != nil {
				t.Fatalf("writeCSV returned error: %v", err)
			}

			r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
			records, err := r.ReadAll()
			if err != nil {
				t.Fatalf("failed to parse CSV: %v", err)
			}

			if len(records) != 2 {
				t.Fatalf("expected 2 records, got %d", len(records))
			}

			row := records[1]
			if row[0] != tc.wantTitle {
				t.Errorf("title = %q, want %q", row[0], tc.wantTitle)
			}
			if row[1] != tc.wantURL {
				t.Errorf("url = %q, want %q", row[1], tc.wantURL)
			}
		})
	}
}

func TestWriteCSV_MultipleEntries(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, Title: "First", URL: "https://first.com", VisitCount: 10, LastVisitTime: 0},
		{ID: 2, Title: "Second", URL: "https://second.com", VisitCount: 20, LastVisitTime: 0},
		{ID: 3, Title: "Third", URL: "https://third.com", VisitCount: 30, LastVisitTime: 0},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 4 {
		t.Fatalf("expected 4 records (header + 3 rows), got %d", len(records))
	}

	// Verify visit counts in order.
	wantCounts := []string{"10", "20", "30"}
	for i, wc := range wantCounts {
		if records[i+1][2] != wc {
			t.Errorf("row %d visit count = %q, want %q", i, records[i+1][2], wc)
		}
	}
}

func TestWriteCSV_VisitCountFormatting(t *testing.T) {
	entries := []HistoryEntry{
		{VisitCount: 0},
		{VisitCount: 1},
		{VisitCount: 999999},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	wantCounts := []string{"0", "1", "999999"}
	for i, wc := range wantCounts {
		if records[i+1][2] != wc {
			t.Errorf("row %d visit count = %q, want %q", i, records[i+1][2], wc)
		}
	}
}

func TestWriteCSV_LastVisitTimeFormatting(t *testing.T) {
	// Chrome epoch: microseconds since 1601-01-01.
	// 13300000000000000 corresponds to approximately 2022-06-23.
	entries := []HistoryEntry{
		{Title: "Zero", LastVisitTime: 0},
		{Title: "NonZero", LastVisitTime: 13300000000000000},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Zero timestamp should produce empty string.
	if records[1][3] != "" {
		t.Errorf("zero timestamp formatted as %q, want empty string", records[1][3])
	}

	// Non-zero timestamp should produce a date-time string.
	if records[2][3] == "" {
		t.Error("non-zero timestamp formatted as empty string")
	}
	if !strings.Contains(records[2][3], "2022") {
		t.Errorf("timestamp %q does not contain expected year", records[2][3])
	}
}

func TestWriteCSV_BOMPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, nil); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 3 {
		t.Fatal("output too short to contain BOM")
	}
	if data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Errorf("BOM bytes = %x %x %x, want EF BB BF", data[0], data[1], data[2])
	}
}

func TestWriteCSV_EmptyEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, []HistoryEntry{}); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record (header only), got %d", len(records))
	}
}

func TestCSVHeader_ColumnCount(t *testing.T) {
	if len(history.CSVHeader) != 4 {
		t.Fatalf("CSVHeader has %d columns, want 4", len(history.CSVHeader))
	}
}

// TestWriteCSV_EmptyURLField verifies that an entry with an empty URL produces
// an empty CSV field (not a parse error or missing column).
func TestWriteCSV_EmptyURLField(t *testing.T) {
	entries := []HistoryEntry{
		{Title: "No URL", URL: "", VisitCount: 1},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	row := records[1]
	if len(row) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(row))
	}
	if row[0] != "No URL" {
		t.Errorf("title = %q, want %q", row[0], "No URL")
	}
	if row[1] != "" {
		t.Errorf("url = %q, want empty string", row[1])
	}
}

// TestWriteCSV_AllEmptyFields verifies that an entry where every field is the
// zero value still produces a valid 4-column CSV row.
func TestWriteCSV_AllEmptyFields(t *testing.T) {
	entries := []HistoryEntry{
		{}, // ID=0, URL="", Title="", VisitCount=0, LastVisitTime=0
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records (header + 1 row), got %d", len(records))
	}

	row := records[1]
	if len(row) != 4 {
		t.Fatalf("expected 4 columns in all-empty row, got %d", len(row))
	}
	if row[0] != "" {
		t.Errorf("title = %q, want empty", row[0])
	}
	if row[1] != "" {
		t.Errorf("url = %q, want empty", row[1])
	}
	if row[2] != "0" {
		t.Errorf("visit count = %q, want \"0\"", row[2])
	}
	if row[3] != "" {
		t.Errorf("last visit = %q, want empty (zero timestamp)", row[3])
	}
}

// TestWriteCSV_MultiRowColumnConsistency verifies that every data row in a
// multi-entry output has exactly 4 columns, matching the header.
func TestWriteCSV_MultiRowColumnConsistency(t *testing.T) {
	entries := []HistoryEntry{
		{Title: "A", URL: "https://a.com", VisitCount: 1},
		{Title: "B,B", URL: "https://b.com?x=1&y=2", VisitCount: 2},
		{Title: `C"C`, URL: "https://c.com/path#section", VisitCount: 3},
		{Title: "D\nD", URL: "https://d.com", VisitCount: 4},
		{Title: "", URL: "", VisitCount: 0},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != len(entries)+1 {
		t.Fatalf("expected %d records, got %d", len(entries)+1, len(records))
	}

	for i, rec := range records {
		if len(rec) != 4 {
			t.Errorf("record[%d] has %d columns, want 4 (content: %q)", i, len(rec), rec)
		}
	}
}

// TestWriteCSV_MultiRowOrderPreserved verifies that entries appear in the CSV
// in the same order they were passed to writeCSV, and that all 4 columns are
// correct for every row.
func TestWriteCSV_MultiRowOrderPreserved(t *testing.T) {
	entries := []HistoryEntry{
		{Title: "Alpha", URL: "https://alpha.example.com", VisitCount: 10},
		{Title: "Beta", URL: "https://beta.example.com", VisitCount: 20},
		{Title: "Gamma", URL: "https://gamma.example.com", VisitCount: 30},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 4 {
		t.Fatalf("expected 4 records (header + 3 rows), got %d", len(records))
	}

	for i, e := range entries {
		row := records[i+1]
		if row[0] != e.Title {
			t.Errorf("row %d title = %q, want %q", i, row[0], e.Title)
		}
		if row[1] != e.URL {
			t.Errorf("row %d url = %q, want %q", i, row[1], e.URL)
		}
		wantCount := fmt.Sprintf("%d", e.VisitCount)
		if row[2] != wantCount {
			t.Errorf("row %d visit count = %q, want %q", i, row[2], wantCount)
		}
	}
}

// TestWriteCSV_URLSpecialChars verifies that URLs containing hash fragments,
// percent-encoded characters, and multiple query parameters survive a CSV
// round-trip without corruption.
func TestWriteCSV_URLSpecialChars(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantURL string
	}{
		{
			name:    "hash fragment",
			url:     "https://example.com/page#section-2",
			wantURL: "https://example.com/page#section-2",
		},
		{
			name:    "percent-encoded characters",
			url:     "https://example.com/search?q=hello%20world&lang=en",
			wantURL: "https://example.com/search?q=hello%20world&lang=en",
		},
		{
			name:    "multiple query params with commas",
			url:     "https://example.com/?a=1,2&b=3,4",
			wantURL: "https://example.com/?a=1,2&b=3,4",
		},
		{
			name:    "unicode path",
			url:     "https://example.com/日本語/page",
			wantURL: "https://example.com/日本語/page",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeCSV(&buf, []HistoryEntry{{Title: "T", URL: tc.url, VisitCount: 1}}); err != nil {
				t.Fatalf("writeCSV returned error: %v", err)
			}

			r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
			records, err := r.ReadAll()
			if err != nil {
				t.Fatalf("failed to parse CSV: %v", err)
			}

			if len(records) != 2 {
				t.Fatalf("expected 2 records, got %d", len(records))
			}
			if records[1][1] != tc.wantURL {
				t.Errorf("url = %q, want %q", records[1][1], tc.wantURL)
			}
		})
	}
}

// TestWriteCSV_NilVsEmptySlice verifies that writeCSV(nil) and
// writeCSV([]HistoryEntry{}) both produce identical output: BOM + header only.
func TestWriteCSV_NilVsEmptySlice(t *testing.T) {
	var bufNil, bufEmpty bytes.Buffer

	if err := writeCSV(&bufNil, nil); err != nil {
		t.Fatalf("writeCSV(nil) returned error: %v", err)
	}
	if err := writeCSV(&bufEmpty, []HistoryEntry{}); err != nil {
		t.Fatalf("writeCSV(empty) returned error: %v", err)
	}

	if !bytes.Equal(bufNil.Bytes(), bufEmpty.Bytes()) {
		t.Errorf("nil and empty slice produce different output:\n  nil:   %q\n  empty: %q",
			bufNil.Bytes(), bufEmpty.Bytes())
	}

	// Both should have only the header row.
	r := csv.NewReader(bytes.NewReader(bufNil.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (header only), got %d", len(records))
	}
}

// TestWriteCSV_LargeVisitCount verifies that very large visit count values
// are formatted correctly as decimal integers without truncation or scientific
// notation.
func TestWriteCSV_LargeVisitCount(t *testing.T) {
	entries := []HistoryEntry{
		{VisitCount: 1<<31 - 1}, // max int32
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	want := "2147483647"
	if records[1][2] != want {
		t.Errorf("large visit count = %q, want %q", records[1][2], want)
	}
}

// TestWriteCSV_HeaderColumnNames verifies the exact column names and their
// order in the header row.
func TestWriteCSV_HeaderColumnNames(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, nil); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) < 1 {
		t.Fatal("no records in output")
	}

	wantHeader := []string{"Title", "URL", "Visit Count", "Last Visit"}
	header := records[0]
	if len(header) != len(wantHeader) {
		t.Fatalf("header has %d columns, want %d", len(header), len(wantHeader))
	}
	for i, want := range wantHeader {
		if header[i] != want {
			t.Errorf("header[%d] = %q, want %q", i, header[i], want)
		}
	}
}

// TestWriteCSV_BOMIsExactlyThreeBytes verifies the BOM is exactly 3 bytes
// (0xEF 0xBB 0xBF) and appears only once at the start of output.
func TestWriteCSV_BOMIsExactlyThreeBytes(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, nil); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 3 {
		t.Fatalf("output too short: %d bytes", len(data))
	}

	// First 3 bytes must be the UTF-8 BOM.
	if data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Errorf("expected BOM EF BB BF, got %02X %02X %02X", data[0], data[1], data[2])
	}

	// The 4th byte onwards must not start with another BOM.
	rest := data[3:]
	if len(rest) >= 3 && rest[0] == 0xEF && rest[1] == 0xBB && rest[2] == 0xBF {
		t.Error("BOM appears twice in output")
	}
}

// TestWriteCSV_CombinedSpecialChars verifies that a field containing both
// commas and double-quote characters survives a CSV round-trip correctly.
// This exercises RFC 4180 quoting where the field must be double-quoted and
// internal double-quotes must be doubled (e.g., "He said ""yes, please""").
func TestWriteCSV_CombinedSpecialChars(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		wantTitle string
	}{
		{
			name:      "comma and double-quote combined",
			title:     `He said "yes, please"`,
			wantTitle: `He said "yes, please"`,
		},
		{
			name:      "leading and trailing quotes with comma",
			title:     `"start, end"`,
			wantTitle: `"start, end"`,
		},
		{
			name:      "multiple quotes and commas",
			title:     `"a", "b", "c"`,
			wantTitle: `"a", "b", "c"`,
		},
		{
			name:      "single double-quote only",
			title:     `"`,
			wantTitle: `"`,
		},
		{
			name:      "empty quoted string appearance",
			title:     `""`,
			wantTitle: `""`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := HistoryEntry{Title: tc.title, URL: "https://example.com", VisitCount: 1}
			var buf bytes.Buffer
			if err := writeCSV(&buf, []HistoryEntry{entry}); err != nil {
				t.Fatalf("writeCSV returned error: %v", err)
			}

			r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
			records, err := r.ReadAll()
			if err != nil {
				t.Fatalf("failed to parse CSV output: %v", err)
			}

			if len(records) != 2 {
				t.Fatalf("expected 2 records (header + 1 row), got %d", len(records))
			}

			if records[1][0] != tc.wantTitle {
				t.Errorf("title = %q, want %q", records[1][0], tc.wantTitle)
			}
		})
	}
}

// TestWriteCSV_QuotingInRawBytes verifies that the raw CSV output uses proper
// RFC 4180 quoting: fields containing commas must be wrapped in double-quotes,
// and fields containing double-quotes must use doubled double-quotes.
func TestWriteCSV_QuotingInRawBytes(t *testing.T) {
	t.Run("comma triggers quoting", func(t *testing.T) {
		entry := HistoryEntry{Title: "shoes, shirts", URL: "https://example.com", VisitCount: 1}
		var buf bytes.Buffer
		if err := writeCSV(&buf, []HistoryEntry{entry}); err != nil {
			t.Fatalf("writeCSV returned error: %v", err)
		}

		// Strip BOM and find first data row.
		raw := string(buf.Bytes()[len(utf8BOM):])
		lines := strings.SplitN(raw, "\n", 3)
		if len(lines) < 2 {
			t.Fatalf("expected at least 2 lines, got %d", len(lines))
		}

		dataLine := lines[1]
		// A title containing a comma must be enclosed in double-quotes in RFC 4180.
		if !strings.HasPrefix(dataLine, `"shoes, shirts"`) {
			t.Errorf("expected comma-containing field to be quoted; data line = %q", dataLine)
		}
	})

	t.Run("double-quote triggers doubled escaping", func(t *testing.T) {
		entry := HistoryEntry{Title: `say "hi"`, URL: "https://example.com", VisitCount: 1}
		var buf bytes.Buffer
		if err := writeCSV(&buf, []HistoryEntry{entry}); err != nil {
			t.Fatalf("writeCSV returned error: %v", err)
		}

		raw := string(buf.Bytes()[len(utf8BOM):])
		lines := strings.SplitN(raw, "\n", 3)
		if len(lines) < 2 {
			t.Fatalf("expected at least 2 lines, got %d", len(lines))
		}

		dataLine := lines[1]
		// RFC 4180: field with double-quotes must be quoted with internal
		// double-quotes escaped as doubled double-quotes: "say ""hi""".
		if !strings.HasPrefix(dataLine, `"say ""hi"""`) {
			t.Errorf("expected double-quote field to use doubled escaping; data line = %q", dataLine)
		}
	})
}

// TestWriteCSV_WriteError verifies that writeCSV propagates write errors from
// the underlying io.Writer back to the caller.
func TestWriteCSV_WriteError(t *testing.T) {
	// errWriter always fails on Write, simulating a disk-full or closed-pipe
	// scenario.
	errWriter := &alwaysErrWriter{}
	err := writeCSV(errWriter, nil)
	if err == nil {
		t.Error("expected writeCSV to return an error when writer fails, got nil")
	}
}

// alwaysErrWriter is an io.Writer that always returns an error.
type alwaysErrWriter struct{}

func (w *alwaysErrWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated write failure")
}

// TestWriteCSV_RawOutputSimpleEntry verifies the exact raw bytes for a simple,
// no-special-character entry to confirm the CSV structure: BOM, header row,
// data row, each terminated with CRLF as required by RFC 4180.
func TestWriteCSV_RawOutputSimpleEntry(t *testing.T) {
	entry := HistoryEntry{Title: "Simple", URL: "https://simple.com", VisitCount: 7, LastVisitTime: 0}

	var buf bytes.Buffer
	if err := writeCSV(&buf, []HistoryEntry{entry}); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	raw := buf.Bytes()

	// Must start with UTF-8 BOM.
	if !bytes.HasPrefix(raw, utf8BOM) {
		t.Error("output does not start with UTF-8 BOM")
	}

	// Strip BOM and parse to confirm parsability.
	r := csv.NewReader(bytes.NewReader(raw[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse output as CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Header must be exact.
	wantHeader := []string{"Title", "URL", "Visit Count", "Last Visit"}
	for i, want := range wantHeader {
		if records[0][i] != want {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], want)
		}
	}

	// Data row must have 4 columns.
	if len(records[1]) != 4 {
		t.Fatalf("data row has %d columns, want 4", len(records[1]))
	}
	if records[1][0] != "Simple" {
		t.Errorf("title = %q, want %q", records[1][0], "Simple")
	}
	if records[1][1] != "https://simple.com" {
		t.Errorf("url = %q, want %q", records[1][1], "https://simple.com")
	}
	if records[1][2] != "7" {
		t.Errorf("visit count = %q, want %q", records[1][2], "7")
	}
	// LastVisitTime is 0 so Last Visit column must be empty.
	if records[1][3] != "" {
		t.Errorf("last visit = %q, want empty for zero timestamp", records[1][3])
	}
}

// TestWriteCSV_NegativeVisitCount verifies that a negative visit count (which
// should not occur in practice but may arise from database corruption) is
// formatted as a decimal integer without wrapping, truncation, or panicking.
func TestWriteCSV_NegativeVisitCount(t *testing.T) {
	entry := HistoryEntry{Title: "T", URL: "https://example.com", VisitCount: -1}

	var buf bytes.Buffer
	if err := writeCSV(&buf, []HistoryEntry{entry}); err != nil {
		t.Fatalf("writeCSV returned error: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[1][2] != "-1" {
		t.Errorf("negative visit count = %q, want \"-1\"", records[1][2])
	}
}

// --- Integration tests for preview command ---

// TestPreview_HeaderLine verifies the summary line format "N matched out of M total".
func TestPreview_HeaderLine(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"google"}, nil)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	wantHeader := "1 matched out of 2 total"
	if !strings.HasPrefix(output, wantHeader) {
		t.Errorf("expected header %q, got first line: %q", wantHeader, strings.SplitN(output, "\n", 2)[0])
	}
}

// TestPreview_EntryFormatting verifies each entry line has the expected format:
// "  [visitCount] title(padded)  url"
func TestPreview_EntryFormatting(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example Site", VisitCount: 42},
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 100, defaultPreviewLimit)
	output := buf.String()

	lines := strings.Split(output, "\n")
	// lines[0] = header, lines[1] = blank (after \n\n), lines[2] = first entry
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %q", len(lines), output)
	}

	entryLine := lines[2]
	// Should contain visit count in brackets.
	if !strings.Contains(entryLine, "[42]") {
		t.Errorf("entry line missing visit count [42]: %q", entryLine)
	}
	// Should contain title.
	if !strings.Contains(entryLine, "Example Site") {
		t.Errorf("entry line missing title: %q", entryLine)
	}
	// Should contain URL.
	if !strings.Contains(entryLine, "https://example.com") {
		t.Errorf("entry line missing URL: %q", entryLine)
	}
	// Should start with spaces (indentation).
	if !strings.HasPrefix(entryLine, "  ") {
		t.Errorf("entry line should be indented: %q", entryLine)
	}
}

// TestPreview_MatchFilterIntegration tests preview with match filtering against
// a real SQLite database, verifying only matched entries appear.
func TestPreview_MatchFilterIntegration(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search?q=test", Title: "test - Google Search",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "user/repo",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.youtube.com/watch?v=abc", Title: "YouTube Video",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match only google entries.
	matched := filterEntries(entries, []string{"google"}, nil)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	// Should show 1 matched out of 3.
	if !strings.Contains(output, "1 matched out of 3 total") {
		t.Errorf("unexpected header in output: %q", output)
	}

	// Should contain the Google URL.
	if !strings.Contains(output, "google.com") {
		t.Errorf("output should contain google.com: %q", output)
	}

	// Should NOT contain github or youtube.
	if strings.Contains(output, "github.com") {
		t.Errorf("output should not contain github.com: %q", output)
	}
	if strings.Contains(output, "youtube.com") {
		t.Errorf("output should not contain youtube.com: %q", output)
	}
}

// TestPreview_ProtectFilterIntegration tests that protected entries are excluded.
func TestPreview_ProtectFilterIntegration(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://bank.example.com/login", Title: "My Bank Login",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://scholar.google.com/scholar", Title: "Google Scholar",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match all, but protect bank entries.
	matched := filterEntries(entries, []string{"*"}, []string{"bank"})

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	// Should show 2 matched (bank excluded).
	if !strings.Contains(output, "2 matched out of 3 total") {
		t.Errorf("unexpected header: %q", output)
	}

	// Bank should not appear.
	if strings.Contains(output, "bank.example.com") {
		t.Errorf("protected bank entry should not appear in output: %q", output)
	}

	// Google entries should appear.
	if !strings.Contains(output, "google.com") {
		t.Errorf("google entries should appear in output: %q", output)
	}
}

// TestPreview_EmptyResults verifies output when no entries match.
func TestPreview_EmptyResults(t *testing.T) {
	var buf bytes.Buffer
	writePreview(&buf, nil, 10, defaultPreviewLimit)
	output := buf.String()

	if !strings.HasPrefix(output, "0 matched out of 10 total") {
		t.Errorf("unexpected header for empty results: %q", output)
	}

	// Should have no entry lines (lines starting with "  [").
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			t.Errorf("empty result should have no entry lines, found: %q", line)
		}
	}
}

// TestPreview_TruncatesAt50 verifies that output is limited to 50 entries
// with a "... and N more" message.
func TestPreview_TruncatesAt50(t *testing.T) {
	// Create 60 entries.
	entries := make([]HistoryEntry, 60)
	for i := range entries {
		entries[i] = HistoryEntry{
			ID:         int64(i + 1),
			URL:        fmt.Sprintf("https://example.com/page%d", i),
			Title:      fmt.Sprintf("Page %d", i),
			VisitCount: i + 1,
		}
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 100, defaultPreviewLimit)
	output := buf.String()

	// Header should show 60 matched.
	if !strings.Contains(output, "60 matched out of 100 total") {
		t.Errorf("unexpected header: %q", strings.SplitN(output, "\n", 2)[0])
	}

	// Should contain "... and 10 more".
	if !strings.Contains(output, "... and 10 more") {
		t.Errorf("missing overflow message in output: %q", output)
	}

	// Count entry lines (lines starting with "  [").
	entryCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryCount++
		}
	}
	if entryCount != 50 {
		t.Errorf("expected 50 entry lines, got %d", entryCount)
	}
}

// TestPreview_Exactly50 verifies no overflow message when exactly 50 entries match.
func TestPreview_Exactly50(t *testing.T) {
	entries := make([]HistoryEntry, 50)
	for i := range entries {
		entries[i] = HistoryEntry{
			ID: int64(i + 1), URL: fmt.Sprintf("https://example.com/%d", i),
			Title: fmt.Sprintf("Page %d", i), VisitCount: 1,
		}
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 50, defaultPreviewLimit)
	output := buf.String()

	if strings.Contains(output, "... and") {
		t.Errorf("should not have overflow message for exactly 50 entries: %q", output)
	}

	entryCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryCount++
		}
	}
	if entryCount != 50 {
		t.Errorf("expected 50 entry lines, got %d", entryCount)
	}
}

// TestPreview_TitleTruncation verifies that long titles are truncated to 40 runes.
func TestPreview_TitleTruncation(t *testing.T) {
	longTitle := "This is a very long title that exceeds forty characters easily by far"
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: longTitle, VisitCount: 1},
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 1, defaultPreviewLimit)
	output := buf.String()

	// The full title should NOT appear in the output.
	if strings.Contains(output, longTitle) {
		t.Errorf("full long title should be truncated: %q", output)
	}

	// The truncated version (first 40 chars) should appear.
	truncated := truncate(longTitle, 40)
	if !strings.Contains(output, truncated) {
		t.Errorf("truncated title %q not found in output: %q", truncated, output)
	}
}

// TestPreview_URLTruncation verifies that long URLs are truncated to 80 runes.
func TestPreview_URLTruncation(t *testing.T) {
	longURL := "https://example.com/very/long/path/" + strings.Repeat("segment/", 20)
	entries := []HistoryEntry{
		{ID: 1, URL: longURL, Title: "Test", VisitCount: 1},
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 1, defaultPreviewLimit)
	output := buf.String()

	// Full URL should NOT appear.
	if strings.Contains(output, longURL) {
		t.Errorf("full long URL should be truncated")
	}

	// Truncated version should appear.
	truncated := truncate(longURL, 80)
	if !strings.Contains(output, truncated) {
		t.Errorf("truncated URL %q not found in output", truncated)
	}
}

// TestPreview_UnicodeHandling verifies correct handling of Unicode titles and URLs.
func TestPreview_UnicodeHandling(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://ko.wikipedia.org/wiki/한국어", Title: "한국어 - 위키백과", VisitCount: 2},
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 1, defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "한국어") {
		t.Errorf("unicode title not found in output: %q", output)
	}
	if !strings.Contains(output, "[2]") {
		t.Errorf("visit count not found in output: %q", output)
	}
}

// TestPreview_EmptyTitle verifies entries with empty titles are rendered correctly.
func TestPreview_EmptyTitle(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://tracker.example.com/pixel.gif", Title: "", VisitCount: 1},
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 1, defaultPreviewLimit)
	output := buf.String()

	// Should still show the visit count and URL even with empty title.
	if !strings.Contains(output, "[1]") {
		t.Errorf("visit count missing: %q", output)
	}
	if !strings.Contains(output, "tracker.example.com") {
		t.Errorf("URL missing from output: %q", output)
	}
}

// TestPreview_RealisticDataIntegration runs preview against the full realistic
// data set with various filter combinations.
func TestPreview_RealisticDataIntegration(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	expectedVisible := tdb.SeedRealisticData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != expectedVisible {
		t.Fatalf("expected %d visible entries, got %d", expectedVisible, len(entries))
	}

	tests := []struct {
		name         string
		match        []string
		protect      []string
		wantMinMatch int
		wantMaxMatch int
		mustContain  []string
		mustNotContain []string
	}{
		{
			name:         "wildcard match all",
			match:        []string{"*"},
			wantMinMatch: expectedVisible,
			wantMaxMatch: expectedVisible,
		},
		{
			name:         "match google",
			match:        []string{"google"},
			wantMinMatch: 2, // google.com/search and scholar.google.com
			wantMaxMatch: 3,
			mustContain:  []string{"google"},
		},
		{
			name:           "match all protect bank",
			match:          []string{"*"},
			protect:        []string{"bank"},
			wantMinMatch:   expectedVisible - 1,
			wantMaxMatch:   expectedVisible - 1,
			mustNotContain: []string{"bank.example.com"},
		},
		{
			name:         "match ads/tracking",
			match:        []string{"doubleclick", "tracker"},
			wantMinMatch: 2,
			wantMaxMatch: 2,
			mustContain:  []string{"doubleclick", "tracker"},
		},
		{
			name:           "match google protect scholar",
			match:          []string{"google"},
			protect:        []string{"scholar"},
			wantMinMatch:   1,
			wantMaxMatch:   2,
			mustNotContain: []string{"scholar"},
		},
		{
			name:         "no matches",
			match:        []string{"nonexistentdomain12345"},
			wantMinMatch: 0,
			wantMaxMatch: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matched := filterEntries(entries, tc.match, tc.protect)

			if len(matched) < tc.wantMinMatch || len(matched) > tc.wantMaxMatch {
				t.Errorf("matched count %d not in range [%d, %d]",
					len(matched), tc.wantMinMatch, tc.wantMaxMatch)
			}

			var buf bytes.Buffer
			writePreview(&buf, matched, len(entries), defaultPreviewLimit)
			output := buf.String()

			// Verify header line.
			wantHeader := fmt.Sprintf("%d matched out of %d total", len(matched), len(entries))
			if !strings.Contains(output, wantHeader) {
				t.Errorf("header mismatch: want %q in output", wantHeader)
			}

			for _, must := range tc.mustContain {
				if !strings.Contains(strings.ToLower(output), must) {
					t.Errorf("output should contain %q", must)
				}
			}

			for _, mustNot := range tc.mustNotContain {
				// Check entry lines only (skip header).
				lines := strings.Split(output, "\n")
				for _, line := range lines[2:] {
					if strings.Contains(strings.ToLower(line), mustNot) {
						t.Errorf("output should not contain %q in entry lines, found in: %q", mustNot, line)
					}
				}
			}
		})
	}
}

// TestPreview_HiddenEntriesExcluded verifies that hidden entries in the DB
// do not appear in preview output.
func TestPreview_HiddenEntriesExcluded(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://visible.example.com", Title: "Visible Page",
		LastVisitTime: now,
	}, 1)
	tdb.InsertURL(urlEntry{
		URL: "https://hidden.example.com", Title: "Hidden Page",
		VisitCount: 1, LastVisitTime: now, Hidden: 1,
	})

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// getAllURLs should only return visible entries.
	if len(entries) != 1 {
		t.Fatalf("expected 1 visible entry, got %d", len(entries))
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "1 matched out of 1 total") {
		t.Errorf("unexpected header: %q", output)
	}
	if strings.Contains(output, "hidden.example.com") {
		t.Error("hidden entry should not appear in preview output")
	}
	if !strings.Contains(output, "visible.example.com") {
		t.Error("visible entry should appear in preview output")
	}
}

// TestPreview_SingleEntry verifies correct output for a single matched entry.
func TestPreview_SingleEntry(t *testing.T) {
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Test Page", VisitCount: 7},
	}

	var buf bytes.Buffer
	writePreview(&buf, entries, 5, defaultPreviewLimit)
	output := buf.String()

	// Check the exact format of the entry line.
	if !strings.Contains(output, "1 matched out of 5 total") {
		t.Errorf("unexpected header: %q", output)
	}
	if !strings.Contains(output, "[7]") {
		t.Errorf("missing visit count: %q", output)
	}
	if !strings.Contains(output, "Test Page") {
		t.Errorf("missing title: %q", output)
	}
	if !strings.Contains(output, "https://example.com") {
		t.Errorf("missing URL: %q", output)
	}
}

// --- Integration tests for delete command ---

// TestDelete_RemovesMatchedURLsAndVisits verifies that deleteEntries removes
// both URL rows and their associated visit rows from the database.
func TestDelete_RemovesMatchedURLsAndVisits(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ads.doubleclick.net/track", Title: "Ad Tracker",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "My Repo",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://tracker.example.com/pixel", Title: "",
		LastVisitTime: now,
	}, 1)

	// Verify initial state.
	if c := tdb.CountURLs(); c != 3 {
		t.Fatalf("expected 3 URLs before delete, got %d", c)
	}
	if c := tdb.CountVisits(); c != 6 {
		t.Fatalf("expected 6 visits before delete, got %d", c)
	}

	// Fetch entries, filter for ad/tracker URLs, then delete.
	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	matched := filterEntries(entries, []string{"doubleclick", "tracker"}, nil)
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched entries, got %d", len(matched))
	}

	if err := deleteEntries(tdb.DB, matched); err != nil {
		t.Fatalf("deleteEntries: %v", err)
	}

	// After delete: only github entry should remain.
	if c := tdb.CountURLs(); c != 1 {
		t.Errorf("expected 1 URL after delete, got %d", c)
	}
	// Only github's 2 visits should remain.
	if c := tdb.CountVisits(); c != 2 {
		t.Errorf("expected 2 visits after delete, got %d", c)
	}

	// Verify the remaining URL is the one we kept.
	remaining, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(remaining))
	}
	if remaining[0].URL != "https://github.com/user/repo" {
		t.Errorf("wrong remaining URL: %q", remaining[0].URL)
	}
}

// TestDelete_WithProtectFilter verifies that protected entries survive deletion.
func TestDelete_WithProtectFilter(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://scholar.google.com/scholar", Title: "Google Scholar",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match google, but protect scholar.
	matched := filterEntries(entries, []string{"google"}, []string{"scholar"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched entry (google search only), got %d", len(matched))
	}
	if matched[0].URL != "https://www.google.com/search" {
		t.Errorf("wrong matched URL: %q", matched[0].URL)
	}

	if err := deleteEntries(tdb.DB, matched); err != nil {
		t.Fatalf("deleteEntries: %v", err)
	}

	// Scholar and example should remain.
	remaining, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs after delete: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining entries, got %d", len(remaining))
	}

	urls := make(map[string]bool)
	for _, e := range remaining {
		urls[e.URL] = true
	}
	if !urls["https://scholar.google.com/scholar"] {
		t.Error("scholar entry should have been protected from deletion")
	}
	if !urls["https://example.com"] {
		t.Error("example entry should not have been deleted")
	}
}

// TestDelete_NoMatches verifies that deleting an empty match set is a no-op.
func TestDelete_NoMatches(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example",
		LastVisitTime: now,
	}, 2)

	beforeURLs := tdb.CountURLs()
	beforeVisits := tdb.CountVisits()

	// Delete empty list.
	if err := deleteEntries(tdb.DB, nil); err != nil {
		t.Fatalf("deleteEntries with empty list: %v", err)
	}

	if c := tdb.CountURLs(); c != beforeURLs {
		t.Errorf("URL count changed: %d -> %d", beforeURLs, c)
	}
	if c := tdb.CountVisits(); c != beforeVisits {
		t.Errorf("visit count changed: %d -> %d", beforeVisits, c)
	}
}

// TestDelete_AllEntries verifies that deleting all entries empties both tables.
func TestDelete_AllEntries(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://a.com", Title: "A", LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://b.com", Title: "B", LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://c.com", Title: "C", LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Delete everything.
	matched := filterEntries(entries, []string{"*"}, nil)
	if err := deleteEntries(tdb.DB, matched); err != nil {
		t.Fatalf("deleteEntries: %v", err)
	}

	if c := tdb.CountURLs(); c != 0 {
		t.Errorf("expected 0 URLs after deleting all, got %d", c)
	}
	if c := tdb.CountVisits(); c != 0 {
		t.Errorf("expected 0 visits after deleting all, got %d", c)
	}
}

// TestDelete_PreservesHiddenEntries verifies that hidden entries are not
// affected by delete operations (since getAllURLs excludes them).
func TestDelete_PreservesHiddenEntries(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://visible.example.com", Title: "Visible",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURL(urlEntry{
		URL: "https://hidden.example.com", Title: "Hidden",
		VisitCount: 1, LastVisitTime: now, Hidden: 1,
	})

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Delete all visible entries.
	matched := filterEntries(entries, []string{"*"}, nil)
	if err := deleteEntries(tdb.DB, matched); err != nil {
		t.Fatalf("deleteEntries: %v", err)
	}

	// Hidden entry should still exist.
	if c := tdb.CountURLs(); c != 1 {
		t.Errorf("expected 1 URL (hidden) after delete, got %d", c)
	}

	var url string
	err = tdb.DB.QueryRow("SELECT url FROM urls").Scan(&url)
	if err != nil {
		t.Fatalf("query remaining URL: %v", err)
	}
	if url != "https://hidden.example.com" {
		t.Errorf("remaining URL should be hidden entry, got %q", url)
	}
}

// TestDelete_RealisticData runs delete against the full realistic data set.
func TestDelete_RealisticData(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	expectedVisible := tdb.SeedRealisticData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != expectedVisible {
		t.Fatalf("expected %d visible entries, got %d", expectedVisible, len(entries))
	}

	// Delete ad/tracking URLs.
	matched := filterEntries(entries, []string{"doubleclick", "tracker"}, nil)
	if len(matched) != 2 {
		t.Fatalf("expected 2 ad/tracker matches, got %d", len(matched))
	}

	if err := deleteEntries(tdb.DB, matched); err != nil {
		t.Fatalf("deleteEntries: %v", err)
	}

	remaining, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs after delete: %v", err)
	}
	if len(remaining) != expectedVisible-2 {
		t.Errorf("expected %d remaining entries, got %d", expectedVisible-2, len(remaining))
	}

	// Verify deleted URLs are gone.
	for _, e := range remaining {
		if strings.Contains(e.URL, "doubleclick") || strings.Contains(e.URL, "tracker") {
			t.Errorf("deleted URL still present: %q", e.URL)
		}
	}
}

// --- Integration tests for export command ---

// TestExport_FullFlow_CSV tests the complete export pipeline:
// DB → getAllURLs → filterEntries → writeCSV, verifying CSV format and data integrity.
func TestExport_FullFlow_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	nowChrome := timeToChrome(now)
	dayAgo := timeToChrome(now.Add(-24 * time.Hour))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search?q=test", Title: "test - Google Search",
		LastVisitTime: nowChrome,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "user/repo: My Project",
		LastVisitTime: dayAgo,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com/page", Title: "Example Page",
		LastVisitTime: dayAgo,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Export all entries (match "*").
	matched := filterEntries(entries, []string{"*"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	// Parse the CSV output.
	data := buf.Bytes()
	if !bytes.HasPrefix(data, utf8BOM) {
		t.Fatal("CSV output missing UTF-8 BOM")
	}

	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse exported CSV: %v", err)
	}

	// Header + 3 data rows.
	if len(records) != 4 {
		t.Fatalf("expected 4 CSV records (1 header + 3 data), got %d", len(records))
	}

	// Verify header columns.
	wantHeader := []string{"Title", "URL", "Visit Count", "Last Visit"}
	for i, col := range wantHeader {
		if records[0][i] != col {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], col)
		}
	}

	// Build a map of URL → row for easy lookup.
	rowByURL := make(map[string][]string)
	for _, rec := range records[1:] {
		rowByURL[rec[1]] = rec
	}

	// Verify data integrity: each row matches what was inserted.
	google := rowByURL["https://www.google.com/search?q=test"]
	if google == nil {
		t.Fatal("Google entry missing from CSV export")
	}
	if google[0] != "test - Google Search" {
		t.Errorf("Google title = %q, want %q", google[0], "test - Google Search")
	}
	if google[2] != "5" {
		t.Errorf("Google visit count = %q, want %q", google[2], "5")
	}

	github := rowByURL["https://github.com/user/repo"]
	if github == nil {
		t.Fatal("GitHub entry missing from CSV export")
	}
	if github[0] != "user/repo: My Project" {
		t.Errorf("GitHub title = %q, want %q", github[0], "user/repo: My Project")
	}
	if github[2] != "3" {
		t.Errorf("GitHub visit count = %q, want %q", github[2], "3")
	}

	example := rowByURL["https://example.com/page"]
	if example == nil {
		t.Fatal("Example entry missing from CSV export")
	}
	if example[2] != "1" {
		t.Errorf("Example visit count = %q, want %q", example[2], "1")
	}
}

// TestExport_WithMatchFilter_CSV verifies that the export flow applies match
// filters correctly and only exports matched entries.
func TestExport_WithMatchFilter_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 10)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/repo", Title: "GitHub Repo",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://youtube.com/watch", Title: "YouTube Video",
		LastVisitTime: now,
	}, 2)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match only google entries.
	matched := filterEntries(entries, []string{"google"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Header + 1 matched row.
	if len(records) != 2 {
		t.Fatalf("expected 2 CSV records (1 header + 1 data), got %d", len(records))
	}

	if records[1][1] != "https://www.google.com/search" {
		t.Errorf("matched URL = %q, want google.com", records[1][1])
	}
}

// TestExport_WithProtectFilter_CSV verifies that the export flow applies
// protect filters correctly, excluding protected entries from export.
func TestExport_WithProtectFilter_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://bank.example.com/login", Title: "My Bank",
		LastVisitTime: now,
	}, 3)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com/page", Title: "Example",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match all, protect bank.
	matched := filterEntries(entries, []string{"*"}, []string{"bank"})

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Header + 2 rows (bank excluded).
	if len(records) != 3 {
		t.Fatalf("expected 3 CSV records (1 header + 2 data), got %d", len(records))
	}

	for _, rec := range records[1:] {
		if strings.Contains(rec[1], "bank") {
			t.Errorf("protected bank URL should not appear in export: %q", rec[1])
		}
	}
}

// TestExport_EmptyDB_CSV verifies that exporting from an empty database
// produces a valid CSV with only the header row.
func TestExport_EmptyDB_CSV(t *testing.T) {
	tdb := newTestDB(t)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record (header only) from empty DB, got %d", len(records))
	}
}

// TestExport_NoMatchResults_CSV verifies that a filter matching nothing
// produces a CSV with only the header row.
func TestExport_NoMatchResults_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com/page", Title: "Example",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match a term that doesn't exist.
	matched := filterEntries(entries, []string{"nonexistent-keyword-xyz"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record (header only), got %d", len(records))
	}
}

// TestExport_LastVisitTimeIntegrity_CSV verifies that exported timestamps
// accurately reflect the data stored in the SQLite database.
func TestExport_LastVisitTimeIntegrity_CSV(t *testing.T) {
	tdb := newTestDB(t)

	// Use a known timestamp so we can verify the formatted output.
	knownTime := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	knownChrome := timeToChrome(knownTime)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Example",
		LastVisitTime: knownChrome,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// The formatted time should match formatTime output.
	wantTime := formatTime(knownChrome)
	if records[1][3] != wantTime {
		t.Errorf("last visit = %q, want %q", records[1][3], wantTime)
	}

	// Verify the formatted time matches formatTime exactly.
	// Note: formatTime uses local timezone, so we compare against the
	// expected output from formatTime rather than hardcoded UTC values.
	if records[1][3] != wantTime {
		t.Errorf("last visit = %q, want %q", records[1][3], wantTime)
	}

	// Verify the formatted time is non-empty and looks like a datetime.
	if len(records[1][3]) < 10 {
		t.Errorf("timestamp %q seems too short for a datetime string", records[1][3])
	}
}

// TestExport_UnicodeDataIntegrity_CSV verifies that Unicode content in titles
// and URLs survives the full export pipeline without corruption.
func TestExport_UnicodeDataIntegrity_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ko.wikipedia.org/wiki/한국어", Title: "한국어 - 위키백과",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ja.wikipedia.org/wiki/日本語", Title: "日本語 - Wikipedia",
		LastVisitTime: now,
	}, 1)

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Build URL → title map from export.
	exported := make(map[string]string)
	for _, rec := range records[1:] {
		exported[rec[1]] = rec[0]
	}

	// Verify Korean entry.
	if title, ok := exported["https://ko.wikipedia.org/wiki/한국어"]; !ok {
		t.Error("Korean URL missing from export")
	} else if title != "한국어 - 위키백과" {
		t.Errorf("Korean title = %q, want %q", title, "한국어 - 위키백과")
	}

	// Verify Japanese entry.
	if title, ok := exported["https://ja.wikipedia.org/wiki/日本語"]; !ok {
		t.Error("Japanese URL missing from export")
	} else if title != "日本語 - Wikipedia" {
		t.Errorf("Japanese title = %q, want %q", title, "日本語 - Wikipedia")
	}
}

// TestExport_HiddenEntriesExcluded_CSV verifies that hidden entries in the
// database are excluded from the export through the getAllURLs query.
func TestExport_HiddenEntriesExcluded_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://visible.example.com", Title: "Visible",
		LastVisitTime: now,
	}, 1)
	// Insert a hidden entry directly (not via InsertURLWithVisits to set hidden=1).
	tdb.InsertURL(urlEntry{
		URL: "https://hidden.example.com", Title: "Hidden",
		VisitCount: 1, LastVisitTime: now, Hidden: 1,
	})

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Header + 1 visible entry only.
	if len(records) != 2 {
		t.Fatalf("expected 2 records (header + 1 visible), got %d", len(records))
	}

	if records[1][1] != "https://visible.example.com" {
		t.Errorf("expected visible URL, got %q", records[1][1])
	}
}

// TestExport_RealisticData_CSV tests the export pipeline with realistic seeded
// data, verifying row count and that all visible entries are represented.
func TestExport_RealisticData_CSV(t *testing.T) {
	tdb := newTestDB(t)

	expectedVisible := tdb.SeedRealisticData()

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	var buf bytes.Buffer
	if err := writeCSV(&buf, matched); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Header + all visible entries.
	if len(records) != expectedVisible+1 {
		t.Fatalf("expected %d CSV records (1 header + %d data), got %d",
			expectedVisible+1, expectedVisible, len(records))
	}

	// Every data row should have exactly 4 columns.
	for i, rec := range records {
		if len(rec) != 4 {
			t.Errorf("row %d has %d columns, want 4", i, len(rec))
		}
	}

	// Every data row should have a non-empty URL.
	for i, rec := range records[1:] {
		if rec[1] == "" {
			t.Errorf("row %d has empty URL", i+1)
		}
	}

	// Visit count should be a parseable positive integer or zero.
	for i, rec := range records[1:] {
		var vc int
		if _, err := fmt.Sscanf(rec[2], "%d", &vc); err != nil {
			t.Errorf("row %d visit count %q is not a valid integer", i+1, rec[2])
		}
		if vc < 0 {
			t.Errorf("row %d visit count = %d, should be non-negative", i+1, vc)
		}
	}
}

// TestExport_SpecialCharsInURL_CSV verifies that URLs with query params,
// percent-encoding, and special characters are preserved through the export.
func TestExport_SpecialCharsInURL_CSV(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	specialURLs := []string{
		"https://example.com/search?q=hello%20world&lang=en",
		"https://example.com/path?a=1&b=2&c=3,4,5",
		"https://example.com/page#section-1",
		"https://user:pass@example.com/path",
	}

	for _, u := range specialURLs {
		tdb.InsertURLWithVisits(urlEntry{
			URL: u, Title: "Test", LastVisitTime: now,
		}, 1)
	}

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	exportedURLs := make(map[string]bool)
	for _, rec := range records[1:] {
		exportedURLs[rec[1]] = true
	}

	for _, u := range specialURLs {
		if !exportedURLs[u] {
			t.Errorf("URL %q not found in CSV export", u)
		}
	}
}

// TestExport_CSVColumnCount_Consistent verifies every row in the CSV
// has exactly 4 columns, matching the header.
func TestExport_CSVColumnCount_Consistent(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	// Insert entries with tricky content (commas, quotes, newlines in titles).
	trickyCases := []urlEntry{
		{URL: "https://a.com", Title: "Title with, comma", LastVisitTime: now},
		{URL: "https://b.com", Title: `Title with "quotes"`, LastVisitTime: now},
		{URL: "https://c.com", Title: "Title with\nnewline", LastVisitTime: now},
		{URL: "https://d.com", Title: "", LastVisitTime: now},
		{URL: "https://e.com", Title: "Normal title", LastVisitTime: now},
	}

	for _, e := range trickyCases {
		tdb.InsertURLWithVisits(e, 1)
	}

	entries, err := getAllURLs(tdb.DB)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, entries); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(buf.Bytes()[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	for i, rec := range records {
		if len(rec) != 4 {
			t.Errorf("row %d has %d columns, want 4: %v", i, len(rec), rec)
		}
	}
}

// =============================================================================
// Integration tests: full pipeline via openReadOnlyDB (temp copy mechanism)
// =============================================================================
//
// The following tests exercise the complete preview and export command flows
// using openReadOnlyDB, which internally calls copyDB to create a temporary
// SQLite copy. This mirrors the actual command execution path, ensuring the
// temp-copy mechanism works end-to-end rather than only testing helper
// functions against a directly-opened DB handle.

// TestPreviewPipeline_ViaTempCopy verifies the complete preview pipeline:
// seed DB → close → openReadOnlyDB (creates temp copy) → getAllURLs →
// filterEntries → sortEntries → writePreview.
func TestPreviewPipeline_ViaTempCopy(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search?q=test", Title: "Google Search",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "My Repo",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://bank.example.com/accounts", Title: "My Bank",
		LastVisitTime: now,
	}, 3)

	// Close before calling openReadOnlyDB to simulate real usage
	// where the original DB is closed and a temp copy is opened.
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match only google; bank should be excluded.
	matched := filterEntries(entries, []string{"google"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "1 matched out of 3 total") {
		t.Errorf("expected '1 matched out of 3 total' in output: %q", output)
	}
	if !strings.Contains(output, "google.com") {
		t.Errorf("expected google.com in output: %q", output)
	}
	if strings.Contains(output, "bank") {
		t.Errorf("bank should not appear in filtered output: %q", output)
	}
	if strings.Contains(output, "github.com") {
		t.Errorf("github should not appear in filtered output: %q", output)
	}
}

// TestPreviewPipeline_TempCopyIsIndependent verifies that the temp DB copy
// produced by openReadOnlyDB is a distinct file from the original DB path.
func TestPreviewPipeline_TempCopyIsIndependent(t *testing.T) {
	tdb := newTestDB(t)
	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://original.example.com", Title: "Original",
		LastVisitTime: now,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	// The temp copy path must differ from the original.
	if tmpPath == tdb.Path {
		t.Error("temp copy path should differ from original DB path")
	}

	// The temp copy must contain the same data as the original.
	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs from temp copy: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in temp copy, got %d", len(entries))
	}
	if entries[0].URL != "https://original.example.com" {
		t.Errorf("unexpected URL in temp copy: %q", entries[0].URL)
	}
}

// TestPreviewPipeline_SortedByLastVisitDescending verifies that preview
// output is sorted with the most recently visited entry first through the
// full openReadOnlyDB pipeline.
func TestPreviewPipeline_SortedByLastVisitDescending(t *testing.T) {
	tdb := newTestDB(t)

	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	newTime := timeToChrome(now)
	midTime := timeToChrome(now.Add(-24 * time.Hour))
	oldTime := timeToChrome(now.Add(-72 * time.Hour))

	// Insert in arbitrary order; sort must fix the order regardless.
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://old.example.com", Title: "Old Site",
		LastVisitTime: oldTime,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://new.example.com", Title: "New Site",
		LastVisitTime: newTime,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://mid.example.com", Title: "Mid Site",
		LastVisitTime: midTime,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	// Extract entry lines (indented lines starting with "  [").
	var entryLines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryLines = append(entryLines, line)
		}
	}
	if len(entryLines) != 3 {
		t.Fatalf("expected 3 entry lines, got %d: %q", len(entryLines), output)
	}

	// Most recent entry should appear first.
	if !strings.Contains(entryLines[0], "new.example.com") {
		t.Errorf("first entry should be new.example.com, got: %q", entryLines[0])
	}
	if !strings.Contains(entryLines[1], "mid.example.com") {
		t.Errorf("second entry should be mid.example.com, got: %q", entryLines[1])
	}
	if !strings.Contains(entryLines[2], "old.example.com") {
		t.Errorf("third entry should be old.example.com, got: %q", entryLines[2])
	}
}

// TestPreviewPipeline_CustomLimit verifies that a caller-supplied limit caps
// the number of displayed entries and generates an overflow message.
func TestPreviewPipeline_CustomLimit(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	for i := 0; i < 10; i++ {
		tdb.InsertURLWithVisits(urlEntry{
			URL:           fmt.Sprintf("https://example%d.com", i),
			Title:         fmt.Sprintf("Site %d", i),
			LastVisitTime: now,
		}, 1)
	}
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	// Use a limit of 3 to display only the first 3 of 10 entries.
	writePreview(&buf, matched, len(entries), 3)
	output := buf.String()

	if !strings.Contains(output, "10 matched out of 10 total") {
		t.Errorf("unexpected header: %q", output)
	}
	if !strings.Contains(output, "... and 7 more") {
		t.Errorf("expected '... and 7 more' overflow message: %q", output)
	}

	// Count visible entry lines.
	entryCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  [") {
			entryCount++
		}
	}
	if entryCount != 3 {
		t.Errorf("expected 3 entry lines with limit=3, got %d", entryCount)
	}
}

// TestPreviewPipeline_ProtectFilter verifies that the protect filter correctly
// excludes matching entries through the full openReadOnlyDB pipeline.
func TestPreviewPipeline_ProtectFilter(t *testing.T) {
	tdb := newTestDB(t)
	tdb.SeedMinimalData() // google (3 visits), github (1), bank (2)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Match all, protect bank → should return 2 entries.
	matched := filterEntries(entries, []string{"*"}, []string{"bank"})
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "2 matched out of 3 total") {
		t.Errorf("expected '2 matched out of 3 total': %q", output)
	}
	if strings.Contains(output, "bank") {
		t.Errorf("bank should not appear in filtered output: %q", output)
	}
	if !strings.Contains(output, "google.com") {
		t.Errorf("google.com should appear in output: %q", output)
	}
}

// TestPreviewPipeline_RealisticData runs the full preview pipeline with the
// realistic data set, verifying counts and content across multiple filter scenarios.
func TestPreviewPipeline_RealisticData(t *testing.T) {
	tdb := newTestDB(t)
	expectedVisible := tdb.SeedRealisticData()
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != expectedVisible {
		t.Fatalf("expected %d visible entries, got %d", expectedVisible, len(entries))
	}

	tests := []struct {
		name           string
		match          []string
		protect        []string
		wantMinMatch   int
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:         "wildcard matches all visible entries",
			match:        []string{"*"},
			wantMinMatch: expectedVisible,
		},
		{
			name:         "google keyword matches search and scholar",
			match:        []string{"google"},
			wantMinMatch: 2,
			mustContain:  []string{"google"},
		},
		{
			name:           "wildcard with bank protected",
			match:          []string{"*"},
			protect:        []string{"bank"},
			wantMinMatch:   expectedVisible - 1,
			mustNotContain: []string{"bank.example.com"},
		},
		{
			name:         "ad/tracker domains match two entries",
			match:        []string{"doubleclick", "tracker"},
			wantMinMatch: 2,
		},
		{
			name:         "no matching domain returns zero",
			match:        []string{"nonexistent-domain-99999"},
			wantMinMatch: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matched := filterEntries(entries, tc.match, tc.protect)
			sortEntries(matched)

			if len(matched) < tc.wantMinMatch {
				t.Errorf("matched count %d < expected minimum %d", len(matched), tc.wantMinMatch)
			}

			var buf bytes.Buffer
			writePreview(&buf, matched, len(entries), defaultPreviewLimit)
			output := buf.String()

			wantHeader := fmt.Sprintf("%d matched out of %d total", len(matched), len(entries))
			if !strings.Contains(output, wantHeader) {
				t.Errorf("header mismatch: want %q in output", wantHeader)
			}
			for _, must := range tc.mustContain {
				if !strings.Contains(strings.ToLower(output), must) {
					t.Errorf("output should contain %q", must)
				}
			}
			for _, mustNot := range tc.mustNotContain {
				for _, line := range strings.Split(output, "\n")[2:] {
					if strings.Contains(strings.ToLower(line), mustNot) {
						t.Errorf("entry line should not contain %q: %q", mustNot, line)
					}
				}
			}
		})
	}
}

// TestPreviewPipeline_HiddenEntriesNotInTempCopy verifies that hidden DB
// entries are excluded from preview through the full openReadOnlyDB pipeline.
func TestPreviewPipeline_HiddenEntriesNotInTempCopy(t *testing.T) {
	tdb := newTestDB(t)
	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://visible.example.com", Title: "Visible Page",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURL(urlEntry{
		URL: "https://hidden.example.com", Title: "Hidden Page",
		VisitCount: 1, LastVisitTime: now, Hidden: 1,
	})
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("getAllURLs should return 1 visible entry, got %d", len(entries))
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	var buf bytes.Buffer
	writePreview(&buf, matched, len(entries), defaultPreviewLimit)
	output := buf.String()

	if !strings.Contains(output, "1 matched out of 1 total") {
		t.Errorf("unexpected header: %q", output)
	}
	if strings.Contains(output, "hidden.example.com") {
		t.Error("hidden entry must not appear in preview output")
	}
	if !strings.Contains(output, "visible.example.com") {
		t.Error("visible entry must appear in preview output")
	}
}

// =============================================================================
// Integration tests: export pipeline writing to a real file on disk
// =============================================================================
//
// These tests exercise the complete export flow including writing the CSV to
// an actual file via os.OpenFile, reading the file back, and validating its
// contents. This mirrors cmdExport's behaviour more closely than in-memory
// bytes.Buffer tests.

// TestExportPipeline_WritesToDisk verifies the end-to-end export flow:
// seed DB → close → openReadOnlyDB → getAllURLs → filterEntries →
// writeCSV to a real temp file → read file back → parse and verify CSV.
func TestExportPipeline_WritesToDisk(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "GitHub Repo",
		LastVisitTime: now,
	}, 3)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "export.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, matched); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	// Verify the file exists and is non-empty.
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("exported CSV file must not be empty")
	}

	// Read back the file and parse as CSV.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !bytes.HasPrefix(data, utf8BOM) {
		t.Error("exported file must begin with UTF-8 BOM")
	}

	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse exported CSV: %v", err)
	}

	// Expect header + 2 data rows.
	if len(records) != 3 {
		t.Fatalf("expected 3 CSV records (1 header + 2 data), got %d", len(records))
	}

	// Every row must have exactly 4 columns.
	for i, rec := range records {
		if len(rec) != 4 {
			t.Errorf("row %d has %d columns, want 4", i, len(rec))
		}
	}

	// Both inserted URLs must be present in the export.
	exportedURLs := make(map[string]bool)
	for _, rec := range records[1:] {
		exportedURLs[rec[1]] = true
	}
	for _, u := range []string{"https://www.google.com/search", "https://github.com/user/repo"} {
		if !exportedURLs[u] {
			t.Errorf("URL %q not found in exported CSV", u)
		}
	}
}

// TestExportPipeline_WritesToDisk_MatchFilter verifies that a match filter
// correctly restricts which rows appear in the on-disk CSV file.
func TestExportPipeline_WritesToDisk_MatchFilter(t *testing.T) {
	tdb := newTestDB(t)

	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://www.google.com/search", Title: "Google Search",
		LastVisitTime: now,
	}, 5)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://github.com/user/repo", Title: "GitHub Repo",
		LastVisitTime: now,
	}, 2)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://ads.doubleclick.net/track", Title: "Ad Tracker",
		LastVisitTime: now,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Export only github-matching entries.
	matched := filterEntries(entries, []string{"github"}, nil)
	sortEntries(matched)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "github_only.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, matched); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// Header + 1 github row only.
	if len(records) != 2 {
		t.Fatalf("expected 2 CSV records (1 header + 1 data), got %d", len(records))
	}
	if !strings.Contains(records[1][1], "github.com") {
		t.Errorf("expected github.com URL in export, got %q", records[1][1])
	}
	if strings.Contains(records[1][1], "doubleclick") || strings.Contains(records[1][1], "google") {
		t.Errorf("non-github URLs should be excluded from export")
	}
}

// TestExportPipeline_WritesToDisk_ProtectFilter verifies that protect filters
// exclude matching entries from the on-disk CSV file.
func TestExportPipeline_WritesToDisk_ProtectFilter(t *testing.T) {
	tdb := newTestDB(t)
	tdb.SeedMinimalData() // google (3 visits), github (1), bank (2)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	// Export all, but protect bank entries.
	matched := filterEntries(entries, []string{"*"}, []string{"bank"})
	sortEntries(matched)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "no_bank.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, matched); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// SeedMinimalData has 3 entries; bank is protected → 2 data rows.
	if len(records) != 3 {
		t.Fatalf("expected 3 records (header + 2 data), got %d", len(records))
	}
	for _, rec := range records[1:] {
		if strings.Contains(strings.ToLower(rec[1]), "bank") {
			t.Errorf("protected bank URL must not appear in export: %q", rec[1])
		}
	}
}

// TestExportPipeline_WritesToDisk_SortedOutput verifies that the on-disk CSV
// rows appear in descending order of LastVisitTime (most recently visited first).
func TestExportPipeline_WritesToDisk_SortedOutput(t *testing.T) {
	tdb := newTestDB(t)

	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	newTime := timeToChrome(now)
	midTime := timeToChrome(now.Add(-24 * time.Hour))
	oldTime := timeToChrome(now.Add(-48 * time.Hour))

	// Insert in a shuffled order to confirm sort is applied.
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://old.example.com", Title: "Old", LastVisitTime: oldTime,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://new.example.com", Title: "New", LastVisitTime: newTime,
	}, 1)
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://mid.example.com", Title: "Mid", LastVisitTime: midTime,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "sorted_export.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, matched); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	if len(records) != 4 {
		t.Fatalf("expected 4 records (1 header + 3 data), got %d", len(records))
	}
	// Row 1 (most recently visited) should be new.example.com.
	if !strings.Contains(records[1][1], "new.example.com") {
		t.Errorf("row 1 should be new.example.com, got %q", records[1][1])
	}
	// Row 2 should be mid.example.com.
	if !strings.Contains(records[2][1], "mid.example.com") {
		t.Errorf("row 2 should be mid.example.com, got %q", records[2][1])
	}
	// Row 3 (oldest) should be old.example.com.
	if !strings.Contains(records[3][1], "old.example.com") {
		t.Errorf("row 3 should be old.example.com, got %q", records[3][1])
	}
}

// TestExportPipeline_WritesToDisk_FilePermissions verifies the exported CSV
// file is created with restrictive permissions (0600) by the caller. The
// permission check is skipped on Windows, which does not support Unix-style
// permission bits in os.FileMode.
func TestExportPipeline_WritesToDisk_FilePermissions(t *testing.T) {
	tdb := newTestDB(t)
	now := timeToChrome(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com", Title: "Test Page", LastVisitTime: now,
	}, 1)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "perms.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, entries); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	// File must be non-empty.
	if info.Size() == 0 {
		t.Error("output CSV file must not be empty")
	}
	// Verify restrictive permissions on platforms that honour Unix mode bits.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("expected file permissions 0600, got %04o", perm)
		}
	}
}

// TestExportPipeline_WritesToDisk_RealisticData exercises the full export
// pipeline with the realistic data set, writing to a real file and verifying
// row count, column count, non-empty URLs, and absence of hidden entries.
func TestExportPipeline_WritesToDisk_RealisticData(t *testing.T) {
	tdb := newTestDB(t)
	expectedVisible := tdb.SeedRealisticData()
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}
	if len(entries) != expectedVisible {
		t.Fatalf("expected %d visible entries from temp copy, got %d", expectedVisible, len(entries))
	}

	matched := filterEntries(entries, []string{"*"}, nil)
	sortEntries(matched)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "realistic.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, matched); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// Header + all visible entries.
	if len(records) != expectedVisible+1 {
		t.Fatalf("expected %d records (1 header + %d data), got %d",
			expectedVisible+1, expectedVisible, len(records))
	}

	// Every row must have exactly 4 columns.
	for i, rec := range records {
		if len(rec) != 4 {
			t.Errorf("row %d has %d columns, want 4", i, len(rec))
		}
	}

	// Every data row must have a non-empty URL.
	for i, rec := range records[1:] {
		if rec[1] == "" {
			t.Errorf("data row %d has empty URL", i+1)
		}
	}

	// Hidden entries must not appear in the export.
	for _, rec := range records[1:] {
		if strings.Contains(rec[1], "hidden.example.com") {
			t.Errorf("hidden URL must not appear in export: %q", rec[1])
		}
	}

	// Visit counts must be non-negative integers.
	for i, rec := range records[1:] {
		var vc int
		if _, scanErr := fmt.Sscanf(rec[2], "%d", &vc); scanErr != nil {
			t.Errorf("row %d visit count %q is not a valid integer", i+1, rec[2])
		}
		if vc < 0 {
			t.Errorf("row %d visit count = %d, want non-negative", i+1, vc)
		}
	}
}

// TestExportPipeline_WritesToDisk_EmptyDB verifies that exporting an empty
// database produces a file containing only the CSV header row.
func TestExportPipeline_WritesToDisk_EmptyDB(t *testing.T) {
	tdb := newTestDB(t)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	matched := filterEntries(entries, []string{"*"}, nil)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "empty.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, matched); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// Only the header row; no data rows.
	if len(records) != 1 {
		t.Fatalf("expected 1 record (header only) for empty DB, got %d", len(records))
	}
	wantHeader := []string{"Title", "URL", "Visit Count", "Last Visit"}
	for i, col := range wantHeader {
		if records[0][i] != col {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], col)
		}
	}
}

// TestExportPipeline_WritesToDisk_DataIntegrity verifies that title, URL,
// visit count, and last-visit timestamp are all written correctly to the file.
func TestExportPipeline_WritesToDisk_DataIntegrity(t *testing.T) {
	tdb := newTestDB(t)

	knownTime := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	knownChrome := timeToChrome(knownTime)

	tdb.InsertURLWithVisits(urlEntry{
		URL: "https://example.com/page", Title: "Example Page",
		LastVisitTime: knownChrome,
	}, 7)
	tdb.Close()

	db, tmpPath, err := openReadOnlyDB(tdb.Path)
	if err != nil {
		t.Fatalf("openReadOnlyDB: %v", err)
	}
	defer cleanupTmp(tmpPath)
	defer db.Close()

	entries, err := getAllURLs(db)
	if err != nil {
		t.Fatalf("getAllURLs: %v", err)
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "integrity.csv")
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	if err := writeCSV(f, entries); err != nil {
		f.Close()
		t.Fatalf("writeCSV: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(data[len(utf8BOM):]))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records (header + 1 data), got %d", len(records))
	}

	row := records[1]
	if row[0] != "Example Page" {
		t.Errorf("title = %q, want %q", row[0], "Example Page")
	}
	if row[1] != "https://example.com/page" {
		t.Errorf("url = %q, want %q", row[1], "https://example.com/page")
	}
	if row[2] != "7" {
		t.Errorf("visit count = %q, want %q", row[2], "7")
	}
	// Timestamp should match formatTime output for the known Chrome timestamp.
	wantTime := formatTime(knownChrome)
	if row[3] != wantTime {
		t.Errorf("last visit = %q, want %q", row[3], wantTime)
	}
	if len(row[3]) < 10 {
		t.Errorf("timestamp %q is too short to be a valid datetime", row[3])
	}
}
