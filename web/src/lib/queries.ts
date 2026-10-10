import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/lib/api"

export const keys = {
  torrents: ["torrents"] as const,
  alerts: (open: boolean) => ["alerts", open] as const,
  disks: ["disks"] as const,
  bucketEntries: (id: number) => ["bucket-entries", id] as const,
  adopters: ["adopters"] as const,
  clients: ["clients"] as const,
  info: ["info"] as const,
  sync: ["sync"] as const,
  settings: ["settings"] as const,
}

export const useTorrents = () => useQuery({ queryKey: keys.torrents, queryFn: api.torrents, refetchInterval: 30_000 })
export const useAlerts = (open = true) =>
  useQuery({ queryKey: keys.alerts(open), queryFn: () => api.alerts(open), refetchInterval: 30_000 })
export const useDisks = () => useQuery({ queryKey: keys.disks, queryFn: api.disks, refetchInterval: 60_000 })
export const useBucketEntries = (id: number, enabled: boolean) =>
  useQuery({ queryKey: keys.bucketEntries(id), queryFn: () => api.bucketEntries(id), enabled })
export const useAdopters = () => useQuery({ queryKey: keys.adopters, queryFn: api.adopters })
export const useClients = () => useQuery({ queryKey: keys.clients, queryFn: api.clients, refetchInterval: 30_000 })
export const useInfo = () => useQuery({ queryKey: keys.info, queryFn: api.info, staleTime: Infinity })
export const useSettings = () => useQuery({ queryKey: keys.settings, queryFn: api.settings })
export const useSyncState = () => useQuery({ queryKey: keys.sync, queryFn: api.syncState, refetchInterval: 5_000 })

/** Invalidates every query: data in bookrr is small and highly interrelated. */
export function useRefreshAll() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries()
}

/** A mutation that refreshes all data and toasts on success/failure. */
export function useAction<TArgs, TResult>(fn: (args: TArgs) => Promise<TResult>, success?: string) {
  const refresh = useRefreshAll()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      if (success) toast.success(success)
      refresh()
    },
    onError: (err: Error) => toast.error(err.message),
  })
}

/** Reads a cloud archive's bucket now and toasts what changed. */
export function useScanDisk() {
  const refresh = useRefreshAll()
  return useMutation({
    mutationFn: (id: number) => api.scanDisk(id),
    onSuccess: (r) => {
      const changes = [
        r.added > 0 && `trovati: ${r.added}`,
        r.removed > 0 && `non più presenti: ${r.removed}`,
        r.alerts > 0 && `nuove segnalazioni: ${r.alerts}`,
      ].filter(Boolean)
      const unmatched = r.disk.s3?.unmatched ? `non riconosciuti: ${r.disk.s3.unmatched}` : ""
      toast.success(`Bucket "${r.disk.label}" letto`, {
        description: [changes.length ? `Torrent ${changes.join(", ")}` : "Nessuna novità", unmatched].filter(Boolean).join(" · "),
      })
      refresh()
    },
    onError: (err: Error) => {
      toast.error("Lettura del bucket fallita", { description: err.message })
      refresh()
    },
  })
}
