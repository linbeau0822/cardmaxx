import type { ReactNode } from 'react'

interface PanelProps {
  title: string
  /** One line explaining what this panel will show once it is built. */
  subtitle: string
  children: ReactNode
}

/** A titled section of the dashboard. */
export function Panel({ title, subtitle, children }: PanelProps) {
  return (
    <section className="rounded-xl border border-white/10 bg-surface-raised p-5">
      <h2 className="text-base font-semibold text-ink">{title}</h2>
      <p className="mt-1 text-sm text-ink-muted">{subtitle}</p>
      <div className="mt-4">{children}</div>
    </section>
  )
}
