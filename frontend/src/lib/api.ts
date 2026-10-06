/**
 * Typed client for the CardWise API.
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
    throw new ApiError(0, 'network_error', 'Could not reach the CardWise API.', { cause })
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

export const api = {
  /** Liveness check. The only endpoint implemented in the current phase. */
  health: () => request<HealthResponse>('/health'),

  /**
   * Requests a Plaid Link token.
   *
   * TODO(phase-2): feed the returned token to Plaid Link, then POST the
   * resulting public token to /api/v1/plaid/exchange-token.
   */
  createLinkToken: () => request<LinkTokenResponse>('/api/v1/plaid/link-token', { method: 'POST' }),
}
