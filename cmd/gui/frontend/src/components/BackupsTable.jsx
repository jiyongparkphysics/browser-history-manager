import React from 'react'
import { formatUnixSeconds } from '../datetime.js'

function formatSize(sizeBytes) {
  if (!Number.isFinite(sizeBytes) || sizeBytes < 1024) {
    return `${sizeBytes || 0} B`
  }

  const units = ['KB', 'MB', 'GB', 'TB']
  let value = sizeBytes / 1024
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unitIndex]}`
}

export default function BackupsTable({
  backups,
  selectedBackupPaths,
  onToggleSelect,
  onSelectAll,
  emptyMessage = 'No backups found for the selected History database.',
}) {
  if (!backups || backups.length === 0) {
    return <p className="empty-message">{emptyMessage}</p>
  }

  const allSelected = backups.length > 0 && backups.every((backup) => selectedBackupPaths.has(backup.path))
  const someSelected = selectedBackupPaths.size > 0 && !allSelected

  return (
    <div className="backups-table-wrapper" role="grid" aria-label="Backup snapshots">
      <div className="backups-table-header" role="row">
        <div className="vcell col-check" role="columnheader">
          <input
            type="checkbox"
            checked={allSelected}
            ref={(el) => {
              if (el) el.indeterminate = someSelected
            }}
            onChange={() => onSelectAll(!allSelected)}
            aria-label="Select all backups"
          />
        </div>
        <div className="vcell backup-col-name" role="columnheader">File Name</div>
        <div className="vcell backup-col-created" role="columnheader">Created</div>
        <div className="vcell backup-col-size" role="columnheader">Size</div>
        <div className="vcell backup-col-items" role="columnheader">Items</div>
      </div>

      <div className="backups-table-scroll">
        {backups.map((backup) => {
          const isSelected = selectedBackupPaths.has(backup.path)
          return (
            <div
              key={backup.path}
              className={`backup-row${isSelected ? ' selected' : ''}`}
              role="row"
              aria-selected={isSelected}
              onClick={(e) => {
                if (e.target.tagName === 'INPUT') return
                onToggleSelect(backup.path)
              }}
            >
              <div className="vcell col-check" role="gridcell">
                <input
                  type="checkbox"
                  checked={isSelected}
                  onChange={() => onToggleSelect(backup.path)}
                  aria-label={`Select ${backup.fileName}`}
                />
              </div>
              <div className="vcell backup-col-name" role="gridcell" title={backup.fileName}>
                {backup.fileName}
              </div>
              <div className="vcell backup-col-created" role="gridcell">
                {formatUnixSeconds(backup.createdUnix)}
              </div>
              <div className="vcell backup-col-size" role="gridcell">
                {formatSize(backup.sizeBytes)}
              </div>
              <div className="vcell backup-col-items" role="gridcell">
                {backup.itemCount.toLocaleString()}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
