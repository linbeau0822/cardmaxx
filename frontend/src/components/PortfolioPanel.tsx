import { useEffect, useState } from 'react'

import { ApiError, api, type PortfolioOption } from '../lib/api'
import { Panel } from './Panel'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })

/** Names a portfolio option by what it adds over the previous size's cards. */
function describeAddition(option: PortfolioOption, previous: PortfolioOption | undefined): string {
  const previousIds = new Set(previous?.cards.map((c) => c.id) ?? [])
  const added = option.cards.filter((c) => !previousIds.has(c.id))
  if (!previous || added.length === option.cards.length) {
    return added.map((c) => `${c.issuer} ${c.name}`).join(', ')
  }
  return `+ ${added.map((c) => `${c.issuer} ${c.name}`).join(', ')}`
}

/**
 * The best combination of catalogue cards at every portfolio size, from one
 * card up, so the user can see exactly where adding another card stops
 * earning more than its own annual fee. Refetches whenever `refreshKey`
 * changes, since linking a new card can change the best combination.
 */
export function PortfolioPanel({ refreshKey }: { refreshKey: number }) {
  const [options, setOptions] = useState<PortfolioOption[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .optimizePortfolio()
      .then(({ options }) => {
        if (!cancelled) setOptions(options)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load portfolio options.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  const bestIndex = options.reduce((best, o, i) => (o.net_value_cents > options[best].net_value_cents ? i : best), 0)

  const body = loading ? (
    <p className="text-sm text-ink-muted">Loading…</p>
  ) : error ? (
    <p className="text-sm text-ink-muted">{error}</p>
  ) : options.length === 0 ? (
    <p className="text-sm text-ink-muted">No cards in the catalogue to combine.</p>
  ) : (
    <ul className="divide-y divide-white/5">
      {options.map((o, i) => (
        <li key={o.cards.map((c) => c.id).join(',')} className="-mx-2 rounded px-2 py-4 transition-colors hover:bg-white/5">
          <div className="flex items-baseline justify-between gap-2">
            <span className="text-sm font-medium text-ink">
              {o.cards.length} card{o.cards.length > 1 ? 's' : ''}
              {i === bestIndex && <span className="ml-2 text-xs font-normal text-accent">best</span>}
            </span>
            <span className="flex-none text-sm font-medium text-ink">{currency.format(o.net_value_cents / 100)}/yr net</span>
          </div>
          <p className="mt-0.5 text-xs text-ink-muted">{describeAddition(o, options[i - 1])}</p>
          {i > 0 && (
            <p className={`mt-0.5 text-xs ${o.marginal_value_cents < 0 ? 'text-ink-muted' : 'text-accent'}`}>
              {o.marginal_value_cents >= 0 ? '+' : ''}
              {currency.format(o.marginal_value_cents / 100)}/yr over {i} card{i > 1 ? 's' : ''}
              {o.marginal_value_cents < 0 && ' — not worth the extra fee'}
            </p>
          )}
        </li>
      ))}
    </ul>
  )

  return (
    <Panel title="Best Portfolio" subtitle="The combination of catalogue cards that earns the most, by how many you'd carry.">
      {body}
    </Panel>
  )
}
