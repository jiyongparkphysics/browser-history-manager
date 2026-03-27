import { createContext, useState, useContext, useCallback, useEffect, useMemo } from 'react'
import {
  AutoDetectBrowser,
  DetectBrowsers,
  ListBrowsersWithProfiles,
  SearchHistory,
  GetDeletionQueueSize,
  ClearDeletionQueue,
} from '../wailsjs/go/main/App'

const DEFAULT_PAGE_SIZE = 50

const AppContext = createContext()

export function AppProvider({ children }) {
  const [activeTab, setActiveTab] = useState('history')
  const [selectedPath, setSelectedPath] = useState('')
  const [browserName, setBrowserName] = useState('')
  const [browsers, setBrowsers] = useState([])
  const [browsersWithProfiles, setBrowsersWithProfiles] = useState([])
  const [autoDetectedPath, setAutoDetectedPath] = useState('')
  const [browsersLoading, setBrowsersLoading] = useState(true)
  const [deletionQueueSize, setDeletionQueueSize] = useState(0)
  const [loading, setLoading] = useState(false)
  const [initialLoading, setInitialLoading] = useState(true)

  // History state
  const [historyResult, setHistoryResult] = useState(null)
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [selectedIds, setSelectedIds] = useState(new Set())

  // Status message
  const [status, setStatus] = useState({ message: '', type: 'info' })

  // Detect browsers and auto-load history on first mount.
  useEffect(() => {
    let cancelled = false

    async function initStartup() {
      const [listResult, profileResult, autoResult] = await Promise.allSettled([
        DetectBrowsers(),
        ListBrowsersWithProfiles(),
        AutoDetectBrowser(),
      ])

      if (cancelled) return

      if (listResult.status === 'fulfilled') {
        const sorted = (listResult.value || []).sort((a, b) =>
          a.name.localeCompare(b.name)
        )
        setBrowsers(sorted)
      }

      if (profileResult.status === 'fulfilled') {
        const sorted = (profileResult.value || []).sort((a, b) =>
          a.name.localeCompare(b.name)
        )
        setBrowsersWithProfiles(sorted)
      }

      setBrowsersLoading(false)

      if (autoResult.status === 'fulfilled' && autoResult.value) {
        const detected = autoResult.value
        setSelectedPath(detected.dbPath)
        setAutoDetectedPath(detected.dbPath)
        setBrowserName(detected.name || '')
        setStatus({
          message: `Auto-detected ${detected.name} browser. Loading history...`,
          type: 'info',
        })
      } else if (profileResult.status === 'fulfilled' && profileResult.value?.length > 0) {
        const sorted = (profileResult.value || []).sort((a, b) =>
          a.name.localeCompare(b.name)
        )
        const chrome = sorted.find((b) => b.name === 'chrome')
        const fallback = chrome || sorted[0]
        const fallbackPath = fallback.profiles[0]?.dbPath
        if (fallbackPath) {
          setSelectedPath(fallbackPath)
          setBrowserName(fallback.name || '')
          if (autoResult.status === 'rejected') {
            setStatus({
              message: `Auto-detection failed, using ${fallback.name}. ${autoResult.reason}`,
              type: 'info',
            })
          }
        }
      } else if (listResult.status === 'fulfilled' && listResult.value?.length > 0) {
        const sorted = (listResult.value || []).sort((a, b) =>
          a.name.localeCompare(b.name)
        )
        const chrome = sorted.find((b) => b.name === 'chrome')
        const chosen = chrome || sorted[0]
        setSelectedPath(chosen.path)
        setBrowserName(chosen.name || '')
        if (autoResult.status === 'rejected') {
          setStatus({
            message: `Auto-detection failed, using ${chrome ? 'Chrome' : sorted[0].name}. ${autoResult.reason}`,
            type: 'info',
          })
        }
      } else {
        setInitialLoading(false)
        const errorMsg =
          autoResult.status === 'rejected'
            ? String(autoResult.reason)
            : listResult.status === 'rejected'
            ? String(listResult.reason)
            : 'No Chromium-based browsers found on this system.'
        setStatus({ message: errorMsg, type: 'error' })
      }
    }

    initStartup()
    return () => { cancelled = true }
  }, [])

  // Fetch history helper ??keeps existing data visible during load
  const fetchHistory = useCallback(
    async (searchQuery, searchPage) => {
      if (!selectedPath) return
      setLoading(true)
      // Don't clear historyResult here ??keep old data visible during fetch
      setSelectedIds(new Set())
      try {
        const res = await SearchHistory(selectedPath, searchQuery, searchPage, pageSize)
        setHistoryResult(res)
        setPage(res?.page || searchPage)
      } catch (err) {
        setStatus({ message: `Search failed: ${err}`, type: 'error' })
        // Only clear on error
        setHistoryResult(null)
      } finally {
        setLoading(false)
      }
    },
    [selectedPath, pageSize]
  )

  // Re-fetch when page size changes
  useEffect(() => {
    if (selectedPath && !initialLoading) {
      setPage(1)
      fetchHistory(query, 1)
    }
  }, [pageSize]) // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-fetch when browser path changes ??use path directly to avoid stale closure
  useEffect(() => {
    if (!selectedPath) return
    setPage(1)
    setLoading(true)
    setSelectedIds(new Set())
    SearchHistory(selectedPath, query, 1, pageSize)
      .then((res) => {
        setHistoryResult(res)
        setPage(res?.page || 1)
        setStatus((prev) =>
          prev.message.includes('Loading history') || prev.message.includes('Switching browser')
            ? { message: '', type: 'info' }
            : prev
        )
      })
      .catch((err) => {
        setStatus({ message: `Search failed: ${err}`, type: 'error' })
        setHistoryResult(null)
      })
      .finally(() => {
        setLoading(false)
        setInitialLoading(false)
      })
  }, [selectedPath]) // eslint-disable-line react-hooks/exhaustive-deps

  // Handle browser selection change
  const handleBrowserChange = useCallback(
    (newPath) => {
      if (newPath === selectedPath) return
      ClearDeletionQueue().catch(() => {})
      setSelectedIds(new Set())
      setQuery('')
      setSelectedPath(newPath)

      // Derive browser name from profiles or browsers list
      let name = ''
      for (const b of browsersWithProfiles) {
        for (const p of b.profiles) {
          if (p.dbPath === newPath) { name = b.name; break }
        }
        if (name) break
      }
      if (!name) {
        const found = browsers.find((b) => b.path === newPath)
        if (found) name = found.name
      }
      setBrowserName(name)

      setStatus({
        message: 'Switching browser... Loading history...',
        type: 'info',
      })
    },
    [selectedPath, browsersWithProfiles, browsers]
  )

  // Refresh: re-read DB for all tabs
  const refreshData = useCallback(async () => {
    await fetchHistory(query, page)
    await refreshQueueSize()
  }, [fetchHistory, query, page])

  // Update queue size
  const refreshQueueSize = useCallback(async () => {
    try {
      const size = await GetDeletionQueueSize()
      setDeletionQueueSize(size)
    } catch {
      // queue size unavailable
    }
  }, [])

  const value = useMemo(() => ({
    activeTab, setActiveTab,
    selectedPath, setSelectedPath,
    browserName, setBrowserName,
    browsers, browsersWithProfiles,
    autoDetectedPath,
    browsersLoading,
    deletionQueueSize, refreshQueueSize,
    loading, setLoading,
    initialLoading,
    historyResult, setHistoryResult,
    query, setQuery,
    page, setPage,
    pageSize, setPageSize,
    selectedIds, setSelectedIds,
    status, setStatus,
    fetchHistory,
    handleBrowserChange,
    refreshData,
  }), [
    activeTab, selectedPath, browserName, browsers, browsersWithProfiles,
    autoDetectedPath, browsersLoading, deletionQueueSize, loading, initialLoading,
    historyResult, query, page, pageSize, selectedIds, status,
    fetchHistory, handleBrowserChange, refreshData, refreshQueueSize,
  ])

  return (
    <AppContext.Provider value={value}>
      {children}
    </AppContext.Provider>
  )
}

export function useAppContext() {
  return useContext(AppContext)
}
