import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { api, type Disk } from "@/lib/api"
import { formatBytes, parseBytes } from "@/lib/format"
import { useAction } from "@/lib/queries"

export const diskKinds = ["HDD", "SSD", "NVMe", "USB", "NAS", "Nastro", "Cloud", "Altro"]

export function DiskFormDialog({
  open,
  disk,
  onOpenChange,
  onSaved,
}: {
  open: boolean
  disk?: Disk | null
  onOpenChange: (open: boolean) => void
  onSaved?: (d: Disk) => void
}) {
  const [form, setForm] = useState({ label: "", kind: "HDD", serial: "", model: "", capacity: "", place: "", notes: "" })
  useEffect(() => {
    if (!open) return
    setForm({
      label: disk?.label ?? "",
      kind: disk?.kind || "HDD",
      serial: disk?.serial ?? "",
      model: disk?.model ?? "",
      capacity: disk?.capacity ? formatBytes(disk.capacity) : "",
      place: disk?.place ?? "",
      notes: disk?.notes ?? "",
    })
  }, [open, disk])

  const save = useAction(
    (d: Partial<Disk>) => (disk ? api.updateDisk(disk.id, d) : api.createDisk(d)),
    disk ? "Disco aggiornato" : "Disco aggiunto"
  )
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))

  const submit = (e: React.FormEvent) => {
    // The dialog can be opened from inside another form (e.g. the resolve dialog).
    e.preventDefault()
    e.stopPropagation()
    save.mutate(
      { ...form, capacity: parseBytes(form.capacity) },
      {
        onSuccess: (d) => {
          onSaved?.(d)
          onOpenChange(false)
        },
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{disk ? "Modifica disco" : "Nuovo disco"}</DialogTitle>
            <DialogDescription>Un disco o un altro archivio dove conservi i torrent offline.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-label">Nome *</Label>
              <Input id="disk-label" value={form.label} onChange={set("label")} placeholder="Archivio 01" required autoFocus />
            </div>
            <div className="grid gap-2">
              <Label>Tipo</Label>
              <Select value={form.kind} onValueChange={(v) => setForm((f) => ({ ...f, kind: v }))}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {diskKinds.map((k) => (
                    <SelectItem key={k} value={k}>
                      {k}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="disk-serial">Numero di serie</Label>
              <Input id="disk-serial" value={form.serial} onChange={set("serial")} placeholder="WD-WCC4E1234567" className="font-mono" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="disk-model">Modello</Label>
              <Input id="disk-model" value={form.model} onChange={set("model")} placeholder="WD Red 4TB" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="disk-capacity">Capacità</Label>
              <Input id="disk-capacity" value={form.capacity} onChange={set("capacity")} placeholder="4 TB" />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-place">Dove si trova</Label>
              <Input id="disk-place" value={form.place} onChange={set("place")} placeholder="Cassetto ufficio, slot 2 del NAS…" />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-notes">Note</Label>
              <Textarea id="disk-notes" value={form.notes} onChange={set("notes")} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Annulla
            </Button>
            <Button type="submit" disabled={save.isPending}>
              Salva
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
