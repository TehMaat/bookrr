import type { ArchiveInput, TorrentInput } from "@/lib/api"
import { parseBytes } from "@/lib/format"

/** A hand-entered torrent as typed in the "Aggiungi" form or read from an import file. */
export type ManualFields = {
  name: string
  hash: string
  size: string
  tags: string
  personal: boolean
  notes: string
  adopterId: number | null
  archive: ArchiveInput
}

/** Placeholder hash given by bookrr to a torrent added without its info hash. */
export const isManualHash = (hash: string) => hash.startsWith("manual-")

export function isValidHash(hash: string): boolean {
  return hash.trim() === "" || /^[0-9a-f]{40}$|^[0-9a-f]{64}$/i.test(hash.trim())
}

/**
 * Builds the body of POST /api/torrents. The form and the bulk import both
 * use it, so a torrent is stored the same way whichever way it was entered.
 */
export function buildTorrentInput(f: ManualFields): TorrentInput {
  return {
    name: f.name.trim(),
    hash: f.hash.trim(),
    size: parseBytes(f.size),
    tags: f.tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean),
    personalRelease: f.personal,
    notes: f.notes,
    archive: f.archive,
    adoption: f.adopterId ? { adopterId: f.adopterId, notes: "" } : undefined,
  }
}
