import { useState } from 'react'

import { ApiError, api } from '../lib/api'
import { Panel } from './Panel'

/**
 * Free-text Q&A over the user's spending, recommendations, and portfolio
 * options (Phase 7). The backend grounds every answer in numbers it already
 * computed; this panel just sends the question and shows the reply.
 */
export function AskAIPanel() {
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  async function ask() {
    const q = question.trim()
    if (!q) return
    setPending(true)
    setError(null)
    setAnswer(null)
    try {
      const { answer } = await api.askAI(q)
      setAnswer(answer)
    } catch (err) {
      setError(
        err instanceof ApiError && err.code === 'service_unavailable'
          ? 'AI assistant is not configured on the server.'
          : err instanceof ApiError
            ? err.message
            : 'Failed to get an answer.',
      )
    } finally {
      setPending(false)
    }
  }

  return (
    <Panel
      title={
        <>
          <svg
            viewBox="0 0 24 24"
            fill="currentColor"
            className="h-4 w-4 flex-none text-accent"
            aria-hidden="true"
          >
            <path d="M12 2l1.8 5.2L19 9l-5.2 1.8L12 16l-1.8-5.2L5 9l5.2-1.8L12 2zM5 15l.9 2.6L8.5 18.5l-2.6.9L5 22l-.9-2.6-2.6-.9 2.6-.9L5 15zM19 15l.9 2.6 2.6.9-2.6.9L19 22l-.9-2.6-2.6-.9 2.6-.9L19 15z" />
          </svg>
          Ask AI
        </>
      }
      subtitle="Ask about your spending, recommendations, or portfolio options."
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void ask()
        }}
        className="flex gap-2"
      >
        <input
          type="text"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="Why is that card recommended?"
          className="flex-1 rounded-lg border border-white/10 bg-surface px-3 py-2 text-sm text-ink placeholder:text-ink-muted"
        />
        <button
          type="submit"
          disabled={pending || !question.trim()}
          className="flex-none rounded-lg bg-accent px-4 py-2 font-display text-sm font-medium text-surface transition hover:brightness-110 disabled:opacity-60"
        >
          {pending ? 'Asking…' : 'Ask'}
        </button>
      </form>
      {error && <p className="mt-3 text-sm text-ink-muted">{error}</p>}
      {answer && <p className="mt-3 text-sm text-ink">{answer}</p>}
    </Panel>
  )
}
