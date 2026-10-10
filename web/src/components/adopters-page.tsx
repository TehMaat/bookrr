import { useEffect, useMemo, useState } from "react"
import { HandHeart, Pencil, Plus, Search, Trash2, X } from "lucide-react"

import { AdopterFormDialog } from "@/components/adopter-form-dialog"
import { PersonalBadge } from "@/components/torrent-badges"
import { TorrentSheet } from "@/components/torrent-sheet"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { api, type Adopter, type Alert, type Torrent } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { useAction, useAdopters, useTorrents } from "@/lib/queries"

export function AdoptersPage({ onResolve }: { onResolve: (a: Alert) => void }) {
  const adopters = useAdopters()
  const torrents = useTorrents()
  const [editing, setEditing] = useState<Adopter | null>(null)
  const [open, setOpen] = useState(false)
  const [deleting, setDeleting] = useState<Adopter | null>(null)
  const [adding, setAdding] = useState<Adopter | null>(null)
  const [selected, setSelected] = useState<string | null>(null)
  const del = useAction((id: number) => api.deleteAdopter(id), "Adottatore eliminato")
  const unadopt = useAction((id: number) => api.deleteAdoption(id), "Adozione rimossa")

  const all = useMemo(() => torrents.data ?? [], [torrents.data])
  const byAdopter = useMemo(() => {
    const m = new Map<number, { t: Torrent; adoptionId: number }[]>()
    for (const t of all)
      for (const a of t.adoptions) {
        const list = m.get(a.adopterId) ?? []
        list.push({ t, adoptionId: a.id })
        m.set(a.adopterId, list)
      }
    return m
  }, [all])
  const selectedTorrent = selected ? (all.find((t) => t.hash === selected) ?? null) : null

  return (
    <div className="grid gap-4">
      <div className="flex items-center gap-2">
        <p className="text-muted-foreground text-sm">Persone che hanno adottato le tue release e continuano a condividerle.</p>
        <Button
          className="ml-auto"
          onClick={() => {
            setEditing(null)
            setOpen(true)
          }}
        >
          <Plus /> Nuovo adottatore
        </Button>
      </div>

      {adopters.isLoading && <Skeleton className="h-32 w-full" />}
      {adopters.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-6 text-center text-sm">
            Nessun adottatore registrato. Aggiungine uno per poterlo scegliere sui torrent.
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {adopters.data?.map((a) => {
          const adopted = byAdopter.get(a.id) ?? []
          return (
            <Card key={a.id} className="gap-4">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <HandHeart className="size-4 text-emerald-600" />
                  {a.name}
                </CardTitle>
                <CardDescription className="break-all">{a.contact || "Contatto non indicato"}</CardDescription>
                <CardAction className="flex gap-1">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Modifica"
                    onClick={() => {
                      setEditing(a)
                      setOpen(true)
                    }}
                  >
                    <Pencil />
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label="Elimina" onClick={() => setDeleting(a)}>
                    <Trash2 />
                  </Button>
                </CardAction>
              </CardHeader>
              <CardContent className="grid gap-3 text-sm">
                <div className="flex items-center gap-2">
                  <span>
                    <span className="font-medium">{a.adoptedCount}</span> torrent · {formatBytes(a.adoptedSize)}
                  </span>
                  <Button size="sm" variant="outline" className="ml-auto" onClick={() => setAdding(a)}>
                    <Plus /> Aggiungi torrent
                  </Button>
                </div>
                {adopted.length > 0 && (
                  <ul className="max-h-56 divide-y overflow-y-auto rounded-lg border">
                    {adopted.map(({ t, adoptionId }) => (
                      <li key={adoptionId} className="flex items-center gap-1 py-1 pr-1 pl-3">
                        <button
                          type="button"
                          className="min-w-0 flex-1 truncate text-left hover:underline"
                          title={t.name}
                          onClick={() => setSelected(t.hash)}
                        >
                          {t.name}
                        </button>
                        <span className="text-muted-foreground shrink-0 text-xs tabular-nums">{formatBytes(t.size)}</span>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="size-7 shrink-0"
                          aria-label="Rimuovi adozione"
                          onClick={() => unadopt.mutate(adoptionId)}
                        >
                          <X />
                        </Button>
                      </li>
                    ))}
                  </ul>
                )}
                {a.notes && <p className="text-muted-foreground text-xs whitespace-pre-wrap">{a.notes}</p>}
              </CardContent>
            </Card>
          )
        })}
      </div>

      <AdopterFormDialog open={open} adopter={editing} onOpenChange={setOpen} />
      <AddTorrentsDialog adopter={adding} torrents={all} onClose={() => setAdding(null)} />
      <TorrentSheet
        torrent={selectedTorrent}
        onClose={() => setSelected(null)}
        onResolve={onResolve}
        onHashChange={setSelected}
      />
      <AlertDialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Eliminare l'adottatore "{deleting?.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting?.adoptedCount
                ? `Verranno rimosse anche le sue ${deleting.adoptedCount} adozioni.`
                : "Non ha adottato nessun torrent."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annulla</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => deleting && del.mutate(deleting.id)}
            >
              Elimina
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

/** Search the known torrents and add them, one click each, to an adopter. */
function AddTorrentsDialog({
  adopter,
  torrents,
  onClose,
}: {
  adopter: Adopter | null
  torrents: Torrent[]
  onClose: () => void
}) {
  const [query, setQuery] = useState("")
  const add = useAction((hash: string) => api.addAdoption(hash, { adopterId: adopter!.id, notes: "" }), "Adozione aggiunta")

  useEffect(() => {
    if (adopter) setQuery("")
  }, [adopter])

  const results = useMemo(() => {
    if (!adopter) return []
    const q = query.trim().toLowerCase()
    return torrents
      .filter((t) => !t.adoptions.some((a) => a.adopterId === adopter.id))
      .filter((t) => !q || t.name.toLowerCase().includes(q) || t.hash.includes(q))
      .sort((a, b) => Number(b.personalRelease) - Number(a.personalRelease))
      .slice(0, 50)
  }, [adopter, torrents, query])

  return (
    <Dialog open={adopter !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Torrent adottati da {adopter?.name}</DialogTitle>
          <DialogDescription>Cerca un torrent e premi "Aggiungi". Le tue release sono in cima.</DialogDescription>
        </DialogHeader>
        <div className="relative">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Cerca per nome o hash…"
            className="pl-8"
            autoFocus
          />
        </div>
        <ul className="max-h-[50vh] divide-y overflow-y-auto rounded-lg border">
          {results.length === 0 && (
            <li className="text-muted-foreground py-6 text-center text-sm">Nessun torrent da aggiungere.</li>
          )}
          {results.map((t) => (
            <li key={t.hash} className="flex items-center gap-2 py-1.5 pr-1.5 pl-3 text-sm">
              <span className="min-w-0 flex-1 truncate" title={t.name}>
                {t.name}
              </span>
              {t.personalRelease && <PersonalBadge />}
              <span className="text-muted-foreground shrink-0 text-xs tabular-nums">{formatBytes(t.size)}</span>
              <Button size="sm" variant="outline" disabled={add.isPending} onClick={() => add.mutate(t.hash)}>
                <Plus /> Aggiungi
              </Button>
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  )
}
