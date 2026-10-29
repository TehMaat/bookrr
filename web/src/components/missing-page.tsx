import { useMemo, useState } from "react"
import {
  ArrowDown,
  Ban,
  Check,
  CheckCheck,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Clapperboard,
  CloudOff,
  Eye,
  EyeOff,
  ExternalLink,
  Gauge,
  Grid3x3,
  Layers,
  Library,
  List,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Search,
  Shuffle,
  ThumbsUp,
  Trash2,
  TriangleAlert,
  Tv,
  Undo2,
  Users,
  X,
} from "lucide-react"
import { toast } from "sonner"

import { NavbarStats, StatCard } from "@/components/navbar-stats"
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
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import {
  api,
  type GapGroup,
  type GapKind,
  type GapListSource,
  type GapMarkInput,
  type GapMarkState,
  type GapPartState,
  type GapPrefs,
  type GapRef,
  type GapRow,
  type GapState,
  type GapThreshold,
  type GapUnit,
  type GapVersion,
  type MissingCheckState,
  type Quality,
} from "@/lib/api"
import { formatDate, formatRelative } from "@/lib/format"
import { useAction, useMissingLists, useMissingSagas, useMissingSeries } from "@/lib/queries"
import { cn } from "@/lib/utils"

const PAGE_SIZE = 30

export type MissingView = "series" | "sagas" | "lists"

/**
 * How a series, a saga or a collection is shown: one cell per season or film with its best version against the
 * minimum quality ("parts"), or one row per quality the user uploaded it in ("grid").
 */
type Layout = "parts" | "grid"

/** What changes between the series, the sagas and the collections. */
type KindText = {
  icon: React.ElementType
  /** Plural noun of the stat card and the pager: "Serie", "Saghe", "Collezioni". */
  noun: string
  /** Singular noun in the menus: "serie", "saga", "collezione". */
  one: string
  /** Name of the view by part: "Per stagione", "Per film". */
  byPart: string
  search: string
  /** The group on TMDB, for series and sagas. */
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

/** "La compagnia dell'anello (2001)": the name of a film without the start shared by the group. */
function filmLabel(u: GapUnit, prefix: number) {
  const name = u.name.slice(prefix)
  return u.airDate ? `${name} (${u.airDate.slice(0, 4)})` : name
}

const kinds: Record<GapKind, KindText> = {
  series: {
    icon: Tv,
    noun: "Serie",
    one: "serie",
    byPart: "Per stagione",
    search: "Cerca una serie…",
    tmdbUrl: (id) => `https://www.themoviedb.org/tv/${id}`,
    units: (n) => (n === 1 ? "stagione uscita" : "stagioni uscite"),
    mine: (n) => (n === 1 ? "tuo upload" : "tuoi upload"),
    cellLabel: (u) => `S${String(u.number).padStart(2, "0")}`,
    intro:
      "Per ogni serie di cui hai caricato almeno un season pack: le stagioni che mancano e se sul tracker le hanno caricate altri. Per stagione: la versione migliore di ogni stagione, mancante solo se sotto la qualità minima. Per qualità: una riga per ogni qualità (tipo, risoluzione, HDR e lingue audio) in cui l'hai caricata. Gli episodi singoli non contano.",
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
    one: "saga",
    byPart: "Per film",
    search: "Cerca una saga o un film…",
    tmdbUrl: (id) => `https://www.themoviedb.org/collection/${id}`,
    units: (n) => (n === 1 ? "film uscito" : "film usciti"),
    mine: (n) => (n === 1 ? "tuo upload" : "tuoi upload"),
    cellLabel: filmLabel,
    intro:
      "Per ogni film che hai caricato e che su TMDB appartiene a una saga: gli altri film della saga che mancano e se sul tracker li hanno caricati altri. Per film: la versione migliore di ogni film, mancante solo se sotto la qualità minima. Per qualità: una riga per ogni qualità (tipo, risoluzione, HDR e lingue audio) in cui hai caricato la saga. I film non ancora usciti non contano.",
    empty: "Nessuno dei tuoi film appartiene a una saga su TMDB.",
    checking: "Sto controllando i tuoi film…",
    noMatch: "Nessuna saga corrisponde ai filtri.",
    unverifiableTitle: "Film non verificabili",
    unverifiableHint:
      "Film di cui non si sa se appartengono a una saga: sul tracker manca l'id TMDB (e quello IMDb non porta a nessun film su TMDB), oppure TMDB non lo conosce.",
  },
  list: {
    icon: Library,
    noun: "Collezioni",
    one: "collezione",
    byPart: "Per film",
    search: "Cerca una collezione o un film…",
    tmdbUrl: (id) => `https://www.themoviedb.org/movie/${id}`,
    units: (n) => (n === 1 ? "film uscito" : "film usciti"),
    mine: (n) => (n === 1 ? "tuo upload" : "tuoi upload"),
    cellLabel: filmLabel,
    intro:
      "Per ogni collezione che segui (una lista di film su MDBList: un universo come l'MCU, i film Pixar, i classici Disney): i film che mancano e se sul tracker li hanno caricati altri, anche se non ne hai caricato nessuno. Per film: la versione migliore di ogni film, mancante solo se sotto la qualità minima. Per qualità: una riga per ogni qualità in cui hai caricato film della collezione. I film non ancora usciti non contano. Le collezioni sono indipendenti dalle saghe: le scelte fatte qui valgono solo qui.",
    empty: "Nessuna collezione: aggiungine una qui sopra con il link di una lista MDBList.",
    checking: "Sto controllando le tue collezioni…",
    noMatch: "Nessuna collezione corrisponde ai filtri.",
    unverifiableTitle: "Liste non trovate",
    unverifiableHint:
      "Collezioni la cui lista non c'è più su MDBList (cancellata o resa privata): correggi il link o smetti di seguirle.",
  },
}

const phases: Record<MissingCheckState["phase"], string> = {
  "": "",
  series: "serie",
  movies: "film",
  sagas: "saghe",
  lists: "collezioni",
}

type Look = { label: string; hint: string; icon: React.ElementType; className: string }

const looks = {
  mine: "border-transparent bg-primary text-primary-foreground",
  available: "border-emerald-500/40 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
  other: "border-dashed border-amber-500/60 bg-amber-500/10 text-amber-800 dark:text-amber-200",
  absent: "border-destructive/40 text-destructive bg-transparent",
  banned: "border-dashed border-pink-500/50 bg-pink-500/10 text-pink-700 dark:text-pink-300",
}

/** States of a cell of the view by quality. */
const states: Record<GapState, Look> = {
  mine: { label: "Mia", hint: "Caricata da te in questa qualità", icon: Check, className: looks.mine },
  available: {
    label: "C'è, di altri",
    hint: "Caricata da altri nella stessa qualità",
    icon: Users,
    className: looks.available,
  },
  banned: {
    label: "Solo gruppi bannati",
    hint: "Caricata da altri nella stessa qualità, ma solo da gruppi che hai bannato: manca, con meno priorità",
    icon: Ban,
    className: looks.banned,
  },
  other_quality: {
    label: "Solo altra qualità",
    hint: "Sul tracker c'è, ma solo in altre qualità",
    icon: Shuffle,
    className: looks.other,
  },
  absent: { label: "Assente", hint: "Non c'è sul tracker in nessuna qualità", icon: X, className: looks.absent },
}

/** States of a cell of the view by part. */
const partStates: Record<GapPartState, Look> = {
  mine: { label: "Tua", hint: "Caricata da te, almeno nella qualità minima", icon: Check, className: looks.mine },
  available: {
    label: "C'è, di altri",
    hint: "Caricata da altri, almeno nella qualità minima",
    icon: Users,
    className: looks.available,
  },
  banned: {
    label: "Solo gruppi bannati",
    hint: "Caricata da altri almeno nella qualità minima, ma solo da gruppi che hai bannato: manca, con meno priorità",
    icon: Ban,
    className: looks.banned,
  },
  below: {
    label: "Sotto la soglia",
    hint: "Sul tracker c'è, ma solo sotto la qualità minima",
    icon: ArrowDown,
    className: looks.other,
  },
  absent: { label: "Assente", hint: "Non c'è sul tracker in nessuna qualità", icon: X, className: looks.absent },
}

/** The user's choices, shown over the state. */
const marks: Record<Exclude<GapMarkState, "">, Look> = {
  accepted: {
    label: "Va bene così",
    hint: "Hai accettato una release di altri: non conta come mancante",
    icon: ThumbsUp,
    className: "border-sky-500/50 bg-sky-500/10 text-sky-800 dark:text-sky-200",
  },
  ignored: {
    label: "Ignorata",
    hint: "L'hai ignorata: non conta come mancante",
    icon: EyeOff,
    className: "border-dashed border-muted-foreground/40 bg-transparent text-muted-foreground",
  },
  complete: {
    label: "Segnata completa",
    hint: "Hai segnato completa la collezione: non conta come mancante",
    icon: CheckCheck,
    className: "border-muted-foreground/30 bg-muted text-muted-foreground",
  },
}

const ANY = "any"

const noPrefs: GapPrefs = { type: "", resolution: "", lang: "", othersCount: false }

const noGroups: string[] = []

/** "Remux · 1080p · ITA", or "qualsiasi". */
function thresholdLabel(t: GapThreshold) {
  return [t.type, t.resolution, t.lang].filter(Boolean).join(" · ") || "qualsiasi"
}

/** "2160p Remux DV HDR": the quality without the languages, for the cells. */
function shortQuality(q: Quality) {
  return [q.resolution, q.type, ...q.hdr].filter(Boolean).join(" ")
}

/**
 * Length of the start the names of the films of a saga (or a collection) share up to a separator
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

type Totals = { missing: number; available: number; otherQuality: number; absent: number; banned: number }

function totals(rows: GapRow[]): Totals {
  const t = { missing: 0, available: 0, otherQuality: 0, absent: 0, banned: 0 }
  for (const r of rows) {
    t.missing += r.missing
    t.available += r.available
    t.otherQuality += r.otherQuality
    t.absent += r.absent
    t.banned += r.banned
  }
  return t
}

/** Whether a group is shown by part: the layout asks for it and the group comes from a recent check. */
function byPart(s: GapGroup, layout: Layout) {
  return layout === "parts" && s.parts !== null
}

/** What is missing in a group, in the layout. */
function missingIn(s: GapGroup, layout: Layout) {
  return byPart(s, layout) ? s.partTotals.missing : totals(s.rows.filter((r) => !r.hidden)).missing
}

/** What is missing in a group, in the layout, leaving out what is there only by banned groups (less priority). */
function urgentIn(s: GapGroup, layout: Layout) {
  const t = byPart(s, layout) ? s.partTotals : totals(s.rows.filter((r) => !r.hidden))
  return t.missing - t.banned
}

/** The release group of a torrent, for the hints. */
function groupOf(r: GapRef) {
  return r.group || "gruppo sconosciuto"
}

const layoutKey = (kind: GapKind) => `bookrr.missing.layout.${kind}`

/** The layout of each tab, remembered by the browser; by part unless chosen otherwise. */
function useLayouts() {
  const read = (kind: GapKind): Layout => {
    try {
      return localStorage.getItem(layoutKey(kind)) === "grid" ? "grid" : "parts"
    } catch {
      return "parts"
    }
  }
  const [layouts, setLayouts] = useState<Record<GapKind, Layout>>(() => ({
    series: read("series"),
    saga: read("saga"),
    list: read("list"),
  }))
  const set = (kind: GapKind, l: Layout) => {
    setLayouts((v) => ({ ...v, [kind]: l }))
    try {
      localStorage.setItem(layoutKey(kind), l)
    } catch {
      /* not remembered */
    }
  }
  return [layouts, set] as const
}

function Legend({ layout }: { layout: Layout }) {
  const all = [...Object.values(layout === "parts" ? partStates : states), ...Object.values(marks)]
  return (
    <div className="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
      {all.map((st) => (
        <span key={st.label} className="flex items-center gap-1.5" title={st.hint}>
          <span className={cn("flex size-4 items-center justify-center rounded border", st.className)}>
            <st.icon className="size-3" />
          </span>
          {st.label}
        </span>
      ))}
    </div>
  )
}

/** A cell of either layout: a season or a film, how it stands, and the quality shown when there is one. */
function CellButton({
  kind,
  look,
  label,
  quality,
  unit,
  hint,
  onOpen,
}: {
  kind: GapKind
  look: Look
  label: string
  quality?: string
  unit: GapUnit
  hint?: React.ReactNode
  onOpen: () => void
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onOpen}
          className={cn(
            "flex h-8 min-w-12 items-center justify-center gap-1 rounded-md border px-1.5 text-xs font-medium tabular-nums transition-opacity hover:opacity-80",
            kind !== "series" && "max-w-80 min-w-0 px-2",
            look.className
          )}
          aria-label={`${unit.name}: ${look.label}`}
        >
          <look.icon className="size-3 shrink-0" />
          <span className="truncate">{label}</span>
          {quality && <span className="shrink-0 font-normal opacity-75">· {quality}</span>}
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        <div className="font-medium">
          {unit.name}
          {unit.airDate && <span className="font-normal opacity-80"> · {formatDate(unit.airDate)}</span>}
        </div>
        <div>{look.hint}</div>
        {hint && <div className="opacity-80">{hint}</div>}
        <div className="opacity-80">Clic per i link al tracker e le scelte</div>
      </TooltipContent>
    </Tooltip>
  )
}

/** A cell opened: a part of a group, in a quality row or in the view by part (row null). */
type Opened = { group: string; unit: number; row: string | null }

/** Changes to the choices of the user on a kind of check. */
type MarkFn = (input: GapMarkInput, remove?: boolean) => void

function TorrentLinks({
  refs,
  accepted,
  onAccept,
  onUnaccept,
  onBan,
}: {
  refs: (GapRef | GapVersion)[]
  accepted: number
  onAccept?: (id: number) => void
  onUnaccept?: () => void
  /** Bans the release group of a torrent of others, or lets it back. */
  onBan?: (group: string, ban: boolean) => void
}) {
  if (refs.length === 0) return <p className="text-muted-foreground text-sm">Nessun torrent sul tracker.</p>
  return (
    <ul className="max-h-80 divide-y overflow-y-auto rounded-lg border">
      {refs.map((r) => (
        <li key={r.id} className="flex items-center gap-2 py-1.5 pr-1.5 pl-3">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm" title={r.name}>
              {r.name}
            </div>
            <div className="text-muted-foreground flex flex-wrap items-center gap-1.5 text-xs">
              {r.quality}
              {r.group && (
                <Badge variant="outline" className="px-1.5 py-0" title="Release group">
                  {r.group}
                </Badge>
              )}
              {r.mine && (
                <Badge variant="brand" className="px-1.5 py-0">
                  tua
                </Badge>
              )}
              {r.banned && (
                <Badge variant="pink" className="px-1.5 py-0">
                  <Ban /> gruppo bannato
                </Badge>
              )}
              {"meets" in r && !r.meets && (
                <Badge variant="warning" className="px-1.5 py-0">
                  sotto la soglia
                </Badge>
              )}
              {r.id === accepted && (
                <Badge variant="info" className="px-1.5 py-0">
                  <ThumbsUp /> accettata
                </Badge>
              )}
            </div>
          </div>
          {!r.mine && r.group && onBan && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className={cn(r.banned && "text-pink-600 dark:text-pink-400")}
                  aria-label={r.banned ? `Togli il ban a ${r.group}` : `Banna ${r.group}`}
                  onClick={() => onBan(r.group, !r.banned)}
                >
                  <Ban />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {r.banned
                  ? `Togli il ban a ${r.group}: le sue release tornano a contare come le altre`
                  : `Banna ${r.group}: le sue release segnalano la parte come mancante, con meno priorità`}
              </TooltipContent>
            </Tooltip>
          )}
          {!r.mine &&
            onAccept &&
            (r.id === accepted ? (
              <Button variant="ghost" size="sm" onClick={onUnaccept}>
                <Undo2 /> Annulla
              </Button>
            ) : (
              <Button variant="outline" size="sm" onClick={() => onAccept(r.id)}>
                <ThumbsUp /> Va bene questa
              </Button>
            ))}
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

/** The minimum quality: type, resolution and audio language. */
function ThresholdDialog({
  open,
  onOpenChange,
  title,
  description,
  initial,
  types,
  resolutions,
  onSave,
  onReset,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  title: string
  description: string
  initial: GapThreshold
  types: string[]
  resolutions: string[]
  onSave: (t: GapThreshold) => void
  onReset?: () => void
}) {
  const [t, setT] = useState(initial)
  const [prev, setPrev] = useState({ open, initial })
  if (prev.open !== open || prev.initial !== initial) {
    setPrev({ open, initial })
    if (open) setT(initial)
  }
  const pick = (field: keyof GapThreshold) => (v: string) => setT({ ...t, [field]: v === ANY ? "" : v })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label>Tipo minimo</Label>
            <Select value={t.type || ANY} onValueChange={pick("type")}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>Qualsiasi</SelectItem>
                {types.map((v) => (
                  <SelectItem key={v} value={v}>
                    {v} o meglio
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label>Risoluzione minima</Label>
            <Select value={t.resolution || ANY} onValueChange={pick("resolution")}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>Qualsiasi</SelectItem>
                {[...resolutions].reverse().map((v) => (
                  <SelectItem key={v} value={v}>
                    {v} o più
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="threshold-lang">Lingua audio richiesta</Label>
            <Input
              id="threshold-lang"
              value={t.lang}
              maxLength={8}
              placeholder="Qualsiasi (es. ITA)"
              onChange={(e) => setT({ ...t, lang: e.target.value.toUpperCase() })}
            />
            <p className="text-muted-foreground text-xs">
              Le release di cui non si conoscono le lingue (o solo MULTI) contano come se l'avessero.
            </p>
          </div>
        </div>
        <DialogFooter>
          {onReset && (
            <Button variant="ghost" className="mr-auto" onClick={onReset}>
              Usa quella generale
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Annulla
          </Button>
          <Button onClick={() => onSave(t)}>Salva</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function GroupCard({
  s,
  layout,
  group,
  onOpen,
  onMark,
  onThreshold,
}: {
  s: GapGroup
  layout: Layout
  /** The quality filter of the grid. */
  group: string
  onOpen: (o: Opened) => void
  onMark: MarkFn
  onThreshold: (s: GapGroup) => void
}) {
  const k = kinds[s.kind]
  const parts = byPart(s, layout)
  const prefix = s.kind !== "series" ? sharedPrefix(s.units.map((u) => u.name)) : 0
  const inFilter = s.rows.filter((r) => inGroup(r.quality.label, group))
  const rows = inFilter.filter((r) => !r.hidden)
  const hiddenRows = inFilter.filter((r) => r.hidden)
  const t = parts
    ? { missing: s.partTotals.missing, available: s.partTotals.available, banned: s.partTotals.banned }
    : (({ missing, available, banned }) => ({ missing, available, banned }))(totals(rows))
  const urgent = t.missing - t.banned
  const unitOf = (n: number) => s.units.find((u) => u.number === n)
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex flex-wrap items-center gap-2">
          <k.icon className="text-muted-foreground size-4" />
          <span>{s.title}</span>
          {s.year && <span className="text-muted-foreground font-normal">({s.year})</span>}
          {!parts && s.rows.length === 0 ? (
            <Badge variant="secondary">Nessun tuo upload</Badge>
          ) : urgent > 0 ? (
            <Badge variant="warning">
              {urgent} {urgent === 1 ? "mancante" : "mancanti"}
            </Badge>
          ) : t.banned > 0 ? null : s.completed ? (
            <Badge variant="secondary">
              <CheckCheck /> Segnata completa
            </Badge>
          ) : (
            <Badge variant="success">
              <Check /> Completa
            </Badge>
          )}
          {t.banned > 0 && (
            <Badge variant="pink" title="Mancanti che sul tracker ci sono solo da gruppi che hai bannato: meno priorità">
              <Ban /> {t.banned} {t.banned === 1 ? "mancante" : "mancanti"}, solo bannati
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
            {s.sourceUrl && (
              <Button variant="ghost" size="sm" asChild>
                <a href={s.sourceUrl} target="_blank" rel="noreferrer">
                  <ExternalLink /> MDBList
                </a>
              </Button>
            )}
            {s.tmdbId > 0 && (
              <Button variant="ghost" size="sm" asChild>
                <a href={k.tmdbUrl(s.tmdbId)} target="_blank" rel="noreferrer">
                  <ExternalLink /> TMDB
                </a>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-sm" aria-label="Altre azioni">
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {s.completed ? (
                  <DropdownMenuItem onClick={() => onMark({ group: s.key, action: "complete" }, true)}>
                    <Undo2 /> Togli il segno di completa
                  </DropdownMenuItem>
                ) : (
                  <DropdownMenuItem onClick={() => onMark({ group: s.key, action: "complete" })}>
                    <CheckCheck /> Segna come completa
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onClick={() => onThreshold(s)}>
                  <Gauge /> Qualità minima di questa {k.one}…
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => onMark({ group: s.key, action: "hide" })}>
                  <EyeOff /> Nascondi questa {k.one}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </span>
        </CardTitle>
        <CardDescription>
          {s.units.length} {k.units(s.units.length)} · {s.mine.length} {k.mine(s.mine.length)}
          {s.customThreshold && <> · qualità minima di questa {k.one}: {thresholdLabel(s.threshold)}</>}
          {layout === "parts" && s.stale && <> · vista {k.byPart.toLowerCase()} dopo il prossimo controllo</>}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4">
        {parts ? (
          <div className="flex flex-wrap gap-1.5">
            {s.parts!.map((p) => {
              const unit = unitOf(p.unit)
              if (!unit) return null
              const look = p.mark ? marks[p.mark] : partStates[p.state]
              // An accepted torrent of others stands for the part.
              const shown = (p.mark === "accepted" && unit.versions.find((v) => v.id === p.accepted)) || p.best
              return (
                <CellButton
                  key={p.unit}
                  kind={s.kind}
                  look={look}
                  unit={unit}
                  label={k.cellLabel(unit, prefix)}
                  quality={shown ? shortQuality(shown.q) : undefined}
                  hint={
                    shown &&
                    `${shown.mine ? "Tua" : shown.banned ? `Di ${shown.group}, bannato` : "Di altri"}: ${shown.quality}${shown.group && !shown.banned ? ` · ${shown.group}` : ""}`
                  }
                  onOpen={() => onOpen({ group: s.key, unit: p.unit, row: null })}
                />
              )
            })}
          </div>
        ) : (
          rows.map((row) => (
            <div
              key={row.quality.key}
              className="group/row grid gap-1.5 md:grid-cols-[minmax(180px,240px)_1fr] md:items-start md:gap-3"
            >
              <div className="flex flex-wrap items-center gap-1 pt-1.5 text-sm">
                <span className="font-medium">{row.quality.label || "Qualità sconosciuta"}</span>
                {row.missing > 0 && (
                  <span className="text-muted-foreground text-xs whitespace-nowrap">
                    · {row.missing} {row.missing === 1 ? "mancante" : "mancanti"}
                  </span>
                )}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="text-muted-foreground size-6 opacity-60 group-hover/row:opacity-100"
                      aria-label="Nascondi questa qualità"
                      onClick={() => onMark({ group: s.key, action: "hide", quality: row.quality.key })}
                    >
                      <EyeOff className="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Nascondi questa qualità: non conta più</TooltipContent>
                </Tooltip>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {row.cells.map((cell) => {
                  const unit = unitOf(cell.unit)
                  if (!unit) return null
                  const look = cell.mark ? marks[cell.mark] : states[cell.state]
                  return (
                    <CellButton
                      key={cell.unit}
                      kind={s.kind}
                      look={look}
                      unit={unit}
                      label={k.cellLabel(unit, prefix)}
                      hint={
                        cell.state === "other_quality"
                          ? cell.qualities.join(", ")
                          : cell.state === "banned"
                            ? [...new Set(cell.torrents.map(groupOf))].join(", ")
                            : undefined
                      }
                      onOpen={() => onOpen({ group: s.key, unit: cell.unit, row: row.quality.key })}
                    />
                  )
                })}
              </div>
            </div>
          ))
        )}
        {!parts && s.rows.length === 0 && (
          <p className="text-muted-foreground text-sm">
            Non hai ancora caricato film di questa {k.one}: le righe per qualità compaiono dal primo. La vista{" "}
            {k.byPart.toLowerCase()} mostra già cosa c'è sul tracker.
          </p>
        )}
        {!parts && hiddenRows.length > 0 && (
          <div className="text-muted-foreground flex flex-wrap items-center gap-1.5 text-xs">
            <EyeOff className="size-3.5" />
            {hiddenRows.length === 1 ? "Qualità nascosta:" : "Qualità nascoste:"}
            {hiddenRows.map((r) => (
              <Button
                key={r.quality.key}
                variant="outline"
                size="sm"
                className="h-6 px-2 text-xs"
                title="Mostra di nuovo"
                onClick={() => onMark({ group: s.key, action: "hide", quality: r.quality.key }, true)}
              >
                <Eye /> {r.quality.label || "Qualità sconosciuta"}
              </Button>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/** The dialog of a cell: its torrents, accepting one of others, ignoring it. */
function CellDialog({
  opened,
  all,
  onClose,
  onMark,
  onBan,
}: {
  opened: Opened | null
  all: GapGroup[]
  onClose: () => void
  onMark: MarkFn
  onBan: (group: string, ban: boolean) => void
}) {
  const s = opened ? all.find((g) => g.key === opened.group) : undefined
  const unit = s?.units.find((u) => u.number === opened?.unit)
  const row = opened?.row != null ? s?.rows.find((r) => r.quality.key === opened.row) : undefined
  const cell = row?.cells.find((c) => c.unit === opened?.unit)
  const part = opened?.row == null ? s?.parts?.find((p) => p.unit === opened?.unit) : undefined
  const show = !!s && !!unit && (!!cell || !!part)

  let body: React.ReactNode = null
  if (show) {
    const mine = (cell ?? part)!.state === "mine"
    const mark = (cell ?? part)!.mark
    const accepted = (cell ?? part)!.accepted
    // The choice is made for this row, or for every row from the view by part.
    const scope = row ? row.quality.key : ""
    const markScope = cell ? cell.markQuality : ""
    const look = mark ? marks[mark] : cell ? states[cell.state] : partStates[part!.state]
    const set = (action: "accept" | "ignore", torrent?: number) =>
      onMark({ group: s.key, action, quality: scope, unit: unit.id, torrent })
    const unset = (action: "accept" | "ignore") => onMark({ group: s.key, action, quality: markScope, unit: unit.id }, true)
    const refs = cell ? cell.torrents : unit.versions
    body = (
      <>
        <DialogHeader>
          <DialogTitle>
            {s.title} · {unit.name}
          </DialogTitle>
          <DialogDescription>
            {look.hint}
            {row
              ? `${cell!.state === "other_quality" ? ": cercavi " : " · "}${row.quality.label}`
              : ` · qualità minima: ${thresholdLabel(s.threshold)}`}
          </DialogDescription>
        </DialogHeader>
        {cell?.state === "other_quality" && (
          <div className="flex flex-wrap gap-1">
            {cell.qualities.map((q) => (
              <Badge key={q} variant="warning">
                {q}
              </Badge>
            ))}
          </div>
        )}
        {mark && mark !== "complete" && cell && markScope === "" && (
          <p className="text-muted-foreground text-xs">Scelta fatta per tutte le qualità, dalla vista {kinds[s.kind].byPart.toLowerCase()}.</p>
        )}
        <TorrentLinks
          refs={refs}
          accepted={mark === "accepted" ? accepted : 0}
          onAccept={mine ? undefined : (id) => set("accept", id)}
          onUnaccept={() => unset("accept")}
          onBan={onBan}
        />
        {!mine && (
          <p className="text-muted-foreground text-xs">
            {row
              ? `"Va bene questa" e "Ignora" valgono solo per ${row.quality.label || "questa qualità"}.`
              : `"Va bene questa" e "Ignora" valgono per tutte le qualità, anche nella vista per qualità.`}
          </p>
        )}
        <DialogFooter className="sm:justify-between">
          {unit.tmdbId ? (
            <Button variant="ghost" size="sm" asChild>
              <a href={`https://www.themoviedb.org/movie/${unit.tmdbId}`} target="_blank" rel="noreferrer">
                <ExternalLink /> Il film su TMDB
              </a>
            </Button>
          ) : (
            <span />
          )}
          {!mine &&
            (mark === "ignored" ? (
              <Button variant="outline" size="sm" onClick={() => unset("ignore")}>
                <Undo2 /> Non ignorare più
              </Button>
            ) : (
              <Button variant="outline" size="sm" onClick={() => set("ignore")}>
                <EyeOff /> Ignora: non è mancante
              </Button>
            ))}
        </DialogFooter>
      </>
    )
  }
  return (
    <Dialog open={show} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-2xl">{body}</DialogContent>
    </Dialog>
  )
}

/** The release groups the user banned, for every kind of check: add some, take some back. */
function BannedDialog({
  open,
  onOpenChange,
  groups,
  onSave,
  saving,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  groups: string[]
  onSave: (groups: string[]) => void
  saving: boolean
}) {
  const [list, setList] = useState(groups)
  const [text, setText] = useState("")
  const [prev, setPrev] = useState({ open, groups })
  if (prev.open !== open || prev.groups !== groups) {
    setPrev({ open, groups })
    if (open) {
      setList(groups)
      setText("")
    }
  }
  // "FLUX, -Arc" → FLUX and Arc, once each whatever the case.
  const words = (t: string) => t.split(/[\s,;]+/).map((w) => w.replace(/^-+/, "")).filter(Boolean)
  const merge = (more: string[]) => {
    const seen = new Set(list.map((g) => g.toLowerCase()))
    const out = [...list]
    for (const w of more)
      if (!seen.has(w.toLowerCase())) {
        seen.add(w.toLowerCase())
        out.push(w)
      }
    return out
  }
  const add = (e: React.FormEvent) => {
    e.preventDefault()
    setList(merge(words(text)))
    setText("")
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Gruppi bannati</DialogTitle>
          <DialogDescription>
            Le release di questi gruppi non bastano: se sul tracker una stagione o un film c'è solo da loro, resta mancante
            ma in rosa, con meno priorità. Il gruppo è quello dopo l'ultimo "-" del nome. Vale per serie, saghe e
            collezioni; le tue release non contano mai come bannate.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={add} className="flex gap-2">
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Es. FLUX, YIFY"
            aria-label="Gruppi da bannare"
            className="flex-1"
          />
          <Button type="submit" variant="outline" disabled={!text.trim()}>
            <Plus /> Aggiungi
          </Button>
        </form>
        {list.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {list.map((g) => (
              <Badge key={g} variant="pink" className="gap-1 pr-1">
                <Ban /> {g}
                <button
                  type="button"
                  className="hover:bg-pink-500/20 rounded p-0.5"
                  aria-label={`Togli il ban a ${g}`}
                  onClick={() => setList(list.filter((x) => x !== g))}
                >
                  <X className="size-3" />
                </button>
              </Badge>
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm">Nessun gruppo bannato.</p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Annulla
          </Button>
          <Button disabled={saving} onClick={() => onSave(text.trim() ? merge(words(text)) : list)}>
            Salva
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** The collections followed: add one from its address on MDBList, see whether it was checked, stop following it. */
function ListSources({
  sources,
  groups,
  enabled,
  disabledReason,
}: {
  sources: GapListSource[]
  groups: GapGroup[]
  enabled: boolean
  disabledReason: string
}) {
  const [url, setUrl] = useState("")
  const [removing, setRemoving] = useState<GapListSource | null>(null)
  const add = useAction((u: string) => api.addGapList(u))
  const remove = useAction((id: number) => api.removeGapList(id), "Collezione rimossa")
  const byKey = new Map(groups.map((g) => [g.key, g]))

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!url.trim()) return
    add.mutate(url.trim(), {
      onSuccess: (l) => {
        setUrl("")
        toast.success(`Collezione "${l.title || l.path}" aggiunta`, {
          description: "La controllo subito: tra poco compaiono i suoi film.",
        })
      },
    })
  }

  return (
    <Card className="gap-3">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Library className="text-muted-foreground size-4" /> Collezioni seguite ({sources.length})
        </CardTitle>
        <CardDescription>
          Incolla il link di una lista pubblica di MDBList: una fatta da te o da altri, anche copiata da IMDb o Trakt.
          Ogni controllo la rilegge, quindi se cambia su MDBList cambia anche qui.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {enabled ? (
          <form onSubmit={submit} className="flex flex-wrap gap-2">
            <Input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://mdblist.com/lists/utente/nome-lista"
              aria-label="Link della lista su MDBList"
              className="min-w-[240px] flex-1"
            />
            <Button type="submit" disabled={add.isPending || !url.trim()}>
              <Plus /> Aggiungi
            </Button>
          </form>
        ) : (
          <p className="text-muted-foreground text-sm">{disabledReason}</p>
        )}
        {sources.length > 0 && (
          <ul className="max-h-72 divide-y overflow-y-auto rounded-lg border">
            {sources.map((l) => {
              const g = byKey.get(`${l.source}:${l.path}`)
              const status = !g
                ? "In attesa del controllo"
                : g.status !== "ok"
                  ? "Lista non trovata su MDBList"
                  : `${g.units.length} ${kinds.list.units(g.units.length)} · ${g.mine.length} ${kinds.list.mine(g.mine.length)}${g.hidden ? " · nascosta" : ""}`
              return (
                <li key={l.id} className="flex items-center gap-2 py-1.5 pr-1.5 pl-3">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{l.title || l.path}</div>
                    <div
                      className={cn(
                        "truncate text-xs",
                        g && g.status !== "ok" ? "text-destructive" : "text-muted-foreground"
                      )}
                    >
                      {status}
                    </div>
                  </div>
                  <Button variant="ghost" size="icon-sm" asChild>
                    <a href={l.url} target="_blank" rel="noreferrer" aria-label="Apri su MDBList">
                      <ExternalLink />
                    </a>
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label="Smetti di seguire" onClick={() => setRemoving(l)}>
                    <Trash2 />
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
      <AlertDialog open={removing !== null} onOpenChange={(o) => !o && setRemoving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Smettere di seguire "{removing?.title || removing?.path}"?</AlertDialogTitle>
            <AlertDialogDescription>
              Spariscono i suoi risultati e le scelte fatte sui suoi film (accettati, ignorati, qualità minima). La
              lista su MDBList resta com'è.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annulla</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => removing && remove.mutate(removing.id)}
            >
              Smetti di seguire
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}

/** The list of one kind: stats, filters, the cards, the hidden and the unverifiable ones. */
function GapList({
  kind,
  all,
  running,
  checkedAt,
  prefs,
  bannedGroups,
  types,
  resolutions,
  layout,
  onLayout,
  onCheck,
  header,
  empty,
}: {
  kind: GapKind
  all: GapGroup[]
  running: boolean
  checkedAt: string | null | undefined
  prefs: GapPrefs
  /** Release groups the user banned, for every kind. */
  bannedGroups: string[]
  types: string[]
  resolutions: string[]
  layout: Layout
  onLayout: (l: Layout) => void
  onCheck?: () => void
  /** Shown under the introduction. */
  header?: React.ReactNode
  /** What to say when there is nothing at all, in place of the usual. */
  empty?: string
}) {
  const k = kinds[kind]
  const [query, setQuery] = useState("")
  const [onlyMissing, setOnlyMissing] = useState(true)
  const [group, setGroup] = useState("all")
  const [page, setPage] = useState(0)
  const [opened, setOpened] = useState<Opened | null>(null)
  const [showHidden, setShowHidden] = useState(false)
  // The minimum quality being edited: of every group (null key) or of one.
  const [editing, setEditing] = useState<{ key: string | null; title: string; initial: GapThreshold; custom: boolean } | null>(
    null
  )

  const mark = useAction(({ input, remove }: { input: GapMarkInput; remove?: boolean }) =>
    remove ? api.removeGapMark(kind, input) : api.setGapMark(kind, input)
  )
  const savePrefs = useAction((p: GapPrefs) => api.saveMissingPrefs(kind, p))
  const onMark: MarkFn = (input, remove) => mark.mutate({ input, remove })
  const [editingBanned, setEditingBanned] = useState(false)
  const saveBanned = useAction((groups: string[]) => api.saveBannedGroups(groups))
  const onBan = (group: string, ban: boolean) =>
    saveBanned.mutate(
      ban ? [...bannedGroups, group] : bannedGroups.filter((g) => g.toLowerCase() !== group.toLowerCase()),
      { onSuccess: () => toast.success(ban ? `${group} bannato` : `Ban tolto a ${group}`) }
    )

  const grid = layout === "grid"
  const verifiable = useMemo(() => all.filter((s) => s.status === "ok"), [all])
  const visibleGroups = useMemo(() => verifiable.filter((s) => !s.hidden), [verifiable])
  const hidden = useMemo(() => verifiable.filter((s) => s.hidden), [verifiable])
  const unverifiable = useMemo(() => all.filter((s) => s.status !== "ok"), [all])
  const stale = layout === "parts" && verifiable.some((s) => s.stale)
  const quality = grid ? group : "all"

  const groups = useMemo(() => {
    const set = new Map<string, number>()
    for (const s of visibleGroups)
      for (const r of s.rows) {
        const g = [r.quality.resolution, r.quality.type].filter(Boolean).join(" ")
        if (g) set.set(g, parseInt(r.quality.resolution) || 0)
      }
    return [...set.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([g]) => g)
  }, [visibleGroups])

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    const matching = visibleGroups.filter((s) => {
      const rows = s.rows.filter((r) => inGroup(r.quality.label, quality))
      // A collection with none of the user's films has no rows: shown anyway, with every quality.
      const noRows = s.rows.length === 0 && quality === "all"
      if (rows.length === 0 && !noRows) return false
      if (onlyMissing && !(noRows && !byPart(s, layout))) {
        const missing = byPart(s, layout) ? s.partTotals.missing : totals(rows.filter((r) => !r.hidden)).missing
        if (missing === 0) return false
      }
      return (
        !q ||
        s.title.toLowerCase().includes(q) ||
        s.mine.some((m) => m.name.toLowerCase().includes(q)) ||
        (kind !== "series" && s.units.some((u) => u.name.toLowerCase().includes(q)))
      )
    })
    // Missing only what banned groups uploaded: less priority, after the others.
    const later = (s: GapGroup) => (urgentIn(s, layout) === 0 && missingIn(s, layout) > 0 ? 1 : 0)
    return matching.sort((a, b) => later(a) - later(b))
  }, [visibleGroups, query, quality, onlyMissing, kind, layout])

  const stats = useMemo(() => {
    const t = visibleGroups
      .map((s) => {
        if (byPart(s, layout)) {
          const p = s.partTotals
          return { missing: p.missing, available: p.available, banned: p.banned, other: p.below, absent: p.absent }
        }
        const rows = s.rows.filter((r) => !r.hidden && inGroup(r.quality.label, quality))
        if (!s.rows.some((r) => inGroup(r.quality.label, quality))) return null
        const x = totals(rows)
        return { missing: x.missing, available: x.available, banned: x.banned, other: x.otherQuality, absent: x.absent }
      })
      .filter((x) => x !== null)
    return {
      groups: t.length,
      withMissing: t.filter((x) => x.missing > x.banned).length,
      available: t.reduce((n, x) => n + x.available, 0),
      banned: t.reduce((n, x) => n + x.banned, 0),
      other: t.reduce((n, x) => n + x.other, 0),
      absent: t.reduce((n, x) => n + x.absent, 0),
    }
  }, [visibleGroups, quality, layout])

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
    {
      label: grid ? "Da altri, stessa qualità" : "Da altri, sopra la soglia",
      value: stats.available,
      icon: Users,
      tone: "text-emerald-600 dark:text-emerald-400",
    },
    { label: "Solo gruppi bannati", value: stats.banned, icon: Ban, tone: "text-pink-600 dark:text-pink-400" },
    {
      label: grid ? "Solo altra qualità" : "Sotto la soglia",
      value: stats.other,
      icon: grid ? Shuffle : ArrowDown,
      tone: "text-amber-600 dark:text-amber-400",
    },
    { label: "Assenti dal tracker", value: stats.absent, icon: X, tone: "text-destructive" },
  ]

  const saveThreshold = (t: GapThreshold) => {
    if (!editing) return
    if (editing.key === null) savePrefs.mutate({ ...prefs, ...t }, { onSuccess: () => setEditing(null) })
    else mark.mutate({ input: { group: editing.key, action: "threshold", threshold: t } }, { onSuccess: () => setEditing(null) })
  }

  return (
    <div className="grid gap-4">
      <p className="text-muted-foreground text-sm">{k.intro}</p>
      {header}

      <NavbarStats>
        {statCards.map((s) => (
          <StatCard key={s.label} label={s.label} value={s.value} icon={s.icon} tone={s.tone} />
        ))}
      </NavbarStats>

      <div className="flex flex-wrap items-center gap-2">
        <div className="bg-muted inline-flex rounded-lg p-0.5">
          <Button
            variant={grid ? "ghost" : "outline"}
            size="sm"
            className={cn("h-7", !grid && "bg-background")}
            onClick={() => reset(onLayout)("parts")}
          >
            <List /> {k.byPart}
          </Button>
          <Button
            variant={grid ? "outline" : "ghost"}
            size="sm"
            className={cn("h-7", grid && "bg-background")}
            onClick={() => reset(onLayout)("grid")}
          >
            <Grid3x3 /> Per qualità
          </Button>
        </div>
        <div className="relative min-w-[200px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input value={query} onChange={(e) => reset(setQuery)(e.target.value)} placeholder={k.search} className="pl-8" />
        </div>
        {grid ? (
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
        ) : (
          <Button
            variant="outline"
            onClick={() =>
              setEditing({ key: null, title: "Qualità minima", initial: prefs, custom: false })
            }
          >
            <Gauge /> Minima: {thresholdLabel(prefs)}
          </Button>
        )}
        <Button variant="outline" onClick={() => setEditingBanned(true)} title="Gruppi le cui release non bastano">
          <Ban /> Gruppi bannati{bannedGroups.length > 0 && `: ${bannedGroups.length}`}
        </Button>
        <div className="flex items-center gap-2 px-1">
          <Switch
            id={`others-count-${kind}`}
            checked={prefs.othersCount}
            disabled={savePrefs.isPending}
            onCheckedChange={(v) => savePrefs.mutate({ ...prefs, othersCount: v })}
          />
          <Label htmlFor={`others-count-${kind}`} className="whitespace-nowrap" title="Quello che hanno caricato altri non conta come mancante">
            Conta quelle di altri
          </Label>
        </div>
        <div className="flex items-center gap-2 px-1">
          <Switch id={`missing-only-${kind}`} checked={onlyMissing} onCheckedChange={reset(setOnlyMissing)} />
          <Label htmlFor={`missing-only-${kind}`} className="whitespace-nowrap">
            Solo con mancanti
          </Label>
        </div>
      </div>

      {stale && (
        <AlertBox>
          <RefreshCw />
          <AlertTitle>Serve un nuovo controllo</AlertTitle>
          <AlertDescription>
            <span>
              Questi risultati vengono da un controllo fatto prima della vista {k.byPart.toLowerCase()}: fino al prossimo
              controllo sono mostrati per qualità.{" "}
              {onCheck && (
                <Button variant="link" className="h-auto p-0" onClick={onCheck}>
                  Controlla ora
                </Button>
              )}
            </span>
          </AlertDescription>
        </AlertBox>
      )}

      <Legend layout={layout} />

      {visible.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {all.length === 0
              ? empty
                ? empty
                : running
                ? k.checking
                : checkedAt
                  ? k.empty
                  : "Nessun controllo ancora: avvialo con Controlla ora, o attendi quello notturno."
              : k.noMatch}
          </CardContent>
        </Card>
      )}
      {visible.map((s) => (
        <GroupCard
          key={s.key}
          s={s}
          layout={layout}
          group={quality}
          onOpen={setOpened}
          onMark={onMark}
          onThreshold={(g) =>
            setEditing({
              key: g.key,
              title: `Qualità minima: ${g.title}`,
              initial: g.customThreshold ? g.threshold : prefs,
              custom: g.customThreshold,
            })
          }
        />
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

      {hidden.length > 0 && (
        <Card className="gap-3">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <EyeOff className="text-muted-foreground size-4" /> Nascoste ({hidden.length})
              <Button variant="ghost" size="sm" className="ml-auto" onClick={() => setShowHidden(!showHidden)}>
                {showHidden ? "Chiudi" : "Mostra l'elenco"}
              </Button>
            </CardTitle>
            <CardDescription>Non contano nei numeri qui sopra né tra le mancanti.</CardDescription>
          </CardHeader>
          {showHidden && (
            <CardContent>
              <ul className="max-h-72 divide-y overflow-y-auto rounded-lg border">
                {hidden.map((s) => (
                  <li key={s.key} className="flex items-center gap-2 px-3 py-1.5 text-sm">
                    <span className="min-w-0 flex-1 truncate">
                      {s.title}
                      {s.year && <span className="text-muted-foreground"> ({s.year})</span>}
                    </span>
                    {missingIn(s, layout) > 0 && (
                      <span className="text-muted-foreground text-xs">
                        {missingIn(s, layout)} {missingIn(s, layout) === 1 ? "mancante" : "mancanti"}
                      </span>
                    )}
                    <Button variant="outline" size="sm" onClick={() => onMark({ group: s.key, action: "hide" }, true)}>
                      <Eye /> Mostra
                    </Button>
                  </li>
                ))}
              </ul>
            </CardContent>
          )}
        </Card>
      )}

      {unverifiable.length > 0 && (!grid || group === "all") && (
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

      <CellDialog opened={opened} all={all} onClose={() => setOpened(null)} onMark={onMark} onBan={onBan} />
      <BannedDialog
        open={editingBanned}
        onOpenChange={setEditingBanned}
        groups={bannedGroups}
        saving={saveBanned.isPending}
        onSave={(groups) => saveBanned.mutate(groups, { onSuccess: () => setEditingBanned(false) })}
      />
      <ThresholdDialog
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
        title={editing?.title ?? ""}
        description={
          editing?.key === null
            ? `Sotto questa qualità ${kind === "series" ? "una stagione" : "un film"} conta come mancante, anche se l'hai caricato tu. Vale per ${k.noun.toLowerCase()} senza una qualità minima propria.`
            : `Solo per questa ${k.one}, al posto della qualità minima generale (${thresholdLabel(prefs)}).`
        }
        initial={editing?.initial ?? prefs}
        types={types}
        resolutions={resolutions}
        onSave={saveThreshold}
        onReset={
          editing?.key && editing.custom
            ? () => mark.mutate({ input: { group: editing.key!, action: "threshold" }, remove: true }, { onSuccess: () => setEditing(null) })
            : undefined
        }
      />
    </div>
  )
}

/** Missing seasons of the series and films of the sagas and the collections, one tab each, with the shared check. */
export function MissingPage({ view, onView }: { view: MissingView; onView: (v: MissingView) => void }) {
  const series = useMissingSeries()
  const sagas = useMissingSagas()
  const lists = useMissingLists()
  const check = useAction(() => api.checkMissing())

  const current = view === "series" ? series : view === "sagas" ? sagas : lists
  const data = current.data
  // The same check fills both tabs.
  const state = data?.state
  const running = !!state?.running || check.isPending
  const seriesAll = useMemo(() => series.data?.series ?? [], [series.data])
  const sagasAll = useMemo(() => sagas.data?.sagas ?? [], [sagas.data])
  const listsAll = useMemo(() => lists.data?.lists ?? [], [lists.data])
  const [layouts, setLayout] = useLayouts()
  // Those missing only what banned groups uploaded are left out: less priority.
  const missingOf = (all: GapGroup[], kind: GapKind) =>
    all.filter((g) => g.status === "ok" && !g.hidden && urgentIn(g, layouts[kind]) > 0).length

  const run = () =>
    check.mutate(undefined, {
      onSuccess: () =>
        toast.success("Controllo avviato", {
          description: "Stagioni, saghe e collezioni da TMDB e MDBList e torrent dal tracker vengono letti in background.",
        }),
    })

  if (series.isLoading || sagas.isLoading || lists.isLoading) return <Skeleton className="h-48 w-full" />
  const failed = series.error ?? sagas.error ?? lists.error
  if (failed)
    return (
      <Card>
        <CardContent className="text-destructive py-6 text-center text-sm">Errore nel caricamento: {failed.message}</CardContent>
      </Card>
    )

  if (data && !data.enabled && seriesAll.length === 0 && sagasAll.length === 0 && listsAll.length === 0)
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CloudOff className="size-4" /> Controllo delle stagioni e dei film mancanti non attivo
          </CardTitle>
          <CardDescription>
            {data.disabledReason} Da allora ogni notte verranno controllate le serie di cui hai caricato almeno una
            stagione, le saghe di cui hai caricato almeno un film e le collezioni che segui.
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
            {missingOf(seriesAll, "series") > 0 && (
              <Badge variant="warning" className="ml-1 px-1.5">
                {missingOf(seriesAll, "series")}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="sagas">
            <Clapperboard /> Saghe
            {missingOf(sagasAll, "saga") > 0 && (
              <Badge variant="warning" className="ml-1 px-1.5">
                {missingOf(sagasAll, "saga")}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="lists">
            <Library /> Collezioni
            {missingOf(listsAll, "list") > 0 && (
              <Badge variant="warning" className="ml-1 px-1.5">
                {missingOf(listsAll, "list")}
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
        <GapList
          kind="series"
          all={seriesAll}
          running={running}
          checkedAt={series.data?.checkedAt}
          prefs={series.data?.prefs ?? noPrefs}
          bannedGroups={series.data?.bannedGroups ?? noGroups}
          types={series.data?.types ?? []}
          resolutions={series.data?.resolutions ?? []}
          layout={layouts.series}
          onLayout={(l) => setLayout("series", l)}
          onCheck={data?.enabled && !running ? run : undefined}
        />
      </TabsContent>
      <TabsContent value="sagas">
        <GapList
          kind="saga"
          all={sagasAll}
          running={running}
          checkedAt={sagas.data?.checkedAt}
          prefs={sagas.data?.prefs ?? noPrefs}
          bannedGroups={sagas.data?.bannedGroups ?? noGroups}
          types={sagas.data?.types ?? []}
          resolutions={sagas.data?.resolutions ?? []}
          layout={layouts.saga}
          onLayout={(l) => setLayout("saga", l)}
          onCheck={data?.enabled && !running ? run : undefined}
        />
      </TabsContent>
      <TabsContent value="lists">
        <GapList
          kind="list"
          all={listsAll}
          running={running}
          checkedAt={lists.data?.checkedAt}
          prefs={lists.data?.prefs ?? noPrefs}
          bannedGroups={lists.data?.bannedGroups ?? noGroups}
          types={lists.data?.types ?? []}
          resolutions={lists.data?.resolutions ?? []}
          layout={layouts.list}
          onLayout={(l) => setLayout("list", l)}
          onCheck={data?.enabled && !running ? run : undefined}
          header={
            <ListSources
              sources={lists.data?.sources ?? []}
              groups={listsAll}
              enabled={!!lists.data?.listsEnabled}
              disabledReason={lists.data?.listsDisabledReason ?? ""}
            />
          }
          empty={lists.data?.sources.length === 0 ? kinds.list.empty : undefined}
        />
      </TabsContent>
    </Tabs>
  )
}
