// Takes the screenshots of the documentation from a running demo
// (go run ./scripts/demo), into docs/screenshots/. Needs Playwright with
// Chromium: npx playwright install chromium, if it is not installed yet.
//
//	node scripts/demo/screenshots.mjs [http://127.0.0.1:8080]
import { mkdirSync } from "node:fs"
import { createRequire } from "node:module"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const require = createRequire(import.meta.url)
const { chromium } = require("playwright")

const base = process.argv[2] ?? "http://127.0.0.1:8080"
const out = join(dirname(fileURLToPath(import.meta.url)), "../../docs/screenshots")
mkdirSync(out, { recursive: true })

const get = async (path) => (await fetch(base + path)).json()
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// The demo is ready once the missing check and the Prowlarr searches ran.
for (let i = 0; ; i++) {
  const [missing, releases, plan] = await Promise.all([get("/api/missing/series"), get("/api/releases"), get("/api/plan")])
  const ready =
    missing.checkedAt && !missing.state?.running && releases.best?.lastRunAt && !releases.best?.running && !plan.entries.some((e) => e.searching)
  if (ready) break
  if (i === 0) console.log("waiting for the demo to finish its searches…")
  await sleep(3000)
}

const browser = await chromium.launch()
const shoot = async (name, hash, { dark = false, width = 1440, height = 900, act } = {}) => {
  const page = await browser.newPage({ viewport: { width, height }, colorScheme: dark ? "dark" : "light" })
  await page.addInitScript(() => localStorage.setItem("bookrr-theme", "system"))
  await page.goto(`${base}/#${hash}`)
  await page.waitForLoadState("networkidle")
  await sleep(600)
  if (act) await act(page)
  await sleep(400)
  await page.screenshot({ path: join(out, `${name}.png`) })
  await page.close()
  console.log(name)
}

await shoot("torrents", "torrents")
await shoot("alerts", "alerts")
await shoot("resolve", "alerts", { act: (p) => p.getByRole("button", { name: "Set location" }).last().click() })

// The open alert shows on every page: handled, the other pages are clearer.
const disks = await get("/api/disks")
for (const a of await get("/api/alerts?open=1")) {
  await fetch(`${base}/api/alerts/${a.id}/resolve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ resolution: "Moved to Archive 02 · /Series", archive: { diskId: disks.find((d) => d.label === "Archive 02")?.id ?? null, path: "/Series", notes: "" } }),
  })
}

await shoot("torrents-dark", "torrents", { dark: true })
await shoot("torrent-details", "torrents", {
  act: (p) => p.getByText("Starfall.Courier.2019", { exact: false }).first().click(),
})
await shoot("releases", "releases")
await shoot("missing-series", "missing")
await shoot("missing-series-by-quality", "missing", { act: (p) => p.getByRole("button", { name: "By quality" }).click() })
await shoot("missing-sagas", "missing/sagas")
await shoot("missing-collections", "missing/lists")
await shoot("plan", "plan")
await shoot("disks", "disks")
await shoot("disk-archives", "disks/1")
await shoot("clients", "clients")
await shoot("settings", "torrents", { act: (p) => p.getByRole("button", { name: "Settings" }).click() })
await shoot("logs", "logs")
await shoot("mobile", "disks/1", { width: 390, height: 844 })
await browser.close()
