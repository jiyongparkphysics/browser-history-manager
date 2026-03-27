export function canDeleteBackups(selectedBackupPaths) {
  return selectedBackupPaths.size > 0
}

export function canRestoreBackup(selectedBackupPaths) {
  return selectedBackupPaths.size === 1
}

export function toggleBackupSelection(selectedBackupPaths, backupPath) {
  const next = new Set(selectedBackupPaths)
  if (next.has(backupPath)) next.delete(backupPath)
  else next.add(backupPath)
  return next
}

export function clearBackupSelection() {
  return new Set()
}
