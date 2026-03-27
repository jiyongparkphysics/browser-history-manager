// Package history — export.go: shared CSV, JSON, and HTML export functionality
// used by both the CLI and GUI frontends.
//
// # Responsibilities
//
// This file owns the CSV, JSON, and HTML export format and writing logic:
//
//   - CSVHeader: the canonical column header row for exported CSV files.
//   - WriteCSV: writes history entries to an io.Writer in CSV format with
//     a UTF-8 BOM prefix for Excel compatibility.
//   - WriteCSVFile: convenience wrapper that writes to a file path.
//   - JSONExport: the envelope type for JSON exports with export metadata.
//   - WriteJSON: writes history entries as JSON with metadata to an io.Writer.
//   - WriteJSONFile: convenience wrapper that writes JSON to a file path.
//   - WriteHTML: writes history entries as a styled standalone HTML page.
//   - WriteHTMLFile: convenience wrapper that writes HTML to a file path.
//
// # Design Notes
//
// Both CLI and GUI share this single export implementation to ensure
// consistent output format. The UTF-8 BOM (byte order mark) is prepended
// to CSV output so that Excel correctly interprets the file as UTF-8 encoded.
// HTML output uses html/template to ensure all user-supplied strings are
// properly escaped, preventing XSS in exported files.
package history

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"time"
)

// CSVHeader is the canonical column header row written to exported CSV files.
// Both CLI and GUI use this same header for consistency.
var CSVHeader = []string{"Title", "URL", "Visit Count", "Last Visit"}

// WriteCSV writes history entries to w in CSV format with a UTF-8 BOM prefix
// for Excel compatibility. Each entry is formatted as a row with title, URL,
// visit count, and last visit time.
func WriteCSV(w io.Writer, entries []HistoryEntry) error {
	// BOM for Excel compatibility
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(CSVHeader); err != nil {
		return err
	}
	for _, e := range entries {
		if err := cw.Write([]string{
			e.Title,
			e.URL,
			fmt.Sprintf("%d", e.VisitCount),
			FormatTime(e.LastVisitTime),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteCSVFile writes history entries to a file at the given path in CSV
// format with a UTF-8 BOM prefix. The file is created with mode 0600.
func WriteCSVFile(path string, entries []HistoryEntry) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return WriteCSV(f, entries)
}

// JSONExport is the envelope type written by WriteJSON. It includes export
// metadata alongside the history entries so consumers can identify when and
// from which browser the data was exported.
type JSONExport struct {
	ExportDate string         `json:"exportDate"`
	Browser    string         `json:"browser"`
	TotalCount int            `json:"totalCount"`
	Entries    []HistoryEntry `json:"entries"`
}

// WriteJSON writes history entries to w as a JSON object with export metadata.
// The output format is: {exportDate (RFC3339), browser, totalCount, entries}.
func WriteJSON(w io.Writer, entries []HistoryEntry, browser string) error {
	if entries == nil {
		entries = []HistoryEntry{}
	}
	payload := JSONExport{
		ExportDate: time.Now().UTC().Format(time.RFC3339),
		Browser:    browser,
		TotalCount: len(entries),
		Entries:    entries,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// WriteJSONFile writes history entries as JSON to a file at the given path.
// The file is created with mode 0600.
func WriteJSONFile(path string, entries []HistoryEntry, browser string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return WriteJSON(f, entries, browser)
}

// htmlPageTmpl is the template for the standalone HTML export page. It uses
// html/template so all user-supplied strings are automatically escaped,
// preventing XSS in exported files.
var htmlPageTmpl = template.Must(template.New("history").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Browser}} History Export</title>
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
body {
  background: #1c1b1b;
  color: #e5e2e1;
  font-family: system-ui, -apple-system, sans-serif;
  font-size: 14px;
  line-height: 1.5;
  padding: 2rem;
}
header {
  margin-bottom: 1.5rem;
}
header h1 {
  font-size: 1.5rem;
  color: #fbbc00;
  margin-bottom: 0.25rem;
}
header p {
  color: #a09e9e;
  font-size: 0.875rem;
}
table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
th {
  background: #2a2929;
  color: #fbbc00;
  text-align: left;
  padding: 0.6rem 0.75rem;
  font-weight: 600;
  border-bottom: 2px solid #fbbc00;
}
td {
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #2e2d2d;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
tr:hover td {
  background: #252424;
}
a {
  color: #fbbc00;
  text-decoration: none;
}
a:hover {
  text-decoration: underline;
}
th:nth-child(1), td:nth-child(1) { width: 28%; }
th:nth-child(2), td:nth-child(2) { width: 38%; }
th:nth-child(3), td:nth-child(3) { width: 10%; text-align: right; }
th:nth-child(4), td:nth-child(4) { width: 24%; }
td:nth-child(3) { text-align: right; }
</style>
</head>
<body>
<header>
  <h1>{{.Browser}} History Export</h1>
  <p>Exported {{.ExportDate}} &mdash; {{.TotalCount}} entries</p>
</header>
<table>
  <thead>
    <tr>
      <th>Title</th>
      <th>URL</th>
      <th>Visits</th>
      <th>Last Visit</th>
    </tr>
  </thead>
  <tbody>
    {{range .Entries}}<tr>
      <td title="{{.Title}}">{{.Title}}</td>
      <td title="{{.URL}}"><a href="{{.URL}}" target="_blank" rel="noopener noreferrer">{{.URL}}</a></td>
      <td>{{.VisitCount}}</td>
      <td>{{.LastVisit}}</td>
    </tr>{{end}}
  </tbody>
</table>
</body>
</html>
`))

// htmlEntry is the per-row view model used by htmlPageTmpl. LastVisit is
// pre-formatted so the template does not need to call any functions.
type htmlEntry struct {
	Title      string
	URL        string
	VisitCount int
	LastVisit  string
}

// htmlPage is the top-level view model passed to htmlPageTmpl.
type htmlPage struct {
	Browser    string
	ExportDate string
	TotalCount int
	Entries    []htmlEntry
}

// WriteHTML writes history entries to w as a styled standalone HTML page.
// It uses html/template to safely escape all user-supplied strings (XSS-safe).
// The page includes a dark-theme stylesheet, a summary header, and a full
// table of history entries with clickable URL links.
func WriteHTML(w io.Writer, entries []HistoryEntry, browser string) error {
	rows := make([]htmlEntry, len(entries))
	for i, e := range entries {
		rows[i] = htmlEntry{
			Title:      e.Title,
			URL:        e.URL,
			VisitCount: e.VisitCount,
			LastVisit:  FormatTime(e.LastVisitTime),
		}
	}

	page := htmlPage{
		Browser:    browser,
		ExportDate: time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		TotalCount: len(entries),
		Entries:    rows,
	}

	return htmlPageTmpl.Execute(w, page)
}

// WriteHTMLFile writes history entries as a styled HTML page to a file at the
// given path. The file is created with mode 0600.
func WriteHTMLFile(path string, entries []HistoryEntry, browser string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return WriteHTML(f, entries, browser)
}
