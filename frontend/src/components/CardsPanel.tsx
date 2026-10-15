import { useEffect, useState } from 'react'

import { ApiError, api, type Account, type Card } from '../lib/api'
import { Panel } from './Panel'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })

/** Formats basis points as a percent, e.g. 150 -> "1.5%". */
function formatBps(bps: number): string {
  return `${(bps / 100).toFixed(bps % 100 === 0 ? 0 : 1)}%`
}

/** The cards backing the user's linked accounts, with what each one earns. */
export function CardsPanel({ accounts }: { accounts: Account[] }) {
  const [catalog, setCatalog] = useState<Card[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    api
      .listCards()
      .then(({ cards }) => {
        if (!cancelled) setCatalog(cards)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load cards.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const linkedCardIds = new Set(accounts.map((a) => a.credit_card_id).filter((id): id is string => id !== null))
  const cards = catalog.filter((c) => linkedCardIds.has(c.id))

  const body = loading ? (
    <p className="text-sm text-ink-muted">Loading…</p>
  ) : error ? (
    <p className="text-sm text-ink-muted">{error}</p>
  ) : cards.length === 0 ? (
    <p className="text-sm text-ink-muted">None of your linked accounts are matched to a known card yet.</p>
  ) : (
    <ul className="divide-y divide-white/5">
      {cards.map((c) => (
        <li key={c.id} className="py-3">
          <div className="flex items-baseline justify-between gap-2">
            <span className="text-sm font-medium text-ink">{c.name}</span>
            <span className="flex-none text-xs text-ink-muted">
              {c.annual_fee_cents === 0 ? 'No annual fee' : `${currency.format(c.annual_fee_cents / 100)}/yr`}
            </span>
          </div>
          <p className="mt-0.5 text-xs text-ink-muted">
            {c.issuer} · {formatBps(c.base_reward_rate_bps)} base
            {c.reward_categories.length > 0 && (
              <>
                {' · '}
                {c.reward_categories
                  .map((rc) => `${formatBps(rc.reward_rate_bps)} ${rc.category_name}`)
                  .join(', ')}
              </>
            )}
          </p>
        </li>
      ))}
    </ul>
  )

  return (
    <Panel title="Your Cards" subtitle="The cards in your wallet and what each one earns you.">
      {body}
    </Panel>
  )
}
