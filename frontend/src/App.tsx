import { useCallback, useEffect, useState } from 'react'
import { usePlaidLink, type PlaidLinkOnExit, type PlaidLinkOnSuccess } from 'react-plaid-link'

import { ApiStatus } from './components/ApiStatus'
import { CardsPanel } from './components/CardsPanel'
import { ComingSoon, Panel } from './components/Panel'
import { SpendingOverview } from './components/SpendingOverview'
import { TransactionsSidebar } from './components/TransactionsSidebar'
import { ApiError, type Account, api } from './lib/api'

function describeError(error: unknown, action: string): string {
  return error instanceof ApiError
    ? `${error.status === 0 ? 'Network' : `HTTP ${error.status}`}: ${error.message}`
    : `Unexpected error ${action}.`
}

export default function App() {
  const [message, setMessage] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  // Plaid Link needs a token before it can open, so the button fetches one
  // and this state holds it until usePlaidLink is `ready`, at which point the
  // effect below opens the widget.
  const [linkToken, setLinkToken] = useState<string | null>(null)
  // Bumped after every successful sync so the transactions sidebar refetches.
  const [transactionsVersion, setTransactionsVersion] = useState(0)
  // Bumped after the user manually links an account to a card, so this
  // account list (and the Your Cards panel reading it) picks up the change.
  const [accountsVersion, setAccountsVersion] = useState(0)
  // null while loading, so the dashboard doesn't flash "no accounts" first.
  const [accounts, setAccounts] = useState<Account[] | null>(null)

  useEffect(() => {
    let cancelled = false
    api
      .listAccounts()
      .then(({ accounts }) => {
        if (!cancelled) setAccounts(accounts)
      })
      .catch(() => {
        if (!cancelled) setAccounts([])
      })
    return () => {
      cancelled = true
    }
  }, [transactionsVersion, accountsVersion])

  const onSuccess = useCallback<PlaidLinkOnSuccess>((publicToken) => {
    if (!publicToken) return
    void (async () => {
      try {
        await api.exchangePublicToken(publicToken)
        const { items } = await api.sync()
        const added = items.reduce((sum, item) => sum + item.added, 0)
        setMessage(`Account connected. Synced ${added} transaction${added === 1 ? '' : 's'}.`)
        setTransactionsVersion((v) => v + 1)
      } catch (error) {
        setMessage(describeError(error, 'syncing the new account'))
      } finally {
        setPending(false)
        setLinkToken(null)
      }
    })()
  }, [])

  const onExit = useCallback<PlaidLinkOnExit>((error) => {
    setPending(false)
    setLinkToken(null)
    if (error) {
      setMessage(`Plaid Link closed: ${error.display_message ?? error.error_message}`)
    }
  }, [])

  const { open, ready } = usePlaidLink({ token: linkToken ?? '', onSuccess, onExit })

  // usePlaidLink only becomes `ready` after it has a token, so opening happens
  // here rather than inline in connectAccount.
  useEffect(() => {
    if (linkToken && ready) {
      open()
    }
  }, [linkToken, ready, open])

  /** Starts the account-connection flow: fetch a Link token, then open Plaid Link. */
  async function connectAccount() {
    setPending(true)
    setMessage(null)
    try {
      const { link_token } = await api.createLinkToken()
      setLinkToken(link_token)
    } catch (error) {
      setMessage(describeError(error, 'starting the connection flow'))
      setPending(false)
    }
  }

  return (
    <main className="mx-auto max-w-6xl px-6 py-12">
      <header className="flex flex-wrap items-baseline justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-ink">CardMaxx</h1>
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

      {accounts !== null && accounts.length === 0 ? (
        <div className="mt-10">
          <Panel title="No accounts linked" subtitle="Connect an account above to see your spending and card recommendations.">
            <p className="text-sm text-ink-muted">Nothing to show yet.</p>
          </Panel>
        </div>
      ) : (
        accounts !== null && (
          <div className="mt-10 flex items-start gap-6">
            <div className="min-w-0 flex-1 grid gap-4">
              <SpendingOverview refreshKey={transactionsVersion} />
              <CardsPanel accounts={accounts} onLinked={() => setAccountsVersion((v) => v + 1)} />
              <Panel
                title="Recommendations"
                subtitle="Cards that would have earned you more on the spending you already do."
              >
                <ComingSoon phase="phase 5" />
              </Panel>
            </div>

            <TransactionsSidebar refreshKey={transactionsVersion} />
          </div>
        )
      )}
    </main>
  )
}
