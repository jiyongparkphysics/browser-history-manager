export function applyPageSelection(selectedIds, entries, select) {
  const next = new Set(selectedIds)
  for (const entry of entries) {
    if (select) next.add(entry.id)
    else next.delete(entry.id)
  }
  return next
}

export function shouldKeepAllGlobalSelected(allGlobalSelected, selectedIds) {
  return allGlobalSelected && selectedIds.size > 0
}
