// Accent colors, named after lightsabers. Each one colors both the logo and the
// dashboard (primary buttons, selection rings, version badge). Kept free of
// imports so scripts/logos.ts can load it with plain node.

export type AccentId = "viola" | "blu" | "verde" | "giallo" | "rosso" | "darksaber"

export type Accent = {
  id: AccentId
  label: string
  saber: string
  // Dashboard: primary color and the text on it, for the light and dark theme.
  light: { brand: string; fg: string }
  dark: { brand: string; fg: string }
  // Logo: background gradient, blade core and glow, script lines on the pages.
  logo: { from: string; to: string; blade: string; glow: string; ink: string }
}

export const accents: Accent[] = [
  {
    id: "viola",
    label: "Viola",
    saber: "Mace Windu",
    light: { brand: "#7c3aed", fg: "#ffffff" },
    dark: { brand: "#a78bfa", fg: "#2e1065" },
    logo: { from: "#7c3aed", to: "#3b0f86", blade: "#ffffff", glow: "#c4b5fd", ink: "#7c3aed" },
  },
  {
    id: "blu",
    label: "Blu",
    saber: "Obi-Wan Kenobi",
    light: { brand: "#2563eb", fg: "#ffffff" },
    dark: { brand: "#60a5fa", fg: "#172554" },
    logo: { from: "#2563eb", to: "#172554", blade: "#ffffff", glow: "#93c5fd", ink: "#2563eb" },
  },
  {
    id: "verde",
    label: "Verde",
    saber: "Yoda",
    light: { brand: "#15803d", fg: "#ffffff" },
    dark: { brand: "#4ade80", fg: "#052e16" },
    logo: { from: "#16a34a", to: "#052e16", blade: "#ffffff", glow: "#86efac", ink: "#15803d" },
  },
  {
    id: "giallo",
    label: "Giallo",
    saber: "Rey",
    light: { brand: "#f59e0b", fg: "#451a03" },
    dark: { brand: "#fbbf24", fg: "#451a03" },
    logo: { from: "#d97706", to: "#78350f", blade: "#ffffff", glow: "#fde68a", ink: "#b45309" },
  },
  {
    id: "rosso",
    label: "Rosso",
    saber: "Sith",
    light: { brand: "#dc2626", fg: "#ffffff" },
    dark: { brand: "#f87171", fg: "#450a0a" },
    logo: { from: "#dc2626", to: "#450a0a", blade: "#ffffff", glow: "#fca5a5", ink: "#b91c1c" },
  },
  {
    id: "darksaber",
    label: "Nero",
    saber: "Darksaber",
    light: { brand: "#18181b", fg: "#fafafa" },
    dark: { brand: "#e4e4e7", fg: "#18181b" },
    logo: { from: "#3f3f46", to: "#09090b", blade: "#09090b", glow: "#f4f4f5", ink: "#52525b" },
  },
]

export const defaultAccent: AccentId = "viola"

export function findAccent(id: string | null | undefined): Accent {
  return accents.find((a) => a.id === id) ?? accents.find((a) => a.id === defaultAccent)!
}

// The logo: an ancient tome from the Jedi sacred texts, its pages spread like
// wings, with a lightsaber as its spine.
export function logoSvg({ logo: c }: Accent): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="${c.from}"/>
      <stop offset="1" stop-color="${c.to}"/>
    </linearGradient>
    <linearGradient id="page" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#fffaf0"/>
      <stop offset="1" stop-color="#ead9b4"/>
    </linearGradient>
    <filter id="glow" filterUnits="userSpaceOnUse" x="0" y="0" width="64" height="64">
      <feGaussianBlur stdDeviation="2.2"/>
    </filter>
  </defs>
  <rect width="64" height="64" rx="14" fill="url(#bg)"/>
  <circle cx="32" cy="33" r="21" fill="none" stroke="${c.glow}" stroke-width="1.5" opacity=".45"/>
  <path d="M32 7V40" stroke="${c.glow}" stroke-width="7" stroke-linecap="round" filter="url(#glow)"/>
  <path d="M32 7V40" stroke="${c.blade}" stroke-width="3" stroke-linecap="round"/>
  <path d="M30.5 38C25 33 17 30 6 22C8 33 9 42 13 47C19 47 25 49 30.5 54Z" fill="url(#page)"/>
  <path d="M33.5 38C39 33 47 30 58 22C56 33 55 42 51 47C45 47 39 49 33.5 54Z" fill="url(#page)"/>
  <rect x="30.5" y="38" width="3" height="17" rx="1" fill="#d4d4d8"/>
  <path d="M30.5 43h3M30.5 46h3M30.5 49h3" stroke="#52525b" stroke-width="1"/>
  <g stroke="${c.ink}" stroke-width="1.4" stroke-linecap="round" stroke-dasharray="5 2.2 2 2.2 3.5 2.2" fill="none" opacity=".6">
    <path d="M11 32C17 35 22 37 27 40.5"/>
    <path d="M12.5 38C18 40 22 41.5 27 45"/>
    <path d="M53 32C47 35 42 37 37 40.5"/>
    <path d="M51.5 38C46 40 42 41.5 37 45"/>
  </g>
</svg>
`
}

export function logoUrl(accent: Accent): string {
  return `data:image/svg+xml,${encodeURIComponent(logoSvg(accent))}`
}
