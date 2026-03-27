import React, { useState } from 'react'
import toast from 'react-hot-toast'
import { OpenDBFolder } from '../../wailsjs/go/main/App'
import { useAppContext } from '../../context/AppContext'

export default function Sidebar({
  onRefresh,
  filterPresets,
  activeTab,
  setActiveTab,
  backupsLoading,
  version,
}) {
  const {
    selectedPath,
    browserName,
    browsersWithProfiles,
    browsers,
    handleBrowserChange,
    loading,
  } = useAppContext()

  const displayBrowserName = browserName
    ? browserName.charAt(0).toUpperCase() + browserName.slice(1)
    : 'the browser'

  const { presets, addKeyword, removeKeyword, toggleKeyword } = filterPresets
  const [showHelp, setShowHelp] = useState(false)
  const [addingTo, setAddingTo] = useState(null)
  const [newKeyword, setNewKeyword] = useState('')

  const friendlyError = (err) => {
    const msg = String(err)
    if (msg.includes('database is locked') || msg.includes('SQLITE_BUSY')) {
      return `Close ${displayBrowserName} first, then try again.`
    }
    return msg
  }

  const formatBrowserName = (name) => {
    if (!name) return 'Unknown'
    return name.charAt(0).toUpperCase() + name.slice(1)
  }

  const normPath = (p) => (p || '').replace(/\\/g, '/').toLowerCase()

  const handleAdd = (type) => {
    if (!newKeyword.trim()) return
    addKeyword(type, newKeyword)
    setNewKeyword('')
    setAddingTo(null)
  }

  const handleOpenDBFolder = async () => {
    if (!selectedPath) return
    try {
      await OpenDBFolder(selectedPath)
      toast.success('Opened DB folder. Backups are created here.')
    } catch (err) {
      toast.error(friendlyError(err))
    }
  }

  const useGrouped = browsersWithProfiles && browsersWithProfiles.length > 0
  const browserOptions = []
  if (useGrouped) {
    for (const browser of browsersWithProfiles) {
      for (const profile of browser.profiles) {
        browserOptions.push({
          label: browser.profiles.length > 1
            ? `${formatBrowserName(browser.name)} - ${profile.name}`
            : formatBrowserName(browser.name),
          value: profile.dbPath,
        })
      }
    }
  } else if (browsers) {
    for (const b of browsers) {
      browserOptions.push({ label: formatBrowserName(b.name), value: b.path })
    }
  }

  const renderKeywordGroup = (type, title, hint, colorClass) => (
    <div className="sb-filter-group">
      <div className="sb-filter-header">
        <div>
          <span className={`sb-filter-label ${colorClass}`}>{title}</span>
          <span className="sb-filter-hint">{hint}</span>
        </div>
        <button
          className="sb-filter-add"
          onClick={() => {
            setAddingTo(addingTo === type ? null : type)
            setNewKeyword('')
          }}
          title="Add keyword"
        >
          <span className="material-symbols-outlined">
            {addingTo === type ? 'close' : 'add_circle'}
          </span>
        </button>
      </div>

      {presets[type].length > 0 ? (
        <div className="sb-chips">
          {presets[type].map((item) => (
            <span
              key={item.id}
              className={`sb-chip ${colorClass}${item.enabled ? ' enabled' : ' disabled'}`}
              onClick={() => toggleKeyword(type, item.id)}
              title={item.enabled ? `Active: ${item.keyword} (click to disable)` : `Disabled: ${item.keyword} (click to enable)`}
            >
              {item.keyword}
              <span
                className="material-symbols-outlined sb-chip-remove"
                onClick={(e) => {
                  e.stopPropagation()
                  removeKeyword(type, item.id)
                }}
                title="Remove"
              >
                close
              </span>
            </span>
          ))}
        </div>
      ) : (
        <span className="sb-empty-hint">No keywords added</span>
      )}

      {addingTo === type && (
        <div className="sb-add-form">
          <input
            type="text"
            placeholder="e.g. example.com, project name, keyword"
            value={newKeyword}
            onChange={(e) => setNewKeyword(e.target.value)}
            className="sb-add-input"
            autoFocus
            onKeyDown={(e) => e.key === 'Enter' && handleAdd(type)}
          />
          <div className="sb-add-actions">
            <button className="sb-add-save" onClick={() => handleAdd(type)} disabled={!newKeyword.trim()}>
              Add
            </button>
            <button
              className="sb-add-cancel"
              onClick={() => {
                setAddingTo(null)
                setNewKeyword('')
              }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  )

  const renderHelpModal = () => (
    <div className="help-modal-overlay" onClick={() => setShowHelp(false)}>
      <div className="help-modal" onClick={(e) => e.stopPropagation()}>
        <div className="help-modal-header">
          <div>
            <h2>Browser History Manager</h2>
            <div className="help-modal-version">Version {version || 'dev'}</div>
          </div>
          <button className="help-modal-close" onClick={() => setShowHelp(false)}>
            <span className="material-symbols-outlined">close</span>
          </button>
        </div>
        <div className="help-modal-body">
          <section className="help-group">
            <h3>History</h3>
            <ul>
              <li><strong>Click</strong> a row to select or deselect it.</li>
              <li><strong>Page</strong> selects or deselects all entries on the current page.</li>
              <li><strong>All</strong> selects every entry across all pages.</li>
              <li><strong>Clear</strong> deselects everything.</li>
            </ul>
          </section>
          <section className="help-group">
            <h3>Filters</h3>
            <ul>
              <li><strong>Include</strong> shows only entries whose title or URL matches these keywords.</li>
              <li><strong>Exclude</strong> hides entries whose title or URL matches these keywords.</li>
              <li>Click a keyword chip to enable or disable it. Use the close icon to remove it.</li>
            </ul>
          </section>
          <section className="help-group">
            <h3>Backups</h3>
            <ul>
              <li><strong>Restore</strong> replaces the active <code>History</code> database after creating a fresh safety backup next to it.</li>
              <li><strong>Delete</strong> removes the chosen backup files and any matching <code>-wal</code>/<code>-shm</code> sidecars.</li>
              <li>Restore requires exactly one selected backup. Delete works with one or more selected backups.</li>
            </ul>
          </section>
          <section className="help-group">
            <h3>Browser</h3>
            <ul>
              <li>Use <strong>Open DB folder</strong> to reveal the active <code>History</code> file and its backups in Explorer.</li>
              <li>The app lists whichever supported browser targets are actually found on this system.</li>
            </ul>
          </section>
        </div>
      </div>
    </div>
  )

  return (
    <aside className="sidebar">
      <div className="sb-logo">
        <h1 className="sb-logo-text">Browser History Manager</h1>
      </div>

      <nav className="sb-nav">
        <section className="sb-tab-section">
          <div className="sb-tab-row">
            <div className="sb-tabs" role="tablist" aria-label="Sidebar views">
              <button
                className={`sb-tab${activeTab === 'history' ? ' active' : ''}`}
                onClick={() => setActiveTab('history')}
                role="tab"
                aria-selected={activeTab === 'history'}
                title="History"
              >
                <span className="material-symbols-outlined">history</span>
                <span>History</span>
              </button>
              <button
                className={`sb-tab${activeTab === 'backups' ? ' active' : ''}`}
                onClick={() => setActiveTab('backups')}
                role="tab"
                aria-selected={activeTab === 'backups'}
                title="Backups"
              >
                <span className="material-symbols-outlined">inventory_2</span>
                <span>Backups</span>
              </button>
            </div>
            <div className="sb-section-actions">
              <button className="sb-icon-btn" onClick={() => setShowHelp(true)} title="Help">
                <span className="material-symbols-outlined">help</span>
              </button>
            </div>
          </div>
          <div className="sb-section-bar" />
        </section>

        {activeTab === 'history' ? (
          <section className="sb-section">
            <div className="sb-section-title muted">
              <span className="material-symbols-outlined">filter_alt</span>
              <span>Filters</span>
            </div>
            <div className="sb-filters-body">
              {renderKeywordGroup('include', 'Include', 'Show only matching entries', 'tertiary-tint')}
              {renderKeywordGroup('exclude', 'Exclude', 'Hide matching entries', 'error-tint')}
            </div>
          </section>
        ) : null}
      </nav>

      <div className="sb-browser">
        <div className="sb-browser-label-row">
          <span className="sb-browser-section-label">Browser</span>
          <div className="sb-browser-tools">
            <button
              className="sb-icon-btn"
              onClick={() => onRefresh()}
              disabled={loading || backupsLoading}
              title="Refresh current view"
              aria-label="Refresh current view"
            >
              <span className={`material-symbols-outlined${loading || backupsLoading ? ' spinning' : ''}`}>refresh</span>
            </button>
            <button
              className="sb-icon-btn"
              onClick={handleOpenDBFolder}
              disabled={!selectedPath}
              title="Open DB folder"
              aria-label="Open DB folder"
            >
              <span className="material-symbols-outlined">folder_open</span>
            </button>
          </div>
        </div>
        {browserOptions.length > 0 ? (
          <div className="sb-browser-list">
            {browserOptions.map((opt) => (
              <button
                key={opt.value}
                className={`sb-browser-item${normPath(opt.value) === normPath(selectedPath) ? ' active' : ''}`}
                onClick={() => handleBrowserChange(opt.value)}
              >
                <div className="sb-browser-item-icon">
                  <span className="material-symbols-outlined">language</span>
                </div>
                <div className="sb-browser-item-info">
                  <span className="sb-browser-item-name">{opt.label}</span>
                </div>
                {normPath(opt.value) === normPath(selectedPath) && (
                  <span className="material-symbols-outlined sb-browser-check">check_circle</span>
                )}
              </button>
            ))}
          </div>
        ) : (
          <div className="sb-browser-empty">
            <span className="material-symbols-outlined">search_off</span>
            No browsers detected
          </div>
        )}
      </div>

      {showHelp && renderHelpModal()}
    </aside>
  )
}
