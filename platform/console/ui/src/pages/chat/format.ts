/** "4s" / "2m" / "1.0h" / "1.0d" — the prototype's own time column, read nowhere else. */
export function relativeAge(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)}h`
  return `${(seconds / 86400).toFixed(1)}d`
}

/** "410ms" / "1.2s" — a tool call's own duration column. */
export function formatDuration(ms?: number): string {
  if (ms === undefined) return '—'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}

/** "512 B" / "1.2 KB" / "3.4 MB" — a tool call's own result-size column. */
export function formatBytes(bytes?: number): string {
  if (bytes === undefined) return '—'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(1)} ${units[unit]}`
}
