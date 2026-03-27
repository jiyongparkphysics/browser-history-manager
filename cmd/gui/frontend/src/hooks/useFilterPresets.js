import { useState, useCallback } from 'react'

const STORAGE_KEY = 'history-manager-filter-presets'

const EMPTY_PRESETS = { include: [], exclude: [] }

function loadPresets() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (!stored) return EMPTY_PRESETS
    const parsed = JSON.parse(stored)
    const migrate = (arr) => (arr || []).map((p) =>
      typeof p === 'string' ? { id: `kw-${Date.now()}`, keyword: p, enabled: true }
        : { ...p, enabled: p.enabled !== false }
    )
    return {
      include: migrate(parsed.include || parsed.match || parsed.whitelist),
      exclude: migrate(parsed.exclude || parsed.protect || parsed.blacklist),
    }
  } catch {
    return EMPTY_PRESETS
  }
}

function savePresets(presets) {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(presets)) } catch {}
}

let idCounter = Date.now()
function nextId() { return `kw-${idCounter++}` }

export default function useFilterPresets() {
  const [presets, setPresets] = useState(loadPresets)

  const addKeyword = useCallback((type, keyword) => {
    const kw = keyword.trim().toLowerCase()
    if (!kw) return
    setPresets((prev) => {
      if (prev[type].some((p) => p.keyword === kw)) return prev
      const updated = { ...prev, [type]: [...prev[type], { id: nextId(), keyword: kw, enabled: true }] }
      savePresets(updated)
      return updated
    })
  }, [])

  const removeKeyword = useCallback((type, id) => {
    setPresets((prev) => {
      const updated = { ...prev, [type]: prev[type].filter((p) => p.id !== id) }
      savePresets(updated)
      return updated
    })
  }, [])

  const toggleKeyword = useCallback((type, id) => {
    setPresets((prev) => {
      const updated = {
        ...prev,
        [type]: prev[type].map((p) => p.id === id ? { ...p, enabled: !p.enabled } : p),
      }
      savePresets(updated)
      return updated
    })
  }, [])

  return { presets, addKeyword, removeKeyword, toggleKeyword }
}
