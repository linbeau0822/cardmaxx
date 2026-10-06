import { useEffect, useState } from 'react'

import { api } from '../lib/api'

type Status = 'checking' | 'ok' | 'unreachable'

/**
 * Live indicator for the backend connection.
 *
 * This is the one place the skeleton proves the React → REST → Go path end to
 * end, which is why it calls the real /health endpoint instead of being faked.
 */
export function ApiStatus() {
  const [status, setStatus] = useState<Status>('checking')

  useEffect(() => {
    let active = true
    api
      .health()
      .then((health) => {
        if (active) setStatus(health.status === 'ok' ? 'ok' : 'unreachable')
      })
      .catch(() => {
        if (active) setStatus('unreachable')
      })
    // Ignore a resolved request after the component is gone.
    return () => {
      active = false
    }
  }, [])

  const label: Record<Status, string> = {
    checking: 'Checking API…',
    ok: 'API connected',
    unreachable: 'API unreachable',
  }
  const dot: Record<Status, string> = {
    checking: 'bg-ink-muted',
    ok: 'bg-accent',
    unreachable: 'bg-red-400',
  }

  return (
    <span className="inline-flex items-center gap-2 text-sm text-ink-muted">
      <span className={`size-2 rounded-full ${dot[status]}`} aria-hidden="true" />
      {label[status]}
    </span>
  )
}
