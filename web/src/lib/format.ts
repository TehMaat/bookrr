const units = ["B", "KB", "MB", "GB", "TB", "PB"]

export function formatBytes(n: number): string {
  if (!n || n <= 0) return "—"
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`
}

/** Parses "4 TB", "500gb", "1.5 T" or a plain byte count. */
export function parseBytes(s: string): number {
  const m = s.trim().match(/^([\d.,]+)\s*([kmgtp]?)(i?b)?$/i)
  if (!m) return 0
  const v = parseFloat(m[1].replace(",", "."))
  const exp = ["", "k", "m", "g", "t", "p"].indexOf(m[2].toLowerCase())
  return Math.round(v * 1024 ** Math.max(exp, 0))
}

const dateFmt = new Intl.DateTimeFormat("it-IT", { dateStyle: "short", timeStyle: "short" })

export function formatDate(s: string | null | undefined): string {
  if (!s) return "—"
  const d = new Date(s)
  return isNaN(d.getTime()) ? s : dateFmt.format(d)
}

const rtf = new Intl.RelativeTimeFormat("it-IT", { numeric: "auto" })

export function formatRelative(s: string | null | undefined): string {
  if (!s) return "mai"
  const diff = (new Date(s).getTime() - Date.now()) / 1000
  const abs = Math.abs(diff)
  if (abs < 60) return rtf.format(Math.round(diff), "second")
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute")
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour")
  return rtf.format(Math.round(diff / 86400), "day")
}

const states: Record<string, string> = {
  uploading: "In seed",
  stalledUP: "In seed (fermo)",
  forcedUP: "Seed forzato",
  queuedUP: "In coda (seed)",
  pausedUP: "In pausa",
  stoppedUP: "Fermato",
  checkingUP: "Verifica",
  downloading: "In download",
  stalledDL: "Download fermo",
  forcedDL: "Download forzato",
  queuedDL: "In coda",
  pausedDL: "In pausa (incompleto)",
  stoppedDL: "Fermato (incompleto)",
  checkingDL: "Verifica",
  metaDL: "Metadati",
  forcedMetaDL: "Metadati",
  checkingResumeData: "Verifica",
  moving: "Spostamento",
  missingFiles: "File mancanti",
  error: "Errore",
}

export function formatState(s: string): string {
  return states[s] ?? s
}

const downloadStates = new Set([
  "downloading",
  "stalledDL",
  "forcedDL",
  "queuedDL",
  "pausedDL",
  "stoppedDL",
  "checkingDL",
  "metaDL",
  "forcedMetaDL",
])

/** True when the client hasn't finished downloading the torrent yet. */
export function isDownloading(l: { state: string; progress: number }): boolean {
  return downloadStates.has(l.state) || (l.progress < 1 && !isErrorState(l.state))
}

/** qBittorrent's added_on is a unix timestamp in seconds. */
export function formatUnix(ts: number): string {
  return ts > 0 ? dateFmt.format(new Date(ts * 1000)) : "—"
}

export function isErrorState(s: string): boolean {
  return s === "missingFiles" || s === "error"
}

const intFmt = new Intl.NumberFormat("it-IT")
const yearsFmt = new Intl.NumberFormat("it-IT", { maximumFractionDigits: 1 })

/** Power-on hours, e.g. "28.734 h (3,3 anni)". */
export function formatHours(h: number): string {
  if (!h || h <= 0) return "—"
  const years = h / (24 * 365)
  return years >= 0.1 ? `${intFmt.format(h)} h (${yearsFmt.format(years)} anni)` : `${intFmt.format(h)} h`
}

/** Badge variant for a SMART health status (PASSED, OK, Good, FAILED, Caution…). */
export function healthVariant(h: string): "success" | "destructive" | "warning" {
  if (/pass|^ok|good|buon/i.test(h)) return "success"
  if (/fail|bad|cattiv/i.test(h)) return "destructive"
  return "warning"
}
