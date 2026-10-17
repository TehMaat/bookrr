import { useEffect, useRef, useState } from "react"
import { ArchiveRestore, ArchiveX, X } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { api, type Archivability } from "@/lib/api"
import { useAction } from "@/lib/queries"

/** A row that can be marked as never to archive: its torrent hash and how it stands. */
export type Pickable = { hash: string; archivable: Archivability | null }

/** Ticks on the uploads still on a client (the only ones with archivability) among rows. */
export function usePicker(rows: Pickable[]) {
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const selectable = rows.filter((r) => r.archivable)
  const chosen = selectable.filter((r) => picked.has(r.hash))
  const all = selectable.length > 0 && chosen.length === selectable.length
  return {
    chosen,
    isPicked: (hash: string) => picked.has(hash),
    toggle: (hash: string, on: boolean) =>
      setPicked((p) => {
        const n = new Set(p)
        if (on) n.add(hash)
        else n.delete(hash)
        return n
      }),
    all,
    some: chosen.length > 0 && !all,
    toggleAll: (on: boolean) => setPicked(on ? new Set(selectable.map((r) => r.hash)) : new Set()),
    clear: () => setPicked(new Set()),
  }
}

/** A checkbox in a clickable row: ticking it does not open the row. */
export function PickBox({
  checked,
  indeterminate = false,
  onChange,
  label,
}: {
  checked: boolean
  indeterminate?: boolean
  onChange: (on: boolean) => void
  label: string
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate
  }, [indeterminate])
  return (
    <input
      ref={ref}
      type="checkbox"
      aria-label={label}
      className="accent-primary size-4 shrink-0 cursor-pointer align-middle"
      checked={checked}
      onClick={(e) => e.stopPropagation()}
      onChange={(e) => onChange(e.target.checked)}
    />
  )
}

/** Marks uploads as never to archive, or archivable again, toasting how many. */
export function useNeverArchive() {
  return useAction(({ hashes, never }: { hashes: string[]; never: boolean }) => api.neverArchive(hashes, never))
}

export function neverArchiveDone(n: number, never: boolean) {
  if (never) toast.success(n === 1 ? "1 torrent non sarà più segnato da archiviare" : `${n} torrent non saranno più segnati da archiviare`)
  else toast.success(n === 1 ? "1 torrent di nuovo segnalato quando è archiviabile" : `${n} torrent di nuovo segnalati quando sono archiviabili`)
}

/**
 * Above the list while picking: what the ticks are for, how many are kept on the clients (with a link to show them),
 * and, once some are ticked, the actions on them.
 */
export function NeverArchiveBar({
  chosen,
  kept,
  onShowKept,
  onClear,
}: {
  chosen: Pickable[]
  kept: number
  onShowKept?: () => void
  onClear: () => void
}) {
  const mark = useNeverArchive()
  const hashes = [...new Set(chosen.map((r) => r.hash))]
  const toKeep = chosen.filter((r) => !r.archivable?.neverArchive).length
  const toRestore = chosen.length - toKeep
  const run = (never: boolean) =>
    mark.mutate(
      { hashes, never },
      {
        onSuccess: (r) => {
          neverArchiveDone(r.updated, never)
          onClear()
        },
      }
    )
  return (
    <div className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
      {chosen.length === 0 ? (
        <span className="text-muted-foreground">
          Spunta i torrent che non vuoi archiviare mai: non saranno più segnati come archiviabili.
        </span>
      ) : (
        <>
          <span className="font-medium">{hashes.length === 1 ? "1 selezionato" : `${hashes.length} selezionati`}</span>
          {toKeep > 0 && (
            <Button size="sm" className="h-7" disabled={mark.isPending} onClick={() => run(true)}>
              <ArchiveX /> Non segnare da archiviare
            </Button>
          )}
          {toRestore > 0 && (
            <Button size="sm" variant="outline" className="h-7" disabled={mark.isPending} onClick={() => run(false)}>
              <ArchiveRestore /> Segnala di nuovo quando archiviabile
            </Button>
          )}
          <Button size="sm" variant="ghost" className="h-7" onClick={onClear}>
            <X /> Deseleziona
          </Button>
        </>
      )}
      {kept > 0 && onShowKept && (
        <button type="button" className="text-muted-foreground ml-auto text-xs underline-offset-2 hover:underline" onClick={onShowKept}>
          {kept === 1 ? "1 da non archiviare" : `${kept} da non archiviare`} · mostra
        </button>
      )}
    </div>
  )
}
