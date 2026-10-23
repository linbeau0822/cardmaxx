import type { ReactNode } from 'react'

import { useReveal } from '../lib/useReveal'

interface PanelProps {
  title: ReactNode
  /** One line explaining what this panel will show once it is built. */
  subtitle: string
  children: ReactNode
  /** Shrinks heading/subtitle/spacing for de-emphasized, tertiary panels. */
  compact?: boolean
}

/** A titled section of the dashboard. */
export function Panel({ title, subtitle, children, compact }: PanelProps) {
  const { ref, revealed } = useReveal<HTMLElement>()

  return (
    <section
      ref={ref}
      className={`border-t border-white/10 ${compact ? 'pt-4' : 'pt-6'} transition-all duration-700 ease-out ${
        revealed ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0'
      }`}
    >
      <h2
        className={`flex items-center gap-2 font-display tracking-tight text-ink ${
          compact ? 'text-base' : 'text-xl sm:text-2xl'
        }`}
      >
        {title}
      </h2>
      <p className={`mt-2 text-ink-muted ${compact ? 'text-xs' : 'text-sm'}`}>{subtitle}</p>
      <div className={compact ? 'mt-4' : 'mt-6'}>{children}</div>
    </section>
  )
}
