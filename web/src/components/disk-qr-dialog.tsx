import { useEffect, useMemo, useState } from "react"
import { Download, Tag, TriangleAlert } from "lucide-react"
import { encode, renderSVG } from "uqr"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type Disk } from "@/lib/api"
import { drawDiskLabel, renderDiskLabel } from "@/lib/disk-label"
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

/** Same QR as the SVG, drawn on a canvas at a whole number of pixels per module (~1024 px). */
function renderPNG(url: string): Promise<Blob | null> {
  const { data, size } = encode(url, { ecc: "M", border: 2 })
  const scale = Math.ceil(1024 / size)
  const canvas = document.createElement("canvas")
  canvas.width = canvas.height = size * scale
  const ctx = canvas.getContext("2d")!
  ctx.fillStyle = "#fff"
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.fillStyle = "#000"
  data.forEach((row, y) => row.forEach((dark, x) => dark && ctx.fillRect(x * scale, y * scale, scale, scale)))
  return new Promise((resolve) => canvas.toBlob(resolve, "image/png"))
}

export function DiskQrDialog({ disk, onClose }: { disk: Disk | null; onClose: () => void }) {
  const settings = useSettings()
  const saved = settings.data?.publicUrl ?? ""
  const [base, setBase] = useState("")
  useEffect(() => {
    if (disk) setBase(saved || window.location.origin)
  }, [disk, saved])

  const save = useAction(() => api.saveSettings({ publicUrl: normalizeBase(base) }), "Address saved")
  const url = disk ? diskUrl(base, disk.id) : ""
  const svg = useMemo(() => (url ? renderSVG(url, { ecc: "M", border: 2 }) : ""), [url])
  const changed = normalizeBase(base) !== saved
  // The label drawn off screen: the dialog's content mounts after this effect runs.
  const preview = useMemo(() => {
    if (!disk) return ""
    const canvas = document.createElement("canvas")
    drawDiskLabel(canvas, disk, url)
    return canvas.toDataURL("image/png")
  }, [disk, url])

  const saveFile = (blob: Blob, name: string) => {
    const a = document.createElement("a")
    a.href = URL.createObjectURL(blob)
    a.download = `bookrr-${disk!.label.replace(/[^\w.-]+/g, "_")}${name}`
    a.click()
    URL.revokeObjectURL(a.href)
  }
  const downloadSVG = () => saveFile(new Blob([svg], { type: "image/svg+xml" }), ".svg")
  const downloadPNG = async () => {
    const png = await renderPNG(url)
    if (png) saveFile(png, ".png")
  }
  const downloadLabel = async () => {
    const png = await renderDiskLabel(disk!, url)
    if (png) saveFile(png, "-label.png")
  }

  return (
    <Dialog open={disk !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>QR code of "{disk?.label}"</DialogTitle>
          <DialogDescription>
            Print the 80×50 mm label and stick it on the disk: scan it with your phone to open the list of torrents
            archived on this disk.
          </DialogDescription>
        </DialogHeader>
        {preview && (
          <img src={preview} alt={`Label of ${disk?.label}`} className="mx-auto aspect-[8/5] w-full max-w-sm rounded-md shadow-sm" />
        )}
        <p className="text-muted-foreground text-center font-mono text-xs break-all">{url}</p>
        <div className="grid gap-2">
          <Label htmlFor="public-url">bookrr address on your network</Label>
          <div className="flex gap-2">
            <Input
              id="public-url"
              value={base}
              onChange={(e) => setBase(e.target.value)}
              placeholder="http://192.168.1.10:8080"
              className="font-mono"
            />
            <Button variant="outline" disabled={!changed || save.isPending} onClick={() => save.mutate(undefined)}>
              Save
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            The IP (or name) and port other devices use to reach the container, e.g. those of the machine running
            Docker. It is saved for all disks.
          </p>
        </div>
        {unreachable(normalizeBase(base)) && (
          <Alert>
            <TriangleAlert />
            <AlertDescription>This address only works from this computer: enter the IP of the machine on your network.</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={downloadSVG} title="Only the QR code, as SVG">
            <Download /> QR SVG
          </Button>
          <Button variant="outline" onClick={downloadPNG} title="Only the QR code, as PNG">
            <Download /> QR PNG
          </Button>
          <Button onClick={downloadLabel}>
            <Tag /> Label PNG
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
