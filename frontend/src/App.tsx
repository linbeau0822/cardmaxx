import { useState } from 'react'

import { ApiStatus } from './components/ApiStatus'
import { ComingSoon, Panel } from './components/Panel'
import { ApiError, api } from './lib/api'

export default function App() {
  const [message, setMessage] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  /**
   * Starts the account-connection flow.
   *
   * TODO(phase-2): on success, open Plaid Link with the returned link token.
   * For now the backend answers 501 and the response is surfaced verbatim, so
   * the button exercises the real request path rather than a stub.
   */
  async function connectAccount() {
    setPending(true)
    setMessage(null)
    try {
      const { link_token } = await api.createLinkToken()
      setMessage(`Received link token ${link_token.slice(0, 12)}…`)
    } catch (error) {
      setMessage(
        error instanceof ApiError
          ? `${error.status === 0 ? 'Network' : `HTTP ${error.status}`}: ${error.message}`
          : 'Unexpected error starting the connection flow.',
      )
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="mx-auto max-w-3xl px-6 py-12">
      <header className="flex flex-wrap items-baseline justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-ink">CardWise</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Analyze your spending and find the credit cards that pay you more.
          </p>
        </div>
        <ApiStatus />
      </header>

      <div className="mt-8">
        <button
          type="button"
          onClick={connectAccount}
          disabled={pending}
          className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-surface transition hover:brightness-110 disabled:opacity-60"
        >
          {pending ? 'Connecting…' : 'Connect Account'}
        </button>
        {message && (
          <p role="status" className="mt-3 text-sm text-ink-muted">
            {message}
          </p>
        )}
      </div>

      <div className="mt-10 grid gap-4">
        <Panel title="Spending Overview" subtitle="Where your money goes, by category and month.">
          <ComingSoon phase="phase 3" />
        </Panel>
        <Panel title="Your Cards" subtitle="The cards in your wallet and what each one earns you.">
          <ComingSoon phase="phase 4" />
        </Panel>
        <Panel
          title="Recommendations"
          subtitle="Cards that would have earned you more on the spending you already do."
        >
          <ComingSoon phase="phase 5" />
        </Panel>
      </div>
    </main>
  )
}
