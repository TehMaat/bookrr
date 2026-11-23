import { useEffect, useMemo, useRef, useState } from "react"
import { Check, CircleAlert, Clapperboard, FolderOpen, ListChecks, Search, Server, TriangleAlert, Tv, Upload, X } from "lucide-react"
import { toast } from "sonner"

import { PersonalBadge } from "@/components/torrent-badges"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import {
  api,
  type ArrMovie,
  type Disk,
  type ListingApplyItem,
  type ListingCheck,
  type ListingItem,
  type ListingPreview,
  type ListingResult,
} from "@/lib/api"
import { formatBytes, plural } from "@/lib/format"
import { useRefreshAll } from "@/lib/queries"
import { cn } from "@/lib/utils"

const checks: Record<ListingCheck, { label: string; variant: "success" | "destructive" | "warning"; hint: string }> = {
  complete: { label: "Complete", variant: "success", hint: "Same size as the torrent." },
  incomplete: { label: "Incomplete", variant: "destructive", hint: "Smaller than the torrent: some data is missing. It can't be archived." },
  larger: { label: "Larger", variant: "warning", hint: "Larger than the torrent: other files beside it, or another version?" },
  unknown: { label: "Size unknown", variant: "warning", hint: "No size to compare: the listing has none, or the torrent has none." },
}

/** Something would be done for the torrent: archive it, remove it from a client, delete it from Radarr or Sonarr. */
function hasWork(it: ListingItem) {
  return !it.archived || it.locations.length > 0 || (it.radarr !== null && radarrDeletable(it)) || (it.sonarr?.files.length ?? 0) > 0
}

const radarrDeletable = (it: ListingItem) => !!it.radarr && (!it.radarr.hasFile || it.radarr.sameFile)

/**
 * Reads the listing of a disk pasted by the user, finds the torrents on it
 * and, for those chosen, records them as archived on the disk and removes
 * them from the clients, Radarr and Sonarr.
 */
export function DiskListingDialog({ disk, onClose }: { disk: Disk | null; onClose: () => void }) {
  const refresh = useRefreshAll()
  const fileInput = useRef<HTMLInputElement>(null)
  const [text, setText] = useState("")
  const [prefix, setPrefix] = useState("")
  const [preview, setPreview] = useState<ListingPreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [anchor, setAnchor] = useState<number | null>(null)
  const [clients, setClients] = useState(true)
  const [radarr, setRadarr] = useState(true)
  const [sonarr, setSonarr] = useState(true)
  const [deleteFiles, setDeleteFiles] = useState<boolean | null>(null)
  const [results, setResults] = useState<ListingResult[] | null>(null)
  // Films of Radarr chosen by hand (null: none), by hash.
  const [linked, setLinked] = useState<Map<string, ArrMovie | null>>(new Map())
  const [linking, setLinking] = useState<ListingItem | null>(null)

  useEffect(() => {
    if (!disk) return
    setText("")
    setPrefix("")
    setPreview(null)
    setResults(null)
  }, [disk])

  const analyze = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!disk) return
    setBusy(true)
    try {
      const p = await api.previewListing(disk.id, text, prefix)
      setPreview(p)
      setSelected(new Set(p.items.filter((it) => it.check === "complete" && hasWork(it)).map((it) => it.hash)))
      setAnchor(null)
      setClients(true)
      setRadarr(p.radarr)
      setSonarr(p.sonarr)
      setDeleteFiles(null)
      setLinked(new Map())
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const loadFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = ""
    if (f) setText(await f.text())
  }

  const items = useMemo(
    () => (preview?.items ?? []).map((it) => (linked.has(it.hash) ? { ...it, radarr: linked.get(it.hash) ?? null } : it)),
    [preview, linked]
  )
  const chosen = items.filter((it) => selected.has(it.hash))
  const toggle = (i: number, shift: boolean) => {
    const it = items[i]
    const on = !selected.has(it.hash)
    const next = new Set(selected)
    const [from, to] = shift && anchor !== null ? [Math.min(anchor, i), Math.max(anchor, i)] : [i, i]
    for (let j = from; j <= to; j++) {
      if (items[j].check === "incomplete") continue
      if (on) next.add(items[j].hash)
      else next.delete(items[j].hash)
    }
    setSelected(next)
    setAnchor(i)
  }

  // What will be done.
  const toArchive = chosen.filter((it) => !it.archived).length
  const onClients = clients ? chosen.filter((it) => it.locations.length > 0) : []
  const films = radarr ? chosen.filter(radarrDeletable) : []
  const series = useMemo(() => {
    const out = new Map<number, { title: string; files: Set<number>; seriesFiles: number }>()
    if (!sonarr) return out
    for (const it of chosen) {
      const e = it.sonarr
      if (!e) continue
      const s = out.get(e.seriesId) ?? { title: e.title, files: new Set<number>(), seriesFiles: e.seriesFiles }
      e.files.forEach((f) => s.files.add(f))
      out.set(e.seriesId, s)
    }
    return out
  }, [chosen, sonarr])
  const wholeSeries = [...series.values()].filter((s) => s.files.size > 0 && s.files.size >= s.seriesFiles)
  const needChoice = onClients.length > 0
  const ready = chosen.length > 0 && (!needChoice || deleteFiles !== null)

  const apply = async () => {
    if (!disk || !preview) return
    setBusy(true)
    try {
      const req: ListingApplyItem[] = chosen.map((it) => {
        const hand = linked.get(it.hash)
        return { hash: it.hash, clients, radarr: radarr && (linked.has(it.hash) ? hand !== null : true), sonarr, radarrId: hand?.id }
      })
      const r = await api.applyListing(disk.id, text, prefix, req, needChoice ? deleteFiles : null)
      setResults(r.results)
      const failed = r.results.filter((x) => x.steps.some((s) => !s.ok)).length
      if (failed) toast.warning(`Done, with problems on ${plural(failed, "torrent")}`)
      else toast.success(`${plural(r.results.length, "torrent")} archived on ${disk.label}`)
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
      refresh()
    }
  }

  return (
    <Dialog open={disk !== null} onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
        {results ? (
          <Results results={results} onClose={onClose} />
        ) : !preview ? (
          <form onSubmit={analyze} className="grid min-w-0 gap-4">
            <DialogHeader>
              <DialogTitle>Read the listing of "{disk?.label}"</DialogTitle>
              <DialogDescription asChild>
                <div className="grid gap-1.5">
                  <p>
                    On the computer the disk is plugged into, list its files and paste the output here: bookrr finds the
                    torrents on it by their name and checks the sizes. Nothing changes until you confirm.
                  </p>
                  <ul className="grid gap-1 text-xs">
                    <li>
                      Linux / macOS: <code className="font-mono">find /mnt/disk -type f -printf '%s %P\n'</code> or{" "}
                      <code className="font-mono">ls -lR /mnt/disk</code>
                    </li>
                    <li>
                      Windows: <code className="font-mono">dir /s D:\</code> or{" "}
                      <code className="font-mono">Get-ChildItem -Recurse D:\</code>
                    </li>
                  </ul>
                </div>
              </DialogDescription>
            </DialogHeader>
            <Textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder={"4123456789 Films/Movie.2020.1080p.BluRay-GRP/Movie.2020.1080p.BluRay-GRP.mkv\n…"}
              className="field-sizing-fixed h-64 resize-y font-mono text-xs"
              spellCheck={false}
              autoFocus
            />
            <div className="grid max-w-md gap-1.5">
              <Label htmlFor="listing-prefix">Folder on the disk (optional)</Label>
              <Input id="listing-prefix" value={prefix} onChange={(e) => setPrefix(e.target.value)} placeholder="e.g. Films" />
              <p className="text-muted-foreground text-xs">
                Put before the paths found, when you listed a folder of the disk rather than all of it.
              </p>
            </div>
            <input ref={fileInput} type="file" accept=".txt,.log,text/*" hidden onChange={loadFile} />
            <DialogFooter className="sm:justify-between">
              <Button type="button" variant="outline" onClick={() => fileInput.current?.click()}>
                <Upload /> Load file
              </Button>
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button type="button" variant="outline" onClick={onClose}>
                  Cancel
                </Button>
                <Button type="submit" disabled={busy || text.trim() === ""}>
                  {busy ? "Reading…" : "Analyze"}
                </Button>
              </div>
            </DialogFooter>
          </form>
        ) : (
          <div className="grid min-w-0 gap-4">
            <DialogHeader>
              <DialogTitle>On "{disk?.label}"</DialogTitle>
              <DialogDescription>
                Read as <span className="font-medium">{preview.format}</span>
                {preview.root && (
                  <>
                    {" "}
                    of <code className="font-mono break-all">{preview.root}</code>
                  </>
                )}
                : {plural(preview.files, "file")}
                {preview.sized && `, ${formatBytes(preview.size)}`}. {plural(items.length, "torrent")} recognized
                {preview.unmatched.length > 0 && `, ${plural(preview.unmatched.length, "other file or folder", "other files or folders")}`}.
              </DialogDescription>
            </DialogHeader>

            {!preview.sized && (
              <Alert variant="warning">
                <TriangleAlert />
                <AlertDescription>
                  The listing has no sizes: bookrr can't check that the copies are complete. Use a command that prints them
                  (find -printf, ls -l, dir, Get-ChildItem).
                </AlertDescription>
              </Alert>
            )}
            {preview.radarrError && (
              <Alert variant="destructive">
                <CircleAlert />
                <AlertDescription>Radarr: {preview.radarrError}</AlertDescription>
              </Alert>
            )}
            {preview.sonarrError && (
              <Alert variant="destructive">
                <CircleAlert />
                <AlertDescription>Sonarr: {preview.sonarrError}</AlertDescription>
              </Alert>
            )}

            {items.length === 0 ? (
              <p className="text-muted-foreground py-4 text-center text-sm">No torrent known to bookrr in this listing.</p>
            ) : (
              <div className="grid gap-2">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <ListChecks className="text-muted-foreground size-4" />
                  <span>
                    {chosen.length} of {items.length} selected · shift-click to tick a range
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="ml-auto"
                    onClick={() => setSelected(new Set(items.filter((it) => it.check !== "incomplete").map((it) => it.hash)))}
                  >
                    All
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())}>
                    None
                  </Button>
                </div>
                <ul className="divide-y rounded-md border">
                  {items.map((it, i) => (
                    <Row
                      key={it.hash}
                      item={it}
                      checked={selected.has(it.hash)}
                      onToggle={(shift) => toggle(i, shift)}
                      onLink={preview.radarr ? () => setLinking(it) : undefined}
                    />
                  ))}
                </ul>
              </div>
            )}

            {preview.unmatched.length > 0 && (
              <details className="text-sm">
                <summary className="text-muted-foreground cursor-pointer">
                  Not recognized ({preview.unmatched.length}): neither named like a torrent bookrr knows nor holding its files
                </summary>
                <ul className="mt-2 grid gap-1">
                  {preview.unmatched.map((o) => (
                    <li key={o.path} className="flex items-start gap-2 text-xs">
                      <FolderOpen className="text-muted-foreground mt-0.5 size-3.5 shrink-0" />
                      <span className="min-w-0 flex-1 font-mono break-all">{o.path}</span>
                      <span className="text-muted-foreground shrink-0 tabular-nums">
                        {plural(o.files, "file")}
                        {o.sized && ` · ${formatBytes(o.size)}`}
                      </span>
                    </li>
                  ))}
                </ul>
              </details>
            )}

            {items.length > 0 && (
              <div className="bg-muted/40 grid gap-3 rounded-md border p-3">
                <div className="flex flex-wrap gap-x-6 gap-y-2">
                  <div className="flex items-center gap-2">
                    <Switch id="ls-clients" checked={clients} onCheckedChange={setClients} />
                    <Label htmlFor="ls-clients">Remove from the clients</Label>
                  </div>
                  {preview.radarr && (
                    <div className="flex items-center gap-2">
                      <Switch id="ls-radarr" checked={radarr} onCheckedChange={setRadarr} />
                      <Label htmlFor="ls-radarr">Delete the films from Radarr</Label>
                    </div>
                  )}
                  {preview.sonarr && (
                    <div className="flex items-center gap-2">
                      <Switch id="ls-sonarr" checked={sonarr} onCheckedChange={setSonarr} />
                      <Label htmlFor="ls-sonarr">Delete the episodes from Sonarr</Label>
                    </div>
                  )}
                </div>
                {needChoice && (
                  <div className="grid gap-1.5">
                    <span className="text-sm font-medium">Files on the clients</span>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant={deleteFiles === false ? "default" : "outline"}
                        onClick={() => setDeleteFiles(false)}
                      >
                        Keep the files
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant={deleteFiles === true ? "destructive" : "outline"}
                        onClick={() => setDeleteFiles(true)}
                      >
                        Delete the files
                      </Button>
                    </div>
                    {deleteFiles === null && <p className="text-muted-foreground text-xs">Choose before archiving.</p>}
                  </div>
                )}
                {wholeSeries.length > 0 && (
                  <Alert variant="warning">
                    <TriangleAlert />
                    <AlertDescription>
                      No file would be left in {wholeSeries.map((s) => `"${s.title}"`).join(", ")}: the{" "}
                      {wholeSeries.length === 1 ? "series is" : "series are"} deleted from Sonarr.
                    </AlertDescription>
                  </Alert>
                )}
                <ul className="text-muted-foreground grid gap-0.5 text-xs">
                  <li>{plural(toArchive, "torrent")} recorded as archived on {disk?.label}</li>
                  {clients && (
                    <li>
                      {plural(onClients.length, "torrent")} removed from the clients
                      {needChoice && deleteFiles !== null && (deleteFiles ? ", with their files" : ", files kept")}
                    </li>
                  )}
                  {radarr && <li>{plural(films.length, "film")} deleted from Radarr with their files</li>}
                  {sonarr && (
                    <li>
                      {plural([...series.values()].reduce((n, s) => n + s.files.size, 0), "episode file")} deleted from Sonarr
                    </li>
                  )}
                </ul>
              </div>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" disabled={busy} onClick={() => setPreview(null)}>
                Back
              </Button>
              <Button
                type="button"
                variant={clients || radarr || sonarr ? "destructive" : "default"}
                disabled={busy || !ready}
                onClick={apply}
              >
                {busy ? "Working…" : `Archive ${plural(chosen.length, "torrent")}`}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
      <RadarrPicker
        item={linking}
        onClose={() => setLinking(null)}
        onPick={(m) => {
          if (linking) setLinked((prev) => new Map(prev).set(linking.hash, m))
          setLinking(null)
        }}
      />
    </Dialog>
  )
}

/** The title to search in Radarr for a torrent: its name up to the year, e.g. "Barbie" for BARBIE.2023.REMUX…. */
function guessTitle(name: string) {
  const words = name.replace(/[._]+/g, " ").split(" ")
  const year = words.findIndex((w, i) => i > 0 && /^(19|20)\d\d$/.test(w))
  return (year > 0 ? words.slice(0, year) : words.slice(0, 3)).join(" ")
}

/** Chooses by hand the film of Radarr deleted for a torrent. */
function RadarrPicker({
  item,
  onPick,
  onClose,
}: {
  item: ListingItem | null
  onPick: (m: ArrMovie | null) => void
  onClose: () => void
}) {
  const [q, setQ] = useState("")
  const [movies, setMovies] = useState<ArrMovie[] | null>(null)
  const [busy, setBusy] = useState(false)

  const search = async (query: string) => {
    if (!query.trim()) return
    setBusy(true)
    try {
      setMovies((await api.searchRadarr(query)).movies)
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    if (!item) return
    const guess = guessTitle(item.name)
    setQ(guess)
    setMovies(null)
    search(guess)
  }, [item])

  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Film of Radarr</DialogTitle>
          <DialogDescription className="[overflow-wrap:anywhere]">
            Which film of Radarr is "{item?.name}"? It is deleted from Radarr with its file when you archive.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            search(q)
          }}
        >
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Title or year" autoFocus />
          <Button type="submit" variant="outline" disabled={busy || !q.trim()}>
            <Search /> Search
          </Button>
        </form>
        {movies !== null &&
          (movies.length === 0 ? (
            <p className="text-muted-foreground py-2 text-center text-sm">No film in Radarr with this title.</p>
          ) : (
            <ul className="divide-y rounded-md border">
              {movies.map((m) => (
                <li key={m.id}>
                  <button type="button" className="hover:bg-muted/50 grid w-full gap-0.5 px-3 py-2 text-left" onClick={() => onPick(m)}>
                    <span className="text-sm font-medium">
                      {m.title} ({m.year})
                    </span>
                    <span className="text-muted-foreground text-xs break-all">
                      {m.hasFile ? `${formatBytes(m.fileSize)} · ${m.filePath}` : "no file in Radarr"}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          ))}
        <DialogFooter>
          {item?.radarr && (
            <Button type="button" variant="outline" onClick={() => onPick(null)}>
              No film: don't touch Radarr
            </Button>
          )}
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({
  item: it,
  checked,
  onToggle,
  onLink,
}: {
  item: ListingItem
  checked: boolean
  onToggle: (shift: boolean) => void
  /** Choose the film of Radarr by hand; missing without Radarr. */
  onLink?: () => void
}) {
  const c = checks[it.check]
  const disabled = it.check === "incomplete"
  return (
    <li className={cn("flex items-start gap-3 px-3 py-2.5", checked && "bg-primary/5", disabled && "opacity-70")}>
      <input
        type="checkbox"
        aria-label={`Select ${it.name}`}
        className="accent-primary mt-1 size-4 shrink-0 cursor-pointer disabled:cursor-not-allowed"
        checked={checked}
        disabled={disabled}
        onChange={() => {}}
        onClick={(e) => onToggle(e.shiftKey)}
      />
      <div className="grid min-w-0 flex-1 gap-1">
        <div className="flex flex-wrap items-start gap-2">
          <span className="min-w-0 flex-1 text-sm font-medium [overflow-wrap:anywhere]">{it.name}</span>
          <Badge
            variant={c.variant}
            title={
              it.via === "name"
                ? c.hint
                : it.check === "complete"
                  ? "Every big file of the torrent is on the disk, with its exact size (nfo and samples don't count)."
                  : "Some big files of the torrent are not on the disk. It can't be archived."
            }
          >
            {c.label}
          </Badge>
        </div>
        <div className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
          {it.personalRelease && <PersonalBadge />}
          <span className="flex min-w-0 items-center gap-1 font-mono break-all">
            <FolderOpen className="size-3.5 shrink-0" /> {it.path}
          </span>
          {it.via !== "name" && (
            <span className="text-foreground" title="The names on the disk are not the torrent's (renamed by Radarr or Sonarr): it was recognized by the exact sizes of its files.">
              {it.via === "radarr" ? "recognized through Radarr" : "recognized by its files"}
              {it.missing > 0 && <span className="text-destructive"> · {plural(it.missing, "file")} of the torrent not on the disk</span>}
            </span>
          )}
          <span className="tabular-nums">
            {it.via !== "name"
              ? formatBytes(it.diskSize)
              : it.check === "unknown" || it.diskSize === it.size
              ? formatBytes(it.size || it.diskSize)
              : `${formatBytes(it.diskSize)} on the disk · ${formatBytes(it.size)} the torrent`}
          </span>
          {it.archived && <span className="text-foreground">already archived here</span>}
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
          <span className="flex items-center gap-1">
            <Server className="text-muted-foreground size-3.5" />
            {it.locations.length === 0 ? (
              <span className="text-muted-foreground">on no client</span>
            ) : (
              it.locations.map((l) => (
                <Badge key={l.clientId} variant="outline" title={l.savePath}>
                  {l.clientName}
                  {l.progress < 1 && ` ${Math.floor(l.progress * 100)}%`}
                </Badge>
              ))
            )}
          </span>
          {it.radarr ? (
            <span className="min-w-0">
              <Clapperboard className="text-muted-foreground mr-1 inline size-3.5 align-[-2px]" />
              Radarr:{" "}
              {it.radarr.url ? (
                <a href={it.radarr.url} target="_blank" rel="noreferrer" className="text-primary underline-offset-2 hover:underline">
                  {it.radarr.title} ({it.radarr.year})
                </a>
              ) : (
                `${it.radarr.title} (${it.radarr.year})`
              )}
              {it.radarr.via === "size" && <span className="text-muted-foreground"> · by the size of its file</span>}
              {it.radarr.via === "hand" && <span className="text-muted-foreground"> · chosen by hand</span>}
              {it.radarr.hasFile && !it.radarr.sameFile && (
                <span className="text-amber-700 dark:text-amber-300"> · has another file now: left as it is</span>
              )}
              {onLink && (
                <button type="button" className="text-muted-foreground hover:text-foreground ml-1.5 underline underline-offset-2" onClick={onLink}>
                  change
                </button>
              )}
            </span>
          ) : (
            onLink &&
            !it.sonarr && (
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground flex items-center gap-1 underline underline-offset-2"
                onClick={onLink}
              >
                <Clapperboard className="size-3.5" /> Link to a film of Radarr…
              </button>
            )
          )}
          {it.sonarr && (
            <span className="min-w-0">
              <Tv className="text-muted-foreground mr-1 inline size-3.5 align-[-2px]" />
              Sonarr:{" "}
              {it.sonarr.url ? (
                <a href={it.sonarr.url} target="_blank" rel="noreferrer" className="text-primary underline-offset-2 hover:underline">
                  {it.sonarr.title}
                </a>
              ) : (
                it.sonarr.title
              )}{" "}
              · {it.sonarr.seasons.map((n) => `S${String(n).padStart(2, "0")}`).join(", ")} ·{" "}

              {plural(it.sonarr.files.length, "file")}
              {it.sonarr.upgraded > 0 && (
                <span className="text-amber-700 dark:text-amber-300">
                  {" "}
                  · {plural(it.sonarr.upgraded, "episode")} with another file now: left as they are
                </span>
              )}
            </span>
          )}
        </div>
      </div>
    </li>
  )
}

function Results({ results, onClose }: { results: ListingResult[]; onClose: () => void }) {
  return (
    <div className="grid min-w-0 gap-4">
      <DialogHeader>
        <DialogTitle>Done</DialogTitle>
        <DialogDescription>What was done for each torrent. The steps not done are in red.</DialogDescription>
      </DialogHeader>
      <ul className="divide-y rounded-md border">
        {results.map((r) => (
          <li key={r.hash} className="grid gap-1 px-3 py-2.5">
            <span className="text-sm font-medium [overflow-wrap:anywhere]">{r.name || r.hash}</span>
            <ul className="grid gap-0.5 text-xs">
              {r.steps.map((s, i) => (
                <li key={i} className={cn("flex items-start gap-1.5", !s.ok && "text-destructive")}>
                  {s.ok ? <Check className="mt-0.5 size-3.5 shrink-0 text-emerald-600" /> : <X className="mt-0.5 size-3.5 shrink-0" />}
                  <span>
                    <span className="font-medium">{s.what}</span>: {s.message}
                  </span>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
      <DialogFooter>
        <Button onClick={onClose}>Close</Button>
      </DialogFooter>
    </div>
  )
}
