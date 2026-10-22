import { BrandBadge, NetworkLogo } from './NetworkLogo'

/** Brand color + wordmark per issuing bank we recognize. */
const ISSUERS: Record<string, { bg: string; label: string }> = {
  chase: { bg: '#117ACA', label: 'CHASE' },
  citi: { bg: '#003B70', label: 'CITI' },
  'american express': { bg: '#016FD0', label: 'AMEX' },
  'capital one': { bg: '#004977', label: 'CAP1' },
}

/**
 * The logo for a card: its issuing bank when we recognize the issuer (Chase,
 * Amex, Citi, Capital One, ...), otherwise falling back to the card's network
 * logo. A card's `issuer` is the bank that actually issues it, which isn't
 * always the brand on the card art - e.g. Amazon Prime Visa is issued by
 * Chase, not Amazon, so there's no "Amazon" issuer to render in the first
 * place; this fallback instead covers cards whose issuer we simply don't
 * have a badge for yet.
 */
export function IssuerLogo({ issuer, network, className = '' }: { issuer: string; network: string; className?: string }) {
  const brand = ISSUERS[(issuer ?? '').toLowerCase()]
  if (!brand) return <NetworkLogo network={network} className={className} />

  return (
    <BrandBadge bg={brand.bg} label={issuer} className={className}>
      <span className="text-[9px] font-bold leading-none tracking-tight text-white">{brand.label}</span>
    </BrandBadge>
  )
}
