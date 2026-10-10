import { useEffect, useState } from "react"
import { Archive, HandHeart, MessageSquareText, Trash2 } from "lucide-react"

import { AdopterSelect } from "@/components/adopter-select"
import { ArchiveFields, emptyArchive } from "@/components/archive-fields"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api, type Alert, type ArchiveInput, type ResolveInput } from "@/lib/api"
import { formatDate } from "@/lib/format"
import { useAction, useAdopters, useDisks } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Kind = "disk" | "adopted" | "deleted" | "other"

const kinds: { value: Kind; label: string; icon: React.ElementType }[] = [
  { value: "disk", label: "Spostato su disco", icon: Archive },
  { value: "adopted", label: "Adottato da qualcuno", icon: HandHeart },
  { value: "deleted", label: "Eliminato", icon: Trash2 },
  { value: "other", label: "Altro", icon: MessageSquareText },
]

/** Asks where the torrents of one or more alerts went; `alerts` empty means closed. */
export function ResolveDialog({ alerts, onClose }: { alerts: Alert[]; onClose: () => void }) {
  const [kind, setKind] = useState<Kind>("disk")
  const [archive, setArchive] = useState<ArchiveInput>(emptyArchive)
  const [adopterId, setAdopterId] = useState<number | null>(null)
  const [note, setNote] = useState("")
  const disks = useDisks()
  const adopters = useAdopters()

  const open = alerts.length > 0
  const alert = alerts.length === 1 ? alerts[0] : null

  useEffect(() => {
    if (open) {
      setKind("disk")
      setArchive(emptyArchive)
      setAdopterId(null)
      setNote("")
    }
  }, [open])

  const resolve = useAction(
    (r: ResolveInput) =>
      alerts.length === 1 ? api.resolveAlert(alerts[0].id, r) : api.resolveAlerts(alerts.map((a) => a.id), r),
    alerts.length === 1 ? "Posizione registrata" : `Posizione registrata per ${alerts.length} release`
  )

  const disk = disks.data?.find((d) => d.id === archive.diskId)
  const adopter = adopters.data?.find((a) => a.id === adopterId)
  const valid =
    (kind === "disk" && (archive.diskId !== null || archive.path.trim() !== "")) ||
    (kind === "adopted" && adopterId !== null) ||
    kind === "deleted" ||
    (kind === "other" && note.trim() !== "")

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const parts: string[] = []
    const r: ResolveInput = { resolution: "" }
    if (kind === "disk") {
      r.archive = archive
      parts.push(`Spostato su ${disk ? `${disk.label}${disk.serial ? ` (SN ${disk.serial})` : ""}` : "archivio"}${archive.path ? ` · ${archive.path}` : ""}`)
    }
    if ((kind === "disk" || kind === "adopted") && adopter) {
      r.adoption = { adopterId: adopter.id, notes: "" }
      parts.push(`Adottato da ${adopter.name}`)
    }
    if (kind === "deleted") parts.push("Eliminato definitivamente")
    if (note.trim()) parts.push(note.trim())
    r.resolution = parts.join(" — ")
    resolve.mutate(r, { onSuccess: onClose })
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{alert ? "Dove è stato spostato?" : `Dove sono state spostate ${alerts.length} release?`}</DialogTitle>
            {alert ? (
              <DialogDescription className="break-all">
                <span className="text-foreground font-medium">{alert.torrentName}</span>
                <br />
                {alert.message} · {formatDate(alert.createdAt)} · via {alert.source === "s3" ? "lettura del bucket" : alert.source === "webhook" ? "webhook" : "sincronizzazione"}
              </DialogDescription>
            ) : (
              <DialogDescription asChild>
                <div>
                  La stessa posizione verrà registrata per tutte:
                  <ul className="mt-1.5 max-h-32 overflow-y-auto rounded-md border px-3 py-1.5">
                    {alerts.map((a) => (
                      <li key={a.id} className="text-foreground truncate text-xs font-medium" title={a.torrentName}>
                        {a.torrentName}
                      </li>
                    ))}
                  </ul>
                </div>
              </DialogDescription>
            )}
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
              <AdopterSelect id="adopted-by" value={adopterId} onChange={setAdopterId} />
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
