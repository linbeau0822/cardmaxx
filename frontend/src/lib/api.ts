/**
 * Typed client for the CardMaxx API.
 *
 * Every call goes through `request`, which is the single place that knows the
 * backend's error envelope. Components therefore handle one error type instead
 * of each re-deriving what a failed response looks like.
 */

/** Base URL for the API. Empty in development, where Vite proxies to the Go server. */
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

/** Error codes the backend can return. Mirrors api.ErrorCode in the backend. */
export type ApiErrorCode =
  | 'not_found'
  | 'method_not_allowed'
  | 'not_implemented'
  | 'bad_request'
  | 'internal_error'
  | 'network_error'

/** Shape of the backend's error responses: `{ "error": { "code", "message" } }`. */
interface ApiErrorEnvelope {
  error?: { code?: ApiErrorCode; message?: string }
}

export class ApiError extends Error {
  readonly status: number
  readonly code: ApiErrorCode

  constructor(status: number, code: ApiErrorCode, message: string, options?: ErrorOptions) {
    super(message, options)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      headers: { Accept: 'application/json', ...init?.headers },
      ...init,
    })
  } catch (cause) {
    throw new ApiError(0, 'network_error', 'Could not reach the CardMaxx API.', { cause })
  }

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ApiErrorEnvelope
    throw new ApiError(
      response.status,
      body.error?.code ?? 'internal_error',
      body.error?.message ?? `Request to ${path} failed with status ${response.status}.`,
    )
  }

  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export interface HealthResponse {
  status: string
}

export interface LinkTokenResponse {
  link_token: string
  expiration: string
}

export interface ExchangeTokenResponse {
  item_id: string
}

export interface ItemSyncResult {
  plaid_item_id: string
  added: number
  modified: number
  removed: number
}

export interface SyncResponse {
  items: ItemSyncResult[]
}

export interface Transaction {
  id: string
  amount_cents: number
  date: string
  name: string
  merchant_name: string | null
  category_id: string | null
  category_name: string | null
  pending: boolean
}

export interface TransactionsResponse {
  transactions: Transaction[]
}

export interface Account {
  id: string
  name: string
  mask: string | null
  /** The catalogue card this account is matched to, if any (matching isn't automatic yet). */
  credit_card_id: string | null
}

export interface AccountsResponse {
  accounts: Account[]
}

export interface CategorySpending {
  category_name: string
  total_cents: number
  transaction_count: number
}

export interface MonthSpending {
  month: string
  total_cents: number
  transaction_count: number
}

export interface MerchantSpending {
  merchant_name: string
  total_cents: number
  transaction_count: number
}

export interface SpendingAnalysisResponse {
  categories: CategorySpending[]
  all_categories: string[]
  months: MonthSpending[]
  selected_month: string
  top_merchants: MerchantSpending[]
}

export interface CardRewardCategory {
  category_name: string
  reward_rate_bps: number
  reward_cap_cents: number | null
}

export interface Card {
  id: string
  name: string
  issuer: string
  annual_fee_cents: number
  base_reward_rate_bps: number
  reward_categories: CardRewardCategory[]
}

export interface CardsResponse {
  cards: Card[]
}

export const api = {
  /** Liveness check. */
  health: () => request<HealthResponse>('/health'),

  /** Requests a Plaid Link token to open the Plaid Link widget. */
  createLinkToken: () => request<LinkTokenResponse>('/api/v1/plaid/link-token', { method: 'POST' }),

  /** Exchanges the public token Plaid Link returns for a stored Item. */
  exchangePublicToken: (publicToken: string) =>
    request<ExchangeTokenResponse>('/api/v1/plaid/exchange-token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ public_token: publicToken }),
    }),

  /** Pulls new transaction activity for every Item the current user has linked. */
  sync: () => request<SyncResponse>('/api/v1/plaid/sync', { method: 'POST' }),

  /** Lists every account linked to the current user. */
  listAccounts: () => request<AccountsResponse>('/api/v1/accounts'),

  /** Lists the current user's transactions, most recent first. */
  listTransactions: (limit = 100) =>
    request<TransactionsResponse>(`/api/v1/transactions?limit=${limit}`),

  /**
   * Totals the current user's reward-eligible spend by month, plus the
   * category breakdown for one month (defaults to the most recent).
   */
  spendingByCategory: (month?: string) =>
    request<SpendingAnalysisResponse>(`/api/v1/analysis/spending${month ? `?month=${month}` : ''}`),

  /** Lists the credit-card catalogue, each with its bonus reward categories. */
  listCards: () => request<CardsResponse>('/api/v1/cards'),
}
