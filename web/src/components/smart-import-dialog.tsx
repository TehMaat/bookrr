import { useEffect, useRef, useState } from "react"
import { TriangleAlert, Upload } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { api, type Disk, type SmartResult } from "@/lib/api"
import { formatBytes, formatHours, healthVariant } from "@/lib/format"
import { useAction, useDisks } from "@/lib/queries"

const NEW = "new"

/**
 * Creates or updates a disk from a SMART report: the report is analyzed first
 * (dry run), then saved on the chosen disk.
 */
export function SmartImportDialog({
  open,
  disk,
  onOpenChange,
}: {
  open: boolean
  /** Disk to update; when missing it is matched by serial number. */
  disk?: Disk | null
  onOpenChange: (open: boolean) => void
}) {
  const disks = useDisks()
  const fileInput = useRef<HTMLInputElement>(null)
  const [report, setReport] = useState("")
  const [preview, setPreview] = useState<SmartResult | null>(null)
  const [analyzing, setAnalyzing] = useState(false)
  const [target, setTarget] = useState(NEW)
  const [label, setLabel] = useState("")

  useEffect(() => {
    if (!open) return
    setReport("")
    setPreview(null)
  }, [open])

  const save = useAction(
    () => api.importSmart(report, target === NEW ? { newLabel: label } : { id: Number(target) }),
    target === NEW ? "Disk created from SMART data" : "Disk updated from SMART data"
  )

  const analyze = async (e: React.FormEvent) => {
    // The dialog can be opened from inside another form.
    e.preventDefault()
    e.stopPropagation()
    setAnalyzing(true)
    try {
      const r = await api.importSmart(report, { id: disk?.id, dryRun: true })
      setPreview(r)
      setTarget(r.created ? NEW : String(r.disk.id))
      setLabel(r.created ? r.disk.label : r.smart.model || r.smart.serial)
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setAnalyzing(false)
    }
  }

  const loadFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = ""
    if (f) setReport(await f.text())
  }

  const confirm = (e: React.FormEvent) => {
    e.preventDefault()
    e.stopPropagation()
    save.mutate(undefined, { onSuccess: () => onOpenChange(false) })
  }

  const s = preview?.smart
  const matched = preview && !preview.created && !disk ? preview.disk : null
  const chosen = target === NEW ? null : disks.data?.find((d) => String(d.id) === target)
  const otherSerial = chosen?.serial && s?.serial && chosen.serial.trim().toLowerCase() !== s.serial.toLowerCase()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        {!preview ? (
          <form onSubmit={analyze} className="grid min-w-0 gap-4">
            <DialogHeader>
              <DialogTitle>{disk ? `Update "${disk.label}" from SMART` : "Import from SMART"}</DialogTitle>
              <DialogDescription>
                Run <code className="font-mono">sudo smartctl -a /dev/sdX</code> (or <code className="font-mono">-x</code>,{" "}
                <code className="font-mono">-j</code> for JSON) and paste the output here. Text copied from CrystalDiskInfo
                works too.
                {!disk && " If a disk with the same serial number already exists it is updated, otherwise a new one is created."}
              </DialogDescription>
            </DialogHeader>
            <Textarea
              value={report}
              onChange={(e) => setReport(e.target.value)}
              placeholder={"=== START OF INFORMATION SECTION ===\nDevice Model:     WDC WD40EFRX-68N32N0\nSerial Number:    WD-WCC7K1234567\n…"}
              className="field-sizing-fixed h-64 resize-y font-mono text-xs"
              spellCheck={false}
              autoFocus
            />
            <input ref={fileInput} type="file" accept=".txt,.log,.json,text/*,application/json" hidden onChange={loadFile} />
            <DialogFooter className="sm:justify-between">
              <Button type="button" variant="outline" onClick={() => fileInput.current?.click()}>
                <Upload /> Load file
              </Button>
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={analyzing || report.trim() === ""}>
                  Analyze
                </Button>
              </div>
            </DialogFooter>
          </form>
        ) : (
          <form onSubmit={confirm} className="grid min-w-0 gap-4">
            <DialogHeader>
              <DialogTitle>SMART data read</DialogTitle>
              <DialogDescription>
                Model, serial, capacity, firmware, health and power-on hours are copied to the disk; name, location and notes
                stay as they are.
              </DialogDescription>
            </DialogHeader>
            {s && (
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
                <dt className="text-muted-foreground">Model</dt>
                <dd className="min-w-0 break-words">{s.model || "—"}</dd>
                <dt className="text-muted-foreground">Serial number</dt>
                <dd className="font-mono break-all">{s.serial || "—"}</dd>
                <dt className="text-muted-foreground">Capacity</dt>
                <dd>{formatBytes(s.capacity)}</dd>
                <dt className="text-muted-foreground">Kind</dt>
                <dd>{s.kind || "—"}</dd>
                <dt className="text-muted-foreground">Firmware</dt>
                <dd className="font-mono">{s.firmware || "—"}</dd>
                <dt className="text-muted-foreground">SMART health</dt>
                <dd>{s.health ? <Badge variant={healthVariant(s.health)}>{s.health}</Badge> : "—"}</dd>
                <dt className="text-muted-foreground">Power-on hours</dt>
                <dd>{formatHours(s.powerOnHours)}</dd>
              </dl>
            )}
            {s && !s.serial && (
              <Alert variant="warning">
                <TriangleAlert />
                <AlertDescription>No serial number in the data: choose the disk to update yourself.</AlertDescription>
              </Alert>
            )}
            {otherSerial && (
              <Alert variant="warning">
                <TriangleAlert />
                <AlertDescription>
                  "{chosen.label}" has a different serial number ({chosen.serial}): it will be replaced with the one read.
                </AlertDescription>
              </Alert>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label>Disk</Label>
                <Select value={target} onValueChange={setTarget}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NEW}>New disk</SelectItem>
                    {disks.data?.map((d) => (
                      <SelectItem key={d.id} value={String(d.id)}>
                        {d.label}
                        {d.serial && <span className="text-muted-foreground font-mono text-xs">{d.serial}</span>}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {matched && target === String(matched.id) && (
                  <p className="text-muted-foreground text-xs">Recognized by its serial number.</p>
                )}
              </div>
              {target === NEW && (
                <div className="grid gap-2">
                  <Label htmlFor="smart-label">Name *</Label>
                  <Input id="smart-label" value={label} onChange={(e) => setLabel(e.target.value)} required />
                </div>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setPreview(null)}>
                Back
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {target === NEW ? "Create disk" : "Update disk"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
