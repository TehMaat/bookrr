import { useMemo, useState } from "react"
import {
  Archive,
  ArchiveX,
  ArrowDown,
  ArrowUp,
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
  TagX,
  TriangleAlert,
} from "lucide-react"
import { toast } from "sonner"

import {
  ActiveFilters,
  ColumnFilter,
  dateMatch,
  isActive,
  noRange,
  OptionsFilter,
  optionsMatch,
  parseCount,
  RangeFilter,
  rangeMatch,
  TextFilter,
  textMatch,
  useColumnFilters,
} from "@/components/column-filter"
import { NavbarStats, StatCard } from "@/components/navbar-stats"
import { NeverArchiveBar, PickBox, usePicker } from "@/components/never-archive"
import {
  ArchivableBadge,
  archivableTone,
  NeverArchiveBadge,
  PeerCounts,
  PersonalBadge,
  StatusBadge,
} from "@/components/torrent-badges"
import { TorrentSheet } from "@/components/torrent-sheet"
import { TableCard } from "@/components/table-card"
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
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
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

type Filter = "all" | ReleaseStatus | "removed" | "archivable"
type SortKey = "date" | "name" | "seeders" | "completed"

const PAGE_SIZE = 50

const noFilters = {
  name: "",
  type: [] as string[],
  resolution: [] as string[],
  date: noRange,
  seeders: noRange,
  completed: noRange,
  status: [] as string[],
}

const releaseStatuses: ReleaseStatus[] = ["client", "missing", "archived", "adopted", "unknown", "untracked"]

/** The distinct non-empty values, sorted. */
function distinct(values: string[]) {
  return [...new Set(values.filter(Boolean))].sort((a, b) => a.localeCompare(b, "it", { numeric: true }))
}

function matches(r: TrackerRelease, f: Filter) {
  if (f === "all") return true
  if (f === "removed") return r.removedAt !== null
  if (f === "archivable") return !!r.archivable?.archivable
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

/** With dead, only the uploads still on the tracker that nobody seeds. */
export function ReleasesPage({ onResolve, dead = false }: { onResolve: (a: Alert) => void; dead?: boolean }) {
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
  const [unflaggingAll, setUnflaggingAll] = useState(false)
  const columns = useColumnFilters(noFilters, () => setPage(0))
  const cf = columns.filters

  const syncNow = useAction(() => api.syncReleases())
  const add = useAction((ids: number[] | "all") => api.addReleases(ids))
  const unflag = useAction((hashes: string[] | "all") => api.unflagReleases(hashes))

  const data = releases.data
  const all = useMemo(() => data?.releases ?? [], [data])
  const scoped = useMemo(
    () => all.filter((r) => (!onlyPersonal || r.personalRelease) && (!dead || (r.seeders === 0 && !r.removedAt))),
    [all, onlyPersonal, dead]
  )
  const state = data?.state
  const running = !!state?.running || syncNow.isPending
  const addable = all.filter((r) => r.status === "untracked" && r.hash)
  const types = useMemo(() => distinct(all.map((r) => r.type)), [all])
  const resolutions = useMemo(() => distinct(all.map((r) => r.resolution)), [all])

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
      archivable: 0,
    }
    for (const r of scoped) for (const f of Object.keys(c) as Filter[]) if (matches(r, f)) c[f]++
    return c
  }, [scoped])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const out = scoped.filter((r) => {
      if (!matches(r, filter)) return false
      if (!textMatch(cf.name, r.name, r.category, r.hash, String(r.id))) return false
      if (!optionsMatch(cf.type, r.type)) return false
      if (!optionsMatch(cf.resolution, r.resolution)) return false
      if (!dateMatch(r.createdAt, cf.date)) return false
      if (!rangeMatch(r.seeders, cf.seeders, parseCount)) return false
      if (!rangeMatch(r.timesCompleted, cf.completed, parseCount)) return false
      const flags = [
        r.status,
        ...(r.archivable?.archivable ? ["archivable"] : []),
        ...(r.archivable?.neverArchive ? ["never"] : []),
        ...(r.removedAt ? ["removed"] : []),
      ]
      if (!optionsMatch(cf.status, ...flags)) return false
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
  }, [scoped, filter, cf, query, sort])

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const allTorrents = torrents.data ?? []
  const selectedTorrent = selected ? (allTorrents.find((t) => t.hash === selected) ?? null) : null
  // Ticks to mark uploads as never to archive, among the archivable ones or those already marked.
  const picking = filter === "archivable" || cf.status.includes("never")
  const picker = usePicker(picking ? rows.map((r) => ({ hash: r.torrentHash, archivable: r.archivable })) : [])
  const kept = scoped.filter((r) => r.archivable?.neverArchive).length
  const cols = picking ? 7 : 6

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

  const tag = info.data?.releaseTag ?? "Personal Release"
  const unflagDone = (r: { unflagged: number; errors: string[] }) => {
    const what = r.unflagged === 1 ? "1 torrent non è più" : `${r.unflagged} torrent non sono più`
    if (r.errors.length === 0) toast.success(`${what} ${tag}`)
    else toast.warning(`${what} ${tag}`, { description: `Tag non rimosso da: ${r.errors.join("; ")}` })
  }

  const allStats: { key: Filter; label: string; icon: React.ElementType; tone?: string }[] = [
    { key: "all", label: "Totale", icon: Layers },
    { key: "client", label: "Su client", icon: Server, tone: "text-sky-600 dark:text-sky-400" },
    { key: "missing", label: "Da localizzare", icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    { key: "archived", label: "Archiviate", icon: HardDrive, tone: "text-violet-600 dark:text-violet-400" },
    { key: "adopted", label: "Adottate", icon: HandHeart, tone: "text-emerald-600 dark:text-emerald-400" },
    { key: "unknown", label: "Posizione ignota", icon: CircleHelp },
    { key: "untracked", label: "Sconosciute", icon: FileWarning, tone: "text-amber-600 dark:text-amber-400" },
    { key: "removed", label: "Rimosse", icon: CloudOff, tone: "text-destructive" },
    { key: "archivable", label: "Archiviabili", icon: Archive, tone: archivableTone },
  ]
  // Dead uploads are still on the tracker: none is removed.
  const stats = dead ? allStats.filter((s) => s.key !== "removed") : allStats

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
          {dead ? "I tuoi upload ancora sul tracker" : "Tutti i torrent che hai caricato sul tracker"}
          {state?.username && (
            <>
              {" "}
              come <span className="text-foreground font-medium">{state.username}</span>
            </>
          )}
          {dead
            ? " che nessuno ha in seed: rimettili in seed da un client o da un archivio."
            : ", collegati ai torrent di bookrr con lo stesso hash."}
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

      {!dead && data && data.notOnTracker.length > 0 && (
        <Card className="border-warning/60 gap-3">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TriangleAlert className="size-4 text-amber-600" />
              {tag} non trovate tra i tuoi upload ({data.notOnTracker.length})
            </CardTitle>
            <CardDescription>
              Torrent di bookrr con il flag {tag} il cui hash non è tra i tuoi upload sul tracker: cancellati o sostituiti,
              caricati altrove, o aggiunti senza hash. Se non sono tue release, togli il flag: il tag viene tolto anche su
              qBittorrent.
            </CardDescription>
            <CardAction>
              <Button variant="outline" size="sm" disabled={unflag.isPending} onClick={() => setUnflaggingAll(true)}>
                <TagX /> Togli il flag a tutte
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            <ul className="max-h-56 divide-y overflow-y-auto rounded-lg border">
              {data.notOnTracker.map((m) => {
                const t = allTorrents.find((t) => t.hash === m.hash)
                return (
                  <li key={m.hash} className="flex items-center gap-2 py-0.5 pr-1 pl-3">
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
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Togli il flag ${tag}`}
                          disabled={unflag.isPending}
                          onClick={() => unflag.mutate([m.hash], { onSuccess: unflagDone })}
                        >
                          <TagX />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Togli il flag {tag}, anche da qBittorrent</TooltipContent>
                    </Tooltip>
                  </li>
                )
              })}
            </ul>
          </CardContent>
        </Card>
      )}

      <NavbarStats>
        {stats.map((s) => (
          <StatCard
            key={s.key}
            label={s.label}
            value={counts[s.key]}
            icon={s.icon}
            tone={s.tone}
            active={filter === s.key}
            onClick={() => {
              picker.clear()
              reset(setFilter)(filter === s.key && s.key !== "all" ? "all" : s.key)
            }}
          />
        ))}
      </NavbarStats>

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
        {!dead && addable.length > 0 && (
          <Button onClick={() => setAddingAll(true)} disabled={add.isPending}>
            <Plus /> Aggiungi tutte ({addable.length})
          </Button>
        )}
      </div>

      {picking && (
        <NeverArchiveBar
          chosen={picker.chosen}
          kept={kept}
          onShowKept={
            cf.status.includes("never")
              ? undefined
              : () => {
                  picker.clear()
                  setFilter("all")
                  columns.set("status", ["never"])
                }
          }
          onClear={picker.clear}
        />
      )}

      <TableCard>
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              {picking && (
                <TableHead className="w-0">
                  <PickBox label="Seleziona tutte" checked={picker.all} indeterminate={picker.some} onChange={picker.toggleAll} />
                </TableHead>
              )}
              <TableHead className="w-full">
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("name")}>
                    Nome <SortIcon k="name" />
                  </button>
                  <ColumnFilter label="Nome" active={isActive(cf.name)} onClear={() => columns.clear("name")}>
                    <TextFilter value={cf.name} onChange={(v) => columns.set("name", v)} placeholder="Nome, categoria, hash, ID…" />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="hidden md:table-cell">
                <div className="flex items-center gap-1">
                  Tipo
                  <ColumnFilter
                    label="Tipo"
                    active={isActive(cf.type) || isActive(cf.resolution)}
                    onClear={() => columns.clear("type", "resolution")}
                  >
                    <OptionsFilter
                      title="Tipo"
                      options={types.map((t) => ({ value: t, label: t }))}
                      value={cf.type}
                      onChange={(v) => columns.set("type", v)}
                    />
                    <OptionsFilter
                      title="Risoluzione"
                      options={resolutions.map((t) => ({ value: t, label: t }))}
                      value={cf.resolution}
                      onChange={(v) => columns.set("resolution", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="hidden lg:table-cell">
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("date")}>
                    Caricata <SortIcon k="date" />
                  </button>
                  <ColumnFilter label="Caricata" active={isActive(cf.date)} onClear={() => columns.clear("date")}>
                    <RangeFilter type="date" value={cf.date} onChange={(v) => columns.set("date", v)} />
                  </ColumnFilter>
                </div>
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
                  <ColumnFilter
                    label="Seed / Completati"
                    active={isActive(cf.seeders) || isActive(cf.completed)}
                    onClear={() => columns.clear("seeders", "completed")}
                  >
                    <RangeFilter title="Seeder" value={cf.seeders} onChange={(v) => columns.set("seeders", v)} parse={parseCount} />
                    <RangeFilter
                      title="Completati"
                      value={cf.completed}
                      onChange={(v) => columns.set("completed", v)}
                      parse={parseCount}
                    />
                  </ColumnFilter>
                </span>
              </TableHead>
              <TableHead>
                <div className="flex items-center gap-1">
                  Stato
                  <ColumnFilter label="Stato" align="end" active={isActive(cf.status)} onClear={() => columns.clear("status")}>
                    <OptionsFilter
                      options={[
                        ...releaseStatuses.map((st) => ({ value: st, label: <ReleaseStatusBadge status={st} /> })),
                        {
                          value: "archivable",
                          label: (
                            <Badge variant="outline" className="border-teal-500/40 bg-teal-500/10 text-teal-700 dark:text-teal-300">
                              <Archive /> Archiviabile
                            </Badge>
                          ),
                        },
                        {
                          value: "never",
                          label: (
                            <Badge variant="outline" className="text-muted-foreground">
                              <ArchiveX /> Da non archiviare
                            </Badge>
                          ),
                        },
                        {
                          value: "removed",
                          label: (
                            <Badge variant="destructive">
                              <CloudOff /> Rimossa
                            </Badge>
                          ),
                        },
                      ]}
                      value={cf.status}
                      onChange={(v) => columns.set("status", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
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
                  {picking && (
                    <TableCell>
                      {r.archivable && (
                        <PickBox
                          label={`Seleziona ${r.name}`}
                          checked={picker.isPicked(r.torrentHash)}
                          onChange={(on) => picker.toggle(r.torrentHash, on)}
                        />
                      )}
                    </TableCell>
                  )}
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
                    <PeerCounts p={r} />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <ReleaseStatusBadge status={r.status} />
                      <ArchivableBadge a={r.archivable} label="Archiviabile" />
                      <NeverArchiveBadge a={r.archivable} />
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
        <div className="text-muted-foreground flex flex-wrap items-center gap-2 border-t px-4 py-2 text-xs sm:px-6">
          <span>
            {rows.length} release · {formatBytes(rows.reduce((s, r) => s + r.size, 0))}
          </span>
          <ActiveFilters count={columns.active} onClear={() => columns.clear()} />
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
      </TableCard>

      <TorrentSheet torrent={selectedTorrent} onClose={() => setSelected(null)} onResolve={onResolve} onHashChange={setSelected} />
      <AlertDialog open={unflaggingAll} onOpenChange={setUnflaggingAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Togliere il flag {tag} a {data?.notOnTracker.length} torrent?</AlertDialogTitle>
            <AlertDialogDescription>
              Non saranno più considerate tue release: bookrr non le segnala più quando spariscono dai client. Sui client
              qBittorrent viene tolto solo il tag «{tag}»: torrent e file restano dove sono.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annulla</AlertDialogCancel>
            <AlertDialogAction onClick={() => unflag.mutate("all", { onSuccess: unflagDone })}>Togli il flag</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
