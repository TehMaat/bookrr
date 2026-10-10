import { useEffect, useState } from "react"

import { AdopterSelect } from "@/components/adopter-select"
import { ArchiveFields, emptyArchive } from "@/components/archive-fields"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api, type ArchiveInput, type TorrentInput } from "@/lib/api"
import { buildTorrentInput, isValidHash } from "@/lib/manual"
import { useAction, useInfo } from "@/lib/queries"

export function TorrentFormDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const info = useInfo()
  const [name, setName] = useState("")
  const [hash, setHash] = useState("")
  const [size, setSize] = useState("")
  const [tags, setTags] = useState("")
  const [personal, setPersonal] = useState(true)
  const [adopterId, setAdopterId] = useState<number | null>(null)
  const [notes, setNotes] = useState("")
  const [archive, setArchive] = useState<ArchiveInput>(emptyArchive)

  useEffect(() => {
    if (!open) return
    setName("")
    setHash("")
    setSize("")
    setTags("")
    setPersonal(true)
    setAdopterId(null)
    setNotes("")
    setArchive(emptyArchive)
  }, [open])

  const create = useAction((t: TorrentInput) => api.createTorrent(t), "Torrent aggiunto")

  const hashValid = isValidHash(hash)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    create.mutate(buildTorrentInput({ name, hash, size, tags, personal, notes, adopterId, archive }), {
      onSuccess: () => onOpenChange(false),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Aggiungi torrent manualmente</DialogTitle>
            <DialogDescription>
              Per i torrent che non sono su nessun client: archiviati, adottati o di cui vuoi solo tenere traccia.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="t-name">Nome *</Label>
              <Input id="t-name" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="t-hash">Info hash</Label>
              <Input
                id="t-hash"
                value={hash}
                onChange={(e) => setHash(e.target.value)}
                placeholder={
                  info.data?.unit3d
                    ? "Opzionale: se lo lasci vuoto, bookrr lo cerca per nome sul tracker UNIT3D"
                    : "Opzionale: se lo inserisci, bookrr lo riconosce quando torna su un client"
                }
                className="font-mono text-sm"
                aria-invalid={!hashValid}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="t-size">Dimensione</Label>
              <Input id="t-size" value={size} onChange={(e) => setSize(e.target.value)} placeholder="12.5 GB" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="t-tags">Tag (separati da virgola)</Label>
              <Input id="t-tags" value={tags} onChange={(e) => setTags(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="t-adopted">Adottato da</Label>
              <AdopterSelect id="t-adopted" value={adopterId} onChange={setAdopterId} />
            </div>
            <div className="flex items-center gap-2 pt-6">
              <Switch id="t-personal" checked={personal} onCheckedChange={setPersonal} />
              <Label htmlFor="t-personal">Personal Release</Label>
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="t-notes">Note</Label>
              <Textarea id="t-notes" value={notes} onChange={(e) => setNotes(e.target.value)} />
            </div>
          </div>
          <Separator />
          <div>
            <h4 className="mb-3 text-sm font-semibold">Spostato su disco (opzionale)</h4>
            <ArchiveFields value={archive} onChange={setArchive} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Annulla
            </Button>
            <Button type="submit" disabled={!name.trim() || !hashValid || create.isPending}>
              Aggiungi
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
