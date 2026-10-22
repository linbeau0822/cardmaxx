import { useEffect, useState } from 'react'

import { ApiError, api, type Account, type Card } from '../lib/api'
import { IssuerLogo } from './IssuerLogo'
import { Panel } from './Panel'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })

/** Formats basis points as a percent, e.g. 150 -> "1.5%". */
function formatBps(bps: number): string {
  return `${(bps / 100).toFixed(bps % 100 === 0 ? 0 : 1)}%`
}

/**
 * The cards backing the user's linked accounts, with what each one earns.
 * Credit accounts sync left unmatched (their name named no catalogue card
 * unambiguously - every Plaid Sandbox account, and some real ones) get a
 * manual picker instead.
 */
export function CardsPanel({ accounts, onLinked }: { accounts: Account[]; onLinked: () => void }) {
  const [catalog, setCatalog] = useState<Card[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [selections, setSelections] = useState<Record<string, string>>({})
  const [linkingId, setLinkingId] = useState<string | null>(null)

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
  const unmatched = accounts.filter((a) => a.type === 'credit' && a.credit_card_id === null)

  async function linkAccount(accountId: string) {
    const cardId = selections[accountId]
    if (!cardId) return
    setLinkingId(accountId)
    try {
      await api.linkAccountCard(accountId, cardId)
      onLinked()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to link card.')
    } finally {
      setLinkingId(null)
    }
  }

  const body = loading ? (
    <p className="text-sm text-ink-muted">Loading…</p>
  ) : error ? (
    <p className="text-sm text-ink-muted">{error}</p>
  ) : cards.length === 0 ? (
    <p className="text-sm text-ink-muted">None of your linked accounts are matched to a known card yet.</p>
  ) : (
    <ul className="divide-y divide-white/5">
      {cards.map((c) => (
        <li key={c.id} className="-mx-2 rounded px-2 py-4 transition-colors hover:bg-white/5">
          <div className="flex items-baseline justify-between gap-2">
            <span className="flex items-center gap-2 text-sm font-medium text-ink">
              <IssuerLogo issuer={c.issuer} network={c.network} />
              {c.name}
            </span>
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
      {!loading && unmatched.length > 0 && (
        <div className="mt-4 border-t border-white/5 pt-4">
          <p className="text-xs text-ink-muted">
            Couldn't automatically match {unmatched.length === 1 ? 'this account' : 'these accounts'} to a card — pick one:
          </p>
          <ul className="mt-2 space-y-2">
            {unmatched.map((a) => (
              <li key={a.id} className="flex items-center gap-2">
                <span className="flex-1 truncate text-sm text-ink">
                  {a.name}
                  {a.mask && <span className="text-ink-muted"> ···{a.mask}</span>}
                </span>
                <select
                  className="rounded border border-white/10 bg-transparent px-2 py-1 text-xs text-ink"
                  value={selections[a.id] ?? ''}
                  onChange={(e) => setSelections((s) => ({ ...s, [a.id]: e.target.value }))}
                >
                  <option value="" className="bg-ink text-ink-muted">
                    Choose a card…
                  </option>
                  {catalog.map((c) => (
                    <option key={c.id} value={c.id} className="bg-ink text-ink-muted">
                      {c.issuer} {c.name}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  onClick={() => linkAccount(a.id)}
                  disabled={!selections[a.id] || linkingId === a.id}
                  className="flex-none rounded bg-accent px-2 py-1 text-xs font-medium text-surface transition hover:brightness-110 disabled:opacity-60"
                >
                  {linkingId === a.id ? 'Linking…' : 'Link'}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Panel>
  )
}
