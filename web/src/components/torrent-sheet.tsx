import { useEffect, useState } from "react"
import { Check, Copy, HardDrive, Plus, Server, Trash2, TriangleAlert } from "lucide-react"
import { toast } from "sonner"

import { ArchiveFields, emptyArchive } from "@/components/archive-fields"
import { DuplicateBadge, PersonalBadge, StatusBadge } from "@/components/torrent-badges"
import { Alert as AlertBox, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api, type Alert, type ArchiveInput, type Torrent, type TorrentPatch } from "@/lib/api"
import { formatBytes, formatDate, formatState, isErrorState } from "@/lib/format"
import { useAction } from "@/lib/queries"

function Section({ title, icon: Icon, children, action }: { title: string; icon?: React.ElementType; children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <section className="grid gap-3">
      <div className="flex items-center gap-2">
        {Icon && <Icon className="text-muted-foreground size-4" />}
        <h3 className="text-sm font-semibold">{title}</h3>
        <div className="ml-auto">{action}</div>
      </div>
      {children}
    </section>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[10rem_1fr] gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 break-all">{children}</span>
    </div>
  )
}

export function TorrentSheet({
  torrent,
  onClose,
  onResolve,
}: {
  torrent: Torrent | null
  onClose: () => void
  onResolve: (a: Alert) => void
}) {
  const t = torrent
  const [adoptedBy, setAdoptedBy] = useState("")
  const [notes, setNotes] = useState("")
  const [name, setName] = useState("")
  const [addingArchive, setAddingArchive] = useState(false)
  const [archive, setArchive] = useState<ArchiveInput>(emptyArchive)
  const [copied, setCopied] = useState(false)

  const hash = t?.hash
  useEffect(() => {
    if (!t) return
    setAdoptedBy(t.adoptedBy)
    setNotes(t.notes)
    setName(t.name)
    setAddingArchive(false)
    setArchive(emptyArchive)
    // Reset only when switching torrent, not on background refreshes.
  }, [hash])

  const update = useAction((p: TorrentPatch) => api.updateTorrent(hash!, p), "Salvato")
  const addArchive = useAction((a: ArchiveInput) => api.addArchive(hash!, a), "Archivio aggiunto")
  const delArchive = useAction((id: number) => api.deleteArchive(id), "Archivio rimosso")
  const del = useAction(() => api.deleteTorrent(hash!), "Torrent eliminato")

  if (!t) return <Sheet open={false} />

  const onClient = t.locations.length > 0
  const dirty = adoptedBy !== t.adoptedBy || notes !== t.notes || (t.manual && name !== t.name)

  const copyHash = async () => {
    try {
      await navigator.clipboard.writeText(t.hash)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error("Copia non riuscita")
    }
  }

  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader className="pr-10">
          <SheetTitle className="leading-snug break-all">{t.name}</SheetTitle>
          <SheetDescription asChild>
            <div className="flex flex-wrap items-center gap-1.5">
              <StatusBadge status={t.status} />
              {t.personalRelease && <PersonalBadge />}
              {t.duplicate && <DuplicateBadge count={t.locations.length} />}
              {t.manual && <Badge variant="outline">Manuale</Badge>}
              {t.tags.map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                </Badge>
              ))}
            </div>
          </SheetDescription>
        </SheetHeader>

        <div className="grid gap-6 px-4 pb-6">
          {t.openAlert && (
            <AlertBox variant="warning">
              <TriangleAlert />
              <AlertTitle>Rimosso: dove è stato spostato?</AlertTitle>
              <AlertDescription>
                <p>
                  {t.openAlert.message} il {formatDate(t.openAlert.createdAt)}.
                </p>
                <Button size="sm" className="mt-1" onClick={() => onResolve(t.openAlert!)}>
                  Indica posizione
                </Button>
              </AlertDescription>
            </AlertBox>
          )}

          {t.duplicate && (
            <AlertBox variant="destructive">
              <Copy />
              <AlertTitle>Presente su {t.locations.length} client</AlertTitle>
              <AlertDescription>Lo stesso torrent (stesso hash) è caricato su più client.</AlertDescription>
            </AlertBox>
          )}

          <div className="grid gap-1.5">
            <Field label="Hash">
              <span className="inline-flex items-center gap-1 font-mono text-xs">
                {t.hash}
                <Button variant="ghost" size="icon-sm" className="size-6" onClick={copyHash} aria-label="Copia hash">
                  {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
                </Button>
              </span>
            </Field>
            <Field label="Dimensione">{formatBytes(t.size)}</Field>
            {t.category && <Field label="Categoria">{t.category}</Field>}
            {t.tracker && <Field label="Tracker">{t.tracker}</Field>}
            <Field label="Visto la prima volta">{formatDate(t.firstSeenAt)}</Field>
            <Field label="Ultima volta su client">{formatDate(t.lastSeenAt)}</Field>
          </div>

          <Separator />

          <Section title={`Sui client (${t.locations.length})`} icon={Server}>
            {t.locations.length === 0 ? (
              <p className="text-muted-foreground text-sm">Non presente su nessun client.</p>
            ) : (
              <ul className="grid gap-2">
                {t.locations.map((l) => (
                  <li key={l.clientId} className="rounded-lg border p-3 text-sm">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{l.clientName}</span>
                      <Badge variant={isErrorState(l.state) ? "destructive" : "secondary"}>{formatState(l.state)}</Badge>
                      <span className="text-muted-foreground ml-auto text-xs">
                        {(l.progress * 100).toFixed(0)}% · ratio {l.ratio.toFixed(2)}
                      </span>
                    </div>
                    <div className="text-muted-foreground mt-1 font-mono text-xs break-all">{l.savePath}</div>
                    {(l.category || l.tags) && (
                      <div className="text-muted-foreground mt-1 text-xs">
                        {l.category && <>Categoria: {l.category} </>}
                        {l.tags && <>· Tag: {l.tags.split(",").join(", ")}</>}
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Section>

          <Separator />

          <Section
            title={`Archivi (${t.archives.length})`}
            icon={HardDrive}
            action={
              !addingArchive && (
                <Button size="sm" variant="outline" onClick={() => setAddingArchive(true)}>
                  <Plus /> Aggiungi
                </Button>
              )
            }
          >
            {t.archives.length === 0 && !addingArchive && (
              <p className="text-muted-foreground text-sm">Nessun archivio registrato.</p>
            )}
            <ul className="grid gap-2">
              {t.archives.map((a) => (
                <li key={a.id} className="flex items-start gap-2 rounded-lg border p-3 text-sm">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{a.diskLabel || "Senza disco"}</span>
                      {a.diskKind && <Badge variant="outline">{a.diskKind}</Badge>}
                      {a.diskSerial && <span className="text-muted-foreground font-mono text-xs">SN {a.diskSerial}</span>}
                    </div>
                    {a.path && <div className="text-muted-foreground mt-1 font-mono text-xs break-all">{a.path}</div>}
                    {a.notes && <div className="mt-1 text-xs">{a.notes}</div>}
                  </div>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => delArchive.mutate(a.id)}
                    aria-label="Rimuovi archivio"
                  >
                    <Trash2 />
                  </Button>
                </li>
              ))}
            </ul>
            {addingArchive && (
              <div className="grid gap-3 rounded-lg border p-3">
                <ArchiveFields value={archive} onChange={setArchive} />
                <div className="flex justify-end gap-2">
                  <Button variant="outline" size="sm" onClick={() => setAddingArchive(false)}>
                    Annulla
                  </Button>
                  <Button
                    size="sm"
                    disabled={(archive.diskId === null && !archive.path.trim()) || addArchive.isPending}
                    onClick={() =>
                      addArchive.mutate(archive, {
                        onSuccess: () => {
                          setAddingArchive(false)
                          setArchive(emptyArchive)
                        },
                      })
                    }
                  >
                    Salva archivio
                  </Button>
                </div>
              </div>
            )}
          </Section>

          <Separator />

          <Section title="Dati personali">
            {t.manual && (
              <div className="grid gap-2">
                <Label htmlFor="ts-name">Nome</Label>
                <Input id="ts-name" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
            )}
            <div className="grid gap-2">
              <Label htmlFor="ts-adopted">Adottato da</Label>
              <Input
                id="ts-adopted"
                value={adoptedBy}
                onChange={(e) => setAdoptedBy(e.target.value)}
                placeholder="Nessuno"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ts-notes">Note</Label>
              <Textarea id="ts-notes" value={notes} onChange={(e) => setNotes(e.target.value)} />
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="ts-personal"
                checked={t.personalRelease}
                disabled={onClient}
                onCheckedChange={(v) => update.mutate({ personalRelease: v })}
              />
              <Label htmlFor="ts-personal">Personal Release</Label>
              {onClient && <span className="text-muted-foreground text-xs">(dal tag sul client)</span>}
            </div>
            <div className="flex justify-end">
              <Button
                disabled={!dirty || update.isPending}
                onClick={() =>
                  update.mutate({
                    adoptedBy,
                    notes,
                    ...(t.manual && name.trim() ? { name: name.trim() } : {}),
                  })
                }
              >
                Salva
              </Button>
            </div>
          </Section>

          {!onClient && (
            <>
              <Separator />
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button variant="outline" className="text-destructive w-fit">
                    <Trash2 /> Elimina da bookrr
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Eliminare questo torrent?</AlertDialogTitle>
                    <AlertDialogDescription>
                      Verranno cancellati da bookrr anche archivi, adozione, note e segnalazioni. I file sui dischi non
                      vengono toccati.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Annulla</AlertDialogCancel>
                    <AlertDialogAction
                      className="bg-destructive hover:bg-destructive/90 text-white"
                      onClick={() => del.mutate(undefined, { onSuccess: onClose })}
                    >
                      Elimina
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
