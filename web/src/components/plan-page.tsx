import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  CircleArrowUp,
  CircleCheck,
  CircleDashed,
  CalendarClock,
  ExternalLink,
  ListTodo,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  SearchX,
  Trash2,
  TriangleAlert,
  Hammer,
  Ban,
  HardDriveDownload,
  Upload,
} from "lucide-react"
import { toast } from "sonner"

import { NavbarStats, StatCard } from "@/components/navbar-stats"
import {
  LoadTorrentDialog,
  MAX_PLAN_TORRENTS,
  PlanTorrents,
  SendTorrentsDialog,
  TorrentLabelDialog,
  type TorrentAction,
} from "@/components/plan-torrents"
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, type Alert, type PlanItem, type PlanLookup, type PlanStatus } from "@/lib/api"
import { formatDate, formatRelative } from "@/lib/format"
import { keys, useAction, usePlan } from "@/lib/queries"
import { cn } from "@/lib/utils"

export const planStatuses: { value: PlanStatus; label: string; icon: React.ElementType; tone: string }[] = [
  { value: "todo", label: "To do", icon: CircleDashed, tone: "text-sky-600 dark:text-sky-400" },
  { value: "doing", label: "In progress", icon: Hammer, tone: "text-amber-600 dark:text-amber-400" },
  { value: "downloaded", label: "Downloaded", icon: HardDriveDownload, tone: "text-violet-600 dark:text-violet-400" },
  { value: "done", label: "Done", icon: CircleCheck, tone: "text-emerald-600 dark:text-emerald-400" },
  { value: "dropped", label: "Dropped", icon: Ban, tone: "text-muted-foreground" },
]
const statusOf = (s: PlanStatus) => planStatuses.find((x) => x.value === s)!

type Filter = "open" | "available" | PlanStatus | "all"

const resolutionRank: Record<string, number> = { "2160p": 5, "1080p": 4, "1080i": 3, "720p": 2, "576p": 1, "480p": 1 }
const typeRank: Record<string, number> = { "Full Disc": 5, Remux: 5, Encode: 4, "WEB-DL": 3, WEBRip: 2, HDTV: 1 }

/** How good the best release found is: type first, then resolution (as Prowlarr's ranking). */
function bestRank(it: PlanItem) {
  const b = it.best
  if (!b?.resolution) return 0
  return (typeRank[b.type] ?? 0) * 10 + (resolutionRank[b.resolution] ?? 0)
}

const statusOrder: Record<PlanStatus, number> = { downloaded: 0, doing: 1, todo: 2, done: 3, dropped: 4 }

/** Resolution and type, as "2160p Remux". */
function grade(resolution: string, type: string) {
  return [resolution, type].filter(Boolean).join(" ")
}

const year = (date: string) => date.slice(0, 4)
/** "2027-05-01" as "01/05/2027". */
const day = (date: string) => date.split("-").reverse().join("/")
const tmdbUrl = (id: number) => `https://www.themoviedb.org/movie/${id}`
const posterUrl = (path: string) => `https://image.tmdb.org/t/p/w92${path}`

/** Where the film was added from, as "Saga: Matrix". */
function sourceLabel(it: { source: string; sourceTitle: string }) {
  if (!it.source) return ""
  const kind = it.source.startsWith("saga:") ? "Saga" : it.source.startsWith("list:") ? "Collection" : "From"
  return `${kind}: ${it.sourceTitle || it.source}`
}

export function PlanPage({ onResolve }: { onResolve: (a: Alert) => void }) {
  const plan = usePlan()
  const data = plan.data
  const [filter, setFilter] = useState<Filter>("open")
  const [query, setQuery] = useState("")
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<PlanItem | null>(null)
  const [removing, setRemoving] = useState<PlanItem | null>(null)
  const [torrentAction, setTorrentAction] = useState<TorrentAction | null>(null)
  // The torrent of the plan open in the sheet of the Torrents page.
  const [openHash, setOpenHash] = useState<string | null>(null)
  const torrents = useQuery({ queryKey: keys.torrents, queryFn: api.torrents, enabled: openHash !== null })
  const openTorrent = openHash ? (torrents.data?.find((t) => t.hash === openHash) ?? null) : null
  const update = useAction(({ id, status }: { id: number; status: PlanStatus }) => api.updatePlan(id, { status }))
  const remove = useAction((id: number) => api.deletePlan(id), "Film removed from the plan")
  const search = useAction((id: number) => api.searchPlan(id), "Prowlarr search started")

  const all = useMemo(() => data?.entries ?? [], [data])
  const open = (it: PlanItem) => it.status === "todo" || it.status === "doing" || it.status === "downloaded"
  const counts: Record<Filter, number> = {
    open: all.filter(open).length,
    available: all.filter((it) => open(it) && it.best?.better).length,
    todo: all.filter((it) => it.status === "todo").length,
    doing: all.filter((it) => it.status === "doing").length,
    downloaded: all.filter((it) => it.status === "downloaded").length,
    done: all.filter((it) => it.status === "done").length,
    dropped: all.filter((it) => it.status === "dropped").length,
    all: all.length,
  }
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return all
      .filter((it) => {
        switch (filter) {
          case "all":
            return true
          case "open":
            return open(it)
          case "available":
            return open(it) && !!it.best?.better
          default:
            return it.status === filter
        }
      })
      .filter(
        (it) =>
          !q ||
          [it.title, it.originalTitle, it.note, it.sourceTitle, String(it.tmdbId)].some((s) =>
            s.toLowerCase().includes(q),
          ),
      )
      .sort((a, b) => statusOrder[a.status] - statusOrder[b.status] || bestRank(b) - bestRank(a))
  }, [all, filter, query])

  const best = data?.best
  const stats: { key: Filter; label: string; icon: React.ElementType; tone?: string }[] = [
    { key: "open", label: "To release", icon: ListTodo, tone: "text-sky-600 dark:text-sky-400" },
    ...(data?.searchEnabled
      ? [
          {
            key: "available" as Filter,
            label: "Found",
            icon: CircleArrowUp,
            tone: "text-amber-600 dark:text-amber-400",
          },
        ]
      : []),
    { key: "doing", label: "In progress", icon: Hammer },
    {
      key: "downloaded",
      label: "Downloaded",
      icon: HardDriveDownload,
      tone: "text-violet-600 dark:text-violet-400",
    },
    { key: "done", label: "Done", icon: CircleCheck, tone: "text-emerald-600 dark:text-emerald-400" },
    { key: "all", label: "All", icon: ListTodo },
  ]

  return (
    <div className="grid gap-4">
      <NavbarStats>
        {stats.map((s) => (
          <StatCard
            key={s.key}
            label={s.label}
            value={counts[s.key]}
            icon={s.icon}
            tone={s.tone}
            active={filter === s.key}
            loading={plan.isLoading}
            onClick={() => setFilter(s.key)}
          />
        ))}
      </NavbarStats>

      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground max-w-3xl text-sm">
          The films you want to release: add them by their TMDB, or from Missing. For each one bookrr searches Prowlarr
          for the best existing version (type first, then resolution). Load its .torrent (two for a merge) and download
          it on a client: the film becomes downloaded when complete, and done when you upload it to the tracker.
        </p>
        <Button className="ml-auto" disabled={!data?.enabled} onClick={() => setAdding(true)}>
          <Plus /> Add film
        </Button>
      </div>

      {data && !data.enabled && (
        <AlertBox>
          <TriangleAlert />
          <AlertTitle>You can't add films</AlertTitle>
          <AlertDescription>{data.disabledReason}</AlertDescription>
        </AlertBox>
      )}
      {data && data.enabled && !data.searchEnabled && (
        <AlertBox>
          <SearchX />
          <AlertTitle>Max resolution not available</AlertTitle>
          <AlertDescription>{data.searchDisabledReason}</AlertDescription>
        </AlertBox>
      )}
      {best?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Last Prowlarr search failed</AlertTitle>
          <AlertDescription>{best.lastError}</AlertDescription>
        </AlertBox>
      )}

      <div className="relative max-w-sm">
        <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search the plan…"
          className="pl-8"
        />
      </div>

      {plan.isLoading ? (
        <Skeleton className="h-40 w-full" />
      ) : (
        <TableCard>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Film</TableHead>
                <TableHead>Best version</TableHead>
                <TableHead className="hidden md:table-cell">On the tracker</TableHead>
                <TableHead>Download</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-muted-foreground py-10 text-center">
                    {all.length === 0
                      ? "The plan is empty: add a film by its TMDB, or from Missing."
                      : "No films match the filters."}
                  </TableCell>
                </TableRow>
              )}
              {visible.map((it) => (
                <TableRow key={it.id} className={cn(it.status === "dropped" && "opacity-60")}>
                  <TableCell className="max-w-0 min-w-[240px] whitespace-normal">
                    <div className="flex items-center gap-3">
                      {it.posterPath ? (
                        <img
                          src={posterUrl(it.posterPath)}
                          alt=""
                          loading="lazy"
                          className="h-14 w-10 shrink-0 rounded object-cover"
                        />
                      ) : (
                        <div className="bg-muted h-14 w-10 shrink-0 rounded" />
                      )}
                      <div className="min-w-0">
                        <a
                          href={tmdbUrl(it.tmdbId)}
                          target="_blank"
                          rel="noreferrer"
                          className="block truncate font-medium hover:underline"
                          title={it.title}
                        >
                          {it.title}
                          {it.releaseDate && (
                            <span className="text-muted-foreground font-normal"> ({year(it.releaseDate)})</span>
                          )}
                        </a>
                        <div className="text-muted-foreground truncate text-xs">
                          {[it.originalTitle !== it.title && it.originalTitle, sourceLabel(it)]
                            .filter(Boolean)
                            .join(" · ")}
                        </div>
                        {it.note && (
                          <div className="text-muted-foreground truncate text-xs italic" title={it.note}>
                            {it.note}
                          </div>
                        )}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <PlanBest it={it} searchEnabled={!!data?.searchEnabled} />
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <OnTracker it={it} />
                  </TableCell>
                  <TableCell className="whitespace-normal">
                    <PlanTorrents it={it} onAction={setTorrentAction} onOpenTorrent={setOpenHash} />
                  </TableCell>
                  <TableCell>
                    <Select
                      value={it.status}
                      onValueChange={(v) => update.mutate({ id: it.id, status: v as PlanStatus })}
                    >
                      <SelectTrigger size="sm" className="w-40">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {planStatuses.map((s) => (
                          <SelectItem key={s.value} value={s.value}>
                            <s.icon className={s.tone} /> {s.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {it.status === "done" && it.doneAt && (
                      <div className="text-muted-foreground mt-1 text-xs">{formatDate(it.doneAt)}</div>
                    )}
                    {it.status === "downloaded" && it.downloadedAt && (
                      <div className="text-muted-foreground mt-1 text-xs">{formatDate(it.downloadedAt)}</div>
                    )}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm" aria-label="More actions">
                          <MoreHorizontal />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {data?.searchEnabled && (
                          <DropdownMenuItem disabled={it.searching} onClick={() => search.mutate(it.id)}>
                            <Search /> Search Prowlarr now
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem onClick={() => setEditing(it)}>
                          <Pencil /> Note
                        </DropdownMenuItem>
                        {it.torrents.length < MAX_PLAN_TORRENTS && (
                          <DropdownMenuItem onClick={() => setTorrentAction({ kind: "load", item: it })}>
                            <Upload /> {it.torrents.length === 0 ? "Load .torrent" : "Second .torrent (merge)"}
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem asChild>
                          <a href={tmdbUrl(it.tmdbId)} target="_blank" rel="noreferrer">
                            <ExternalLink /> The film on TMDB
                          </a>
                        </DropdownMenuItem>
                        {it.trackerUrl && (
                          <DropdownMenuItem asChild>
                            <a href={it.trackerUrl} target="_blank" rel="noreferrer">
                              <ExternalLink /> The film on the tracker
                            </a>
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onClick={() => setRemoving(it)}>
                          <Trash2 /> Remove from the plan
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableCard>
      )}

      {best && (
        <p className="text-muted-foreground text-xs">
          Every film still to do is searched on Prowlarr when you add it and then every {best.recheck}, with the daily
          max resolution search;{" "}
          {best.running
            ? `searching: ${best.done} of ${best.total} films.`
            : best.lastRunAt
              ? `last search ${formatRelative(best.lastRunAt)}: ${best.lastResult}.`
              : "no search yet."}{" "}
          Films not out yet are searched when they come out.
        </p>
      )}

      <AddPlanDialog open={adding} onOpenChange={setAdding} />
      <NoteDialog item={editing} onClose={() => setEditing(null)} />
      <LoadTorrentDialog
        action={torrentAction?.kind === "load" ? torrentAction : null}
        onClose={() => setTorrentAction(null)}
      />
      <SendTorrentsDialog
        action={torrentAction?.kind === "send" ? torrentAction : null}
        onClose={() => setTorrentAction(null)}
      />
      <TorrentLabelDialog
        action={torrentAction?.kind === "label" ? torrentAction : null}
        onClose={() => setTorrentAction(null)}
      />
      <TorrentSheet torrent={openTorrent} onClose={() => setOpenHash(null)} onResolve={onResolve} />
      <AlertDialog open={removing !== null} onOpenChange={(o) => !o && setRemoving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove "{removing?.title}" from the plan?</AlertDialogTitle>
            <AlertDialogDescription>
              If you just want to remember you won't do it, mark it as "Dropped": it stays in the plan and is no longer
              searched.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => removing && remove.mutate(removing.id)}
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

/** The best release found with Prowlarr for a film of the plan, or why there is none yet. */
function PlanBest({ it, searchEnabled }: { it: PlanItem; searchEnabled: boolean }) {
  const b = it.best
  if (it.searching)
    return (
      <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
        <Loader2 className="size-3 animate-spin" /> Searching…
      </span>
    )
  if (!b) {
    if (!it.released)
      return (
        <Badge variant="outline" className="text-muted-foreground">
          <CalendarClock /> {it.releaseDate ? `Out on ${day(it.releaseDate)}` : "Release date unknown"}
        </Badge>
      )
    return <span className="text-muted-foreground text-xs">{searchEnabled ? "To search" : "—"}</span>
  }
  const mine = grade(b.mine, b.mineType)
  if (b.checkedAt && !b.resolution)
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="outline" className="text-muted-foreground border-dashed">
            <SearchX /> Not found
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          Prowlarr found no release of this film with a known resolution (searched {formatRelative(b.checkedAt)})
        </TooltipContent>
      </Tooltip>
    )
  const found = grade(b.resolution, b.type)
  const badge = (
    <Badge
      variant="outline"
      className={cn(
        b.better ? "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300" : "text-muted-foreground",
      )}
    >
      {b.better ? <CircleArrowUp /> : <CircleCheck />} {found || mine}
    </Badge>
  )
  return (
    <div className="grid gap-0.5">
      <Tooltip>
        <TooltipTrigger asChild>
          {b.url ? (
            <a href={b.url} target="_blank" rel="noreferrer" className="w-fit">
              {badge}
            </a>
          ) : (
            <span className="w-fit">{badge}</span>
          )}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm break-all">
          {b.name ? `${b.name}${b.indexer ? ` (${b.indexer})` : ""}` : `You already have it in ${mine}: no need to search`}
          {b.checkedAt && ` · searched ${formatRelative(b.checkedAt)}`}
        </TooltipContent>
      </Tooltip>
      {mine && (
        <span className="text-muted-foreground text-xs">{b.better ? `yours: ${mine}` : "you have the best"}</span>
      )}
    </div>
  )
}

/** The user's uploads of the film and the torrents of others on the tracker. */
function OnTracker({ it }: { it: PlanItem }) {
  if (it.uploads.length === 0 && it.others.length === 0)
    return it.trackerUrl ? (
      <a
        href={it.trackerUrl}
        target="_blank"
        rel="noreferrer"
        className="text-muted-foreground text-xs hover:underline"
      >
        Search the tracker
      </a>
    ) : (
      <span className="text-muted-foreground text-xs">—</span>
    )
  return (
    <div className="flex max-w-xs flex-wrap gap-1">
      {it.uploads.map((u) => (
        <Tooltip key={u.id}>
          <TooltipTrigger asChild>
            <a href={u.url || undefined} target="_blank" rel="noreferrer">
              <Badge variant="brand">Yours · {u.quality || "?"}</Badge>
            </a>
          </TooltipTrigger>
          <TooltipContent className="break-all">{u.name}</TooltipContent>
        </Tooltip>
      ))}
      {it.others.slice(0, 3).map((r) => (
        <Tooltip key={r.id}>
          <TooltipTrigger asChild>
            <a href={r.url || undefined} target="_blank" rel="noreferrer">
              <Badge variant="secondary">{r.quality || "?"}</Badge>
            </a>
          </TooltipTrigger>
          <TooltipContent className="break-all">{r.name}</TooltipContent>
        </Tooltip>
      ))}
      {it.others.length > 3 &&
        (it.trackerUrl ? (
          <a href={it.trackerUrl} target="_blank" rel="noreferrer">
            <Badge variant="outline">+{it.others.length - 3}</Badge>
          </a>
        ) : (
          <Badge variant="outline">+{it.others.length - 3}</Badge>
        ))}
    </div>
  )
}

/** Looks a film up on TMDB by id, link or IMDb id, then adds it to the plan. */
function AddPlanDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [q, setQ] = useState("")
  const [note, setNote] = useState("")
  const [found, setFound] = useState<PlanLookup | null>(null)
  const [looking, setLooking] = useState(false)
  const add = useAction((args: { q: string; note: string }) => api.addPlan(args.q, args.note), "Film added to the plan")

  const reset = () => {
    setQ("")
    setNote("")
    setFound(null)
  }
  const lookup = async () => {
    if (!q.trim()) return
    setLooking(true)
    try {
      setFound(await api.lookupPlan(q))
    } catch (err) {
      setFound(null)
      toast.error((err as Error).message)
    } finally {
      setLooking(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset()
        onOpenChange(o)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add a film to the plan</DialogTitle>
          <DialogDescription>
            The film's TMDB id (603), its themoviedb.org link or its IMDb id (tt0133093).
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            lookup()
          }}
        >
          <Input
            value={q}
            onChange={(e) => {
              setQ(e.target.value)
              setFound(null)
            }}
            placeholder="https://www.themoviedb.org/movie/603-the-matrix"
            autoFocus
          />
          <Button type="submit" variant="outline" disabled={looking || !q.trim()}>
            {looking ? <Loader2 className="animate-spin" /> : <Search />} Search
          </Button>
        </form>
        {found && (
          <div className="grid gap-3">
            <div className="flex items-center gap-3 rounded-lg border p-3">
              {found.movie.posterPath ? (
                <img
                  src={posterUrl(found.movie.posterPath)}
                  alt=""
                  className="h-20 w-14 shrink-0 rounded object-cover"
                />
              ) : (
                <div className="bg-muted h-20 w-14 shrink-0 rounded" />
              )}
              <div className="min-w-0">
                <a
                  href={tmdbUrl(found.movie.tmdbId)}
                  target="_blank"
                  rel="noreferrer"
                  className="font-medium hover:underline"
                >
                  {found.movie.title}
                </a>
                <div className="text-muted-foreground text-sm">
                  {[
                    found.movie.originalTitle !== found.movie.title && found.movie.originalTitle,
                    found.movie.releaseDate ? day(found.movie.releaseDate) : "release date unknown",
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </div>
                {found.inPlan > 0 && (
                  <Badge variant="info" className="mt-1">
                    Already in the plan
                  </Badge>
                )}
              </div>
            </div>
            {found.inPlan === 0 && (
              <div className="grid gap-1.5">
                <Label htmlFor="plan-note">Note</Label>
                <Textarea
                  id="plan-note"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="Optional: where to get audio, subtitles…"
                  rows={2}
                />
              </div>
            )}
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <Button
            disabled={!found || found.inPlan > 0 || add.isPending}
            onClick={() =>
              found &&
              add.mutate(
                { q: String(found.movie.tmdbId), note },
                {
                  onSuccess: () => {
                    reset()
                    onOpenChange(false)
                  },
                },
              )
            }
          >
            <Plus /> Add to the plan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function NoteDialog({ item, onClose }: { item: PlanItem | null; onClose: () => void }) {
  const [note, setNote] = useState("")
  const [prev, setPrev] = useState<PlanItem | null>(null)
  if (item !== prev) {
    setPrev(item)
    setNote(item?.note ?? "")
  }
  const save = useAction(
    (args: { id: number; note: string }) => api.updatePlan(args.id, { note: args.note }),
    "Note saved",
  )
  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Note · {item?.title}</DialogTitle>
          <DialogDescription>What you need to make this release.</DialogDescription>
        </DialogHeader>
        <Textarea value={note} onChange={(e) => setNote(e.target.value)} rows={4} autoFocus />
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={save.isPending}
            onClick={() => item && save.mutate({ id: item.id, note }, { onSuccess: onClose })}
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** The status of a film in the plan, as a small badge (for the pages that link to it). */
export function PlanStatusBadge({ status }: { status: PlanStatus }) {
  const s = statusOf(status)
  return (
    <Badge variant="outline" className="gap-1">
      <s.icon className={s.tone} /> In the plan · {s.label}
    </Badge>
  )
}
