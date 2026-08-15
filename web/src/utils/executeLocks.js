/** Button lock policy for Execute. A stuck "running" job must not freeze Refresh/Cleanup. */

export function executeControlLocks({ status, canAdmin, cleaning }) {
  const running = status === 'running'
  const admin = !!canAdmin
  return {
    running,
    template: running,
    refresh: !!cleaning,
    execute: running || !admin,
    cancel: !running || !admin,
    cleanup: !!cleaning || !admin,
    maxPodsRead: !!cleaning || !admin,
    maxPodsWrite: running || !!cleaning || !admin,
  }
}
