import { useEffect, useState } from "react"
import { Archive, HandHeart, MessageSquareText, Trash2 } from "lucide-react"

import { ArchiveFields, emptyArchive } from "@/components/archive-fields"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api, type Alert, type ArchiveInput, type ResolveInput } from "@/lib/api"
import { formatDate } from "@/lib/format"
import { useAction, useDisks } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Kind = "disk" | "adopted" | "deleted" | "other"

const kinds: { value: Kind; label: string; icon: React.ElementType }[] = [
  { value: "disk", label: "Archiviato su disco", icon: Archive },
  { value: "adopted", label: "Adottato da qualcuno", icon: HandHeart },
  { value: "deleted", label: "Eliminato", icon: Trash2 },
  { value: "other", label: "Altro", icon: MessageSquareText },
]

export function ResolveDialog({ alert, onClose }: { alert: Alert | null; onClose: () => void }) {
  const [kind, setKind] = useState<Kind>("disk")
  const [archive, setArchive] = useState<ArchiveInput>(emptyArchive)
  const [adoptedBy, setAdoptedBy] = useState("")
  const [note, setNote] = useState("")
  const disks = useDisks()

  useEffect(() => {
    if (alert) {
      setKind("disk")
      setArchive(emptyArchive)
      setAdoptedBy("")
      setNote("")
    }
  }, [alert])

  const resolve = useAction((r: ResolveInput) => api.resolveAlert(alert!.id, r), "Posizione registrata")

  const disk = disks.data?.find((d) => d.id === archive.diskId)
  const valid =
    (kind === "disk" && (archive.diskId !== null || archive.path.trim() !== "")) ||
    (kind === "adopted" && adoptedBy.trim() !== "") ||
    kind === "deleted" ||
    (kind === "other" && note.trim() !== "")

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const parts: string[] = []
    const r: ResolveInput = { resolution: "" }
    if (kind === "disk") {
      r.archive = archive
      parts.push(`Archiviato su ${disk ? `${disk.label}${disk.serial ? ` (SN ${disk.serial})` : ""}` : "archivio"}${archive.path ? ` · ${archive.path}` : ""}`)
    }
    if ((kind === "disk" || kind === "adopted") && adoptedBy.trim()) {
      r.adoptedBy = adoptedBy.trim()
      parts.push(`Adottato da ${adoptedBy.trim()}`)
    }
    if (kind === "deleted") parts.push("Eliminato definitivamente")
    if (note.trim()) parts.push(note.trim())
    r.resolution = parts.join(" — ")
    resolve.mutate(r, { onSuccess: onClose })
  }

  return (
    <Dialog open={alert !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Dove è stato spostato?</DialogTitle>
            <DialogDescription className="break-all">
              <span className="text-foreground font-medium">{alert?.torrentName}</span>
              <br />
              {alert?.message} · {formatDate(alert?.createdAt)} · via {alert?.source === "webhook" ? "webhook" : "sincronizzazione"}
            </DialogDescription>
          </DialogHeader>

          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {kinds.map((k) => (
              <button
                key={k.value}
                type="button"
                onClick={() => setKind(k.value)}
                className={cn(
                  "hover:bg-accent flex cursor-pointer flex-col items-center gap-1.5 rounded-lg border p-3 text-center text-xs font-medium transition-colors",
                  kind === k.value && "border-primary bg-accent ring-primary/20 ring-2"
                )}
              >
                <k.icon className="size-5" />
                {k.label}
              </button>
            ))}
          </div>

          {kind === "disk" && <ArchiveFields value={archive} onChange={setArchive} />}

          {(kind === "disk" || kind === "adopted") && (
            <div className="grid gap-2">
              <Label htmlFor="adopted-by">{kind === "adopted" ? "Chi l'ha adottato? *" : "Adottato anche da (opzionale)"}</Label>
              <Input id="adopted-by" value={adoptedBy} onChange={(e) => setAdoptedBy(e.target.value)} placeholder="Nome utente" />
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="resolve-note">{kind === "other" ? "Cosa è successo? *" : "Note"}</Label>
            <Textarea id="resolve-note" value={note} onChange={(e) => setNote(e.target.value)} />
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Più tardi
            </Button>
            <Button type="submit" disabled={!valid || resolve.isPending}>
              Conferma
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
