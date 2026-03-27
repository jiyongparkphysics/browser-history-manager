import React, { useCallback, useState, useMemo, useEffect, useRef } from 'react'
import toast from 'react-hot-toast'
import { AppProvider, useAppContext } from './context/AppContext'
import Sidebar from './components/sidebar/Sidebar'
import SearchBar from './components/SearchBar'
import HistoryTable from './components/HistoryTable'
import BackupsTable from './components/BackupsTable'
import ConfirmModal from './components/common/ConfirmModal'
import useFilterPresets from './hooks/useFilterPresets'
import {
  SearchHistoryAll,
  QueueDeletions,
  CommitDeletions,
  ExportSelectedCSV,
  ListBackups,
  DeleteBackups,
  RestoreBackup,
  GetVersion,
} from './wailsjs/go/main/App'
import { applyPageSelection, shouldKeepAllGlobalSelected } from './selectionState'
import { clampPage, getTotalPages, paginateEntries } from './paginationState'
import {
  canDeleteBackups,
  canRestoreBackup,
  clearBackupSelection,
  toggleBackupSelection,
} from './backupState'
import { filterBackupsByFileName } from './backupListState'
import './style.css'

function passesFilter(entry, includeList, excludeList) {
  const url = (entry.url || '').toLowerCase()
  const title = (entry.title || '').toLowerCase()
  for (const ex of excludeList) {
    if (url.includes(ex) || title.includes(ex)) return false
  }
  if (includeList.length === 0) return true
  for (const inc of includeList) {
    if (url.includes(inc) || title.includes(inc)) return true
  }
  return false
}

function AppContent() {
  const {
    activeTab,
    setActiveTab,
    selectedPath,
    browserName,
    loading,
    setLoading,
    initialLoading,
    historyResult,
    query,
    setQuery,
    page,
    setPage,
    selectedIds,
    setSelectedIds,
    fetchHistory,
    refreshData,
    pageSize,
    setPageSize,
    status,
    setStatus,
  } = useAppContext()

  const filterPresets = useFilterPresets()
  const { presets } = filterPresets
  const [allGlobalSelected, setAllGlobalSelected] = useState(false)
  const [confirmModal, setConfirmModal] = useState({ isOpen: false, title: '', message: '', danger: false, onConfirm: null })
  const [allEntries, setAllEntries] = useState(null)
  const [filterPage, setFilterPage] = useState(1)
  const [backups, setBackups] = useState([])
  const [backupsLoading, setBackupsLoading] = useState(false)
  const [backupQuery, setBackupQuery] = useState('')
  const [backupPage, setBackupPage] = useState(1)
  const [backupPageSize, setBackupPageSize] = useState(50)
  const [selectedBackupPaths, setSelectedBackupPaths] = useState(clearBackupSelection())
  const [backupStatus, setBackupStatus] = useState({ message: '', type: 'info' })
  const [guiVersion, setGuiVersion] = useState('dev')
  const allEntriesRequestRef = useRef(0)

  const displayBrowserName = browserName
    ? browserName.charAt(0).toUpperCase() + browserName.slice(1)
    : 'the browser'
  const filterFetchErrorPrefix = 'Filter reload failed:'

  const closeModal = () => setConfirmModal((p) => ({ ...p, isOpen: false }))

  const friendlyError = useCallback((err) => {
    const msg = String(err)
    if (msg.includes('database is locked') || msg.includes('SQLITE_BUSY')) {
      return `Close ${displayBrowserName} first, then try again.`
    }
    return msg
  }, [displayBrowserName])

  const includeList = useMemo(() => presets.include.filter((p) => p.enabled).map((p) => p.keyword), [presets.include])
  const excludeList = useMemo(() => presets.exclude.filter((p) => p.enabled).map((p) => p.keyword), [presets.exclude])
  const hasFilters = includeList.length > 0 || excludeList.length > 0

  useEffect(() => {
    let cancelled = false
    GetVersion()
      .then((value) => {
        if (!cancelled && value) setGuiVersion(value)
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  const refetchAllEntries = useCallback(() => {
    if (!hasFilters || !selectedPath) {
      allEntriesRequestRef.current += 1
      setAllEntries(null)
      setFilterPage(1)
      setStatus((prev) => (
        prev.message.startsWith(filterFetchErrorPrefix)
          ? { message: '', type: 'info' }
          : prev
      ))
      return Promise.resolve()
    }

    const requestId = allEntriesRequestRef.current + 1
    allEntriesRequestRef.current = requestId
    return SearchHistoryAll(selectedPath, query)
      .then((entries) => {
        if (allEntriesRequestRef.current !== requestId) return
        setAllEntries(entries || [])
        setFilterPage(1)
        setStatus((prev) => (
          prev.message.startsWith(filterFetchErrorPrefix)
            ? { message: '', type: 'info' }
            : prev
        ))
      })
      .catch((err) => {
        if (allEntriesRequestRef.current !== requestId) return
        const message = `${filterFetchErrorPrefix} ${friendlyError(err)}`
        setAllEntries([])
        setFilterPage(1)
        toast.error(message)
        setStatus({ message, type: 'error' })
      })
  }, [friendlyError, hasFilters, query, selectedPath, setStatus])

  useEffect(() => {
    refetchAllEntries()
  }, [refetchAllEntries])

  useEffect(() => {
    if (!shouldKeepAllGlobalSelected(allGlobalSelected, selectedIds)) {
      setAllGlobalSelected(false)
    }
  }, [allGlobalSelected, selectedIds])

  useEffect(() => {
    setSelectedBackupPaths(clearBackupSelection())
    setBackupQuery('')
    setBackupPage(1)
  }, [selectedPath])

  const loadBackups = useCallback(async () => {
    if (!selectedPath) {
      setBackups([])
      setSelectedBackupPaths(clearBackupSelection())
      setBackupStatus({ message: '', type: 'info' })
      return
    }

    setBackupsLoading(true)
    try {
      const snapshots = await ListBackups(selectedPath)
      setBackups(snapshots || [])
      setSelectedBackupPaths(clearBackupSelection())
      setBackupStatus({ message: '', type: 'info' })
    } catch (err) {
      const message = `Backup reload failed: ${friendlyError(err)}`
      setBackups([])
      setSelectedBackupPaths(clearBackupSelection())
      setBackupStatus({ message, type: 'error' })
      toast.error(message)
    } finally {
      setBackupsLoading(false)
    }
  }, [friendlyError, selectedPath])

  useEffect(() => {
    if (activeTab === 'backups') {
      loadBackups()
    }
  }, [activeTab, loadBackups])

  const filteredAll = useMemo(() => {
    if (!hasFilters || !allEntries) return null
    return allEntries.filter((e) => passesFilter(e, includeList, excludeList))
  }, [allEntries, includeList, excludeList, hasFilters])

  const filteredTotalPages = filteredAll ? getTotalPages(filteredAll.length, pageSize) : 0
  const clampedFilterPage = filteredAll ? clampPage(filterPage, filteredTotalPages) : 1
  const filteredPageEntries = useMemo(() => {
    if (!filteredAll) return null
    return paginateEntries(filteredAll, clampedFilterPage, pageSize)
  }, [filteredAll, clampedFilterPage, pageSize])

  useEffect(() => {
    if (!filteredAll) return
    setFilterPage((prev) => clampPage(prev, filteredTotalPages))
  }, [filteredAll, filteredTotalPages])

  const filteredBackups = useMemo(() => (
    filterBackupsByFileName(backups, backupQuery)
  ), [backups, backupQuery])

  const backupTotalPages = getTotalPages(filteredBackups.length, backupPageSize)
  const clampedBackupPage = clampPage(backupPage, backupTotalPages)
  const displayBackups = useMemo(() => (
    paginateEntries(filteredBackups, clampedBackupPage, backupPageSize)
  ), [filteredBackups, clampedBackupPage, backupPageSize])

  useEffect(() => {
    setBackupPage((prev) => clampPage(prev, backupTotalPages))
  }, [backupTotalPages])

  const isFilterMode = hasFilters && filteredPageEntries !== null
  const displayEntries = isFilterMode ? filteredPageEntries : (historyResult?.entries || [])
  const displayTotal = isFilterMode ? filteredAll.length : (historyResult?.total || 0)
  const displayPage = isFilterMode ? clampedFilterPage : (historyResult?.page || 1)
  const displayTotalPages = isFilterMode ? filteredTotalPages : (historyResult?.totalPages || 1)

  const handleSearch = useCallback((newQuery) => {
    setAllGlobalSelected(false)
    setAllEntries(null)
    setFilterPage(1)
    setPage(1)
    if (query !== newQuery) {
      setQuery(newQuery)
    }
    fetchHistory(newQuery, 1)
  }, [fetchHistory, query, setQuery, setPage])

  const handlePageChange = (newPage) => {
    if (isFilterMode) setFilterPage(clampPage(newPage, filteredTotalPages))
    else {
      setPage(newPage)
      fetchHistory(query, newPage)
    }
  }

  const handleToggleSelect = (id) => {
    setAllGlobalSelected(false)
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const handleSelectAll = (select) => {
    if (!displayEntries.length) return
    setAllGlobalSelected(false)
    setSelectedIds((prev) => applyPageSelection(prev, displayEntries, select))
  }

  const handleSelectAllGlobal = () => {
    if (isFilterMode && filteredAll) {
      setSelectedIds(new Set(filteredAll.map((e) => e.id)))
      setAllGlobalSelected(true)
      toast.success(`Selected ${filteredAll.length.toLocaleString()} entries`)
      return
    }
    if (!selectedPath || !historyResult) return
    setLoading(true)
    SearchHistoryAll(selectedPath, query)
      .then((entries) => {
        if (entries) {
          setSelectedIds(new Set(entries.map((e) => e.id)))
          setAllGlobalSelected(true)
          toast.success(`Selected ${entries.length.toLocaleString()} entries`)
        }
      })
      .catch((err) => toast.error(`Failed: ${err}`))
      .finally(() => setLoading(false))
  }

  const handleDeselectAll = () => {
    setSelectedIds(new Set())
    setAllGlobalSelected(false)
  }

  const refreshHistoryView = useCallback(async () => {
    await refreshData()
    await refetchAllEntries()
  }, [refreshData, refetchAllEntries])

  const handleRefresh = useCallback(async () => {
    if (activeTab === 'backups') {
      await loadBackups()
      return
    }
    await refreshHistoryView()
  }, [activeTab, loadBackups, refreshHistoryView])

  const handleBackupSearch = useCallback((newQuery) => {
    setBackupQuery(newQuery)
    setBackupPage(1)
  }, [])

  const handleDelete = () => {
    if (selectedIds.size === 0) return
    setConfirmModal({
      isOpen: true,
      title: 'Delete',
      message: `Permanently delete ${selectedIds.size.toLocaleString()} entries? A backup will be created next to the selected History database.`,
      danger: true,
      onConfirm: async () => {
        closeModal()
        setLoading(true)
        try {
          await QueueDeletions(selectedPath, Array.from(selectedIds))
          const result = await CommitDeletions()
          toast.success(`Deleted ${result.deleted} entries. Backup created next to the selected History database.`)
          setSelectedIds(new Set())
          setAllGlobalSelected(false)
          await refreshHistoryView()
        } catch (err) {
          toast.error(friendlyError(err))
        } finally {
          setLoading(false)
        }
      },
    })
  }

  const handleExport = async () => {
    if (!selectedPath || selectedIds.size === 0) return
    setLoading(true)
    try {
      const count = await ExportSelectedCSV(selectedPath, Array.from(selectedIds), 'history_export.csv')
      toast.success(`Exported ${count} entries to CSV`)
    } catch (err) {
      toast.error(friendlyError(err))
    } finally {
      setLoading(false)
    }
  }

  const handleToggleBackupSelect = (backupPath) => {
    setSelectedBackupPaths((prev) => toggleBackupSelection(prev, backupPath))
  }

  const handleSelectAllBackups = (select) => {
    if (!displayBackups.length) return
    if (select) {
      setSelectedBackupPaths((prev) => {
        const next = new Set(prev)
        displayBackups.forEach((backup) => next.add(backup.path))
        return next
      })
      return
    }
    setSelectedBackupPaths((prev) => {
      const next = new Set(prev)
      displayBackups.forEach((backup) => next.delete(backup.path))
      return next
    })
  }

  const handleClearBackupSelection = () => {
    setSelectedBackupPaths(clearBackupSelection())
  }

  const handleDeleteBackups = () => {
    if (!canDeleteBackups(selectedBackupPaths)) return
    setConfirmModal({
      isOpen: true,
      title: 'Delete',
      message: `Delete ${selectedBackupPaths.size.toLocaleString()} selected backups? Matching -wal and -shm sidecars will be removed with them.`,
      danger: true,
      onConfirm: async () => {
        closeModal()
        setBackupsLoading(true)
        try {
          const deleted = await DeleteBackups(selectedPath, Array.from(selectedBackupPaths))
          setSelectedBackupPaths(clearBackupSelection())
          toast.success(`Deleted ${deleted} backup files.`)
          await loadBackups()
        } catch (err) {
          toast.error(friendlyError(err))
        } finally {
          setBackupsLoading(false)
        }
      },
    })
  }

  const handleRestoreSelectedBackup = () => {
    if (!canRestoreBackup(selectedBackupPaths)) return
    const [backupPath] = Array.from(selectedBackupPaths)
    setConfirmModal({
      isOpen: true,
      title: 'Restore',
      message: 'Restore the selected backup into the active History database? The current database will first be preserved as a fresh safety backup next to it.',
      danger: false,
      onConfirm: async () => {
        closeModal()
        setBackupsLoading(true)
        try {
          const safetyBackupPath = await RestoreBackup(selectedPath, backupPath)
          setSelectedBackupPaths(clearBackupSelection())
          toast.success(`Backup restored. Safety backup created at ${safetyBackupPath}.`)
          await refreshHistoryView()
          await loadBackups()
        } catch (err) {
          toast.error(friendlyError(err))
        } finally {
          setBackupsLoading(false)
        }
      },
    })
  }

  const allPageSelected = displayEntries.length > 0 &&
    displayEntries.every((e) => selectedIds.has(e.id))
  const hasMultiplePages = displayTotalPages > 1
  const selectedBackupCount = selectedBackupPaths.size
  const allVisibleBackupsSelected = displayBackups.length > 0 &&
    displayBackups.every((backup) => selectedBackupPaths.has(backup.path))

  return (
    <div className="app-layout">
      <Sidebar
        onRefresh={handleRefresh}
        filterPresets={filterPresets}
        activeTab={activeTab}
        setActiveTab={setActiveTab}
        backupsLoading={backupsLoading}
        version={guiVersion}
      />

      <main className="main-content">
        {activeTab === 'history' ? (
          <>
            <header className="main-search-area">
              <SearchBar currentQuery={query} onSearch={handleSearch} disabled={!selectedPath || loading} />
            </header>

            {historyResult && (
              <div className="sel-toolbar">
                <div className="sel-left">
                  <span className={`filter-badge${isFilterMode ? ' active' : ''}`} title={isFilterMode ? `${displayTotal.toLocaleString()} matched` : 'No filters'}>
                    <span className="material-symbols-outlined">filter_alt</span>
                    <span className="filter-badge-count">{isFilterMode ? displayTotal.toLocaleString() : '-'}</span>
                  </span>
                  <div className="sel-divider" />
                  <span className={`sel-count${selectedIds.size > 0 ? '' : ' empty'}`}>
                    {selectedIds.size > 0
                      ? `${selectedIds.size.toLocaleString()} SELECTED`
                      : 'NONE SELECTED'}
                  </span>
                  <div className="sel-divider" />
                  <button className="sel-btn" onClick={() => handleSelectAll(!allPageSelected)}>
                    Page
                  </button>
                  {hasMultiplePages && (
                    <button
                      className="sel-btn sel-btn-gold"
                      onClick={allGlobalSelected ? handleDeselectAll : handleSelectAllGlobal}
                      disabled={loading}
                    >
                      All
                    </button>
                  )}
                  <button className="sel-btn" onClick={handleDeselectAll} disabled={selectedIds.size === 0}>
                    Clear
                  </button>
                </div>
                <div className="sel-right">
                  <button className="sel-pill cyan" onClick={handleExport} disabled={loading || selectedIds.size === 0}>
                    Export
                  </button>
                  <button className="sel-pill red" onClick={handleDelete} disabled={loading || selectedIds.size === 0}>
                    Delete
                  </button>
                </div>
              </div>
            )}

            {initialLoading && !historyResult && !status.message && (
              <div className="loading-indicator startup-loading">
                Detecting browser and loading history...
              </div>
            )}

            {status.message && status.type === 'error' && (
              <div className="status-error-bar">
                <span className="material-symbols-outlined">error</span>
                {status.message.includes('database is locked') || status.message.includes('SQLITE_BUSY')
                  ? `Close ${displayBrowserName} and click Refresh to load history.`
                  : status.message}
              </div>
            )}

            {historyResult && (
              <div className="table-nav">
                <span className="table-nav-total">{displayTotal.toLocaleString()} entries</span>
                <div className="table-nav-controls">
                  <button onClick={() => handlePageChange(displayPage - 1)} disabled={loading || displayPage <= 1}>
                    <span className="material-symbols-outlined">chevron_left</span>
                  </button>
                  <input
                    className="table-nav-input"
                    type="number"
                    min={1}
                    max={displayTotalPages}
                    defaultValue={displayPage}
                    key={displayPage}
                    onBlur={(e) => {
                      const val = parseInt(e.target.value, 10)
                      if (val >= 1 && val <= displayTotalPages && val !== displayPage) handlePageChange(val)
                      else e.target.value = displayPage
                    }}
                    onKeyDown={(e) => { if (e.key === 'Enter') e.target.blur() }}
                    disabled={loading}
                  />
                  <span className="table-nav-of">/ {displayTotalPages}</span>
                  <button onClick={() => handlePageChange(displayPage + 1)} disabled={loading || displayPage >= displayTotalPages}>
                    <span className="material-symbols-outlined">chevron_right</span>
                  </button>
                  <select
                    className="table-nav-size"
                    value={pageSize}
                    onChange={(e) => setPageSize(Number(e.target.value))}
                    disabled={loading}
                  >
                    {[25, 50, 100, 200].map((s) => <option key={s} value={s}>{s} rows</option>)}
                  </select>
                </div>
              </div>
            )}

            <div className={`table-area${loading ? ' loading-fade' : ''}`}>
              {loading && (
                <div className="loading-overlay">
                  <span className="material-symbols-outlined spinning">sync</span>
                </div>
              )}
              <HistoryTable
                entries={displayEntries}
                selectedIds={selectedIds}
                onToggleSelect={handleToggleSelect}
                onSelectAll={handleSelectAll}
              />
            </div>
          </>
        ) : (
          <>
            <header className="main-search-area">
              <SearchBar
                currentQuery={backupQuery}
                onSearch={handleBackupSearch}
                disabled={!selectedPath || backupsLoading}
                placeholder="Search backup file names... (Enter to search)"
                ariaLabel="Search backups"
              />
            </header>

            <div className="sel-toolbar">
              <div className="sel-left">
                <span className={`sel-count${selectedBackupCount > 0 ? '' : ' empty'}`}>
                  {selectedBackupCount > 0
                    ? `${selectedBackupCount.toLocaleString()} SELECTED`
                    : 'NONE SELECTED'}
                </span>
                <div className="sel-divider" />
                <span className="backup-toolbar-note">
                  {backupsLoading
                    ? 'SCANNING BACKUPS'
                    : `${filteredBackups.length.toLocaleString()} MATCHES`}
                </span>
                <div className="sel-divider" />
                <button className="sel-btn" onClick={() => handleSelectAllBackups(!allVisibleBackupsSelected)}>
                  Page
                </button>
                <button className="sel-btn" onClick={handleClearBackupSelection} disabled={selectedBackupCount === 0}>
                  Clear
                </button>
              </div>
              <div className="sel-right">
                <button
                  className="sel-pill cyan"
                  onClick={handleRestoreSelectedBackup}
                  disabled={backupsLoading || !canRestoreBackup(selectedBackupPaths)}
                >
                  Restore
                </button>
                <button
                  className="sel-pill red"
                  onClick={handleDeleteBackups}
                  disabled={backupsLoading || !canDeleteBackups(selectedBackupPaths)}
                >
                  Delete
                </button>
              </div>
            </div>

            {backupStatus.message && backupStatus.type === 'error' && (
              <div className="status-error-bar">
                <span className="material-symbols-outlined">error</span>
                {backupStatus.message}
              </div>
            )}

            <div className="table-nav">
              <span className="table-nav-total">{filteredBackups.length.toLocaleString()} backups</span>
              <div className="table-nav-controls">
                <button onClick={() => setBackupPage(clampedBackupPage - 1)} disabled={backupsLoading || clampedBackupPage <= 1}>
                  <span className="material-symbols-outlined">chevron_left</span>
                </button>
                <input
                  className="table-nav-input"
                  type="number"
                  min={1}
                  max={backupTotalPages}
                  defaultValue={clampedBackupPage}
                  key={`backup-${clampedBackupPage}`}
                  onBlur={(e) => {
                    const val = parseInt(e.target.value, 10)
                    if (val >= 1 && val <= backupTotalPages && val !== clampedBackupPage) setBackupPage(val)
                    else e.target.value = clampedBackupPage
                  }}
                  onKeyDown={(e) => { if (e.key === 'Enter') e.target.blur() }}
                  disabled={backupsLoading}
                />
                <span className="table-nav-of">/ {backupTotalPages}</span>
                <button onClick={() => setBackupPage(clampedBackupPage + 1)} disabled={backupsLoading || clampedBackupPage >= backupTotalPages}>
                  <span className="material-symbols-outlined">chevron_right</span>
                </button>
                <select
                  className="table-nav-size"
                  value={backupPageSize}
                  onChange={(e) => {
                    setBackupPageSize(Number(e.target.value))
                    setBackupPage(1)
                  }}
                  disabled={backupsLoading}
                >
                  {[25, 50, 100, 200].map((s) => <option key={s} value={s}>{s} rows</option>)}
                </select>
              </div>
            </div>

            <div className={`table-area${backupsLoading ? ' loading-fade' : ''}`}>
              {backupsLoading && (
                <div className="loading-overlay">
                  <span className="material-symbols-outlined spinning">sync</span>
                </div>
              )}
              <BackupsTable
                backups={displayBackups}
                selectedBackupPaths={selectedBackupPaths}
                onToggleSelect={handleToggleBackupSelect}
                onSelectAll={handleSelectAllBackups}
                emptyMessage={
                  backupQuery.trim()
                    ? 'No backups match the current file-name search.'
                    : 'No backups found for the selected History database.'
                }
              />
            </div>
          </>
        )}
      </main>

      <ConfirmModal
        isOpen={confirmModal.isOpen}
        title={confirmModal.title}
        message={confirmModal.message}
        danger={confirmModal.danger}
        onConfirm={confirmModal.onConfirm}
        onCancel={closeModal}
      />
    </div>
  )
}

export default function App() {
  return (
    <AppProvider>
      <AppContent />
    </AppProvider>
  )
}
