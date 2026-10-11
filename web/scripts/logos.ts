// Writes the logo in every accent color to docs/logo/ and the default one to
// public/favicon.svg. Run after changing src/lib/accent.ts: npm run logos
import { mkdirSync, writeFileSync } from "node:fs"

import { accents, defaultAccent, findAccent, logoSvg } from "../src/lib/accent.ts"

const dir = new URL("../../docs/logo/", import.meta.url)
mkdirSync(dir, { recursive: true })
for (const a of accents) writeFileSync(new URL(`bookrr-${a.id}.svg`, dir), logoSvg(a))
writeFileSync(new URL("../public/favicon.svg", import.meta.url), logoSvg(findAccent(defaultAccent)))
