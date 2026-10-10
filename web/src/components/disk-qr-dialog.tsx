import { useEffect, useMemo, useState } from "react"
import { Download, Printer, TriangleAlert } from "lucide-react"
import { renderSVG } from "uqr"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type Disk } from "@/lib/api"
import { useAction, useSettings } from "@/lib/queries"

/** "192.168.1.10:8080/" → "http://192.168.1.10:8080", as the server saves it. */
function normalizeBase(base: string) {
  const b = base.trim().replace(/\/+$/, "")
  if (!b) return window.location.origin
  return b.includes("://") ? b : `http://${b}`
}

/** Link to the page with the torrents archived on a disk. */
export const diskUrl = (base: string, id: number) => `${normalizeBase(base)}/#disks/${id}`

/** Addresses that only work on the machine running the browser (or inside Docker). */
function unreachable(base: string) {
  try {
    const host = new URL(base).hostname
    return host === "localhost" || host === "0.0.0.0" || host === "[::1]" || host.startsWith("127.")
  } catch {
    return true
  }
}

const escape = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!)

export function DiskQrDialog({ disk, onClose }: { disk: Disk | null; onClose: () => void }) {
  const settings = useSettings()
  const saved = settings.data?.publicUrl ?? ""
  const [base, setBase] = useState("")
  useEffect(() => {
    if (disk) setBase(saved || window.location.origin)
  }, [disk, saved])

  const save = useAction(() => api.saveSettings({ publicUrl: normalizeBase(base) }), "Indirizzo salvato")
  const url = disk ? diskUrl(base, disk.id) : ""
  const svg = useMemo(() => (url ? renderSVG(url, { ecc: "M", border: 2 }) : ""), [url])
  const changed = normalizeBase(base) !== saved

  const download = () => {
    const a = document.createElement("a")
    a.href = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }))
    a.download = `bookrr-${disk!.label.replace(/[^\w.-]+/g, "_")}.svg`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  const print = () => {
    const w = window.open("", "_blank", "width=420,height=560")
    if (!w) return
    const d = disk!
    w.document.write(`<!doctype html><html><head><meta charset="utf-8"><title>${escape(d.label)}</title>
<style>
  @page { margin: 8mm; }
  body { font-family: system-ui, sans-serif; margin: 0; }
  .label { width: 50mm; text-align: center; }
  .label svg { width: 50mm; height: 50mm; display: block; }
  .name { font-size: 14pt; font-weight: 600; margin-top: 2mm; word-break: break-word; }
  .meta { font-size: 8pt; font-family: ui-monospace, monospace; word-break: break-all; }
</style></head><body><div class="label">${svg}
<div class="name">${escape(d.label)}</div>
${d.serial ? `<div class="meta">SN ${escape(d.serial)}</div>` : ""}
</div><script>window.onafterprint = () => window.close(); window.onload = () => window.print()</script></body></html>`)
    w.document.close()
  }

  return (
    <Dialog open={disk !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>QR code di "{disk?.label}"</DialogTitle>
          <DialogDescription>
            Stampalo e attaccalo al disco: inquadrandolo con il telefono apri l'elenco dei torrent archiviati su questo disco.
          </DialogDescription>
        </DialogHeader>
        <div className="mx-auto size-56 rounded-md bg-white p-1 [&>svg]:size-full" dangerouslySetInnerHTML={{ __html: svg }} />
        <p className="text-muted-foreground text-center font-mono text-xs break-all">{url}</p>
        <div className="grid gap-2">
          <Label htmlFor="public-url">Indirizzo di bookrr nella tua rete</Label>
          <div className="flex gap-2">
            <Input
              id="public-url"
              value={base}
              onChange={(e) => setBase(e.target.value)}
              placeholder="http://192.168.1.10:8080"
              className="font-mono"
            />
            <Button variant="outline" disabled={!changed || save.isPending} onClick={() => save.mutate(undefined)}>
              Salva
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            L'IP (o il nome) e la porta con cui gli altri dispositivi raggiungono il container, ad esempio quelli della
            macchina che ospita Docker. Viene salvato per tutti i dischi.
          </p>
        </div>
        {unreachable(normalizeBase(base)) && (
          <Alert>
            <TriangleAlert />
            <AlertDescription>Questo indirizzo funziona solo da questo computer: indica l'IP della macchina nella tua rete.</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={download}>
            <Download /> Scarica SVG
          </Button>
          <Button onClick={print}>
            <Printer /> Stampa etichetta
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
