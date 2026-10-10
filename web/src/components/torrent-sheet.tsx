import { useEffect, useState } from "react"
import {
  Check,
  Copy,
  Download,
  ExternalLink,
  FileDown,
  HandHeart,
  HardDrive,
  ListChecks,
  Plus,
  Search,
  Server,
  Trash2,
  TriangleAlert,
} from "lucide-react"
import { toast } from "sonner"

import { canRemoveDuplicate, RemoveFromClientDialog } from "@/components/remove-from-client-dialog"
import { AdopterSelect } from "@/components/adopter-select"
import { ArchiveFields, emptyArchive } from "@/components/archive-fields"
import { DuplicateBadge, PersonalBadge, StatusBadge } from "@/components/torrent-badges"
import { TrackerPickDialog } from "@/components/tracker-pick-dialog"
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
import {
  api,
  type AdoptionInput,
  type Alert,
  type ArchiveInput,
  type Location,
  type Torrent,
  type TorrentPatch,
} from "@/lib/api"
import { formatBytes, formatDate, formatState, formatUnix, isDownloading, isErrorState } from "@/lib/format"
import { isManualHash } from "@/lib/manual"
import { useAction, useInfo } from "@/lib/queries"

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

const splitTags = (s: string) =>
  s
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean)

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
  onHashChange,
}: {
  torrent: Torrent | null
  onClose: () => void
  onResolve: (a: Alert) => void
  /** The torrent got its real info hash: keep it selected under the new one. */
  onHashChange?: (hash: string) => void
}) {
  const t = torrent
  const info = useInfo()
  const [notes, setNotes] = useState("")
  const [name, setName] = useState("")
  const [tags, setTags] = useState("")
  const [picking, setPicking] = useState(false)
  const [addingArchive, setAddingArchive] = useState(false)
  const [archive, setArchive] = useState<ArchiveInput>(emptyArchive)
  const [addingAdoption, setAddingAdoption] = useState(false)
  const [adopterId, setAdopterId] = useState<number | null>(null)
  const [adoptionNotes, setAdoptionNotes] = useState("")
  const [copied, setCopied] = useState(false)
  const [removing, setRemoving] = useState<Location | null>(null)

  const hash = t?.hash
  useEffect(() => {
    if (!t) return
    setNotes(t.notes)
    setName(t.name)
    setTags(t.tags.join(", "))
    setPicking(false)
    setAddingArchive(false)
    setArchive(emptyArchive)
    setAddingAdoption(false)
    setAdopterId(null)
    setAdoptionNotes("")
    setRemoving(null)
    // Reset only when switching torrent, not on background refreshes.
  }, [hash])

  const update = useAction((p: TorrentPatch) => api.updateTorrent(hash!, p), "Salvato")
  const addArchive = useAction((a: ArchiveInput) => api.addArchive(hash!, a), "Spostamento registrato")
  const delArchive = useAction((id: number) => api.deleteArchive(id), "Spostamento rimosso")
  const addAdoption = useAction((a: AdoptionInput) => api.addAdoption(hash!, a), "Adozione aggiunta")
  const delAdoption = useAction((id: number) => api.deleteAdoption(id), "Adozione rimossa")
  const del = useAction(() => api.deleteTorrent(hash!), "Torrent eliminato")
  const lookup = useAction(async () => {
    const found = await api.lookupHash(hash!)
    onHashChange?.(found.hash)
    return found
  }, "Hash trovato sul tracker")

  if (!t) return <Sheet open={false} />

  const onClient = t.locations.length > 0
  const tagList = splitTags(tags)
  const tagsChanged = !onClient && tagList.join(",") !== t.tags.join(",")
  const dirty = notes !== t.notes || (t.manual && name !== t.name) || tagsChanged
  const downloading = t.locations.filter(isDownloading)

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
              <AlertDescription>
                Lo stesso torrent (stesso hash) è caricato su più client.
                {downloading.length > 0 && (
                  <p>
                    In download su: {downloading.map((l) => `${l.clientName} (${Math.floor(l.progress * 100)}%)`).join(", ")}.
                  </p>
                )}
              </AlertDescription>
            </AlertBox>
          )}

          <div className="grid gap-1.5">
            <Field label="Hash">
              {isManualHash(t.hash) ? (
                <span className="grid justify-items-start gap-1">
                  <span className="text-muted-foreground">Non indicato</span>
                  {info.data?.unit3d && (
                    <>
                      <span className={t.hashLookupError ? "text-destructive text-xs" : "text-muted-foreground text-xs"}>
                        {t.hashLookupError
                          ? `Tracker: ${t.hashLookupError}${t.hashLookupAt ? ` (${formatDate(t.hashLookupAt)})` : ""}`
                          : "In coda per la ricerca sul tracker UNIT3D."}
                      </span>
                      <span className="flex flex-wrap gap-2">
                        <Button size="sm" variant="outline" disabled={lookup.isPending} onClick={() => lookup.mutate(undefined)}>
                          <Search /> {lookup.isPending ? "Ricerca…" : "Cerca ora sul tracker"}
                        </Button>
                        <Button size="sm" variant="outline" onClick={() => setPicking(true)}>
                          <ListChecks /> Scegli dal tracker
                        </Button>
                      </span>
                      <TrackerPickDialog
                        torrent={t}
                        open={picking}
                        onOpenChange={setPicking}
                        onPicked={(h) => onHashChange?.(h)}
                      />
                    </>
                  )}
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 font-mono text-xs">
                  {t.hash}
                  <Button variant="ghost" size="icon-sm" className="size-6" onClick={copyHash} aria-label="Copia hash">
                    {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
                  </Button>
                </span>
              )}
            </Field>
            {t.trackerUrl && (
              <Field label="Sul tracker">
                <a
                  href={t.trackerUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 underline-offset-2 hover:underline"
                >
                  <ExternalLink className="size-3.5" /> Apri pagina del torrent
                </a>
              </Field>
            )}
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
                      {isDownloading(l) ? (
                        <Badge variant="warning">
                          <Download /> {formatState(l.state)}
                        </Badge>
                      ) : (
                        <Badge variant={isErrorState(l.state) ? "destructive" : "secondary"}>{formatState(l.state)}</Badge>
                      )}
                      <span className="text-muted-foreground ml-auto text-xs">
                        {Math.floor(l.progress * 100)}% · ratio {l.ratio.toFixed(2)}
                      </span>
                      {canRemoveDuplicate(t) && (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="size-6"
                          onClick={() => setRemoving(l)}
                          aria-label={`Rimuovi da ${l.clientName}`}
                          title={`Rimuovi da ${l.clientName}`}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      )}
                    </div>
                    {isDownloading(l) && (
                      <div className="bg-muted mt-2 h-1.5 overflow-hidden rounded-full">
                        <div className="h-full rounded-full bg-amber-500" style={{ width: `${l.progress * 100}%` }} />
                      </div>
                    )}
                    <div className="text-muted-foreground mt-1 text-xs">Aggiunto il {formatUnix(l.addedOn)}</div>
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
            <RemoveFromClientDialog torrent={t} location={removing} onClose={() => setRemoving(null)} />
          </Section>

          <Separator />

          <Section
            title={`Spostato su disco (${t.archives.length})`}
            icon={HardDrive}
            action={
              !addingArchive && (
                <Button size="sm" variant="outline" onClick={() => setAddingArchive(true)}>
                  <HardDrive /> Sposta su disco
                </Button>
              )
            }
          >
            {t.hasTorrentFile && (
              <Button variant="outline" size="sm" className="w-fit" asChild>
                <a href={api.torrentFileUrl(t.hash)} download>
                  <FileDown /> Scarica il file .torrent
                </a>
              </Button>
            )}
            {t.archives.length === 0 && !addingArchive && (
              <p className="text-muted-foreground text-sm">
                Non è stato spostato su nessun disco.
                {onClient && " Registra lo spostamento prima di toglierlo dal client: la rimozione non verrà segnalata."}
              </p>
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
                    <div className="text-muted-foreground mt-1 text-xs">
                      {a.source === "s3"
                        ? `Trovato nel bucket il ${formatDate(a.createdAt)}: si aggiorna da solo a ogni lettura`
                        : `Spostato il ${formatDate(a.createdAt)}`}
                    </div>
                  </div>
                  {a.source !== "s3" && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => delArchive.mutate(a.id)}
                      aria-label="Rimuovi spostamento"
                    >
                      <Trash2 />
                    </Button>
                  )}
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
                    Registra spostamento
                  </Button>
                </div>
              </div>
            )}
          </Section>

          <Separator />

          <Section
            title={`Adottato da (${t.adoptions.length})`}
            icon={HandHeart}
            action={
              !addingAdoption && (
                <Button size="sm" variant="outline" onClick={() => setAddingAdoption(true)}>
                  <Plus /> Aggiungi
                </Button>
              )
            }
          >
            {t.adoptions.length === 0 && !addingAdoption && (
              <p className="text-muted-foreground text-sm">Nessuno l'ha adottato.</p>
            )}
            <ul className="grid gap-2">
              {t.adoptions.map((a) => (
                <li key={a.id} className="flex items-start gap-2 rounded-lg border p-3 text-sm">
                  <div className="min-w-0 flex-1">
                    <span className="font-medium">{a.adopterName}</span>
                    {a.notes && <div className="mt-1 text-xs">{a.notes}</div>}
                    <div className="text-muted-foreground mt-1 text-xs">Dal {formatDate(a.createdAt)}</div>
                  </div>
                  <Button variant="ghost" size="icon-sm" onClick={() => delAdoption.mutate(a.id)} aria-label="Rimuovi adozione">
                    <Trash2 />
                  </Button>
                </li>
              ))}
            </ul>
            {addingAdoption && (
              <div className="grid gap-3 rounded-lg border p-3">
                <div className="grid gap-2">
                  <Label htmlFor="ts-adopter">Adottatore</Label>
                  <AdopterSelect
                    id="ts-adopter"
                    value={adopterId}
                    onChange={setAdopterId}
                    exclude={t.adoptions.map((a) => a.adopterId)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="ts-adoption-notes">Note adozione</Label>
                  <Input id="ts-adoption-notes" value={adoptionNotes} onChange={(e) => setAdoptionNotes(e.target.value)} />
                </div>
                <div className="flex justify-end gap-2">
                  <Button variant="outline" size="sm" onClick={() => setAddingAdoption(false)}>
                    Annulla
                  </Button>
                  <Button
                    size="sm"
                    disabled={adopterId === null || addAdoption.isPending}
                    onClick={() =>
                      addAdoption.mutate(
                        { adopterId: adopterId!, notes: adoptionNotes },
                        {
                          onSuccess: () => {
                            setAddingAdoption(false)
                            setAdopterId(null)
                            setAdoptionNotes("")
                          },
                        }
                      )
                    }
                  >
                    Salva adozione
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
              <Label htmlFor="ts-tags">Tag (separati da virgola)</Label>
              <Input
                id="ts-tags"
                value={tags}
                disabled={onClient}
                onChange={(e) => setTags(e.target.value)}
                placeholder="Personal Release, 4K"
              />
              {onClient && (
                <span className="text-muted-foreground text-xs">
                  Il torrent è su un client: i tag si cambiano in qBittorrent e bookrr li legge alla sincronizzazione.
                </span>
              )}
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
                    notes,
                    ...(t.manual && name.trim() ? { name: name.trim() } : {}),
                    ...(tagsChanged ? { tags: tagList } : {}),
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
                      Verranno cancellati da bookrr anche spostamenti, adozioni, note e segnalazioni. I file sui dischi non
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
