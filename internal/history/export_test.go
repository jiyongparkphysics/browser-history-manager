package history

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCSV_BOM(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", VisitCount: 3, LastVisitTime: 13350000000000000},
	}

	if err := WriteCSV(&buf, entries); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("output missing UTF-8 BOM")
	}
}

func TestWriteCSV_Header(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, nil); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	// Skip BOM.
	content := buf.String()[3:]
	r := csv.NewReader(strings.NewReader(content))
	header, err := r.Read()
	if err != nil {
		t.Fatalf("failed to read CSV header: %v", err)
	}

	expected := []string{"Title", "URL", "Visit Count", "Last Visit"}
	if len(header) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(header))
	}
	for i, col := range expected {
		if header[i] != col {
			t.Errorf("header[%d] = %q, want %q", i, header[i], col)
		}
	}
}

func TestWriteCSV_EntryFormat(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example Site", VisitCount: 7, LastVisitTime: 13350000000000000},
	}

	if err := WriteCSV(&buf, entries); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	content := buf.String()[3:] // Skip BOM.
	r := csv.NewReader(strings.NewReader(content))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 { // header + 1 entry
		t.Fatalf("expected 2 records (header + 1 entry), got %d", len(records))
	}

	row := records[1]
	if row[0] != "Example Site" {
		t.Errorf("title = %q, want %q", row[0], "Example Site")
	}
	if row[1] != "https://example.com" {
		t.Errorf("url = %q, want %q", row[1], "https://example.com")
	}
	if row[2] != "7" {
		t.Errorf("visit count = %q, want %q", row[2], "7")
	}
	if row[3] == "" {
		t.Error("last visit time should not be empty")
	}
}

func TestWriteCSV_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://a.com", Title: "A", VisitCount: 1, LastVisitTime: 13350000000000000},
		{ID: 2, URL: "https://b.com", Title: "B", VisitCount: 2, LastVisitTime: 13349000000000000},
		{ID: 3, URL: "https://c.com", Title: "C", VisitCount: 3, LastVisitTime: 13348000000000000},
	}

	if err := WriteCSV(&buf, entries); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	content := buf.String()[3:]
	r := csv.NewReader(strings.NewReader(content))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 4 { // header + 3 entries
		t.Fatalf("expected 4 records, got %d", len(records))
	}
}

func TestWriteCSV_EmptyEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, []HistoryEntry{}); err != nil {
		t.Fatalf("WriteCSV with empty entries failed: %v", err)
	}

	content := buf.String()[3:]
	r := csv.NewReader(strings.NewReader(content))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 1 { // header only
		t.Fatalf("expected 1 record (header only), got %d", len(records))
	}
}

func TestWriteCSVFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "export.csv")
	entries := []HistoryEntry{
		{ID: 1, URL: "https://test.com", Title: "Test", VisitCount: 5, LastVisitTime: 13350000000000000},
		{ID: 2, URL: "https://other.com", Title: "Other", VisitCount: 2, LastVisitTime: 13349000000000000},
	}

	if err := WriteCSVFile(outPath, entries); err != nil {
		t.Fatalf("WriteCSVFile failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	// Verify BOM.
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Error("output file missing UTF-8 BOM")
	}

	// Verify content.
	content := string(data[3:])
	if !strings.Contains(content, "https://test.com") {
		t.Error("CSV missing test.com URL")
	}
	if !strings.Contains(content, "https://other.com") {
		t.Error("CSV missing other.com URL")
	}

	// Verify file permissions (0600).
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("failed to stat output file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestWriteCSVFile_InvalidPath(t *testing.T) {
	err := WriteCSVFile(filepath.Join(t.TempDir(), "nonexistent", "dir", "file.csv"), nil)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestWriteJSON_Structure(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", VisitCount: 3, LastVisitTime: 13350000000000000},
	}

	if err := WriteJSON(&buf, entries, "chrome"); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var result JSONExport
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if result.Browser != "chrome" {
		t.Errorf("browser = %q, want %q", result.Browser, "chrome")
	}
	if result.TotalCount != 1 {
		t.Errorf("totalCount = %d, want 1", result.TotalCount)
	}
	if result.ExportDate == "" {
		t.Error("exportDate should not be empty")
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].URL != "https://example.com" {
		t.Errorf("entry URL = %q, want %q", result.Entries[0].URL, "https://example.com")
	}
}

func TestWriteJSON_EmptyEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, []HistoryEntry{}, "edge"); err != nil {
		t.Fatalf("WriteJSON with empty entries failed: %v", err)
	}

	var result JSONExport
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("totalCount = %d, want 0", result.TotalCount)
	}
	if result.Entries == nil {
		t.Error("entries should be an empty array, not null")
	}
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result.Entries))
	}
	if result.Browser != "edge" {
		t.Errorf("browser = %q, want %q", result.Browser, "edge")
	}
}

func TestWriteJSON_NilEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, nil, "chrome"); err != nil {
		t.Fatalf("WriteJSON with nil entries failed: %v", err)
	}

	var result JSONExport
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("totalCount = %d, want 0", result.TotalCount)
	}
	if result.Entries == nil {
		t.Error("entries should be an empty array, not null")
	}
}

func TestWriteJSON_ExportDateRFC3339(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, nil, "chrome"); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var result JSONExport
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	// exportDate must be a valid RFC3339 timestamp.
	if !strings.Contains(result.ExportDate, "T") || !strings.Contains(result.ExportDate, "Z") {
		t.Errorf("exportDate %q does not look like RFC3339", result.ExportDate)
	}
}

func TestWriteJSONFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "export.json")
	entries := []HistoryEntry{
		{ID: 1, URL: "https://test.com", Title: "Test", VisitCount: 5, LastVisitTime: 13350000000000000},
		{ID: 2, URL: "https://other.com", Title: "Other", VisitCount: 2, LastVisitTime: 13349000000000000},
	}

	if err := WriteJSONFile(outPath, entries, "chrome"); err != nil {
		t.Fatalf("WriteJSONFile failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var result JSONExport
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse JSON file: %v", err)
	}

	if result.TotalCount != 2 {
		t.Errorf("totalCount = %d, want 2", result.TotalCount)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}

	// Verify file is non-empty.
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("failed to stat output file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestWriteJSONFile_InvalidPath(t *testing.T) {
	err := WriteJSONFile(filepath.Join(t.TempDir(), "nonexistent", "dir", "file.json"), nil, "chrome")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestWriteHTML_Doctype(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", VisitCount: 3, LastVisitTime: 13350000000000000},
	}

	if err := WriteHTML(&buf, entries, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	if !strings.HasPrefix(content, "<!DOCTYPE html>") {
		t.Error("output should start with <!DOCTYPE html>")
	}
}

func TestWriteHTML_Charset(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTML(&buf, nil, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `charset="UTF-8"`) && !strings.Contains(content, "charset=UTF-8") {
		t.Error("output should declare UTF-8 charset")
	}
}

func TestWriteHTML_StyleTag(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTML(&buf, nil, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "<style>") {
		t.Error("output should contain an embedded <style> tag")
	}
	if !strings.Contains(content, "#1c1b1b") {
		t.Error("output should contain dark theme background color #1c1b1b")
	}
	if !strings.Contains(content, "#e5e2e1") {
		t.Error("output should contain text color #e5e2e1")
	}
	if !strings.Contains(content, "#fbbc00") {
		t.Error("output should contain accent color #fbbc00")
	}
}

func TestWriteHTML_Header(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", VisitCount: 1, LastVisitTime: 13350000000000000},
		{ID: 2, URL: "https://other.com", Title: "Other", VisitCount: 2, LastVisitTime: 13349000000000000},
	}

	if err := WriteHTML(&buf, entries, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "chrome") {
		t.Error("output should contain the browser name")
	}
	if !strings.Contains(content, "2") {
		t.Error("output should contain the entry count")
	}
}

func TestWriteHTML_TableColumns(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example Site", VisitCount: 7, LastVisitTime: 13350000000000000},
	}

	if err := WriteHTML(&buf, entries, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	for _, col := range []string{"Title", "URL", "Visits", "Last Visit"} {
		if !strings.Contains(content, col) {
			t.Errorf("output should contain column header %q", col)
		}
	}
}

func TestWriteHTML_EntryData(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example Site", VisitCount: 7, LastVisitTime: 13350000000000000},
	}

	if err := WriteHTML(&buf, entries, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Example Site") {
		t.Error("output should contain the entry title")
	}
	if !strings.Contains(content, "https://example.com") {
		t.Error("output should contain the entry URL")
	}
	if !strings.Contains(content, "7") {
		t.Error("output should contain the visit count")
	}
	// URL should be a link.
	if !strings.Contains(content, `<a `) {
		t.Error("output should contain anchor links for URLs")
	}
}

func TestWriteHTML_XSSEscaping(t *testing.T) {
	var buf bytes.Buffer
	entries := []HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "<script>alert('xss')</script>", VisitCount: 1, LastVisitTime: 13350000000000000},
	}

	if err := WriteHTML(&buf, entries, "chrome"); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}

	content := buf.String()
	if strings.Contains(content, "<script>alert") {
		t.Error("output should escape XSS payload in title")
	}
}

func TestWriteHTML_EmptyEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTML(&buf, []HistoryEntry{}, "edge"); err != nil {
		t.Fatalf("WriteHTML with empty entries failed: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "<table") {
		t.Error("output should contain a table even with no entries")
	}
}

func TestWriteHTML_NilEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTML(&buf, nil, "chrome"); err != nil {
		t.Fatalf("WriteHTML with nil entries failed: %v", err)
	}

	content := buf.String()
	if content == "" {
		t.Error("output should not be empty")
	}
}

func TestWriteHTMLFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "export.html")
	entries := []HistoryEntry{
		{ID: 1, URL: "https://test.com", Title: "Test", VisitCount: 5, LastVisitTime: 13350000000000000},
		{ID: 2, URL: "https://other.com", Title: "Other", VisitCount: 2, LastVisitTime: 13349000000000000},
	}

	if err := WriteHTMLFile(outPath, entries, "chrome"); err != nil {
		t.Fatalf("WriteHTMLFile failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "<!DOCTYPE html>") {
		t.Error("file should start with <!DOCTYPE html>")
	}
	if !strings.Contains(content, "https://test.com") {
		t.Error("file should contain test.com URL")
	}
	if !strings.Contains(content, "https://other.com") {
		t.Error("file should contain other.com URL")
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("failed to stat output file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestWriteHTMLFile_InvalidPath(t *testing.T) {
	err := WriteHTMLFile(filepath.Join(t.TempDir(), "nonexistent", "dir", "file.html"), nil, "chrome")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
