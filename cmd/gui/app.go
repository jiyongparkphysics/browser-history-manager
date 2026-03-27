package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"chrome-history-manager/internal/history"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	_ "modernc.org/sqlite"
)

// App is the Wails application struct. Its exported methods are bound
// to the frontend and callable from JavaScript via the Wails runtime.
type App struct {
	ctx            context.Context
	deletionQueue  *history.DeletionQueue
	deletionDBPath string // tracks which DB the queue targets
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the Wails app starts. The context is saved
// so it can be used for runtime calls (e.g. dialogs, events).
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// BrowserEntry represents a detected browser and its History database path.
type BrowserEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// HistoryResult is the paginated response returned by SearchHistory.
type HistoryResult struct {
	Entries    []history.HistoryEntry `json:"entries"`
	Total      int                    `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	TotalPages int                    `json:"totalPages"`
}

// SearchHistory performs a paginated, parameterized SQL search over the
// Chrome History database. All user-supplied values are bound via
// placeholders (?) to prevent SQL injection.
func (a *App) SearchHistory(dbPath string, query string, page int, pageSize int) (*HistoryResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	var total int
	var rows *sql.Rows

	if query == "" {
		// No search query - return all non-hidden entries paginated.
		err = db.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&total)
		if err != nil {
			return nil, fmt.Errorf("failed to count entries: %w", err)
		}
	} else {
		// Parameterized LIKE search - user input is bound, never interpolated.
		likePattern := "%" + query + "%"
		err = db.QueryRow(
			"SELECT COUNT(*) FROM urls WHERE hidden = 0 AND (url LIKE ? OR title LIKE ?)",
			likePattern, likePattern,
		).Scan(&total)
		if err != nil {
			return nil, fmt.Errorf("failed to count entries: %w", err)
		}
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * pageSize
	if query == "" {
		rows, err = db.Query(
			"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0 ORDER BY last_visit_time DESC LIMIT ? OFFSET ?",
			pageSize, offset,
		)
	} else {
		likePattern := "%" + query + "%"
		rows, err = db.Query(
			"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0 AND (url LIKE ? OR title LIKE ?) ORDER BY last_visit_time DESC LIMIT ? OFFSET ?",
			likePattern, likePattern, pageSize, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	var entries []history.HistoryEntry
	for rows.Next() {
		var e history.HistoryEntry
		if err := rows.Scan(&e.ID, &e.URL, &e.Title, &e.VisitCount, &e.LastVisitTime); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return &HistoryResult{
		Entries:    entries,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// SearchHistoryAll returns all non-hidden entries matching the optional query,
// without pagination. This is used by the GUI when a full result set is needed
// for client-side filtering or true global selection.
func (a *App) SearchHistoryAll(dbPath string, query string) ([]history.HistoryEntry, error) {
	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	entries, err := queryAllMatching(db, query)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		return []history.HistoryEntry{}, nil
	}
	return entries, nil
}

// DetectBrowsers returns the list of detected browser History database paths
// on the current system, delegating to the shared history package.
func (a *App) DetectBrowsers() []BrowserEntry {
	found := history.DetectBrowserPaths()
	result := make([]BrowserEntry, 0, len(found))
	for name, path := range found {
		result = append(result, BrowserEntry{Name: name, Path: path})
	}
	return result
}

// AutoDetectBrowser discovers installed browsers, prioritises Chrome, and
// returns the detected browser's name and History database path. This is
// used by the GUI frontend on startup to pre-select a browser.
func (a *App) AutoDetectBrowser() (*history.BrowserDetection, error) {
	return history.AutoDetectBrowser()
}

// BrowserProfileEntry represents a single profile within a browser,
// serialised for the frontend.
type BrowserProfileEntry struct {
	Name   string `json:"name"`
	DBPath string `json:"dbPath"`
}

// BrowserWithProfiles represents a browser and all its discovered profiles,
// serialised for the frontend.
type BrowserWithProfiles struct {
	Name     string                `json:"name"`
	Profiles []BrowserProfileEntry `json:"profiles"`
}

// BackupSnapshot represents a single backup file for the currently selected
// History database. The frontend uses this to populate the Backups tab table.
type BackupSnapshot struct {
	Path        string `json:"path"`
	FileName    string `json:"fileName"`
	CreatedUnix int64  `json:"createdUnix"`
	SizeBytes   int64  `json:"sizeBytes"`
	ItemCount   int    `json:"itemCount"`
}

// ListBrowsersWithProfiles returns all detected browsers on the current
// system along with their profile directories that contain a History
// database. This allows the frontend to offer a browser + profile picker.
func (a *App) ListBrowsersWithProfiles() []BrowserWithProfiles {
	infos := history.ListBrowsersWithProfiles()
	result := make([]BrowserWithProfiles, 0, len(infos))
	for _, info := range infos {
		profiles := make([]BrowserProfileEntry, len(info.Profiles))
		for i, p := range info.Profiles {
			profiles[i] = BrowserProfileEntry{Name: p.Name, DBPath: p.DBPath}
		}
		result = append(result, BrowserWithProfiles{
			Name:     info.Name,
			Profiles: profiles,
		})
	}
	return result
}

func backupItemCount(backupPath string) (int, error) {
	db, tmpPath, err := history.OpenReadOnlyDB(backupPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM urls WHERE hidden = 0").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func validateBackupPathForDB(dbPath string, backupPath string) (string, error) {
	safeDBPath, err := history.ValidateDBPath(dbPath)
	if err != nil {
		return "", err
	}
	safeBackupPath, err := history.ValidateDBPath(backupPath)
	if err != nil {
		return "", err
	}

	if filepath.Dir(safeBackupPath) != filepath.Dir(safeDBPath) {
		return "", fmt.Errorf("backup path must be in the selected DB folder")
	}

	fileName := filepath.Base(safeBackupPath)
	if !strings.HasPrefix(fileName, "History_backup_") {
		return "", fmt.Errorf("invalid backup file name: %s", fileName)
	}
	if strings.HasSuffix(fileName, "-wal") || strings.HasSuffix(fileName, "-shm") {
		return "", fmt.Errorf("backup sidecars cannot be selected directly: %s", fileName)
	}

	return safeBackupPath, nil
}

func restoreBackupSidecars(srcBase string, dstBase string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		srcSidecar := srcBase + suffix
		dstSidecar := dstBase + suffix

		if _, err := os.Stat(srcSidecar); err == nil {
			if err := history.CopyFile(srcSidecar, dstSidecar); err != nil {
				return fmt.Errorf("failed to restore backup sidecar %q: %w", filepath.Base(srcSidecar), err)
			}
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat backup sidecar %q: %w", filepath.Base(srcSidecar), err)
		}

		if err := os.Remove(dstSidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove stale DB sidecar %q: %w", filepath.Base(dstSidecar), err)
		}
	}

	return nil
}

// ListBackups returns backup snapshots for the selected History database.
// Only base backup files in the same directory are included; sidecars are
// treated as implementation details of the backup file itself.
func (a *App) ListBackups(dbPath string) ([]BackupSnapshot, error) {
	safePath, err := history.ValidateDBPath(dbPath)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(safePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan backup directory: %w", err)
	}

	backups := make([]BackupSnapshot, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "History_backup_") {
			continue
		}
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
			continue
		}

		path := filepath.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("failed to stat backup %q: %w", name, err)
		}

		itemCount, err := backupItemCount(path)
		if err != nil {
			return nil, fmt.Errorf("failed to count backup items for %q: %w", name, err)
		}

		backups = append(backups, BackupSnapshot{
			Path:        path,
			FileName:    name,
			CreatedUnix: info.ModTime().Unix(),
			SizeBytes:   info.Size(),
			ItemCount:   itemCount,
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		if backups[i].CreatedUnix == backups[j].CreatedUnix {
			return backups[i].FileName > backups[j].FileName
		}
		return backups[i].CreatedUnix > backups[j].CreatedUnix
	})

	return backups, nil
}

// DeleteBackups removes one or more selected backup snapshots from the active
// DB folder. Matching -wal and -shm sidecars are removed with each base file.
func (a *App) DeleteBackups(dbPath string, backupPaths []string) (int, error) {
	if len(backupPaths) == 0 {
		return 0, fmt.Errorf("no backups selected")
	}

	deleted := 0
	for _, backupPath := range backupPaths {
		safeBackupPath, err := validateBackupPathForDB(dbPath, backupPath)
		if err != nil {
			return deleted, err
		}

		if err := os.Remove(safeBackupPath); err != nil {
			return deleted, fmt.Errorf("failed to remove backup %q: %w", filepath.Base(safeBackupPath), err)
		}

		for _, suffix := range []string{"-wal", "-shm"} {
			sidecar := safeBackupPath + suffix
			if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
				return deleted, fmt.Errorf("failed to remove backup sidecar %q: %w", filepath.Base(sidecar), err)
			}
		}

		deleted++
	}

	return deleted, nil
}

// RestoreBackup replaces the active History database with the selected backup.
// The current DB state is first preserved as a fresh safety backup.
func (a *App) RestoreBackup(dbPath string, backupPath string) (string, error) {
	safeDBPath, err := history.ValidateDBPath(dbPath)
	if err != nil {
		return "", err
	}
	safeBackupPath, err := validateBackupPathForDB(safeDBPath, backupPath)
	if err != nil {
		return "", err
	}
	if err := history.ValidateChromeDB(safeBackupPath); err != nil {
		return "", fmt.Errorf("invalid backup database: %w", err)
	}

	safetyBackupPath, err := history.BackupDB(safeDBPath)
	if err != nil {
		return "", fmt.Errorf("failed to preserve current DB before restore: %w", err)
	}

	if err := history.CopyFile(safeBackupPath, safeDBPath); err != nil {
		return safetyBackupPath, fmt.Errorf("failed to restore backup file: %w", err)
	}
	if err := restoreBackupSidecars(safeBackupPath, safeDBPath); err != nil {
		return safetyBackupPath, err
	}

	return safetyBackupPath, nil
}

func folderPathForDB(dbPath string) (string, error) {
	safePath, err := history.ValidateDBPath(dbPath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(safePath), nil
}

func openFolderCommandForOS(goos string, folder string) (string, []string, error) {
	switch goos {
	case "windows":
		return "explorer.exe", []string{folder}, nil
	case "darwin":
		return "open", []string{folder}, nil
	case "linux":
		return "xdg-open", []string{folder}, nil
	default:
		return "", nil, fmt.Errorf("unsupported OS for opening folders: %s", goos)
	}
}

// OpenDBFolder opens the directory containing the selected History database.
// Backups are created in the same location, so this gives the user direct
// access to both the active DB and any generated backup files.
func (a *App) OpenDBFolder(dbPath string) (string, error) {
	folder, err := folderPathForDB(dbPath)
	if err != nil {
		return "", err
	}

	command, args, err := openFolderCommandForOS(goruntime.GOOS, folder)
	if err != nil {
		return "", err
	}

	if err := exec.Command(command, args...).Start(); err != nil {
		return "", fmt.Errorf("failed to open folder: %w", err)
	}

	return folder, nil
}

// GetVersion returns the current GUI build version for About/Help surfaces.
func (a *App) GetVersion() string {
	return version
}

// QueueDeletions adds one or more entry IDs to the batch deletion queue.
// If the dbPath changes, the queue is reset to target the new database.
// Items are not deleted until CommitDeletions is called.
func (a *App) QueueDeletions(dbPath string, ids []int64) int {
	if a.deletionQueue == nil || a.deletionDBPath != dbPath {
		a.deletionQueue = history.NewDeletionQueue(dbPath)
		a.deletionDBPath = dbPath
	}
	a.deletionQueue.QueueForDeletion(ids...)
	return a.deletionQueue.QueueSize()
}

// UnqueueDeletions removes one or more entry IDs from the batch deletion queue.
func (a *App) UnqueueDeletions(ids []int64) int {
	if a.deletionQueue == nil {
		return 0
	}
	a.deletionQueue.RemoveFromQueue(ids...)
	return a.deletionQueue.QueueSize()
}

// GetDeletionQueueSize returns the number of entries currently queued for deletion.
func (a *App) GetDeletionQueueSize() int {
	if a.deletionQueue == nil {
		return 0
	}
	return a.deletionQueue.QueueSize()
}

// IsDeletionQueued reports whether a given ID is in the deletion queue.
func (a *App) IsDeletionQueued(id int64) bool {
	if a.deletionQueue == nil {
		return false
	}
	return a.deletionQueue.IsQueued(id)
}

// ClearDeletionQueue removes all entries from the deletion queue without
// performing any database operations.
func (a *App) ClearDeletionQueue() {
	if a.deletionQueue != nil {
		a.deletionQueue.ClearQueue()
	}
}

// BatchDeleteResult contains the outcome of a CommitDeletions call.
type BatchDeleteResult struct {
	Deleted    int    `json:"deleted"`
	BackupPath string `json:"backupPath"`
}

// CommitDeletions creates a single backup and deletes all queued entries
// in one transaction. Returns the number of deleted entries and the backup path.
func (a *App) CommitDeletions() (*BatchDeleteResult, error) {
	if a.deletionQueue == nil {
		return &BatchDeleteResult{}, nil
	}
	deleted, backupPath, err := a.deletionQueue.CommitDeletions()
	if err != nil {
		return nil, err
	}
	return &BatchDeleteResult{
		Deleted:    deleted,
		BackupPath: backupPath,
	}, nil
}

// DeleteEntries removes the specified history entries after creating a backup.
// It follows the same preview-then-apply pattern as the CLI.
func (a *App) DeleteEntries(dbPath string, ids []int64) error {
	_, err := history.BackupDB(dbPath)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	db, err := history.OpenWriteDB(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database for writing: %w", err)
	}
	defer db.Close()

	entries := make([]history.HistoryEntry, len(ids))
	for i, id := range ids {
		entries[i] = history.HistoryEntry{ID: id}
	}
	return history.DeleteEntries(db, entries)
}

// ExportCSV exports matching history entries to a CSV file. Uses in-memory
// filtering via the shared history package, consistent with the CLI.
func (a *App) ExportCSV(dbPath string, matchKeywords []string, protectKeywords []string, outPath string) (int, error) {
	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	entries, err := history.GetAllURLs(db)
	if err != nil {
		return 0, fmt.Errorf("failed to get entries: %w", err)
	}

	filtered := history.FilterEntries(entries, matchKeywords, protectKeywords)

	if err := history.WriteCSVFile(outPath, filtered); err != nil {
		return 0, fmt.Errorf("failed to write CSV: %w", err)
	}

	return len(filtered), nil
}

// ExportSelectedCSV exports only the entries with the given IDs to a CSV file.
// IDs are int64 values sourced from the database, bound via parameterized
// queries to prevent SQL injection.
func (a *App) ExportSelectedCSV(dbPath string, ids []int64, outPath string) (int, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("no entries selected for export")
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	entries, err := history.GetEntriesByIDs(db, ids)
	if err != nil {
		return 0, fmt.Errorf("failed to get entries: %w", err)
	}

	if err := history.WriteCSVFile(outPath, entries); err != nil {
		return 0, fmt.Errorf("failed to write CSV: %w", err)
	}

	return len(entries), nil
}

// ExportFilteredCSV exports all entries matching the current search query to CSV.
// Uses parameterized LIKE search, consistent with SearchHistory, but without
// pagination ??all matching results are exported.
func (a *App) ExportFilteredCSV(dbPath string, query string, outPath string) (int, error) {
	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	var rows *sql.Rows
	if query == "" {
		rows, err = db.Query(
			"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0 ORDER BY last_visit_time DESC",
		)
	} else {
		likePattern := "%" + query + "%"
		rows, err = db.Query(
			"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0 AND (url LIKE ? OR title LIKE ?) ORDER BY last_visit_time DESC",
			likePattern, likePattern,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	var entries []history.HistoryEntry
	for rows.Next() {
		var e history.HistoryEntry
		if err := rows.Scan(&e.ID, &e.URL, &e.Title, &e.VisitCount, &e.LastVisitTime); err != nil {
			return 0, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("row iteration error: %w", err)
	}

	if err := history.WriteCSVFile(outPath, entries); err != nil {
		return 0, fmt.Errorf("failed to write CSV: %w", err)
	}

	return len(entries), nil
}

// GetDeletionQueueItems returns full HistoryEntry data for all IDs currently
// in the deletion queue. Returns an empty slice if the queue is nil or empty.
func (a *App) GetDeletionQueueItems() ([]history.HistoryEntry, error) {
	if a.deletionQueue == nil {
		return []history.HistoryEntry{}, nil
	}
	ids := a.deletionQueue.QueuedIDs()
	if len(ids) == 0 {
		return []history.HistoryEntry{}, nil
	}

	db, tmpPath, err := history.OpenReadOnlyDB(a.deletionDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	entries, err := history.GetEntriesByIDs(db, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get entries: %w", err)
	}
	return entries, nil
}

// SearchHistoryOptions configures an advanced paginated history search.
type SearchHistoryOptions struct {
	DBPath    string `json:"dbPath"`
	Keyword   string `json:"keyword"`
	StartDate string `json:"startDate"` // YYYY-MM-DD or empty
	EndDate   string `json:"endDate"`   // YYYY-MM-DD or empty
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
	SortBy    string `json:"sortBy"`    // "title", "url", "visits", "time" (default: "time")
	SortOrder string `json:"sortOrder"` // "asc" or "desc" (default: "desc")
}

// dateToChromeTime converts a YYYY-MM-DD string to a Chrome timestamp
// (microseconds since 1601-01-01). Returns 0 and no error for empty input.
func dateToChromeTime(dateStr string) (int64, error) {
	if dateStr == "" {
		return 0, nil
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0, fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", dateStr, err)
	}
	// Chrome timestamps: microseconds since 1601-01-01.
	// Unix epoch offset in microseconds: 11644473600 * 1_000_000.
	return t.UnixMicro() + 11644473600*1000000, nil
}

// SearchHistoryAdvanced performs a paginated search with optional keyword
// filter, date range, and configurable sort order. All user-supplied values
// are bound via placeholders (?) to prevent SQL injection.
func (a *App) SearchHistoryAdvanced(opts SearchHistoryOptions) (*HistoryResult, error) {
	// Apply defaults.
	page := opts.Page
	if page < 1 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}
	sortBy := opts.SortBy
	switch sortBy {
	case "title", "url", "visits", "time":
	default:
		sortBy = "time"
	}
	sortOrder := strings.ToLower(opts.SortOrder)
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	// Map sortBy to column name.
	columnMap := map[string]string{
		"title":  "title",
		"url":    "url",
		"visits": "visit_count",
		"time":   "last_visit_time",
	}
	orderCol := columnMap[sortBy]

	// Convert dates to Chrome timestamps.
	startCT, err := dateToChromeTime(opts.StartDate)
	if err != nil {
		return nil, err
	}
	var endCT int64
	if opts.EndDate != "" {
		// Include the full end day by adding one day's worth of microseconds.
		endCT, err = dateToChromeTime(opts.EndDate)
		if err != nil {
			return nil, err
		}
		endCT += 86400 * 1000000 // advance to start of next day
	}

	db, tmpPath, err := history.OpenReadOnlyDB(opts.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	// Build WHERE clause dynamically with parameterized placeholders.
	whereParts := []string{"hidden = 0"}
	args := []interface{}{}

	if opts.Keyword != "" {
		like := "%" + opts.Keyword + "%"
		whereParts = append(whereParts, "(url LIKE ? OR title LIKE ?)")
		args = append(args, like, like)
	}
	if startCT > 0 {
		whereParts = append(whereParts, "last_visit_time >= ?")
		args = append(args, startCT)
	}
	if endCT > 0 {
		whereParts = append(whereParts, "last_visit_time < ?")
		args = append(args, endCT)
	}

	where := strings.Join(whereParts, " AND ")

	// Count total matching rows.
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM urls WHERE %s", where)
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count entries: %w", err)
	}

	// Fetch paginated results. ORDER BY uses a trusted column name (not user input).
	offset := (page - 1) * pageSize
	selectQuery := fmt.Sprintf(
		"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE %s ORDER BY %s %s LIMIT ? OFFSET ?",
		where, orderCol, sortOrder,
	)
	pageArgs := append(args, pageSize, offset)
	rows, err := db.Query(selectQuery, pageArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	var entries []history.HistoryEntry
	for rows.Next() {
		var e history.HistoryEntry
		if err := rows.Scan(&e.ID, &e.URL, &e.Title, &e.VisitCount, &e.LastVisitTime); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

	return &HistoryResult{
		Entries:    entries,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// queryAllMatching returns all history entries matching a keyword LIKE filter,
// without pagination. Used by export-with-dialog methods.
func queryAllMatching(db *sql.DB, keyword string) ([]history.HistoryEntry, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if keyword == "" {
		rows, err = db.Query(
			"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0 ORDER BY last_visit_time DESC",
		)
	} else {
		like := "%" + keyword + "%"
		rows, err = db.Query(
			"SELECT id, url, title, visit_count, last_visit_time FROM urls WHERE hidden = 0 AND (url LIKE ? OR title LIKE ?) ORDER BY last_visit_time DESC",
			like, like,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	var entries []history.HistoryEntry
	for rows.Next() {
		var e history.HistoryEntry
		if err := rows.Scan(&e.ID, &e.URL, &e.Title, &e.VisitCount, &e.LastVisitTime); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ExportJSONWithDialog opens a save-file dialog, then exports all entries
// matching query to the chosen path as JSON. Returns the count of exported entries.
func (a *App) ExportJSONWithDialog(dbPath string, query string) (int, error) {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export History as JSON",
		DefaultFilename: "history.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON Files (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("dialog error: %w", err)
	}
	if filePath == "" {
		// User cancelled the dialog.
		return 0, nil
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	entries, err := queryAllMatching(db, query)
	if err != nil {
		return 0, err
	}

	if err := history.WriteJSONFile(filePath, entries, ""); err != nil {
		return 0, fmt.Errorf("failed to write JSON: %w", err)
	}
	return len(entries), nil
}

// ExportHTMLWithDialog opens a save-file dialog, then exports all entries
// matching query to the chosen path as HTML. Returns the count of exported entries.
func (a *App) ExportHTMLWithDialog(dbPath string, query string) (int, error) {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export History as HTML",
		DefaultFilename: "history.html",
		Filters: []runtime.FileFilter{
			{DisplayName: "HTML Files (*.html)", Pattern: "*.html"},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("dialog error: %w", err)
	}
	if filePath == "" {
		// User cancelled the dialog.
		return 0, nil
	}

	db, tmpPath, err := history.OpenReadOnlyDB(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	defer history.CleanupTmp(tmpPath)

	entries, err := queryAllMatching(db, query)
	if err != nil {
		return 0, err
	}

	if err := history.WriteHTMLFile(filePath, entries, ""); err != nil {
		return 0, fmt.Errorf("failed to write HTML: %w", err)
	}
	return len(entries), nil
}
