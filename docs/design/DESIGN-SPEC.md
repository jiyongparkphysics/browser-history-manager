# Browser History Manager GUI Design Specification

## Scope

This document describes the current Wails GUI that ships in `cmd/gui/`.
It replaces the earlier AmberHearth concept document, which no longer
matched the implemented product.

## Layout

The GUI is a single-screen desktop layout with two persistent regions:

- Left sidebar: product title, `History | Backups` tabs, Help action, and browser selection with browser-scoped utilities
- Main content: tab-specific toolbar and table content

There is no right-side persistent panel and no slide-in export/delete drawer in
the current implementation.

## Core User Flow

1. Auto-detect an available browser target on startup
2. Load the selected browser profile's history
3. Work in either `History` or `Backups` for the same selected `History` database
4. In `History`, search by URL or title and narrow results with include/exclude filter chips
5. In `History`, select current-page entries or all matching entries
6. In `Backups`, review existing backup snapshots, restore one selected backup, or delete one or more selected backups
7. Delete selected history entries with backup creation, or export selected entries to CSV

## Components

### Sidebar

- Product title
- `History | Backups` peer tabs with icon + active-state styling
- Help button stays in the tab header; Refresh moves into the browser block next to Open DB folder
- `History` sidebar content:
  include/exclude chips plus browser/profile list
- `Backups` sidebar content:
  browser/profile list only
- Chip behavior:
  click the chip body to toggle enabled/disabled, click the close icon to remove
- Browser/profile list:
  switches the active database source for both tabs

### Search Bar

- Free-text search over URL and title
- Search runs on Enter or form submit
- Clear button resets the query
- Input mirrors externally-reset query state, including browser switches

### Selection Toolbar

- `History` toolbar:
  filter badge, selection count, `Page`, `All`, `Clear`, `Delete`, `Export`
- `Backups` toolbar:
  selection count, backup count note, `Page`, `Clear`, `Restore`, `Delete`
- `Restore` is enabled only when exactly one backup row is selected
- `Delete` is enabled when one or more backup rows are selected

### History Table

- Virtualized table for large result sets
- Columns: selection, page title, URL, visits, last visit
- Row click toggles a single entry selection
- Pagination and page-size controls above the table

### Backups Table

- Search backup file names before paging through backup results
- Non-virtual table aligned to the same visual rhythm as the History table
- Columns: `File Name`, `Created`, `Size`, `Items`
- Row click toggles a single backup selection
- `Items` shows the count of non-hidden entries stored in the backup database
- Pagination controls mirror the History table navigation pattern
- Delete removes the selected backup base file plus matching `-wal` / `-shm` sidecars
- Restore replaces the active `History` database after creating a fresh safety backup first

## Data Behavior

- Backend search is paginated for the main table view
- GUI filter mode uses a full matching result set so include/exclude filters and
  global selection apply across all matching entries, not just the current page
- The selected browser/profile and current History state are preserved when switching
  between `History` and `Backups`
- Delete operations queue selected IDs, create one backup, then commit one batch delete
- CSV export writes the currently selected entries
- Backup listing scans the active `History` database folder for `History_backup_*`
  base files and derives `Items` by opening each backup read-only
- Backup restore creates a new safety backup of the active database before copying
  the selected backup into place

## Visual Tokens

- Dark background and warm gold accents
- Fonts:
  Plus Jakarta Sans for headings, Manrope for UI text, JetBrains Mono for tabular data
- Rounded containers and pill-style action buttons
- Material Symbols for icons
- Shared spacing, button sizing, and row heights between `History` and `Backups`
- Version is shown in the Help modal, not in the top application header

## Non-Goals

- Bookmark management
- Multi-panel workspace layout
- JSON/HTML export from the main GUI toolbar
- Persistent right-side action drawers
