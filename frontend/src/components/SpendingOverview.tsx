import { useEffect, useState } from 'react'

import { ApiError, api, type CategorySpending, type MerchantSpending, type MonthSpending } from '../lib/api'
import { Panel } from './Panel'

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })
// `month` values from the API are date-only UTC strings (e.g. "2026-08-01"). Formatting them
// without pinning the timezone shifts the label back a day in any UTC-negative timezone, rolling
// the 1st into the previous month and mislabeling every option by one.
const monthLabelFormat = new Intl.DateTimeFormat('en-US', { month: 'short', year: 'numeric', timeZone: 'UTC' })

/** Formats a Plaid category constant like FOOD_AND_DRINK as "Food and drink". */
function formatCategoryName(name: string): string {
  const words = name.toLowerCase().split('_')
  return words.map((w, i) => (i === 0 ? w[0].toUpperCase() + w.slice(1) : w)).join(' ')
}

const BAND_HEIGHT = 72 // px, one bar band above and (if needed) below the zero baseline

// Water-smooth easing (ease-out-quint) for bar height/opacity transitions between months.
const FLOW_TRANSITION = 'height 500ms cubic-bezier(0.22, 1, 0.36, 1), opacity 400ms ease-out'

/** Fills in every category from the fixed, all-time universe at zero so the chart's columns never appear or disappear across months. */
function withZeroFill(all: string[], data: CategorySpending[]): CategorySpending[] {
  const byName = new Map(data.map((c) => [c.category_name, c]))
  return all.map((name) => byName.get(name) ?? { category_name: name, total_cents: 0, transaction_count: 0 })
}

/** Columns of per-category spend for one month, growing up from a zero baseline (down for a net refund). Categories with no spend render as a nonexistent (zero-height) bar so the chart's shape flows between months instead of cutting. */
function CategoryBarChart({ categories }: { categories: CategorySpending[] }) {
  if (categories.length === 0) return <p className="text-sm text-ink-muted">No categorized spending yet.</p>

  const maxAbs = Math.max(1, ...categories.map((c) => Math.abs(c.total_cents)))
  const hasNegative = categories.some((c) => c.total_cents < 0)

  return (
    <div className="grid grid-flow-col gap-1" style={{ gridTemplateColumns: `repeat(${categories.length}, minmax(0, 1fr))` }}>
      {categories.map((c) => {
        const isZero = c.total_cents === 0
        const ratio = isZero ? 0 : Math.abs(c.total_cents) / maxAbs
        const barHeight = isZero ? 0 : Math.max(2, Math.round(ratio * BAND_HEIGHT))
        const isNegative = c.total_cents < 0
        return (
          <div
            key={c.category_name}
            className="flex flex-col items-center"
            title={`${formatCategoryName(c.category_name)}: ${currency.format(c.total_cents / 100)} (${c.transaction_count} transactions)`}
          >
            <div className="flex flex-col items-center justify-end" style={{ height: BAND_HEIGHT }}>
              {!isNegative && (
                <>
                  <span
                    className="mb-1 font-display text-xs tabular-nums text-ink-muted"
                    style={{ opacity: isZero ? 0 : 1, transition: FLOW_TRANSITION }}
                  >
                    {currency.format(c.total_cents / 100)}
                  </span>
                  <div
                    className="w-full max-w-6 rounded-t bg-accent"
                    style={{ height: barHeight, transition: FLOW_TRANSITION }}
                  />
                </>
              )}
            </div>
            <div className="h-px w-full bg-white/15" />
            {hasNegative && (
              <div className="flex flex-col items-center justify-start" style={{ height: BAND_HEIGHT }}>
                {isNegative && (
                  <>
                    <div
                      className="w-full max-w-6 rounded-b bg-accent/60"
                      style={{ height: barHeight, transition: FLOW_TRANSITION }}
                    />
                    <span
                      className="mt-1 font-display text-xs tabular-nums text-ink-muted"
                      style={{ opacity: isZero ? 0 : 1, transition: FLOW_TRANSITION }}
                    >
                      {currency.format(c.total_cents / 100)}
                    </span>
                  </>
                )}
              </div>
            )}
            <span className="mt-2 w-full truncate text-center text-[10px] text-ink-muted" title={formatCategoryName(c.category_name)}>
              {formatCategoryName(c.category_name)}
            </span>
          </div>
        )
      })}
    </div>
  )
}

/** Prev/next arrows plus a dropdown for picking which month's spend to chart. */
function MonthPicker({
  months,
  selectedMonth,
  onChange,
}: {
  months: MonthSpending[]
  selectedMonth: string
  onChange: (month: string) => void
}) {
  const index = months.findIndex((m) => m.month === selectedMonth)

  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        className="rounded px-1.5 py-0.5 text-ink-muted hover:bg-white/10 disabled:opacity-30"
        disabled={index <= 0}
        onClick={() => onChange(months[index - 1].month)}
        aria-label="Previous month"
      >
        ‹
      </button>
      <select
        className="rounded bg-transparent text-sm text-ink"
        value={selectedMonth}
        onChange={(e) => onChange(e.target.value)}
      >
        {months.map((m) => (
          <option key={m.month} value={m.month} className="bg-ink text-ink-muted">
            {monthLabelFormat.format(new Date(m.month))}
          </option>
        ))}
      </select>
      <button
        type="button"
        className="rounded px-1.5 py-0.5 text-ink-muted hover:bg-white/10 disabled:opacity-30"
        disabled={index === -1 || index >= months.length - 1}
        onClick={() => onChange(months[index + 1].month)}
        aria-label="Next month"
      >
        ›
      </button>
    </div>
  )
}

/** Ranked list of all-time spend per merchant, as horizontal bars against the top merchant's total. */
function TopMerchants({ merchants }: { merchants: MerchantSpending[] }) {
  if (merchants.length === 0) return <p className="text-sm text-ink-muted">No categorized spending yet.</p>

  const maxCents = Math.max(1, ...merchants.map((m) => m.total_cents))

  return (
    <ul className="space-y-2">
      {merchants.map((m) => (
        <li key={m.merchant_name} className="flex items-center gap-3 text-sm">
          <span className="w-32 flex-none truncate text-ink" title={m.merchant_name}>
            {m.merchant_name}
          </span>
          <div className="h-2 flex-1 rounded bg-white/10">
            <div className="h-2 rounded bg-accent" style={{ width: `${Math.max(0, (m.total_cents / maxCents) * 100)}%` }} />
          </div>
          <span className="w-24 flex-none text-right tabular-nums text-ink-muted">
            {currency.format(m.total_cents / 100)} · {m.transaction_count}
          </span>
        </li>
      ))}
    </ul>
  )
}

/** Category breakdown, as a bar chart, for a month the user can step through. */
export function SpendingOverview({ refreshKey }: { refreshKey: number }) {
  const [categories, setCategories] = useState<CategorySpending[]>([])
  const [allCategories, setAllCategories] = useState<string[]>([])
  const [months, setMonths] = useState<MonthSpending[]>([])
  const [selectedMonth, setSelectedMonth] = useState<string | null>(null)
  const [topMerchants, setTopMerchants] = useState<MerchantSpending[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  // refreshKey changes (new sync) reload everything, including which month is selected.
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .spendingByCategory()
      .then((res) => {
        if (!cancelled) {
          setCategories(res.categories)
          setAllCategories(res.all_categories)
          setMonths(res.months)
          setSelectedMonth(res.selected_month)
          setTopMerchants(res.top_merchants)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load spending.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  function selectMonth(month: string) {
    setSelectedMonth(month)
    api
      .spendingByCategory(month)
      .then((res) => setCategories(res.categories))
      .catch((err: unknown) => setError(err instanceof ApiError ? err.message : 'Failed to load spending.'))
  }

  const body = loading ? (
    <p className="text-sm text-ink-muted">Loading…</p>
  ) : error ? (
    <p className="text-sm text-ink-muted">{error}</p>
  ) : months.length === 0 || selectedMonth === null ? (
    <p className="text-sm text-ink-muted">No categorized spending yet.</p>
  ) : (
    <div className="grid gap-5">
      <div>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-xs font-medium uppercase tracking-wide text-ink-muted">Spend by category</h3>
          <MonthPicker months={months} selectedMonth={selectedMonth} onChange={selectMonth} />
        </div>
        <CategoryBarChart categories={withZeroFill(allCategories, categories)} />
      </div>
      <ul className="divide-y divide-white/5">
        {categories.map((c) => (
          <li key={c.category_name} className="flex items-center justify-between gap-2 py-2 text-sm">
            <span className="text-ink">{formatCategoryName(c.category_name)}</span>
            <span className="flex-none text-ink-muted">
              {currency.format(c.total_cents / 100)} · {c.transaction_count}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )

  return (
    <>
      <Panel title="Spending Overview" subtitle="Where your money goes, by category and month.">
        {body}
      </Panel>
      {!loading && !error && topMerchants.length > 0 && (
        <Panel title="Top Merchants" subtitle="Your highest all-time spend by merchant.">
          <TopMerchants merchants={topMerchants} />
        </Panel>
      )}
    </>
  )
}
