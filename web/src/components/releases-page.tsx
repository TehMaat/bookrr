import { useMemo, useState } from "react"
import {
  Archive,
  ArchiveX,
  ArrowDown,
  ArrowUp,
  ChevronLeft,
  ChevronRight,
  CircleArrowUp,
  CircleCheck,
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
  SearchX,
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
import { api, type Alert, type BestResolution, type ReleaseStatus, type TrackerRelease } from "@/lib/api"
import { formatBytes, formatDate, formatRelative, plural } from "@/lib/format"
import { useAction, useInfo, useReleases, useTorrents } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Filter = "all" | ReleaseStatus | "removed" | "archivable" | "better"
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
  best: [] as string[],
}

const releaseStatuses: ReleaseStatus[] = ["client", "missing", "archived", "adopted", "unknown", "untracked"]

/** The distinct non-empty values, sorted. */
function distinct(values: string[]) {
  return [...new Set(values.filter(Boolean))].sort((a, b) => a.localeCompare(b, "en", { numeric: true }))
}

function matches(r: TrackerRelease, f: Filter) {
  if (f === "all") return true
  if (f === "removed") return r.removedAt !== null
  if (f === "archivable") return !!r.archivable?.archivable
  if (f === "better") return !!r.best?.better
  return r.status === f
}

type BestKind = "better" | "max" | "none"

/** How a film stands against the best release found; null when it was not searched (or is not a film). */
function bestKind(b: BestResolution | null): BestKind | null {
  if (!b) return null
  if (b.better) return "better"
  if (b.checkedAt && !b.resolution) return "none"
  return "max"
}

/** Resolution and type, as "2160p Remux". */
function grade(resolution: string, type: string) {
  return [resolution, type].filter(Boolean).join(" ")
}

/** The best release found with Prowlarr for the film of an upload. */
function BestBadge({ b }: { b: BestResolution | null }) {
  const kind = bestKind(b)
  if (!b || !kind) return <span className="text-muted-foreground text-xs">—</span>
  const mine = grade(b.mine, b.mineType)
  const searched = b.checkedAt ? ` · searched ${formatRelative(b.checkedAt)}` : ""
  const found = b.name && `${b.name}${b.indexer ? ` (${b.indexer})` : ""}`
  const badge =
    kind === "better" ? (
      <Badge variant="outline" className="border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300">
        <CircleArrowUp /> {grade(b.resolution, b.type)}
      </Badge>
    ) : kind === "none" ? (
      <Badge variant="outline" className="text-muted-foreground border-dashed">
        <SearchX /> Not found
      </Badge>
    ) : (
      <Badge variant="outline" className="text-muted-foreground">
        <CircleCheck /> {mine || grade(b.resolution, b.type)}
      </Badge>
    )
  const tip =
    kind === "better"
      ? `Exists in ${grade(b.resolution, b.type)}, you have it in ${mine || "unknown quality"}: ${found}`
      : kind === "none"
        ? "Prowlarr found no release of this film with a known resolution"
        : b.checkedAt
          ? `You already have the best existing version${found ? `: the best found is ${found}` : ""}`
          : `You already have it in ${mine}: no need to search`
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {kind === "better" && b.url ? (
          <a href={b.url} target="_blank" rel="noreferrer" onClick={(e) => e.stopPropagation()}>
            {badge}
          </a>
        ) : (
          <span>{badge}</span>
        )}
      </TooltipTrigger>
      <TooltipContent className="max-w-sm break-words">
        {tip}
        {searched}
      </TooltipContent>
    </Tooltip>
  )
}

export function ReleaseStatusBadge({ status }: { status: ReleaseStatus }) {
  if (status !== "untracked") return <StatusBadge status={status} />
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className="border-dashed">
          <CircleHelp /> Unknown
        </Badge>
      </TooltipTrigger>
      <TooltipContent>On the tracker, but bookrr knows no torrent with this hash</TooltipContent>
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
      better: 0,
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
      const kind = bestKind(r.best)
      if (!optionsMatch(cf.best, ...(kind ? [kind] : []))) return false
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
      if (c === 0) c = a.name.localeCompare(b.name, "en", { sensitivity: "base" })
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
  const best = data?.best
  const cols = (picking ? 7 : 6) + (best ? 1 : 0)

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
      onSuccess: () => toast.success("Refresh started", { description: "Uploads are being read from the tracker in the background." }),
    })
  const addOne = (r: TrackerRelease) =>
    add.mutate([r.id], {
      onSuccess: () => {
        toast.success("Release added to bookrr", { description: "Open its details to set where it is." })
        setSelected(r.hash)
      },
    })

  const tag = info.data?.releaseTag ?? "Personal Release"
  const unflagDone = (r: { unflagged: number; errors: string[] }) => {
    const what = r.unflagged === 1 ? "1 torrent is no longer" : `${r.unflagged} torrents are no longer`
    if (r.errors.length === 0) toast.success(`${what} ${tag}`)
    else toast.warning(`${what} ${tag}`, { description: `Tag not removed from: ${r.errors.join("; ")}` })
  }

  const allStats: { key: Filter; label: string; icon: React.ElementType; tone?: string }[] = [
    { key: "all", label: "Total", icon: Layers },
    { key: "client", label: "On a client", icon: Server, tone: "text-sky-600 dark:text-sky-400" },
    { key: "missing", label: "To locate", icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    { key: "archived", label: "Archived", icon: HardDrive, tone: "text-violet-600 dark:text-violet-400" },
    { key: "adopted", label: "Adopted", icon: HandHeart, tone: "text-emerald-600 dark:text-emerald-400" },
    { key: "unknown", label: "Unknown location", icon: CircleHelp },
    { key: "untracked", label: "Unknown", icon: FileWarning, tone: "text-amber-600 dark:text-amber-400" },
    { key: "removed", label: "Removed", icon: CloudOff, tone: "text-destructive" },
    { key: "archivable", label: "Archivable", icon: Archive, tone: archivableTone },
    { key: "better", label: "Upgradable", icon: CircleArrowUp, tone: "text-amber-600 dark:text-amber-400" },
  ]
  // Dead uploads are still on the tracker: none is removed. Without Prowlarr no film is searched.
  const stats = allStats.filter((s) => !(dead && s.key === "removed") && (best || s.key !== "better"))

  if (releases.isLoading) return <Skeleton className="h-48 w-full" />
  if (releases.isError)
    return (
      <Card>
        <CardContent className="text-destructive py-6 text-center text-sm">Loading failed: {releases.error.message}</CardContent>
      </Card>
    )

  if (data && !data.enabled && all.length === 0)
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CloudOff className="size-4" /> Release history not enabled
          </CardTitle>
          <CardDescription>
            The history of your uploads needs a UNIT3D tracker: set it in Settings (icon at the bottom left), Tracker
            tab.
          </CardDescription>
        </CardHeader>
      </Card>
    )

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground text-sm">
          {dead ? "Your uploads still on the tracker" : "All the torrents you uploaded to the tracker"}
          {state?.username && (
            <>
              {" "}
              as <span className="text-foreground font-medium">{state.username}</span>
            </>
          )}
          {dead
            ? " that nobody is seeding: seed them again from a client or an archive."
            : ", linked to the bookrr torrents with the same hash."}
        </p>
        <div className="ml-auto flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground text-xs">
                {running
                  ? state?.pending
                    ? `Downloading the .torrent files (${state.pending} to go)…`
                    : "Refreshing…"
                  : `Refreshed ${formatRelative(data?.syncedAt)}`}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {state?.lastResult || "No refresh yet"}
              {info.data?.releasesInterval && ` · every ${info.data.releasesInterval}`}
            </TooltipContent>
          </Tooltip>
          {data?.enabled && (
            <Button variant="outline" size="sm" disabled={running} onClick={refresh}>
              <RefreshCw className={cn(running && "animate-spin")} /> Refresh now
            </Button>
          )}
        </div>
      </div>

      {data && !data.enabled && (
        <AlertBox variant="warning">
          <CloudOff />
          <AlertTitle>UNIT3D tracker not configured</AlertTitle>
          <AlertDescription>
            Releases are no longer refreshed: set the tracker in Settings, Tracker tab.
          </AlertDescription>
        </AlertBox>
      )}
      {state?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Last refresh failed</AlertTitle>
          <AlertDescription>{state.lastError}</AlertDescription>
        </AlertBox>
      )}

      {best?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Last Prowlarr search failed</AlertTitle>
          <AlertDescription>{best.lastError}</AlertDescription>
        </AlertBox>
      )}

      {!dead && data && data.notOnTracker.length > 0 && (
        <Card className="border-warning/60 gap-3">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TriangleAlert className="size-4 text-amber-600" />
              {tag} not found among your uploads ({data.notOnTracker.length})
            </CardTitle>
            <CardDescription>
              bookrr torrents flagged {tag} whose hash is not among your uploads on the tracker: deleted or replaced, uploaded
              elsewhere, or added without a hash. If they aren't your releases, remove the flag: the tag is removed on
              qBittorrent too.
            </CardDescription>
            <CardAction>
              <Button variant="outline" size="sm" disabled={unflag.isPending} onClick={() => setUnflaggingAll(true)}>
                <TagX /> Unflag all
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
                        no hash
                      </Badge>
                    )}
                    {t && <StatusBadge status={t.status} />}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Remove the ${tag} flag`}
                          disabled={unflag.isPending}
                          onClick={() => unflag.mutate([m.hash], { onSuccess: unflagDone })}
                        >
                          <TagX />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Remove the {tag} flag, on qBittorrent too</TooltipContent>
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
            placeholder="Search by name, type, resolution, category, hash…"
            className="pl-8"
          />
        </div>
        <div className="flex items-center gap-2 px-1">
          <Switch id="releases-personal" checked={onlyPersonal} onCheckedChange={reset(setOnlyPersonal)} />
          <Label htmlFor="releases-personal" className="whitespace-nowrap">
            Only Personal Release
          </Label>
        </div>
        {!dead && addable.length > 0 && (
          <Button onClick={() => setAddingAll(true)} disabled={add.isPending}>
            <Plus /> Add all ({addable.length})
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
                  <PickBox label="Select all" checked={picker.all} indeterminate={picker.some} onChange={picker.toggleAll} />
                </TableHead>
              )}
              <TableHead className="w-full">
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("name")}>
                    Name <SortIcon k="name" />
                  </button>
                  <ColumnFilter label="Name" active={isActive(cf.name)} onClear={() => columns.clear("name")}>
                    <TextFilter value={cf.name} onChange={(v) => columns.set("name", v)} placeholder="Name, category, hash, ID…" />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="hidden md:table-cell">
                <div className="flex items-center gap-1">
                  Type
                  <ColumnFilter
                    label="Type"
                    active={isActive(cf.type) || isActive(cf.resolution)}
                    onClear={() => columns.clear("type", "resolution")}
                  >
                    <OptionsFilter
                      title="Type"
                      options={types.map((t) => ({ value: t, label: t }))}
                      value={cf.type}
                      onChange={(v) => columns.set("type", v)}
                    />
                    <OptionsFilter
                      title="Resolution"
                      options={resolutions.map((t) => ({ value: t, label: t }))}
                      value={cf.resolution}
                      onChange={(v) => columns.set("resolution", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              {best && (
                <TableHead className="hidden md:table-cell">
                  <div className="flex items-center gap-1">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span>Max</span>
                      </TooltipTrigger>
                      <TooltipContent className="max-w-sm">
                        The best existing version of each film, in any language, searched with Prowlarr once a day (each film
                        every {best.recheck}): the type counts first (Remux &gt; Encode &gt; WEB-DL &gt; WEBRip &gt; HDTV), then the
                        resolution, so a 1080p Remux beats a 2160p WEB-DL.{" "}
                        {best.running
                          ? `Searching: ${best.done} of ${best.total} films.`
                          : best.lastRunAt
                            ? `Last search ${formatRelative(best.lastRunAt)}: ${best.lastResult}.`
                            : `First search ${formatRelative(best.nextRunAt)}.`}
                      </TooltipContent>
                    </Tooltip>
                    <ColumnFilter label="Max" active={isActive(cf.best)} onClear={() => columns.clear("best")}>
                      <OptionsFilter
                        options={[
                          { value: "better", label: "Exists in a better version" },
                          { value: "max", label: "You have the best" },
                          { value: "none", label: "Not found" },
                        ]}
                        value={cf.best}
                        onChange={(v) => columns.set("best", v)}
                      />
                    </ColumnFilter>
                  </div>
                </TableHead>
              )}
              <TableHead className="hidden lg:table-cell">
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("date")}>
                    Uploaded <SortIcon k="date" />
                  </button>
                  <ColumnFilter label="Uploaded" active={isActive(cf.date)} onClear={() => columns.clear("date")}>
                    <RangeFilter type="date" value={cf.date} onChange={(v) => columns.set("date", v)} />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="hidden sm:table-cell">
                <span className="flex items-center gap-2">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("seeders")}>
                    Seeds <SortIcon k="seeders" />
                  </button>
                  <span className="text-muted-foreground">/</span>
                  <button className="flex items-center gap-1" onClick={() => toggleSort("completed")}>
                    Completed <SortIcon k="completed" />
                  </button>
                  <ColumnFilter
                    label="Seeds / Completed"
                    active={isActive(cf.seeders) || isActive(cf.completed)}
                    onClear={() => columns.clear("seeders", "completed")}
                  >
                    <RangeFilter title="Seeders" value={cf.seeders} onChange={(v) => columns.set("seeders", v)} parse={parseCount} />
                    <RangeFilter
                      title="Completed"
                      value={cf.completed}
                      onChange={(v) => columns.set("completed", v)}
                      parse={parseCount}
                    />
                  </ColumnFilter>
                </span>
              </TableHead>
              <TableHead>
                <div className="flex items-center gap-1">
                  Status
                  <ColumnFilter label="Status" align="end" active={isActive(cf.status)} onClear={() => columns.clear("status")}>
                    <OptionsFilter
                      options={[
                        ...releaseStatuses.map((st) => ({ value: st, label: <ReleaseStatusBadge status={st} /> })),
                        {
                          value: "archivable",
                          label: (
                            <Badge variant="outline" className="border-teal-500/40 bg-teal-500/10 text-teal-700 dark:text-teal-300">
                              <Archive /> Archivable
                            </Badge>
                          ),
                        },
                        {
                          value: "never",
                          label: (
                            <Badge variant="outline" className="text-muted-foreground">
                              <ArchiveX /> Never archive
                            </Badge>
                          ),
                        },
                        {
                          value: "removed",
                          label: (
                            <Badge variant="destructive">
                              <CloudOff /> Removed
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
                      ? "Reading your uploads from the tracker…"
                      : "No uploads found on the tracker."
                    : "No releases match the filters."}
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
                          label={`Select ${r.name}`}
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
                              <CloudOff /> Removed
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent>No longer among your uploads since {formatDate(r.removedAt)}</TooltipContent>
                        </Tooltip>
                      )}
                    </div>
                    <div className="text-muted-foreground truncate text-xs">
                      {[r.category, formatBytes(r.size), r.hash ? null : r.hashError ? `hash not computed: ${r.hashError}` : "hash to compute"]
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
                  {best && (
                    <TableCell className="hidden md:table-cell">
                      <BestBadge b={r.best} />
                    </TableCell>
                  )}
                  <TableCell className="text-muted-foreground hidden text-xs lg:table-cell">{formatDate(r.createdAt)}</TableCell>
                  <TableCell className="hidden sm:table-cell">
                    <PeerCounts p={r} />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <ReleaseStatusBadge status={r.status} />
                      <ArchivableBadge a={r.archivable} label="Archivable" />
                      <NeverArchiveBadge a={r.archivable} />
                      {r.status === "untracked" && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-7"
                          disabled={!r.hash || add.isPending}
                          title={r.hash ? undefined : "Hash not computed yet"}
                          onClick={(e) => {
                            e.stopPropagation()
                            addOne(r)
                          }}
                        >
                          <Plus /> Add to bookrr
                        </Button>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    {r.detailsLink && (
                      <Button variant="ghost" size="icon-sm" asChild onClick={(e) => e.stopPropagation()}>
                        <a href={r.detailsLink} target="_blank" rel="noreferrer" aria-label="Open on the tracker">
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
            {plural(rows.length, "release")} · {formatBytes(rows.reduce((s, r) => s + r.size, 0))}
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
            <AlertDialogTitle>Remove the {tag} flag from {plural(data?.notOnTracker.length ?? 0, "torrent")}?</AlertDialogTitle>
            <AlertDialogDescription>
              They won't be considered your releases anymore: bookrr no longer reports them when they disappear from the
              clients. On the qBittorrent clients only the "{tag}" tag is removed: torrents and files stay where they are.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => unflag.mutate("all", { onSuccess: unflagDone })}>Remove flag</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={addingAll} onOpenChange={setAddingAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Add {plural(addable.length, "release")} to bookrr?</AlertDialogTitle>
            <AlertDialogDescription>
              Each unknown release becomes a torrent with its hash, name, size, .torrent file and the Personal Release
              flag. You can then set where it is (disk, adopter) from the torrent details.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() =>
                add.mutate("all", { onSuccess: (r) => toast.success(`${plural(r.added, "release")} added to bookrr`) })
              }
            >
              Add all
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
