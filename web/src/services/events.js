export const eventsPath = '/api/v1/events'

export function openLiveStream({ after = 0, onLog, onRun, onCleanup, onOvn } = {}) {
  const q = new URLSearchParams()
  if (after) q.set('after', String(after))
  const url = q.toString() ? `${eventsPath}?${q}` : eventsPath
  const es = new EventSource(url, { withCredentials: true })
  const wrap = (fn) => (e) => {
    if (!fn) return
    try {
      fn(JSON.parse(e.data))
    } catch {
      /* ignore malformed frames */
    }
  }
  es.addEventListener('log', wrap(onLog))
  es.addEventListener('run', wrap(onRun))
  es.addEventListener('cleanup', wrap(onCleanup))
  es.addEventListener('ovn', wrap(onOvn))
  return es
}
