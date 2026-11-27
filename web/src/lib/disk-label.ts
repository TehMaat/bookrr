import { encode } from "uqr"

import type { Disk } from "@/lib/api"
import { formatBytes } from "@/lib/format"

/** The label is 80×50 mm at 300 dpi, black on white for thermal label printers. */
const DPI = 300
const mm = (v: number) => Math.round((v * DPI) / 25.4)
const pt = (v: number) => (v * DPI) / 72
export const LABEL_WIDTH = mm(80)
export const LABEL_HEIGHT = mm(50)

const SANS = 'system-ui, -apple-system, "Segoe UI", Roboto, "DejaVu Sans", sans-serif'
const MONO = 'ui-monospace, "SF Mono", Menlo, Consolas, "DejaVu Sans Mono", monospace'

// The bookrr logo in one colour: an open book with the saber in the middle (64×64).
const LOGO = {
  box: new Path2D("M14 0H50A14 14 0 0 1 64 14V50A14 14 0 0 1 50 64H14A14 14 0 0 1 0 50V14A14 14 0 0 1 14 0Z"),
  left: new Path2D("M30 39C25 34 17 31 7 24C9 34 10 42 14 47C20 47 25 49 30 53Z"),
  right: new Path2D("M34 39C39 34 47 31 57 24C55 34 54 42 50 47C44 47 39 49 34 53Z"),
  saber: new Path2D("M30 8H34V40H30Z"),
}

/** "8.00 TB" → "8 TB", "1.50 TB" → "1.5 TB". */
const capacity = (n: number) => (n > 0 ? formatBytes(n).replace(/\.?0+ /, " ") : "")

/**
 * Splits text into at most maxLines lines of maxWidth, or null when it doesn't
 * fit. Lines break at spaces and after "-", "_", "." and "/", so names like
 * "backup-tv-2019-2024" wrap too.
 */
function wrap(ctx: CanvasRenderingContext2D, text: string, maxWidth: number, maxLines: number): string[] | null {
  const lines: string[] = []
  let line = ""
  for (const word of text.split(/\s+/).filter(Boolean)) {
    for (const [i, piece] of (word.match(/[^-_./]*[-_./]+|[^-_./]+/g) ?? [word]).entries()) {
      const sep = line && i === 0 ? " " : ""
      if (ctx.measureText(line + sep + piece).width <= maxWidth) {
        line += sep + piece
        continue
      }
      if (!line || ctx.measureText(piece).width > maxWidth) return null
      lines.push(line)
      line = piece
    }
  }
  if (line) lines.push(line)
  return lines.length <= maxLines ? lines : null
}

/** Cuts text with "…" so that it fits in maxWidth. */
function ellipsis(ctx: CanvasRenderingContext2D, text: string, maxWidth: number) {
  if (ctx.measureText(text).width <= maxWidth) return text
  let t = text
  while (t && ctx.measureText(`${t}…`).width > maxWidth) t = t.slice(0, -1)
  return `${t.trimEnd()}…`
}

/** Draws the label of a disk: name, kind, capacity and model, serial and the QR code of its page. */
export function drawDiskLabel(canvas: HTMLCanvasElement, disk: Disk, url: string) {
  canvas.width = LABEL_WIDTH
  canvas.height = LABEL_HEIGHT
  const ctx = canvas.getContext("2d")!
  ctx.fillStyle = "#fff"
  ctx.fillRect(0, 0, LABEL_WIDTH, LABEL_HEIGHT)
  ctx.fillStyle = ctx.strokeStyle = "#000"
  ctx.textBaseline = "alphabetic"

  // Rounded frame, a little inside the edge so the printer doesn't cut it.
  const inset = mm(1.5)
  ctx.lineWidth = mm(0.6)
  ctx.beginPath()
  ctx.roundRect(inset, inset, LABEL_WIDTH - 2 * inset, LABEL_HEIGHT - 2 * inset, mm(3))
  ctx.stroke()

  const pad = mm(5)
  const right = LABEL_WIDTH - pad

  // QR code at the top right, a whole number of pixels per module so it stays sharp.
  const qr = encode(url, { ecc: "M", border: 0 })
  const cell = Math.floor(mm(30) / qr.size)
  const qrSize = cell * qr.size
  const qrX = right - qrSize
  const qrY = pad
  qr.data.forEach((row, y) => row.forEach((dark, x) => dark && ctx.fillRect(qrX + x * cell, qrY + y * cell, cell, cell)))

  // Footer: dashed line, logo and name of the app, serial number.
  const footY = LABEL_HEIGHT - pad - mm(6.5)
  ctx.lineWidth = mm(0.3)
  ctx.setLineDash([mm(1.2), mm(0.8)])
  ctx.beginPath()
  ctx.moveTo(pad, footY)
  ctx.lineTo(right, footY)
  ctx.stroke()
  ctx.setLineDash([])

  const base = LABEL_HEIGHT - pad - mm(0.6)
  const logo = mm(4.2)
  ctx.save()
  ctx.translate(pad, base - logo + mm(0.6))
  ctx.scale(logo / 64, logo / 64)
  ctx.fill(LOGO.box)
  ctx.fillStyle = "#fff"
  ctx.fill(LOGO.left)
  ctx.fill(LOGO.right)
  ctx.fill(LOGO.saber)
  ctx.restore()
  ctx.font = `700 ${pt(7.5)}px ${SANS}`
  ctx.fillText("bookrr", pad + logo + mm(1.5), base)
  const brandEnd = pad + logo + mm(1.5) + ctx.measureText("bookrr").width + mm(4)
  if (disk.serial) {
    ctx.font = `400 ${pt(7)}px ${MONO}`
    ctx.textAlign = "right"
    ctx.fillText(ellipsis(ctx, `SN ${disk.serial}`, right - brandEnd), right, base)
    ctx.textAlign = "left"
  }

  // Left column: kind, name (as big as fits in two lines), capacity and model.
  const textW = qrX - mm(4) - pad
  let y = pad + pt(6)
  ctx.font = `600 ${pt(6)}px ${SANS}`
  if ("letterSpacing" in ctx) ctx.letterSpacing = `${mm(0.5)}px`
  ctx.fillText(ellipsis(ctx, (disk.kind || "Disk").toUpperCase(), textW), pad, y)
  if ("letterSpacing" in ctx) ctx.letterSpacing = "0px"

  // As big as fits in two lines, then smaller in three; a word longer than the column is cut.
  let size = 18
  let lines: string[] | null = null
  for (; size >= 8 && !lines; size -= 0.5) {
    ctx.font = `800 ${pt(size)}px ${SANS}`
    lines = wrap(ctx, disk.label, textW, size >= 11 ? 2 : 3)
  }
  size += 0.5
  if (!lines) lines = [ellipsis(ctx, disk.label, textW)]
  y += mm(1.5)
  for (const l of lines) {
    y += pt(size) * 1.02
    ctx.fillText(l, pad, y)
  }

  const details = [capacity(disk.capacity), disk.model].filter(Boolean).join(" · ")
  if (details) {
    // One line, down to 7 pt; then two lines.
    let ds = 9
    for (; ds > 7; ds -= 0.5) {
      ctx.font = `400 ${pt(ds)}px ${SANS}`
      if (ctx.measureText(details).width <= textW) break
    }
    ctx.font = `400 ${pt(ds)}px ${SANS}`
    y += mm(1.5) + pt(ds)
    const fit = wrap(ctx, details, textW, 2) ?? [ellipsis(ctx, details, textW)]
    for (const [i, l] of fit.entries()) ctx.fillText(l, pad, y + i * pt(ds) * 1.2)
  }
}

/** CRC-32 of a PNG chunk (type and data). */
function crc32(bytes: Uint8Array) {
  let c = ~0
  for (const b of bytes) {
    c ^= b
    for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1))
  }
  return ~c >>> 0
}

/** Adds the resolution (pHYs) to a PNG, so that apps that read it print it at 80×50 mm. */
async function withDPI(png: Blob, dpi: number) {
  const src = new Uint8Array(await png.arrayBuffer())
  const chunk = new Uint8Array(21)
  const view = new DataView(chunk.buffer)
  const ppm = Math.round(dpi / 0.0254)
  view.setUint32(0, 9)
  chunk.set([0x70, 0x48, 0x59, 0x73], 4) // "pHYs"
  view.setUint32(8, ppm)
  view.setUint32(12, ppm)
  chunk[16] = 1 // metre
  view.setUint32(17, crc32(chunk.subarray(4, 17)))
  const at = 8 + 25 // right after the signature and IHDR
  return new Blob([src.subarray(0, at), chunk, src.subarray(at)], { type: "image/png" })
}

/** The label as a PNG of 945×591 px (80×50 mm at 300 dpi). */
export async function renderDiskLabel(disk: Disk, url: string): Promise<Blob | null> {
  const canvas = document.createElement("canvas")
  drawDiskLabel(canvas, disk, url)
  const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"))
  return png && withDPI(png, DPI)
}
