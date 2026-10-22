import type { ReactNode } from 'react'

import { useReveal } from '../lib/useReveal'

interface PanelProps {
  title: ReactNode
  /** One line explaining what this panel will show once it is built. */
  subtitle: string
  children: ReactNode
}

/** A titled section of the dashboard. */
export function Panel({ title, subtitle, children }: PanelProps) {
  const { ref, revealed } = useReveal<HTMLElement>()

  return (
    <section
      ref={ref}
      className={`border-t border-white/10 pt-6 transition-all duration-700 ease-out ${
        revealed ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0'
      }`}
    >
      <h2 className="flex items-center gap-2 font-display text-xl tracking-tight text-ink sm:text-2xl">{title}</h2>
      <p className="mt-2 text-sm text-ink-muted">{subtitle}</p>
      <div className="mt-6">{children}</div>
    </section>
  )
}
