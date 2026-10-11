import { useMemo, useState } from "react"
import {
  ArrowDown,
  ArrowUp,
  CheckCheck,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  CloudOff,
  ExternalLink,
  FileWarning,
  HandHeart,
  HardDrive,
  Layers,
  Plus,
  RefreshCw,
  Search,
  Server,
  TriangleAlert,
} from "lucide-react"
import { toast } from "sonner"

import { PersonalBadge, StatusBadge } from "@/components/torrent-badges"
import { TorrentSheet } from "@/components/torrent-sheet"
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
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, type Alert, type ReleaseStatus, type TrackerRelease } from "@/lib/api"
import { formatBytes, formatDate, formatRelative } from "@/lib/format"
import { useAction, useInfo, useReleases, useTorrents } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Filter = "all" | ReleaseStatus | "removed"
type SortKey = "date" | "name" | "seeders" | "completed"

const PAGE_SIZE = 50

function matches(r: TrackerRelease, f: Filter) {
  if (f === "all") return true
  if (f === "removed") return r.removedAt !== null
  return r.status === f
}

export function ReleaseStatusBadge({ status }: { status: ReleaseStatus }) {
  if (status !== "untracked") return <StatusBadge status={status} />
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className="border-dashed">
          <CircleHelp /> Sconosciuta
        </Badge>
      </TooltipTrigger>
      <TooltipContent>Sul tracker, ma bookrr non conosce nessun torrent con questo hash</TooltipContent>
    </Tooltip>
  )
}

/** Seeders, leechers and completed downloads, as on the tracker. */
function Peers({ r }: { r: TrackerRelease }) {
  return (
    <div className="flex items-center gap-2 text-xs tabular-nums">
      <span className={cn("flex items-center gap-0.5", r.seeders === 0 ? "text-destructive" : "text-emerald-600 dark:text-emerald-400")}>
        <ArrowUp className="size-3" />
        {r.seeders}
      </span>
      <span className="text-muted-foreground flex items-center gap-0.5">
        <ArrowDown className="size-3" />
        {r.leechers}
      </span>
      <span className="text-muted-foreground flex items-center gap-0.5">
        <CheckCheck className="size-3" />
        {r.timesCompleted}
      </span>
    </div>
  )
}

export function ReleasesPage({ onResolve }: { onResolve: (a: Alert) => void }) {
  const releases = useReleases()
  const torrents = useTorrents()
  const info = useInfo()
  const [filter, setFilter] = useState<Filter>("all")
  const [query, setQuery] = useState("")
  const [onlyPersonal, setOnlyPersonal] = useState(false)
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "date", desc: true })
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<string | null>(null)
  const [addingAll, setAddingAll] = useState(false)

  const syncNow = useAction(() => api.syncReleases())
  const add = useAction((ids: number[] | "all") => api.addReleases(ids))

  const data = releases.data
  const all = useMemo(() => data?.releases ?? [], [data])
  const scoped = useMemo(() => (onlyPersonal ? all.filter((r) => r.personalRelease) : all), [all, onlyPersonal])
  const state = data?.state
  const running = !!state?.running || syncNow.isPending
  const addable = all.filter((r) => r.status === "untracked" && r.hash)

  const counts = useMemo(() => {
    const c: Record<Filter, number> = {
      all: 0,
      client: 0,
      missing: 0,
      archived: 0,
      adopted: 0,
      unknown: 0,
      untracked: 0,
      removed: 0,
    }
    for (const r of scoped) for (const f of Object.keys(c) as Filter[]) if (matches(r, f)) c[f]++
    return c
  }, [scoped])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const out = scoped.filter((r) => {
      if (!matches(r, filter)) return false
      if (!q) return true
      return (
        r.name.toLowerCase().includes(q) ||
        r.type.toLowerCase().includes(q) ||
        r.resolution.toLowerCase().includes(q) ||
        r.category.toLowerCase().includes(q) ||
        r.hash.includes(q) ||
        String(r.id) === q
      )
    })
    const dir = sort.desc ? -1 : 1
    out.sort((a, b) => {
      let c = 0
      if (sort.key === "date") c = a.createdAt.localeCompare(b.createdAt) || a.id - b.id
      else if (sort.key === "seeders") c = a.seeders - b.seeders
      else if (sort.key === "completed") c = a.timesCompleted - b.timesCompleted
      if (c === 0) c = a.name.localeCompare(b.name, "it", { sensitivity: "base" })
      return c * dir
    })
    return out
  }, [scoped, filter, query, sort])

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const allTorrents = torrents.data ?? []
  const selectedTorrent = selected ? (allTorrents.find((t) => t.hash === selected) ?? null) : null
  const cols = 6

  const reset = <T,>(fn: (v: T) => void) => (v: T) => {
    fn(v)
    setPage(0)
  }
  const toggleSort = (key: SortKey) =>
    setSort((s) => (s.key === key ? { key, desc: !s.desc } : { key, desc: key !== "name" }))
  const SortIcon = ({ k }: { k: SortKey }) =>
    sort.key === k ? sort.desc ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" /> : null

  const refresh = () =>
    syncNow.mutate(undefined, {
      onSuccess: () => toast.success("Aggiornamento avviato", { description: "Gli upload vengono letti dal tracker in background." }),
    })
  const addOne = (r: TrackerRelease) =>
    add.mutate([r.id], {
      onSuccess: () => {
        toast.success("Release aggiunta a bookrr", { description: "Apri la scheda per indicare dove si trova." })
        setSelected(r.hash)
      },
    })

  const stats: { key: Filter; label: string; icon: React.ElementType; tone?: string }[] = [
    { key: "all", label: "Totale", icon: Layers },
    { key: "client", label: "Su client", icon: Server, tone: "text-sky-600 dark:text-sky-400" },
    { key: "missing", label: "Da localizzare", icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    { key: "archived", label: "Archiviate", icon: HardDrive, tone: "text-violet-600 dark:text-violet-400" },
    { key: "adopted", label: "Adottate", icon: HandHeart, tone: "text-emerald-600 dark:text-emerald-400" },
    { key: "unknown", label: "Posizione ignota", icon: CircleHelp },
    { key: "untracked", label: "Sconosciute", icon: FileWarning, tone: "text-amber-600 dark:text-amber-400" },
    { key: "removed", label: "Rimosse", icon: CloudOff, tone: "text-destructive" },
  ]

  if (releases.isLoading) return <Skeleton className="h-48 w-full" />
  if (releases.isError)
    return (
      <Card>
        <CardContent className="text-destructive py-6 text-center text-sm">Errore nel caricamento: {releases.error.message}</CardContent>
      </Card>
    )

  if (data && !data.enabled && all.length === 0)
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CloudOff className="size-4" /> Storico release non attivo
          </CardTitle>
          <CardDescription>
            Lo storico dei tuoi upload richiede un tracker UNIT3D: imposta <code>BOOKRR_UNIT3D_URL</code> e{" "}
            <code>BOOKRR_UNIT3D_API_KEY</code> e riavvia bookrr.
          </CardDescription>
        </CardHeader>
      </Card>
    )

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground text-sm">
          Tutti i torrent che hai caricato sul tracker
          {state?.username && (
            <>
              {" "}
              come <span className="text-foreground font-medium">{state.username}</span>
            </>
          )}
          , collegati ai torrent di bookrr con lo stesso hash.
        </p>
        <div className="ml-auto flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground text-xs">
                {running
                  ? state?.pending
                    ? `Scarico i .torrent (${state.pending} da fare)…`
                    : "Aggiornamento in corso…"
                  : `Aggiornato ${formatRelative(data?.syncedAt)}`}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {state?.lastResult || "Nessun aggiornamento ancora"}
              {info.data?.releasesInterval && ` · ogni ${info.data.releasesInterval}`}
            </TooltipContent>
          </Tooltip>
          {data?.enabled && (
            <Button variant="outline" size="sm" disabled={running} onClick={refresh}>
              <RefreshCw className={cn(running && "animate-spin")} /> Aggiorna ora
            </Button>
          )}
        </div>
      </div>

      {data && !data.enabled && (
        <AlertBox variant="warning">
          <CloudOff />
          <AlertTitle>Tracker UNIT3D non configurato</AlertTitle>
          <AlertDescription>
            Le release non vengono più aggiornate: imposta BOOKRR_UNIT3D_URL e BOOKRR_UNIT3D_API_KEY.
          </AlertDescription>
        </AlertBox>
      )}
      {state?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Ultimo aggiornamento non riuscito</AlertTitle>
          <AlertDescription>{state.lastError}</AlertDescription>
        </AlertBox>
      )}

      {data && data.notOnTracker.length > 0 && (
        <Card className="border-warning/60 gap-3">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TriangleAlert className="size-4 text-amber-600" />
              {info.data?.releaseTag ?? "Personal Release"} non trovate tra i tuoi upload ({data.notOnTracker.length})
            </CardTitle>
            <CardDescription>
              Torrent di bookrr con il flag Personal Release il cui hash non è tra i tuoi upload sul tracker: cancellati o
              sostituiti, caricati altrove, o aggiunti senza hash.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="max-h-56 divide-y overflow-y-auto rounded-lg border">
              {data.notOnTracker.map((m) => {
                const t = allTorrents.find((t) => t.hash === m.hash)
                return (
                  <li key={m.hash} className="flex items-center gap-2 py-1 pr-2 pl-3">
                    <button
                      type="button"
                      className="min-w-0 flex-1 truncate text-left text-sm hover:underline"
                      title={m.name}
                      onClick={() => setSelected(m.hash)}
                    >
                      {m.name}
                    </button>
                    {m.hash.startsWith("manual-") && (
                      <Badge variant="outline" className="text-muted-foreground">
                        senza hash
                      </Badge>
                    )}
                    {t && <StatusBadge status={t.status} />}
                  </li>
                )
              })}
            </ul>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-8">
        {stats.map((s) => (
          <button
            key={s.key}
            type="button"
            onClick={() => reset(setFilter)(filter === s.key && s.key !== "all" ? "all" : s.key)}
            className="text-left"
          >
            <Card
              className={cn(
                "hover:bg-accent/50 gap-1 px-4 py-3 transition-colors",
                filter === s.key && "ring-primary/40 border-primary/50 ring-2"
              )}
            >
              <div className="text-muted-foreground flex items-center gap-1.5 text-xs">
                <s.icon className={cn("size-3.5", s.tone)} />
                {s.label}
              </div>
              <div className={cn("text-2xl font-semibold tabular-nums", counts[s.key] > 0 && s.tone)}>{counts[s.key]}</div>
            </Card>
          </button>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => reset(setQuery)(e.target.value)}
            placeholder="Cerca per nome, tipo, risoluzione, categoria, hash…"
            className="pl-8"
          />
        </div>
        <div className="flex items-center gap-2 px-1">
          <Switch id="releases-personal" checked={onlyPersonal} onCheckedChange={reset(setOnlyPersonal)} />
          <Label htmlFor="releases-personal" className="whitespace-nowrap">
            Solo Personal Release
          </Label>
        </div>
        {addable.length > 0 && (
          <Button onClick={() => setAddingAll(true)} disabled={add.isPending}>
            <Plus /> Aggiungi tutte ({addable.length})
          </Button>
        )}
      </div>

      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="w-full">
                <button className="flex items-center gap-1" onClick={() => toggleSort("name")}>
                  Nome <SortIcon k="name" />
                </button>
              </TableHead>
              <TableHead className="hidden md:table-cell">Tipo</TableHead>
              <TableHead className="hidden lg:table-cell">
                <button className="flex items-center gap-1" onClick={() => toggleSort("date")}>
                  Caricata <SortIcon k="date" />
                </button>
              </TableHead>
              <TableHead className="hidden sm:table-cell">
                <span className="flex items-center gap-2">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("seeders")}>
                    Seed <SortIcon k="seeders" />
                  </button>
                  <span className="text-muted-foreground">/</span>
                  <button className="flex items-center gap-1" onClick={() => toggleSort("completed")}>
                    Completati <SortIcon k="completed" />
                  </button>
                </span>
              </TableHead>
              <TableHead>Stato</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={cols} className="text-muted-foreground py-10 text-center">
                  {all.length === 0
                    ? running
                      ? "Sto leggendo i tuoi upload dal tracker…"
                      : "Nessun upload trovato sul tracker."
                    : "Nessuna release corrisponde ai filtri."}
                </TableCell>
              </TableRow>
            )}
            {visible.map((r) => {
              const linked = r.torrentHash !== ""
              return (
                <TableRow
                  key={r.id}
                  onClick={linked ? () => setSelected(r.torrentHash) : undefined}
                  className={cn(linked && "cursor-pointer", r.removedAt && "opacity-70")}
                >
                  <TableCell className="max-w-0 min-w-[260px] whitespace-normal">
                    <div className="flex items-center gap-1.5">
                      <span className={cn("truncate font-medium", r.removedAt && "line-through")} title={r.name}>
                        {r.name}
                      </span>
                      {r.personalRelease && <PersonalBadge />}
                      {r.removedAt && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Badge variant="destructive">
                              <CloudOff /> Rimossa
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent>Non più tra i tuoi upload dal {formatDate(r.removedAt)}</TooltipContent>
                        </Tooltip>
                      )}
                    </div>
                    <div className="text-muted-foreground truncate text-xs">
                      {[r.category, formatBytes(r.size), r.hash ? null : r.hashError ? `hash non calcolato: ${r.hashError}` : "hash da calcolare"]
                        .filter(Boolean)
                        .join(" · ")}
                    </div>
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <div className="flex flex-wrap gap-1">
                      {r.type && <Badge variant="secondary">{r.type}</Badge>}
                      {r.resolution && <Badge variant="outline">{r.resolution}</Badge>}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground hidden text-xs lg:table-cell">{formatDate(r.createdAt)}</TableCell>
                  <TableCell className="hidden sm:table-cell">
                    <Peers r={r} />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5">
                      <ReleaseStatusBadge status={r.status} />
                      {r.status === "untracked" && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-7"
                          disabled={!r.hash || add.isPending}
                          title={r.hash ? undefined : "Hash non ancora calcolato"}
                          onClick={(e) => {
                            e.stopPropagation()
                            addOne(r)
                          }}
                        >
                          <Plus /> Aggiungi a bookrr
                        </Button>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    {r.detailsLink && (
                      <Button variant="ghost" size="icon-sm" asChild onClick={(e) => e.stopPropagation()}>
                        <a href={r.detailsLink} target="_blank" rel="noreferrer" aria-label="Apri sul tracker">
                          <ExternalLink />
                        </a>
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
        <div className="text-muted-foreground flex flex-wrap items-center gap-2 border-t px-4 py-2 text-xs">
          <span>
            {rows.length} release · {formatBytes(rows.reduce((s, r) => s + r.size, 0))}
          </span>
          {pages > 1 && (
            <div className="ml-auto flex items-center gap-1">
              <Button variant="ghost" size="icon-sm" disabled={current === 0} onClick={() => setPage(current - 1)}>
                <ChevronLeft />
              </Button>
              <span className="tabular-nums">
                {current + 1} / {pages}
              </span>
              <Button variant="ghost" size="icon-sm" disabled={current >= pages - 1} onClick={() => setPage(current + 1)}>
                <ChevronRight />
              </Button>
            </div>
          )}
        </div>
      </Card>

      <TorrentSheet torrent={selectedTorrent} onClose={() => setSelected(null)} onResolve={onResolve} onHashChange={setSelected} />
      <AlertDialog open={addingAll} onOpenChange={setAddingAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Aggiungere {addable.length} release a bookrr?</AlertDialogTitle>
            <AlertDialogDescription>
              Ogni release sconosciuta diventa un torrent con il suo hash, nome, dimensione, file .torrent e il flag Personal
              Release. Poi potrai indicare dove si trova (disco, adottatore) dalla scheda del torrent.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annulla</AlertDialogCancel>
            <AlertDialogAction
              onClick={() =>
                add.mutate("all", { onSuccess: (r) => toast.success(`${r.added} release aggiunte a bookrr`) })
              }
            >
              Aggiungi tutte
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
