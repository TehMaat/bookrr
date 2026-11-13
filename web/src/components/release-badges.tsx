import { CircleArrowUp, CircleCheck, CircleHelp, CloudOff, SearchX } from "lucide-react"

import { StatusBadge } from "@/components/torrent-badges"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { BestResolution, ReleaseStatus } from "@/lib/api"
import { formatDate, formatRelative } from "@/lib/format"

export type BestKind = "better" | "max" | "none"

/** How a film stands against the best release found; null when it was not searched (or is not a film). */
export function bestKind(b: BestResolution | null | undefined): BestKind | null {
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
export function BestBadge({ b }: { b: BestResolution | null | undefined }) {
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

/** The status of a torrent, or of an upload bookrr has no torrent for. */
export function ReleaseStatusBadge({ status }: { status: ReleaseStatus }) {
  if (status !== "untracked") return <StatusBadge status={status} />
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className="border-dashed">
          <CircleHelp /> Not in bookrr
        </Badge>
      </TooltipTrigger>
      <TooltipContent>On the tracker, but bookrr knows no torrent with this hash</TooltipContent>
    </Tooltip>
  )
}

/** An upload no longer on the tracker; without since, only the badge (for the filters). */
export function RemovedBadge({ since }: { since?: string }) {
  const badge = (
    <Badge variant="destructive">
      <CloudOff /> Removed
    </Badge>
  )
  if (!since) return badge
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent>No longer among your uploads since {formatDate(since)}</TooltipContent>
    </Tooltip>
  )
}
