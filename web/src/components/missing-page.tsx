import { useMemo, useState } from "react"
import {
  Check,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Clapperboard,
  CloudOff,
  ExternalLink,
  Layers,
  RefreshCw,
  Search,
  Shuffle,
  TriangleAlert,
  Tv,
  Users,
  X,
} from "lucide-react"
import { toast } from "sonner"

import { Alert as AlertBox, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import {
  api,
  type GapCell,
  type GapGroup,
  type GapKind,
  type GapRef,
  type GapRow,
  type GapState,
  type GapUnit,
  type MissingCheckState,
} from "@/lib/api"
import { formatDate, formatRelative } from "@/lib/format"
import { useAction, useMissingSagas, useMissingSeries } from "@/lib/queries"
import { cn } from "@/lib/utils"

const PAGE_SIZE = 30

export type MissingView = "series" | "sagas"

/** What changes between the series and the sagas. */
type KindText = {
  icon: React.ElementType
  /** Plural noun of the stat card and the pager: "Serie", "Saghe". */
  noun: string
  search: string
  tmdbUrl: (id: number) => string
  units: (n: number) => string
  mine: (n: number) => string
  /** Short label of a cell; prefix is the start of the name shared by every part. */
  cellLabel: (u: GapUnit, prefix: number) => string
  intro: string
  empty: string
  checking: string
  noMatch: string
  unverifiableTitle: string
  unverifiableHint: string
}

const kinds: Record<GapKind, KindText> = {
  series: {
    icon: Tv,
    noun: "Serie",
    search: "Cerca una serie…",
    tmdbUrl: (id) => `https://www.themoviedb.org/tv/${id}`,
    units: (n) => (n === 1 ? "stagione uscita" : "stagioni uscite"),
    mine: (n) => (n === 1 ? "tuo upload" : "tuoi upload"),
    cellLabel: (u) => `S${String(u.number).padStart(2, "0")}`,
    intro:
      "Per ogni serie di cui hai caricato almeno un season pack: le stagioni che mancano nella stessa qualità (tipo, risoluzione, HDR e lingue audio) e se sul tracker le hanno caricate altri. Gli episodi singoli non contano.",
    empty: "Nessuna serie tra i tuoi upload: servono season pack (S01, S01-S05, Complete).",
    checking: "Sto controllando le tue serie…",
    noMatch: "Nessuna serie corrisponde ai filtri.",
    unverifiableTitle: "Serie non verificabili",
    unverifiableHint:
      "Serie di cui non si conoscono le stagioni: sul tracker manca l'id TMDB (e quello TVDB non porta a nessuna serie su TMDB), oppure TMDB non la conosce.",
  },
  saga: {
    icon: Clapperboard,
    noun: "Saghe",
    search: "Cerca una saga o un film…",
    tmdbUrl: (id) => `https://www.themoviedb.org/collection/${id}`,
    units: (n) => (n === 1 ? "film uscito" : "film usciti"),
    mine: (n) => (n === 1 ? "tuo upload" : "tuoi upload"),
    cellLabel: (u, prefix) => {
      const name = u.name.slice(prefix)
      return u.airDate ? `${name} (${u.airDate.slice(0, 4)})` : name
    },
    intro:
      "Per ogni film che hai caricato e che su TMDB appartiene a una saga: gli altri film della saga che mancano nella stessa qualità (tipo, risoluzione, HDR e lingue audio) e se sul tracker li hanno caricati altri. I film non ancora usciti non contano.",
    empty: "Nessuno dei tuoi film appartiene a una saga su TMDB.",
    checking: "Sto controllando i tuoi film…",
    noMatch: "Nessuna saga corrisponde ai filtri.",
    unverifiableTitle: "Film non verificabili",
    unverifiableHint:
      "Film di cui non si sa se appartengono a una saga: sul tracker manca l'id TMDB (e quello IMDb non porta a nessun film su TMDB), oppure TMDB non lo conosce.",
  },
}

const phases: Record<MissingCheckState["phase"], string> = {
  "": "",
  series: "serie",
  movies: "film",
  sagas: "saghe",
}

const states: Record<GapState, { label: string; hint: string; icon: React.ElementType; className: string }> = {
  mine: {
    label: "Mia",
    hint: "Caricata da te in questa qualità",
    icon: Check,
    className: "border-transparent bg-primary text-primary-foreground",
  },
  available: {
    label: "C'è, di altri",
    hint: "Caricata da altri nella stessa qualità",
    icon: Users,
    className: "border-emerald-500/40 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
  },
  other_quality: {
    label: "Solo altra qualità",
    hint: "Sul tracker c'è, ma solo in altre qualità",
    icon: Shuffle,
    className: "border-dashed border-amber-500/60 bg-amber-500/10 text-amber-800 dark:text-amber-200",
  },
  absent: {
    label: "Assente",
    hint: "Non c'è sul tracker in nessuna qualità",
    icon: X,
    className: "border-destructive/40 text-destructive bg-transparent",
  },
}

/**
 * Length of the start the names of the films of a saga share up to a separator
 * ("Il Signore degli Anelli - "), to leave out of the cells; 0 if none.
 */
function sharedPrefix(names: string[]): number {
  if (names.length < 2) return 0
  let common = names[0]
  for (const n of names) while (!n.startsWith(common)) common = common.slice(0, -1)
  const end = common.match(/^.*(?: [-–] |: )/)?.[0].length ?? 0
  return names.every((n) => n.length > end) ? end : 0
}

/** "2160p WEB-DL" of a quality label "2160p WEB-DL DV HDR ITA ENG". */
function inGroup(label: string, group: string) {
  return group === "all" || label === group || label.startsWith(group + " ")
}

type Totals = { missing: number; available: number; otherQuality: number; absent: number }

function totals(rows: GapRow[]): Totals {
  const t = { missing: 0, available: 0, otherQuality: 0, absent: 0 }
  for (const r of rows) {
    t.missing += r.missing
    t.available += r.available
    t.otherQuality += r.otherQuality
    t.absent += r.absent
  }
  return t
}

function Legend() {
  return (
    <div className="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
      {(Object.keys(states) as GapState[]).map((s) => {
        const st = states[s]
        return (
          <span key={s} className="flex items-center gap-1.5" title={st.hint}>
            <span className={cn("flex size-4 items-center justify-center rounded border", st.className)}>
              <st.icon className="size-3" />
            </span>
            {st.label}
          </span>
        )
      })}
    </div>
  )
}

function UnitCell({
  kind,
  cell,
  unit,
  prefix,
  onOpen,
}: {
  kind: GapKind
  cell: GapCell
  unit: GapUnit
  prefix: number
  onOpen: () => void
}) {
  const st = states[cell.state]
  const label = kinds[kind].cellLabel(unit, prefix)
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onOpen}
          disabled={cell.state === "absent"}
          className={cn(
            "flex h-8 min-w-12 items-center justify-center gap-1 rounded-md border px-1.5 text-xs font-medium tabular-nums transition-opacity",
            kind === "saga" && "max-w-64 min-w-0 px-2",
            st.className,
            cell.state !== "absent" && "hover:opacity-80"
          )}
          aria-label={`${unit.name}: ${st.label}`}
        >
          <st.icon className="size-3 shrink-0" />
          <span className="truncate">{label}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        <div className="font-medium">
          {unit.name}
          {unit.airDate && <span className="font-normal opacity-80"> · {formatDate(unit.airDate)}</span>}
        </div>
        <div>{st.hint}</div>
        {cell.state === "other_quality" && <div className="opacity-80">{cell.qualities.join(", ")}</div>}
        {cell.torrents.length > 0 && <div className="opacity-80">Clic per i link al tracker</div>}
      </TooltipContent>
    </Tooltip>
  )
}

type Opened = { group: GapGroup; row: GapRow; cell: GapCell; unit: GapUnit }

function TorrentLinks({ refs }: { refs: GapRef[] }) {
  return (
    <ul className="divide-y rounded-lg border">
      {refs.map((r) => (
        <li key={r.id} className="flex items-center gap-2 py-1.5 pr-1.5 pl-3">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm" title={r.name}>
              {r.name}
            </div>
            <div className="text-muted-foreground flex items-center gap-1.5 text-xs">
              {r.quality}
              {r.mine && (
                <Badge variant="brand" className="px-1.5 py-0">
                  tua
                </Badge>
              )}
            </div>
          </div>
          {r.url && (
            <Button variant="ghost" size="icon-sm" asChild>
              <a href={r.url} target="_blank" rel="noreferrer" aria-label="Apri sul tracker">
                <ExternalLink />
              </a>
            </Button>
          )}
        </li>
      ))}
    </ul>
  )
}

function GroupCard({ s, rows, onOpen }: { s: GapGroup; rows: GapRow[]; onOpen: (o: Opened) => void }) {
  const t = totals(rows)
  const k = kinds[s.kind]
  const prefix = s.kind === "saga" ? sharedPrefix(s.units.map((u) => u.name)) : 0
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex flex-wrap items-center gap-2">
          <k.icon className="text-muted-foreground size-4" />
          <span>{s.title}</span>
          {s.year && <span className="text-muted-foreground font-normal">({s.year})</span>}
          {t.missing > 0 ? (
            <Badge variant="warning">
              {t.missing} {t.missing === 1 ? "mancante" : "mancanti"}
            </Badge>
          ) : (
            <Badge variant="success">
              <Check /> Completa
            </Badge>
          )}
          {t.available > 0 && <Badge variant="success">{t.available} da altri</Badge>}
          <span className="ml-auto flex items-center gap-1">
            {s.searchUrl && (
              <Button variant="ghost" size="sm" asChild>
                <a href={s.searchUrl} target="_blank" rel="noreferrer">
                  <ExternalLink /> Tracker
                </a>
              </Button>
            )}
            <Button variant="ghost" size="sm" asChild>
              <a href={k.tmdbUrl(s.tmdbId)} target="_blank" rel="noreferrer">
                <ExternalLink /> TMDB
              </a>
            </Button>
          </span>
        </CardTitle>
        <CardDescription>
          {s.units.length} {k.units(s.units.length)} · {s.mine.length} {k.mine(s.mine.length)}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4">
        {rows.map((row) => (
          <div key={row.quality.key} className="grid gap-1.5 md:grid-cols-[minmax(180px,240px)_1fr] md:items-start md:gap-3">
            <div className="flex flex-wrap items-center gap-1 pt-1.5 text-sm">
              <span className="font-medium">{row.quality.label || "Qualità sconosciuta"}</span>
              {row.missing > 0 && (
                <span className="text-muted-foreground text-xs whitespace-nowrap">
                  · {row.missing} {row.missing === 1 ? "mancante" : "mancanti"}
                </span>
              )}
            </div>
            <div className="flex flex-wrap gap-1.5">
              {row.cells.map((cell) => {
                const unit = s.units.find((u) => u.number === cell.unit)
                if (!unit) return null
                return (
                  <UnitCell
                    key={cell.unit}
                    kind={s.kind}
                    cell={cell}
                    unit={unit}
                    prefix={prefix}
                    onOpen={() => onOpen({ group: s, row, cell, unit })}
                  />
                )
              })}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

/** The list of one kind: stats, filters, the cards and the unverifiable ones. */
function GapList({
  kind,
  all,
  running,
  checkedAt,
}: {
  kind: GapKind
  all: GapGroup[]
  running: boolean
  checkedAt: string | null | undefined
}) {
  const k = kinds[kind]
  const [query, setQuery] = useState("")
  const [onlyMissing, setOnlyMissing] = useState(true)
  const [group, setGroup] = useState("all")
  const [page, setPage] = useState(0)
  const [opened, setOpened] = useState<Opened | null>(null)

  const verifiable = useMemo(() => all.filter((s) => s.status === "ok"), [all])
  const unverifiable = useMemo(() => all.filter((s) => s.status !== "ok"), [all])

  const groups = useMemo(() => {
    const set = new Map<string, number>()
    for (const s of verifiable)
      for (const r of s.rows) {
        const g = [r.quality.resolution, r.quality.type].filter(Boolean).join(" ")
        if (g) set.set(g, parseInt(r.quality.resolution) || 0)
      }
    return [...set.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([g]) => g)
  }, [verifiable])

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    return verifiable
      .map((s) => ({ s, rows: s.rows.filter((r) => inGroup(r.quality.label, group)) }))
      .filter(({ s, rows }) => {
        if (rows.length === 0) return false
        if (onlyMissing && totals(rows).missing === 0) return false
        return (
          !q ||
          s.title.toLowerCase().includes(q) ||
          s.mine.some((m) => m.name.toLowerCase().includes(q)) ||
          (kind === "saga" && s.units.some((u) => u.name.toLowerCase().includes(q)))
        )
      })
  }, [verifiable, query, group, onlyMissing, kind])

  const stats = useMemo(() => {
    const scoped = verifiable.map((s) => s.rows.filter((r) => inGroup(r.quality.label, group))).filter((r) => r.length > 0)
    const t = scoped.map(totals)
    return {
      groups: scoped.length,
      withMissing: t.filter((x) => x.missing > 0).length,
      available: t.reduce((n, x) => n + x.available, 0),
      otherQuality: t.reduce((n, x) => n + x.otherQuality, 0),
      absent: t.reduce((n, x) => n + x.absent, 0),
    }
  }, [verifiable, group])

  const pages = Math.max(1, Math.ceil(shown.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = shown.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const reset = <T,>(fn: (v: T) => void) => (v: T) => {
    fn(v)
    setPage(0)
  }

  const statCards: { label: string; value: number; icon: React.ElementType; tone?: string }[] = [
    { label: k.noun, value: stats.groups, icon: Layers },
    { label: "Con mancanti", value: stats.withMissing, icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    { label: "Da altri, stessa qualità", value: stats.available, icon: Users, tone: "text-emerald-600 dark:text-emerald-400" },
    { label: "Solo altra qualità", value: stats.otherQuality, icon: Shuffle, tone: "text-amber-600 dark:text-amber-400" },
    { label: "Assenti dal tracker", value: stats.absent, icon: X, tone: "text-destructive" },
  ]

  return (
    <div className="grid gap-4">
      <p className="text-muted-foreground text-sm">{k.intro}</p>

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
        {statCards.map((s) => (
          <Card key={s.label} className="gap-1 px-4 py-3">
            <div className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <s.icon className={cn("size-3.5", s.tone)} />
              {s.label}
            </div>
            <div className={cn("text-2xl font-semibold tabular-nums", s.value > 0 && s.tone)}>{s.value}</div>
          </Card>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input value={query} onChange={(e) => reset(setQuery)(e.target.value)} placeholder={k.search} className="pl-8" />
        </div>
        <Select value={group} onValueChange={reset(setGroup)}>
          <SelectTrigger className="w-[200px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tutte le qualità</SelectItem>
            {groups.map((g) => (
              <SelectItem key={g} value={g}>
                {g}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex items-center gap-2 px-1">
          <Switch id={`missing-only-${kind}`} checked={onlyMissing} onCheckedChange={reset(setOnlyMissing)} />
          <Label htmlFor={`missing-only-${kind}`} className="whitespace-nowrap">
            Solo con mancanti
          </Label>
        </div>
      </div>

      <Legend />

      {visible.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {all.length === 0
              ? running
                ? k.checking
                : checkedAt
                  ? k.empty
                  : "Nessun controllo ancora: avvialo con Controlla ora, o attendi quello notturno."
              : k.noMatch}
          </CardContent>
        </Card>
      )}
      {visible.map(({ s, rows }) => (
        <GroupCard key={s.key} s={s} rows={rows} onOpen={setOpened} />
      ))}
      {pages > 1 && (
        <div className="text-muted-foreground flex items-center justify-end gap-1 text-xs">
          <span className="mr-auto">
            {shown.length} {k.noun.toLowerCase()}
          </span>
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

      {unverifiable.length > 0 && group === "all" && (
        <Card className="gap-3">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <CircleHelp className="text-muted-foreground size-4" /> {k.unverifiableTitle} ({unverifiable.length})
            </CardTitle>
            <CardDescription>{k.unverifiableHint}</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="max-h-72 divide-y overflow-y-auto rounded-lg border">
              {unverifiable.map((s) => (
                <li key={s.key} className="grid gap-0.5 px-3 py-2">
                  <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                    {s.title}
                    <span className="text-muted-foreground text-xs font-normal">{s.note}</span>
                  </div>
                  {s.mine.map((m) => (
                    <a
                      key={m.id}
                      href={m.url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-muted-foreground truncate text-xs hover:underline"
                      title={m.name}
                    >
                      {m.name}
                    </a>
                  ))}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <Dialog open={opened !== null} onOpenChange={(o) => !o && setOpened(null)}>
        <DialogContent className="sm:max-w-xl">
          {opened && (
            <>
              <DialogHeader>
                <DialogTitle>
                  {opened.group.title} · {opened.unit.name}
                </DialogTitle>
                <DialogDescription>
                  {states[opened.cell.state].hint}
                  {opened.cell.state === "other_quality" ? ": cercavi " : " · "}
                  {opened.row.quality.label}
                </DialogDescription>
              </DialogHeader>
              {opened.cell.state === "other_quality" && (
                <div className="flex flex-wrap gap-1">
                  {opened.cell.qualities.map((q) => (
                    <Badge key={q} variant="warning">
                      {q}
                    </Badge>
                  ))}
                </div>
              )}
              <TorrentLinks refs={opened.cell.torrents} />
              {opened.unit.tmdbId ? (
                <Button variant="ghost" size="sm" className="justify-self-start" asChild>
                  <a href={`https://www.themoviedb.org/movie/${opened.unit.tmdbId}`} target="_blank" rel="noreferrer">
                    <ExternalLink /> Il film su TMDB
                  </a>
                </Button>
              ) : null}
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

/** Missing seasons of the series and films of the sagas, one tab each, with the shared check. */
export function MissingPage({ view, onView }: { view: MissingView; onView: (v: MissingView) => void }) {
  const series = useMissingSeries()
  const sagas = useMissingSagas()
  const check = useAction(() => api.checkMissing())

  const current = view === "series" ? series : sagas
  const data = current.data
  // The same check fills both tabs.
  const state = data?.state
  const running = !!state?.running || check.isPending
  const seriesAll = useMemo(() => series.data?.series ?? [], [series.data])
  const sagasAll = useMemo(() => sagas.data?.sagas ?? [], [sagas.data])
  const missingOf = (all: GapGroup[]) => all.filter((g) => g.status === "ok" && g.missing > 0).length

  const run = () =>
    check.mutate(undefined, {
      onSuccess: () =>
        toast.success("Controllo avviato", {
          description: "Stagioni e saghe da TMDB e torrent dal tracker vengono letti in background.",
        }),
    })

  if (series.isLoading || sagas.isLoading) return <Skeleton className="h-48 w-full" />
  const failed = series.error ?? sagas.error
  if (failed)
    return (
      <Card>
        <CardContent className="text-destructive py-6 text-center text-sm">Errore nel caricamento: {failed.message}</CardContent>
      </Card>
    )

  if (data && !data.enabled && seriesAll.length === 0 && sagasAll.length === 0)
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CloudOff className="size-4" /> Controllo delle stagioni e dei film mancanti non attivo
          </CardTitle>
          <CardDescription>
            {data.disabledReason} Poi riavvia bookrr: ogni notte verranno controllate le serie di cui hai caricato almeno una
            stagione e le saghe di cui hai caricato almeno un film.
          </CardDescription>
        </CardHeader>
      </Card>
    )

  const phase = state?.phase ? phases[state.phase] : ""

  return (
    <Tabs value={view} onValueChange={(v) => onView(v as MissingView)} className="gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <TabsList>
          <TabsTrigger value="series">
            <Tv /> Serie TV
            {missingOf(seriesAll) > 0 && (
              <Badge variant="warning" className="ml-1 px-1.5">
                {missingOf(seriesAll)}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="sagas">
            <Clapperboard /> Saghe
            {missingOf(sagasAll) > 0 && (
              <Badge variant="warning" className="ml-1 px-1.5">
                {missingOf(sagasAll)}
              </Badge>
            )}
          </TabsTrigger>
        </TabsList>
        <div className="ml-auto flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground text-xs">
                {running
                  ? state?.total
                    ? `Controllo in corso: ${phase} (${state.done}/${state.total})…`
                    : "Controllo in corso…"
                  : `Controllato ${formatRelative(data?.checkedAt)}`}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {state?.lastResult || "Nessun controllo ancora"}
              {data?.checkAt && ` · ogni notte alle ${data.checkAt}`}
            </TooltipContent>
          </Tooltip>
          {data?.enabled && (
            <Button variant="outline" size="sm" disabled={running} onClick={run}>
              <RefreshCw className={cn(running && "animate-spin")} /> Controlla ora
            </Button>
          )}
        </div>
      </div>

      {data && !data.enabled && (
        <AlertBox variant="warning">
          <CloudOff />
          <AlertTitle>Controllo non attivo</AlertTitle>
          <AlertDescription>Questi risultati non vengono più aggiornati. {data.disabledReason}</AlertDescription>
        </AlertBox>
      )}
      {state?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Ultimo controllo non riuscito</AlertTitle>
          <AlertDescription>{state.lastError}</AlertDescription>
        </AlertBox>
      )}

      <TabsContent value="series">
        <GapList kind="series" all={seriesAll} running={running} checkedAt={series.data?.checkedAt} />
      </TabsContent>
      <TabsContent value="sagas">
        <GapList kind="saga" all={sagasAll} running={running} checkedAt={sagas.data?.checkedAt} />
      </TabsContent>
    </Tabs>
  )
}
