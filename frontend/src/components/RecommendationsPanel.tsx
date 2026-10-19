import { useEffect, useState } from 'react'

import { ApiError, api, type CardRecommendation } from '../lib/api'
import { Panel } from './Panel'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })

/** Formats basis points as a percent, e.g. 150 -> "1.5%". */
function formatBps(bps: number): string {
  return `${(bps / 100).toFixed(bps % 100 === 0 ? 0 : 1)}%`
}

/**
 * Catalogue cards the user doesn't hold yet, ranked by the extra annual
 * value each would add on top of the cards they already have, net of its
 * annual fee. Refetches whenever `refreshKey` changes, since linking a new
 * card can change who already wins each spend category.
 */
export function RecommendationsPanel({ refreshKey }: { refreshKey: number }) {
  const [recommendations, setRecommendations] = useState<CardRecommendation[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .recommendations()
      .then(({ recommendations }) => {
        if (!cancelled) setRecommendations(recommendations)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load recommendations.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  const body = loading ? (
    <p className="text-sm text-ink-muted">Loading…</p>
  ) : error ? (
    <p className="text-sm text-ink-muted">{error}</p>
  ) : recommendations.length === 0 ? (
    <p className="text-sm text-ink-muted">No card in the catalogue beats what you already have.</p>
  ) : (
    <ul className="divide-y divide-white/5">
      {recommendations.map((r) => (
        <li key={r.card.id} className="py-3">
          <div className="flex items-baseline justify-between gap-2">
            <span className="text-sm font-medium text-ink">
              {r.card.issuer} {r.card.name}
            </span>
            <span className={`flex-none text-sm font-medium ${r.incremental_value_cents >= 0 ? 'text-accent' : 'text-ink-muted'}`}>
              {r.incremental_value_cents >= 0 ? '+' : ''}
              {currency.format(r.incremental_value_cents / 100)}/yr
            </span>
          </div>
          {r.categories_won.length > 0 && (
            <p className="mt-0.5 text-xs text-ink-muted">
              Beats your current cards on{' '}
              {r.categories_won
                .map((w) => `${w.category_name} (${formatBps(w.previous_rate_bps)} → ${formatBps(w.new_rate_bps)})`)
                .join(', ')}
            </p>
          )}
        </li>
      ))}
    </ul>
  )

  return (
    <Panel title="Recommendations" subtitle="Cards that would have earned you more on the spending you already do.">
      {body}
    </Panel>
  )
}
