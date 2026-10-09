import { CircleHelp, Copy, HandHeart, HardDrive, Server, Star, TriangleAlert } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Torrent, TorrentStatus } from "@/lib/api"
import { formatState, isErrorState } from "@/lib/format"

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
  const empty = t.locations.length === 0 && t.archives.length === 0 && !t.adoptedBy
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
          </TooltipContent>
        </Tooltip>
      ))}
      {t.adoptedBy && (
        <Badge variant="success">
          <HandHeart /> {t.adoptedBy}
        </Badge>
      )}
      {empty && <span className="text-muted-foreground text-xs">—</span>}
    </div>
  )
}
