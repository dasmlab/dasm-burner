/** Operator-facing Prometheus / kube-burner values (units, not scientific notation). */

export function metricLabel(name) {
  const s = String(name || '')
  if (!s) return 'Metric'
  const spaced = s.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/_/g, ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

export function formatMetric(name, value) {
  const v = Number(value)
  if (!Number.isFinite(v)) return '—'
  const key = String(name || '').toLowerCase()
  if (key.includes('memory') || key.includes('bytes')) return formatBytes(v)
  if (key.includes('cpu')) return formatCPU(v)
  if (key.includes('latency') || key.includes('wal') || key.includes('fsync') || key.includes('duration')) {
    return formatSeconds(v)
  }
  if (key.includes('errorrate')) return formatRate(v, 'err/s')
  if (key.includes('requestrate')) return formatRate(v, 'req/s')
  if (key.includes('ready')) {
    if (v >= 0 && v <= 1.05) return `${(v * 100).toFixed(1)}%`
    return String(Math.round(v))
  }
  return formatPlain(v)
}

function formatBytes(v) {
  const n = Math.abs(v)
  if (n >= 1024 ** 3) return `${(v / 1024 ** 3).toFixed(2)} GiB`
  if (n >= 1024 ** 2) return `${Math.round(v / 1024 ** 2)} MiB`
  if (n >= 1024) return `${(v / 1024).toFixed(1)} KiB`
  return `${Math.round(v)} B`
}

function formatCPU(v) {
  if (v < 0.01) return `${(v * 1000).toFixed(2)} mCPU`
  if (v < 1) return `${v.toFixed(3)} CPU`
  return `${v.toFixed(2)} CPU`
}

function formatSeconds(v) {
  if (v < 1) return `${(v * 1000).toFixed(1)} ms`
  return `${v.toFixed(2)} s`
}

function formatRate(v, unit) {
  if (v === 0) return `0 ${unit}`
  if (v >= 100) return `${Math.round(v)} ${unit}`
  return `${v.toFixed(2)} ${unit}`
}

function formatPlain(v) {
  if (v === 0) return '0'
  const a = Math.abs(v)
  if (a >= 100) return v.toFixed(1)
  return v.toFixed(3)
}

/** Split a concatenated Why paragraph (legacy snapshots) into bullets. */
export function splitWhy(why) {
  if (!why) return []
  if (Array.isArray(why)) return why.filter(Boolean)
  const text = String(why).trim()
  if (!text) return []
  if (text.includes('\n')) {
    return text.split('\n').map((s) => s.trim()).filter(Boolean)
  }
  return text
    .split(/(?=OVN\d{3}\b)|(?=Confidence:)/)
    .map((s) => s.trim())
    .filter(Boolean)
}
