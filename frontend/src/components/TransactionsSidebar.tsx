import { useEffect, useState } from 'react'

import { ApiError, api, type Transaction } from '../lib/api'
import { useReveal } from '../lib/useReveal'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' })
const dateFormat = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' })

/** Scrollable side panel listing synced transactions, most recent first. */
export function TransactionsSidebar({ refreshKey }: { refreshKey: number }) {
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const { ref, revealed } = useReveal<HTMLElement>()

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .listTransactions()
      .then(({ transactions }) => {
        if (!cancelled) setTransactions(transactions)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load transactions.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  return (
    <aside
      ref={ref}
      className={`flex flex-none flex-col border-l border-white/10 pl-6 transition-all duration-700 ease-out ${
        revealed ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0'
      }`}
    >
      <h2 className="flex-none font-display text-base tracking-tight text-ink">Transactions</h2>
      <div className="mt-4 flex-1 overflow-y-auto">
        {loading && <p className="py-3 text-sm text-ink-muted">Loading…</p>}
        {error && <p className="py-3 text-sm text-ink-muted">{error}</p>}
        {!loading && !error && transactions.length === 0 && (
          <p className="py-3 text-sm text-ink-muted">No transactions yet.</p>
        )}
        <ul className="divide-y divide-white/5">
          {transactions.map((t) => (
            <li key={t.id} className="-mx-2 rounded px-2 py-3 transition-colors hover:bg-white/5">
              <div className="flex items-start justify-between gap-2">
                <span className="truncate text-sm text-ink">{t.merchant_name ?? t.name}</span>
                <span className="flex-none text-sm font-medium text-ink">
                  {currency.format(-t.amount_cents / 100)}
                </span>
              </div>
              <div className="mt-0.5 flex items-center justify-between gap-2 text-xs text-ink-muted">
                <span>{t.category_name ?? 'Uncategorized'}</span>
                <span>{dateFormat.format(new Date(t.date))}</span>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  )
}
