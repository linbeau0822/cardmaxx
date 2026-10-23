import { useCallback, useEffect, useState } from 'react'
import { usePlaidLink, type PlaidLinkOnExit, type PlaidLinkOnSuccess } from 'react-plaid-link'

import { ApiStatus } from './components/ApiStatus'
import { AskAIPanel } from './components/AskAIPanel'
import { CardsPanel } from './components/CardsPanel'
import { Panel } from './components/Panel'
import { PortfolioPanel } from './components/PortfolioPanel'
import { RecommendationsPanel } from './components/RecommendationsPanel'
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
    <main className="mx-auto max-w-6xl px-6 py-16 sm:px-10 md:py-24 lg:px-16">
      <header className="grid grid-cols-12 items-start gap-6">
        <div className="col-span-12 lg:col-span-8">
          <h1 className="flex items-center gap-3 font-display text-5xl tracking-tight text-ink sm:text-6xl md:text-7xl leading-[0.95]">
            <img src="/favicon.svg" alt="" className="h-10 w-10 sm:h-12 sm:w-12" />
            CardMaxx
          </h1>
          <p className="mt-3 text-sm text-ink-muted">
            Analyze your spending and find the credit cards that pay you more.
          </p>
        </div>
        <div className="col-span-12 lg:col-span-4 lg:text-right">
          <ApiStatus />
          <div className="mt-3">
            <button
              type="button"
              onClick={connectAccount}
              disabled={pending}
              className="rounded-lg bg-accent px-3 py-1.5 text-sm font-medium text-surface transition hover:brightness-110 disabled:opacity-60"
            >
              {pending ? 'Connecting…' : 'Connect Account'}
            </button>
          </div>
          {message && (
            <p role="status" className="mt-2 text-xs text-ink-muted">
              {message}
            </p>
          )}
        </div>
      </header>

      {accounts !== null && accounts.length === 0 ? (
        <div className="mt-16">
          <Panel title="No accounts linked" subtitle="Connect an account above to see your spending and card recommendations.">
            <p className="text-sm text-ink-muted">Nothing to show yet.</p>
          </Panel>
        </div>
      ) : (
        accounts !== null && (
          <div className="mt-16 grid grid-cols-1 gap-10 lg:grid-cols-[1fr_22rem] lg:gap-14">
            <div className="grid min-w-0 gap-16">
              <RecommendationsPanel refreshKey={accountsVersion} />
              <PortfolioPanel refreshKey={accountsVersion} />
              <div className="grid gap-10 opacity-90">
                <CardsPanel accounts={accounts} onLinked={() => setAccountsVersion((v) => v + 1)} compact />
                <SpendingOverview refreshKey={transactionsVersion} compact />
              </div>
              <div className="md:ml-16 md:max-w-xl">
                <AskAIPanel />
              </div>
            </div>

            <TransactionsSidebar refreshKey={transactionsVersion} />
          </div>
        )
      )}
    </main>
  )
}
