export function pad2(value) {
  return String(value).padStart(2, '0')
}

export function formatLocalDateTimeParts(date) {
  if (!(date instanceof Date) || Number.isNaN(date.getTime())) {
    return ''
  }

  return [
    date.getFullYear(),
    '-',
    pad2(date.getMonth() + 1),
    '-',
    pad2(date.getDate()),
    ' ',
    pad2(date.getHours()),
    ':',
    pad2(date.getMinutes()),
    ':',
    pad2(date.getSeconds()),
  ].join('')
}

export function formatChromeTime(chromeTimestamp) {
  if (!chromeTimestamp) return ''
  const chromeEpochOffset = 11644473600000
  const ms = chromeTimestamp / 1000 - chromeEpochOffset
  return formatLocalDateTimeParts(new Date(ms))
}

export function formatUnixSeconds(unixSeconds) {
  if (!unixSeconds) return ''
  return formatLocalDateTimeParts(new Date(unixSeconds * 1000))
}
