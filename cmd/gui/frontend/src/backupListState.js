export function filterBackupsByFileName(backups, query) {
  if (!Array.isArray(backups) || backups.length === 0) return []
  const needle = String(query || '').trim().toLowerCase()
  if (!needle) return backups
  return backups.filter((backup) => String(backup.fileName || '').toLowerCase().includes(needle))
}
