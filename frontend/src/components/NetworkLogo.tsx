import type { ReactNode } from 'react'

/** The fixed-size, same-shape box every brand badge (network or issuer) renders in. */
export function BrandBadge({
  bg,
  label,
  className = '',
  children,
}: {
  bg: string
  label: string
  className?: string
  children: ReactNode
}) {
  return (
    <span
      className={`inline-flex h-5 w-9 flex-none items-center justify-center rounded-sm ${className}`}
      style={{ backgroundColor: bg }}
      title={label}
    >
      {children}
    </span>
  )
}

/** Brand color per card network, so unrecognized networks still render a plain badge instead of breaking. */
const NETWORK_BG: Record<string, string> = {
  visa: '#1434CB',
  mastercard: '#1B1B1B',
  'american express': '#016FD0',
  discover: '#FF6000',
}

/**
 * A fixed-size, same-shape badge per card network (Visa/Mastercard/Amex/
 * Discover), so the row of cards on the front page looks uniform regardless
 * of how inconsistent real card art would be. Mastercard gets its real
 * two-circle mark; other networks get their wordmark, since those logos
 * are text-based to begin with.
 */
export function NetworkLogo({ network, className = '' }: { network: string; className?: string }) {
  const key = (network ?? '').toLowerCase()
  const bg = NETWORK_BG[key] ?? '#4B5563'

  return (
    <BrandBadge bg={bg} label={network} className={className}>
      {key === 'mastercard' ? (
        <svg viewBox="0 0 24 14" className="h-3 w-5" aria-label="Mastercard">
          <circle cx="9" cy="7" r="7" fill="#EB001B" />
          <circle cx="15" cy="7" r="7" fill="#FF5F00" opacity="0.85" />
        </svg>
      ) : (
        <span
          className="text-[9px] font-bold italic leading-none tracking-tight text-white"
          style={{ fontStyle: key === 'visa' ? 'italic' : 'normal' }}
        >
          {key === 'american express' ? 'AMEX' : key === 'discover' ? 'DISC' : key.toUpperCase().slice(0, 5)}
        </span>
      )}
    </BrandBadge>
  )
}
