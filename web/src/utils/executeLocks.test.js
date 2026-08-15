import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { executeControlLocks } from './executeLocks.js'

describe('executeControlLocks', () => {
  it('does not freeze Refresh or Cleanup while a run is stuck running', () => {
    const l = executeControlLocks({ status: 'running', canAdmin: true, cleaning: false })
    assert.equal(l.refresh, false)
    assert.equal(l.cleanup, false)
    assert.equal(l.execute, true)
    assert.equal(l.cancel, false)
    assert.equal(l.maxPodsRead, false)
    assert.equal(l.maxPodsWrite, true)
    assert.equal(l.template, true)
  })

  it('locks cleanup only while a cleanup is in flight', () => {
    const l = executeControlLocks({ status: 'failed', canAdmin: true, cleaning: true })
    assert.equal(l.cleanup, true)
    assert.equal(l.refresh, true)
  })

  it('guest cannot mutate; can still refresh', () => {
    const l = executeControlLocks({ status: 'running', canAdmin: false, cleaning: false })
    assert.equal(l.refresh, false)
    assert.equal(l.cleanup, true)
    assert.equal(l.cancel, true)
    assert.equal(l.execute, true)
  })

  it('idle admin can execute and cleanup', () => {
    const l = executeControlLocks({ status: 'idle', canAdmin: true, cleaning: false })
    assert.equal(l.execute, false)
    assert.equal(l.cleanup, false)
    assert.equal(l.cancel, true)
    assert.equal(l.refresh, false)
  })
})
