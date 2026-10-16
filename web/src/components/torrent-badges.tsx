import {
  Archive,
  ArrowDown,
  ArrowUp,
  CheckCheck,
  CircleHelp,
  Copy,
  HandHeart,
  HardDrive,
  Server,
  Star,
  TriangleAlert,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Archivability, Peers, Torrent, TorrentStatus } from "@/lib/api"
import { formatDate, formatState, isErrorState } from "@/lib/format"
import { cn } from "@/lib/utils"

export const statusLabels: Record<TorrentStatus, string> = {
  client: "Su client",
  missing: "Da localizzare",
  archived: "Archiviato",
  adopted: "Adottato",
  unknown: "Posizione ignota",
}

export function StatusBadge({ status }: { status: TorrentStatus }) {
  switch (status) {
    case "client":
      return (
        <Badge variant="info">
          <Server /> {statusLabels[status]}
        </Badge>
      )
    case "missing":
      return (
        <Badge variant="warning">
          <TriangleAlert /> {statusLabels[status]}
        </Badge>
      )
    case "archived":
      return (
        <Badge variant="violet">
          <HardDrive /> {statusLabels[status]}
        </Badge>
      )
    case "adopted":
      return (
        <Badge variant="success">
          <HandHeart /> {statusLabels[status]}
        </Badge>
      )
    default:
      return (
        <Badge variant="outline" className="text-muted-foreground">
          <CircleHelp /> {statusLabels[status]}
        </Badge>
      )
  }
}

export function PersonalBadge() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className="border-amber-500/40 text-amber-700 dark:text-amber-300">
          <Star className="fill-current" /> PR
        </Badge>
      </TooltipTrigger>
      <TooltipContent>Personal Release</TooltipContent>
    </Tooltip>
  )
}

export function DuplicateBadge({ count }: { count: number }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="destructive">
          <Copy /> ×{count}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>Lo stesso torrent è su {count} client</TooltipContent>
    </Tooltip>
  )
}

/** Compact "where is it" summary: clients, archives and adopter. */
export function WhereBadges({ t }: { t: Torrent }) {
  const empty = t.locations.length === 0 && t.archives.length === 0 && t.adoptions.length === 0
  return (
    <div className="flex flex-wrap gap-1">
      {t.locations.map((l) => (
        <Tooltip key={`c${l.clientId}`}>
          <TooltipTrigger asChild>
            <Badge variant={isErrorState(l.state) ? "destructive" : "info"}>
              <Server /> {l.clientName}
            </Badge>
          </TooltipTrigger>
          <TooltipContent className="max-w-sm font-mono break-all">
            {formatState(l.state)} · ratio {l.ratio.toFixed(2)}
            <br />
            {l.savePath}
          </TooltipContent>
        </Tooltip>
      ))}
      {t.archives.map((a) => (
        <Tooltip key={`a${a.id}`}>
          <TooltipTrigger asChild>
            <Badge variant="violet">
              <HardDrive /> {a.diskLabel || "Archivio"}
            </Badge>
          </TooltipTrigger>
          <TooltipContent className="max-w-sm break-all">
            {a.diskSerial && (
              <>
                SN <span className="font-mono">{a.diskSerial}</span>
                <br />
              </>
            )}
            <span className="font-mono">{a.path || "—"}</span>
            <br />
            Spostato il {formatDate(a.createdAt)}
          </TooltipContent>
        </Tooltip>
      ))}
      {t.adoptions.map((a) => (
        <Badge key={`d${a.id}`} variant="success">
          <HandHeart /> {a.adopterName}
        </Badge>
      ))}
      {empty && <span className="text-muted-foreground text-xs">—</span>}
    </div>
  )
}

/** Color of the archivable uploads, in badges and in the filters. */
export const archivableTone = "text-teal-600 dark:text-teal-400"

/** "da oggi", "da 1 giorno", "da 12 giorni". */
export function sinceDays(days: number) {
  return days === 0 ? "da oggi" : days === 1 ? "da 1 giorno" : `da ${days} giorni`
}

const daysRule = (d: number) => (d === 0 ? "subito" : d === 1 ? "da almeno 1 giorno" : `da almeno ${d} giorni di fila`)

/** The numbers behind the assessment: seeders on the tracker, mine, others, completed and the rule. */
export function ArchivableNumbers({ a }: { a: Archivability }) {
  const rows: [string, React.ReactNode][] = [
    ["Seeder sul tracker", a.seeders],
    ["Tuoi client in seed", a.mine],
    ["Altri seeder", <span className={cn(a.others >= a.rule.minOthers && "font-semibold")}>{a.others}</span>],
    ["Completati", <span className={cn(a.timesCompleted >= a.rule.minCompleted && "font-semibold")}>{a.timesCompleted}</span>],
    ["Dati del tracker", formatDate(a.trackerAt)],
  ]
  if (a.since) rows.push(["Condizioni rispettate dal", formatDate(a.since)])
  return (
    <div className="grid gap-1">
      <dl className="grid grid-cols-[auto_auto] gap-x-3 gap-y-0.5">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="opacity-80">{k}</dt>
            <dd className="text-right tabular-nums">{v}</dd>
          </div>
        ))}
      </dl>
      <p className="opacity-80">
        Regola: altri seeder ≥ {a.rule.minOthers}, completati ≥ {a.rule.minCompleted}, {daysRule(a.rule.days)}.
      </p>
    </div>
  )
}

/** Shown only for archivable uploads: "Sì · da 3 giorni", with the numbers in the tooltip. */
export function ArchivableBadge({ a, label = "Sì" }: { a: Archivability | null; label?: string }) {
  if (!a?.archivable) return null
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className="border-teal-500/40 bg-teal-500/10 text-teal-700 dark:text-teal-300">
          <Archive /> {label} · {sinceDays(a.days)}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        <p className="mb-1 font-medium">Altri la seminano: puoi toglierla dai client e archiviarla.</p>
        <ArchivableNumbers a={a} />
      </TooltipContent>
    </Tooltip>
  )
}

/** One line on where an upload stands: archivable, waiting for the days, or why not. */
export function archivableSummary(a: Archivability): string {
  if (a.archivable) return `Archiviabile ${sinceDays(a.days)}: altri la seminano, puoi toglierla dai client.`
  if (a.met) return `Condizioni rispettate dal ${formatDate(a.since)}: archiviabile dal ${formatDate(a.readyAt)} se restano tali.`
  const missing = [
    a.others < a.rule.minOthers && `altri seeder ${a.others} su ${a.rule.minOthers}`,
    a.timesCompleted < a.rule.minCompleted && `completati ${a.timesCompleted} su ${a.rule.minCompleted}`,
  ].filter(Boolean)
  return `Non ancora archiviabile: ${missing.join(", ")}.`
}

/** Seeders, leechers and completed downloads, as on the tracker. */
export function PeerCounts({ p }: { p: Pick<Peers, "seeders" | "leechers" | "timesCompleted"> }) {
  return (
    <div className="flex items-center gap-2 text-xs tabular-nums">
      <span className={cn("flex items-center gap-0.5", p.seeders === 0 ? "text-destructive" : "text-emerald-600 dark:text-emerald-400")}>
        <ArrowUp className="size-3" />
        {p.seeders}
      </span>
      <span className="text-muted-foreground flex items-center gap-0.5">
        <ArrowDown className="size-3" />
        {p.leechers}
      </span>
      <span className="text-muted-foreground flex items-center gap-0.5">
        <CheckCheck className="size-3" />
        {p.timesCompleted}
      </span>
    </div>
  )
}
