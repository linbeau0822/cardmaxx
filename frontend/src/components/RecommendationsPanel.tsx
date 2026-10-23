import { useEffect, useState } from 'react'

import { ApiError, api, type CardRecommendation } from '../lib/api'
import { Panel } from './Panel'
import { useReveal } from '../lib/useReveal'

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

  const { ref, revealed } = useReveal<HTMLElement>()
  const [top, ...rest] = recommendations

  const spotlightBody = loading ? (
    <p className="mt-4 text-sm text-ink-muted">Loading…</p>
  ) : error ? (
    <p className="mt-4 text-sm text-ink-muted">{error}</p>
  ) : !top ? (
    <p className="mt-4 text-sm text-ink-muted">No card in the catalogue beats what you already have.</p>
  ) : (
    <>
      <h2 className="mt-3 font-display text-3xl tracking-tight text-ink sm:text-4xl md:text-5xl">
        {top.card.issuer} {top.card.name}
      </h2>
      <p className="mt-4 text-2xl font-medium text-accent sm:text-3xl">
        {top.incremental_value_cents >= 0 ? '+' : ''}
        {currency.format(top.incremental_value_cents / 100)}/yr
      </p>
      {top.categories_won.length > 0 && (
        <p className="mt-3 text-sm text-ink-muted">
          Beats your current cards on{' '}
          {top.categories_won
            .map((w) => `${w.category_name} (${formatBps(w.previous_rate_bps)} → ${formatBps(w.new_rate_bps)})`)
            .join(', ')}
        </p>
      )}
    </>
  )

  return (
    <>
      <section
        ref={ref}
        className={`rounded-2xl border border-accent/30 bg-accent/5 p-8 transition-all duration-700 ease-out sm:p-10 ${
          revealed ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0'
        }`}
      >
        <p className="text-xs font-medium uppercase tracking-wide text-accent">Next best card</p>
        {spotlightBody}
      </section>
      {!loading && !error && rest.length > 0 && (
        <Panel title="Other recommendations" subtitle="More cards that would beat what you already have, ranked lower.">
          <ul className="divide-y divide-white/5">
            {rest.map((r) => (
              <li key={r.card.id} className="-mx-2 rounded px-2 py-4 transition-colors hover:bg-white/5">
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
        </Panel>
      )}
    </>
  )
}
