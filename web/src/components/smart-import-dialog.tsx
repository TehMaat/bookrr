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
    target === NEW ? "Disco creato dai dati SMART" : "Disco aggiornato dai dati SMART"
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
              <DialogTitle>{disk ? `Aggiorna "${disk.label}" da SMART` : "Importa da SMART"}</DialogTitle>
              <DialogDescription>
                Esegui <code className="font-mono">sudo smartctl -a /dev/sdX</code> (o <code className="font-mono">-x</code>,{" "}
                <code className="font-mono">-j</code> per il JSON) e incolla qui l'output. Va bene anche il testo copiato da
                CrystalDiskInfo.
                {!disk && " Se esiste già un disco con lo stesso numero di serie viene aggiornato, altrimenti ne viene creato uno nuovo."}
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
                <Upload /> Carica file
              </Button>
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  Annulla
                </Button>
                <Button type="submit" disabled={analyzing || report.trim() === ""}>
                  Analizza
                </Button>
              </div>
            </DialogFooter>
          </form>
        ) : (
          <form onSubmit={confirm} className="grid min-w-0 gap-4">
            <DialogHeader>
              <DialogTitle>Dati SMART letti</DialogTitle>
              <DialogDescription>
                Modello, seriale, capacità, firmware, stato e ore di accensione vengono copiati sul disco; nome, posizione e note
                restano invariati.
              </DialogDescription>
            </DialogHeader>
            {s && (
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
                <dt className="text-muted-foreground">Modello</dt>
                <dd className="min-w-0 break-words">{s.model || "—"}</dd>
                <dt className="text-muted-foreground">Numero di serie</dt>
                <dd className="font-mono break-all">{s.serial || "—"}</dd>
                <dt className="text-muted-foreground">Capacità</dt>
                <dd>{formatBytes(s.capacity)}</dd>
                <dt className="text-muted-foreground">Tipo</dt>
                <dd>{s.kind || "—"}</dd>
                <dt className="text-muted-foreground">Firmware</dt>
                <dd className="font-mono">{s.firmware || "—"}</dd>
                <dt className="text-muted-foreground">Stato SMART</dt>
                <dd>{s.health ? <Badge variant={healthVariant(s.health)}>{s.health}</Badge> : "—"}</dd>
                <dt className="text-muted-foreground">Ore di accensione</dt>
                <dd>{formatHours(s.powerOnHours)}</dd>
              </dl>
            )}
            {s && !s.serial && (
              <Alert variant="warning">
                <TriangleAlert />
                <AlertDescription>Nessun numero di serie nei dati: scegli tu il disco da aggiornare.</AlertDescription>
              </Alert>
            )}
            {otherSerial && (
              <Alert variant="warning">
                <TriangleAlert />
                <AlertDescription>
                  "{chosen.label}" ha un altro numero di serie ({chosen.serial}): verrà sostituito con quello letto.
                </AlertDescription>
              </Alert>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label>Disco</Label>
                <Select value={target} onValueChange={setTarget}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NEW}>Nuovo disco</SelectItem>
                    {disks.data?.map((d) => (
                      <SelectItem key={d.id} value={String(d.id)}>
                        {d.label}
                        {d.serial && <span className="text-muted-foreground font-mono text-xs">{d.serial}</span>}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {matched && target === String(matched.id) && (
                  <p className="text-muted-foreground text-xs">Riconosciuto dal numero di serie.</p>
                )}
              </div>
              {target === NEW && (
                <div className="grid gap-2">
                  <Label htmlFor="smart-label">Nome *</Label>
                  <Input id="smart-label" value={label} onChange={(e) => setLabel(e.target.value)} required />
                </div>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setPreview(null)}>
                Indietro
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {target === NEW ? "Crea disco" : "Aggiorna disco"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
