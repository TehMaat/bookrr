import { useState } from "react"
import {
  ChevronRight,
  CircleCheck,
  Clapperboard,
  Download,
  FileDown,
  FileUp,
  GitMerge,
  ListTree,
  Loader2,
  MoreHorizontal,
  Pencil,
  RotateCcw,
  Send,
  Tag,
  Trash2,
  TriangleAlert,
  Upload,
} from "lucide-react"
import { toast } from "sonner"

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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, type PlanItem, type PlanMerge, type PlanSendInput, type PlanSendResult, type PlanTorrent } from "@/lib/api"
import { formatBytes, formatRelative, formatState, isErrorState } from "@/lib/format"
import { useAction, useClients, useRefreshAll, useSettings } from "@/lib/queries"
import { cn } from "@/lib/utils"

/** How many .torrent files a film takes: two for a release merged from two sources. */
export const MAX_PLAN_TORRENTS = 2

/** Complete on a client. */
export const torrentComplete = (t: PlanTorrent) => t.downloads.some((d) => d.progress >= 1)

/** What the user asked to do with the .torrent files of a film. */
export type TorrentAction =
  | { kind: "load"; item: PlanItem; position?: number }
  | { kind: "send"; item: PlanItem; positions?: number[] }
  | { kind: "label"; item: PlanItem; torrent: PlanTorrent }

/** Toasts how sending went. */
function toastSent(r: PlanSendResult) {
  const failed = r.results.filter((x) => !x.ok)
  const lines = r.results.map(
    (x) => `${x.name}: ${x.ok ? (x.already ? "already there" : "added") : `failed (${x.error})`}`,
  )
  const m = r.merge
  if (m?.planId && (m.status === "WAITING" || m.status === "PAIRED" || m.status === "ERROR"))
    lines.push(`pair planned on the Wayfinderr of ${m.clientName}`)
  else if (m?.error) {
    lines.push(`pair not planned on Wayfinderr: ${m.error}`)
    toast.warning(`Sent to ${r.client}`, { description: lines.join(" · ") })
    return
  }
  if (failed.length > 0) toast.warning(`Sent to ${r.client}, with errors`, { description: lines.join(" · ") })
  else toast.success(`Sent to ${r.client}`, { description: lines.join(" · ") })
}

/**
 * The .torrent files of a film of the plan and how their download goes, with the button to load one or to send them
 * to a client.
 */
export function PlanTorrents({
  it,
  onAction,
  onOpenTorrent,
}: {
  it: PlanItem
  onAction: (a: TorrentAction) => void
  onOpenTorrent: (hash: string) => void
}) {
  const settings = useSettings()
  const clients = useClients()
  const refresh = useRefreshAll()
  const defaultClient = (clients.data ?? []).find((c) => c.id === settings.data?.planClientId && c.enabled)
  const [sending, setSending] = useState(false)
  const remove = useAction(
    ({ position }: { position: number }) => api.deletePlanTorrent(it.id, position),
    ".torrent removed from the film",
  )
  const toWayfinderr = useAction(
    ({ position }: { position: number }) => api.sendPlanTorrentToWayfinderr(it.id, position),
    ".torrent sent to Wayfinderr",
  )
  const planMerge = useAction(() => api.planMerge(it.id), "Pair planned on Wayfinderr")
  const removeMerge = useAction(() => api.removeMerge(it.id), "Pair removed from Wayfinderr")
  const pairAgain = useAction(() => api.pairMergeAgain(it.id), "Pair created again on Wayfinderr")
  // Clients that hand their downloads over to a Wayfinderr.
  const withWayfinderr = (clients.data ?? []).some((c) => c.enabled && c.wayfinderrUrl)

  /** Sends to the default client without asking, else asks which one. */
  const send = async (positions?: number[]) => {
    if (!defaultClient) {
      onAction({ kind: "send", item: it, positions })
      return
    }
    setSending(true)
    try {
      toastSent(await api.sendPlanTorrents(it.id, { positions }))
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setSending(false)
      refresh()
    }
  }

  if (it.torrents.length === 0)
    return (
      <Button variant="outline" size="sm" onClick={() => onAction({ kind: "load", item: it })}>
        <Upload /> Load .torrent
      </Button>
    )

  // The torrents on no client yet: "Download" sends them.
  const unsent = it.torrents.filter((t) => t.downloads.length === 0 && (!t.sentAt || t.gone))
  const merge = it.torrents.length > 1
  const done = it.torrents.filter(torrentComplete).length

  return (
    <div className="grid max-w-xs min-w-[200px] gap-2">
      {merge && (
        <div className="grid gap-0.5">
          <div className="text-muted-foreground flex items-center gap-1 text-xs">
            <GitMerge className="size-3.5" />
            <span className="flex-1">
              Merge
              {done > 0 && done < it.torrents.length && ` · ${done} of ${it.torrents.length} downloaded`}
            </span>
            {withWayfinderr && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon-sm" className="size-6 shrink-0" aria-label="Merge actions">
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem disabled={planMerge.isPending} onClick={() => planMerge.mutate(undefined)}>
                    <Clapperboard /> {mergeOpen(it.merge) ? "Plan the pair again" : "Plan the pair on Wayfinderr"}
                  </DropdownMenuItem>
                  {it.merge?.status === "ERROR" && (
                    <DropdownMenuItem disabled={pairAgain.isPending} onClick={() => pairAgain.mutate(undefined)}>
                      <RotateCcw /> Create the pair again
                    </DropdownMenuItem>
                  )}
                  {it.merge && (mergeOpen(it.merge) || (!it.merge.planId && it.merge.status === "")) && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        disabled={removeMerge.isPending}
                        onClick={() => removeMerge.mutate(undefined)}
                      >
                        <Trash2 /> Remove the pair from Wayfinderr
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
          <MergeState m={it.merge} />
        </div>
      )}
      {it.torrents.map((t) => (
        <div key={t.position} className="grid gap-1">
          <div className="flex min-w-0 items-center gap-1">
            {t.label && (
              <Badge variant="secondary" className="max-w-24 truncate">
                {t.label}
              </Badge>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="min-w-0 flex-1 truncate text-xs font-medium">{t.name}</span>
              </TooltipTrigger>
              <TooltipContent className="max-w-sm break-all">
                {t.name} · {formatBytes(t.size)}
                <br />
                <span className="font-mono">{t.hash}</span>
              </TooltipContent>
            </Tooltip>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-sm" className="size-6 shrink-0" aria-label="Torrent actions">
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => onAction({ kind: "send", item: it, positions: [t.position] })}>
                  <Send /> Send to a client…
                </DropdownMenuItem>
                {t.downloads.length > 0 && (
                  <DropdownMenuItem onClick={() => onOpenTorrent(t.hash)}>
                    <ListTree /> Details in Torrents
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onClick={() => onAction({ kind: "label", item: it, torrent: t })}>
                  <Tag /> Label
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onAction({ kind: "load", item: it, position: t.position })}>
                  <Pencil /> Replace .torrent
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <a href={api.planTorrentFileUrl(it.id, t.position)} download>
                    <FileDown /> Save the .torrent file{t.dropped ? " (from Wayfinderr)" : ""}
                  </a>
                </DropdownMenuItem>
                {withWayfinderr && (
                  <DropdownMenuItem onClick={() => toWayfinderr.mutate({ position: t.position })}>
                    <Clapperboard /> {t.wayfinderrAt ? "Send to Wayfinderr again" : "Send to Wayfinderr"}
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onClick={() => remove.mutate({ position: t.position })}>
                  <Trash2 /> Remove from the film
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <TorrentProgress t={t} />
          <WayfinderrState t={t} />
        </div>
      ))}
      <div className="flex flex-wrap items-center gap-1">
        {unsent.length > 0 && (
          <Button
            size="sm"
            disabled={sending}
            onClick={() => send(unsent.length === it.torrents.length ? undefined : unsent.map((t) => t.position))}
          >
            {sending ? <Loader2 className="animate-spin" /> : <Download />}
            Download{defaultClient ? ` on ${defaultClient.name}` : ""}
          </Button>
        )}
        {it.torrents.length < MAX_PLAN_TORRENTS && (
          <Button variant="ghost" size="sm" onClick={() => onAction({ kind: "load", item: it })}>
            <FileUp /> Second .torrent (merge)
          </Button>
        )}
      </div>
    </div>
  )
}

/** The pair is planned on Wayfinderr and still open there (waiting for its files, or in error). */
const mergeOpen = (m: PlanMerge | null) => !!m?.planId && (m.status === "WAITING" || m.status === "ERROR")

/** The pair of the two torrents of a merge on Wayfinderr: planned, paired, in error, removed, or why it is not planned. */
function MergeState({ m }: { m: PlanMerge | null }) {
  if (!m) return null
  const via = m.clientName ? `the Wayfinderr of ${m.clientName}` : "Wayfinderr"
  const state = (() => {
    switch (m.status) {
      case "WAITING":
        return {
          text: "Pair planned on Wayfinderr",
          tone: "muted",
          tip: `Planned on ${via}${m.plannedAt ? ` ${formatRelative(m.plannedAt)}` : ""}: it holds each file as it arrives, and pairs and analyzes the two once both are there.`,
        }
      case "PAIRED":
        return {
          text: "Paired on Wayfinderr",
          tone: "ok",
          tip: `Both files arrived: ${via} created the pair and analyzes it. The merge waits for your confirmation there.`,
        }
      case "ERROR":
        return {
          text: "Pair in error on Wayfinderr",
          tone: "error",
          tip: `Both files arrived, but ${via} could not create the pair${m.message ? `: ${m.message}` : ""}. Create it again from the menu once fixed.`,
        }
      case "GONE":
        return {
          text: "Pair no longer on Wayfinderr",
          tone: "muted",
          tip: `${via} no longer has the planned pair (removed there). Plan it again from the menu if needed.`,
        }
      case "REMOVED":
        return {
          text: "Pair removed from Wayfinderr",
          tone: "muted",
          tip: "Not planned again by itself: plan it again from the menu, or load other .torrent files.",
        }
      default:
        return m.error
          ? { text: "Pair not planned on Wayfinderr", tone: "error", tip: `${m.error}. Tried again at the next sync.` }
          : null
    }
  })()
  if (!state) return null
  const tip = m.error && m.status !== "" ? `${state.tip} Last check failed: ${m.error}.` : state.tip
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center gap-1 text-xs",
            state.tone === "muted" && "text-muted-foreground",
            state.tone === "ok" && "text-emerald-700 dark:text-emerald-300",
            state.tone === "error" && "text-destructive",
          )}
        >
          {state.tone === "error" ? (
            <TriangleAlert className="size-3" />
          ) : state.tone === "ok" ? (
            <CircleCheck className="size-3" />
          ) : (
            <Clapperboard className="size-3" />
          )}
          {state.text}
          {m.error && m.status !== "" && <TriangleAlert className="text-destructive size-3" />}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs break-words">{tip}</TooltipContent>
    </Tooltip>
  )
}

/** The .torrent on Wayfinderr: sent as the source .torrent of the film, dropped here once released, or why sending failed. */
function WayfinderrState({ t }: { t: PlanTorrent }) {
  if (t.wayfinderrAt)
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
            <Clapperboard className="size-3" />
            {t.dropped ? "On Wayfinderr only" : "On Wayfinderr"}
          </span>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">
          Sent to the Wayfinderr of {t.wayfinderrClientName || "a removed client"} {formatRelative(t.wayfinderrAt)}: it
          goes into the upload once the rip is done.{" "}
          {t.dropped
            ? "The film is released, so bookrr dropped its copy: saving or sending it takes it from Wayfinderr."
            : "bookrr drops its copy once the film is released."}
        </TooltipContent>
      </Tooltip>
    )
  if (t.wayfinderrError)
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="text-destructive inline-flex items-center gap-1 text-xs">
            <TriangleAlert className="size-3" /> Not sent to Wayfinderr yet
          </span>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs break-words">{t.wayfinderrError}. Tried again at the next sync.</TooltipContent>
      </Tooltip>
    )
  return null
}

/** Where a .torrent of the plan is: progress on each client, sent and waiting, gone, or never sent. */
function TorrentProgress({ t }: { t: PlanTorrent }) {
  if (t.downloads.length > 0)
    return (
      <div className="grid gap-1">
        {t.downloads.map((d) => {
          const complete = d.progress >= 1
          const failed = isErrorState(d.state)
          const pct = Math.floor(d.progress * 1000) / 10
          return (
            <Tooltip key={d.clientId}>
              <TooltipTrigger asChild>
                <div className="grid gap-0.5">
                  <div className="flex items-center justify-between gap-2 text-xs">
                    <span
                      className={cn(
                        "inline-flex items-center gap-1",
                        complete && "text-emerald-700 dark:text-emerald-300",
                        failed && "text-destructive",
                      )}
                    >
                      {complete ? (
                        <CircleCheck className="size-3" />
                      ) : failed ? (
                        <TriangleAlert className="size-3" />
                      ) : null}
                      {complete ? "Downloaded" : formatState(d.state)}
                    </span>
                    <span className="text-muted-foreground tabular-nums">
                      {d.clientName} · {pct}%
                    </span>
                  </div>
                  <div className="bg-muted h-1.5 overflow-hidden rounded-full">
                    <div
                      className={cn(
                        "h-full rounded-full transition-[width]",
                        complete ? "bg-emerald-500" : failed ? "bg-destructive" : "bg-sky-500",
                      )}
                      style={{ width: `${Math.min(100, d.progress * 100)}%` }}
                    />
                  </div>
                </div>
              </TooltipTrigger>
              <TooltipContent>
                {formatState(d.state)} on {d.clientName}, read {formatRelative(d.seenAt)}
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
    )
  if (t.gone)
    return (
      <span className="text-destructive inline-flex items-center gap-1 text-xs">
        <TriangleAlert className="size-3" /> No longer on {t.sentClientName || "the client"}
      </span>
    )
  if (t.sentAt)
    return (
      <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
        <Loader2 className="size-3 animate-spin" /> Sent to {t.sentClientName || "the client"}, waiting for it to be read
      </span>
    )
  return <span className="text-muted-foreground text-xs">Not sent yet · {formatBytes(t.size)}</span>
}

/** Loads a .torrent file on a film, or replaces one, with an optional label. */
export function LoadTorrentDialog({
  action,
  onClose,
}: {
  action: Extract<TorrentAction, { kind: "load" }> | null
  onClose: () => void
}) {
  const [file, setFile] = useState<File | null>(null)
  const [label, setLabel] = useState("")
  const [prev, setPrev] = useState(action)
  if (action !== prev) {
    setPrev(action)
    setFile(null)
    setLabel(action?.position ? (action.item.torrents.find((t) => t.position === action.position)?.label ?? "") : "")
  }
  const load = useAction(
    (a: { id: number; file: File; label: string; position?: number }) =>
      api.uploadPlanTorrent(a.id, a.file, a.label, a.position),
    action?.position ? ".torrent replaced" : ".torrent loaded",
  )
  const it = action?.item
  const second = !!it && !action?.position && it.torrents.length > 0
  return (
    <Dialog open={action !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {action?.position ? "Replace the .torrent" : second ? "Second .torrent (merge)" : "Load a .torrent"} ·{" "}
            {it?.title}
          </DialogTitle>
          <DialogDescription>
            {second
              ? "For a release merged from two sources: the film is downloaded when both are complete."
              : "bookrr keeps it: you can send it to a client to download it, also more than once."}
          </DialogDescription>
        </DialogHeader>
        <form
          id="load-torrent"
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (it && file) load.mutate({ id: it.id, file, label, position: action?.position }, { onSuccess: onClose })
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor="torrent-file">.torrent file</Label>
            <Input
              id="torrent-file"
              type="file"
              accept=".torrent,application/x-bittorrent"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              autoFocus
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="torrent-label">Label</Label>
            <Input
              id="torrent-label"
              value={label}
              maxLength={100}
              onChange={(e) => setLabel(e.target.value)}
              placeholder={second ? "Optional: e.g. audio ITA" : "Optional: e.g. video"}
            />
            <p className="text-muted-foreground text-xs">What to take from it, to remember it when merging.</p>
          </div>
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="load-torrent" disabled={!file || load.isPending}>
            {load.isPending ? <Loader2 className="animate-spin" /> : <Upload />}{" "}
            {action?.position ? "Replace" : "Load"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Chooses the client to send the .torrent files of a film to, with category, tags and folder under Advanced. */
export function SendTorrentsDialog({
  action,
  onClose,
}: {
  action: Extract<TorrentAction, { kind: "send" }> | null
  onClose: () => void
}) {
  const clients = useClients()
  const settings = useSettings()
  const enabled = (clients.data ?? []).filter((c) => c.enabled)
  const [clientId, setClientId] = useState("")
  const [advanced, setAdvanced] = useState(false)
  const [opts, setOpts] = useState({ category: "", tags: "", savePath: "" })
  const [prev, setPrev] = useState(action)
  if (action !== prev) {
    setPrev(action)
    const def = enabled.find((c) => c.id === settings.data?.planClientId)
    setClientId(def ? String(def.id) : enabled.length === 1 ? String(enabled[0].id) : "")
    setAdvanced(false)
    setOpts({ category: "", tags: "", savePath: "" })
  }
  const refresh = useRefreshAll()
  const [pending, setPending] = useState(false)
  const it = action?.item
  const torrents = it?.torrents.filter((t) => !action?.positions || action.positions.includes(t.position)) ?? []

  const submit = async () => {
    if (!it || !clientId) return
    const p: PlanSendInput = { clientId: Number(clientId), positions: action?.positions, ...opts }
    setPending(true)
    try {
      toastSent(await api.sendPlanTorrents(it.id, p))
      onClose()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setPending(false)
      refresh()
    }
  }

  return (
    <Dialog open={action !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Download · {it?.title}</DialogTitle>
          <DialogDescription>
            {torrents.length > 1
              ? `The ${torrents.length} .torrent files go to the client you choose.`
              : `${torrents[0]?.name ?? "The .torrent"} goes to the client you choose.`}{" "}
            Its progress shows here in the plan.
          </DialogDescription>
        </DialogHeader>
        <form
          id="send-torrents"
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <div className="grid gap-1.5">
            <Label>Client</Label>
            {enabled.length === 0 ? (
              <p className="text-muted-foreground text-sm">No client enabled: add one in Clients.</p>
            ) : (
              <Select value={clientId} onValueChange={setClientId}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Choose the client" />
                </SelectTrigger>
                <SelectContent>
                  {enabled.map((c) => (
                    <SelectItem key={c.id} value={String(c.id)}>
                      {c.name}
                      {c.id === settings.data?.planClientId && " (default)"}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
          <div className="grid gap-3">
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm"
              aria-expanded={advanced}
              onClick={() => setAdvanced((a) => !a)}
            >
              <ChevronRight className={cn("size-4 transition-transform", advanced && "rotate-90")} /> Advanced
            </button>
            {advanced && (
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid content-start gap-1.5">
                  <Label htmlFor="send-category">Category</Label>
                  <Input
                    id="send-category"
                    value={opts.category}
                    onChange={(e) => setOpts((o) => ({ ...o, category: e.target.value }))}
                  />
                </div>
                <div className="grid content-start gap-1.5">
                  <Label htmlFor="send-tags">Tags</Label>
                  <Input
                    id="send-tags"
                    value={opts.tags}
                    onChange={(e) => setOpts((o) => ({ ...o, tags: e.target.value }))}
                    placeholder="tag1, tag2"
                  />
                </div>
                <div className="grid content-start gap-1.5 sm:col-span-2">
                  <Label htmlFor="send-path">Save path</Label>
                  <Input
                    id="send-path"
                    value={opts.savePath}
                    onChange={(e) => setOpts((o) => ({ ...o, savePath: e.target.value }))}
                    placeholder="/downloads/merge"
                  />
                </div>
                <p className="text-muted-foreground text-xs sm:col-span-2">
                  Empty ones leave the client's defaults. The category must exist on the client.
                </p>
              </div>
            )}
          </div>
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="send-torrents" disabled={!clientId || pending}>
            {pending ? <Loader2 className="animate-spin" /> : <Download />} Download
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Changes the label of a .torrent of the plan. */
export function TorrentLabelDialog({
  action,
  onClose,
}: {
  action: Extract<TorrentAction, { kind: "label" }> | null
  onClose: () => void
}) {
  const [label, setLabel] = useState("")
  const [prev, setPrev] = useState(action)
  if (action !== prev) {
    setPrev(action)
    setLabel(action?.torrent.label ?? "")
  }
  const save = useAction(
    (a: { id: number; position: number; label: string }) => api.updatePlanTorrent(a.id, a.position, a.label),
    "Label saved",
  )
  return (
    <Dialog open={action !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Label</DialogTitle>
          <DialogDescription className="break-all">{action?.torrent.name}</DialogDescription>
        </DialogHeader>
        <form
          id="torrent-label-form"
          onSubmit={(e) => {
            e.preventDefault()
            if (action)
              save.mutate({ id: action.item.id, position: action.torrent.position, label }, { onSuccess: onClose })
          }}
        >
          <Input
            value={label}
            maxLength={100}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="e.g. video, audio ITA"
            autoFocus
          />
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="torrent-label-form" disabled={save.isPending}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
