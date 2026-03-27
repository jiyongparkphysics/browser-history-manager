export function getTotalPages(totalEntries, pageSize) {
  const safePageSize = Number.isFinite(pageSize) && pageSize > 0 ? Math.trunc(pageSize) : 1
  const safeTotal = Number.isFinite(totalEntries) && totalEntries > 0 ? Math.trunc(totalEntries) : 0
  return Math.max(1, Math.ceil(safeTotal / safePageSize))
}

export function clampPage(page, totalPages) {
  const safeTotalPages = getTotalPages(totalPages, 1)
  const safePage = Number.isFinite(page) ? Math.trunc(page) : 1
  return Math.min(Math.max(safePage, 1), safeTotalPages)
}

export function paginateEntries(entries, page, pageSize) {
  if (!Array.isArray(entries) || entries.length === 0) return []
  const safePageSize = Number.isFinite(pageSize) && pageSize > 0 ? Math.trunc(pageSize) : 1
  const totalPages = getTotalPages(entries.length, safePageSize)
  const safePage = clampPage(page, totalPages)
  const start = (safePage - 1) * safePageSize
  return entries.slice(start, start + safePageSize)
}