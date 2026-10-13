import { useEffect, useState } from 'react'

import { ApiError, api, type Transaction } from '../lib/api'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' })
const dateFormat = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' })

/** Scrollable side panel listing synced transactions, most recent first. */
export function TransactionsSidebar({ refreshKey }: { refreshKey: number }) {
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

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
    <aside className="flex h-[calc(100vh-6rem)] w-72 flex-none flex-col rounded-xl border border-white/10 bg-surface-raised">
      <h2 className="flex-none border-b border-white/10 px-4 py-3 text-sm font-semibold text-ink">
        Transactions
      </h2>
      <div className="flex-1 overflow-y-auto">
        {loading && <p className="px-4 py-3 text-sm text-ink-muted">Loading…</p>}
        {error && <p className="px-4 py-3 text-sm text-ink-muted">{error}</p>}
        {!loading && !error && transactions.length === 0 && (
          <p className="px-4 py-3 text-sm text-ink-muted">No transactions yet.</p>
        )}
        <ul className="divide-y divide-white/5">
          {transactions.map((t) => (
            <li key={t.id} className="px-4 py-3">
              <div className="flex items-start justify-between gap-2">
                <span className="truncate text-sm text-ink">{t.merchant_name ?? t.name}</span>
                <span className="flex-none text-sm font-medium text-ink">
                  {currency.format(-t.amount_cents / 100)}
                </span>
              </div>
              <div className="mt-0.5 flex items-center justify-between gap-2 text-xs text-ink-muted">
                <span>{t.category_id ? t.category_id : 'Uncategorized'}</span>
                <span>{dateFormat.format(new Date(t.date))}</span>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  )
}
