import React, { useRef, useState, useCallback, useEffect, useMemo } from 'react';
import { formatChromeTime } from '../datetime.js';

/**
 * Row height in pixels - must match the CSS for .vrow.
 */
const ROW_HEIGHT = 36;

/**
 * Number of extra rows to render above/below the visible area (buffer).
 */
const OVERSCAN = 8;

/**
 * Default visible height for the virtual scroll container (px).
 * The container will also respond to the CSS max-height / height.
 */
const DEFAULT_VISIBLE_HEIGHT = 600;

/**
 * Truncate a string to maxLen characters, appending '...' if truncated.
 */
function truncate(s, maxLen) {
  if (!s) return '';
  if (s.length <= maxLen) return s;
  return s.slice(0, maxLen) + '...';
}

/**
 * VirtualRow renders a single history entry row positioned absolutely.
 * Supports click-to-select while preserving link and checkbox behaviour.
 */
const VirtualRow = React.memo(function VirtualRow({ entry, index, isSelected, onToggle, onRowClick }) {
  const handleClick = useCallback(
    (e) => {
      // Don't intercept clicks on links or the checkbox itself.
      if (e.target.tagName === 'A' || e.target.tagName === 'INPUT') return;
      onRowClick(index);
    },
    [index, onRowClick]
  );

  return (
    <div
      className={`vrow${isSelected ? ' selected' : ''}`}
      style={{ top: index * ROW_HEIGHT }}
      role="row"
      aria-rowindex={index + 2} /* +2: 1-based, skip header */
      aria-selected={isSelected}
      onClick={handleClick}
    >
      <div className="vcell col-check" role="gridcell">
        <input
          type="checkbox"
          checked={isSelected}
          onChange={() => onToggle(entry.id)}
          aria-label={`Select ${entry.title || entry.url}`}
        />
      </div>
      <div className="vcell col-title" role="gridcell" title={entry.title}>
        {truncate(entry.title, 60)}
      </div>
      <div className="vcell col-url" role="gridcell" title={entry.url}>
        <a href={entry.url} target="_blank" rel="noopener noreferrer">
          {truncate(entry.url, 80)}
        </a>
      </div>
      <div className="vcell col-visits" role="gridcell">
        {entry.visitCount}
      </div>
      <div className="vcell col-time" role="gridcell">
        {formatChromeTime(entry.lastVisitTime)}
      </div>
    </div>
  );
});

/**
 * HistoryTable renders a virtualized table of history entries with
 * single-click and select-all support.
 *
 * Only rows within the visible viewport (plus an overscan buffer) are mounted
 * in the DOM, enabling smooth scrolling for pages with many entries.
 *
 * Props:
 *   entries        - array of history entry objects
 *   selectedIds    - Set of selected entry IDs
 *   onToggleSelect - callback(id) to toggle a single entry
 *   onSelectAll    - callback(boolean) to select/deselect all
 */
export default function HistoryTable({ entries, selectedIds, onToggleSelect, onSelectAll, compact = false }) {
  const scrollRef = useRef(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(DEFAULT_VISIBLE_HEIGHT);

  // Reset scroll position when entries change (e.g. page change / new search).
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = 0;
      setScrollTop(0);
    }
  }, [entries]);

  // Observe container height so the virtualisation adapts to CSS-driven sizing.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const updateHeight = () => {
      const h = el.clientHeight;
      if (h > 0) setViewportHeight(h);
    };

    updateHeight();

    if (typeof ResizeObserver !== 'undefined') {
      const ro = new ResizeObserver(updateHeight);
      ro.observe(el);
      return () => ro.disconnect();
    }
  }, []);

  const handleScroll = useCallback((e) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  const handleRowClick = useCallback(
    (index) => {
      onToggleSelect(entries[index].id);
    },
    [entries, onToggleSelect]
  );

  const totalHeight = entries.length * ROW_HEIGHT;
  const startIdx = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const endIdx = Math.min(
    entries.length,
    Math.ceil((scrollTop + viewportHeight) / ROW_HEIGHT) + OVERSCAN
  );

  const visibleRows = useMemo(() => {
    const rows = [];
    for (let i = startIdx; i < endIdx; i++) {
      const entry = entries[i];
      rows.push(
        <VirtualRow
          key={entry.id}
          entry={entry}
          index={i}
          isSelected={selectedIds.has(entry.id)}
          onToggle={onToggleSelect}
          onRowClick={handleRowClick}
        />
      );
    }
    return rows;
  }, [entries, startIdx, endIdx, selectedIds, onToggleSelect, handleRowClick]);

  if (!entries || entries.length === 0) {
    return <p className="empty-message">No entries to display.</p>;
  }

  const allSelected = entries.length > 0 && entries.every((e) => selectedIds.has(e.id));
  const someSelected = selectedIds.size > 0 && !allSelected;

  return (
    <div className={`history-table-wrapper${compact ? ' compact' : ''}`} role="grid" aria-label="History entries">
      <div className="vtable-header" role="row">
        <div className="vcell col-check" role="columnheader">
          <input
            type="checkbox"
            checked={allSelected}
            ref={(el) => {
              if (el) el.indeterminate = someSelected;
            }}
            onChange={() => onSelectAll(!allSelected)}
            title={allSelected ? 'Deselect all on this page' : 'Select all on this page'}
            aria-label="Select all entries on this page"
          />
        </div>
        <div className="vcell col-title" role="columnheader">Page Title</div>
        <div className="vcell col-url" role="columnheader">URL</div>
        <div className="vcell col-visits" role="columnheader">Visits</div>
        <div className="vcell col-time" role="columnheader">Last Visit</div>
      </div>

      <div className="vtable-scroll" ref={scrollRef} onScroll={handleScroll}>
        <div className="vtable-body" style={{ height: totalHeight }}>
          {visibleRows}
        </div>
      </div>
    </div>
  );
}
