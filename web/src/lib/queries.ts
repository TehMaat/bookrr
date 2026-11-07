import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, type PlanTorrent } from "@/lib/api"

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
  config: ["config"] as const,
  releases: ["releases"] as const,
  missingSeries: ["missing-series"] as const,
  missingSagas: ["missing-sagas"] as const,
  missingLists: ["missing-lists"] as const,
  logFiles: ["log-files"] as const,
  plan: ["plan"] as const,
}

/** Being downloaded, or sent and not listed by the client yet. */
export const torrentPending = (t: PlanTorrent) =>
  (t.downloads.length > 0 && !t.downloads.some((d) => d.progress >= 1)) || (!!t.sentAt && t.downloads.length === 0 && !t.gone)

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
export const useConfig = (enabled = true) => useQuery({ queryKey: keys.config, queryFn: api.config, enabled })
export const useReleases = () =>
  useQuery({
    queryKey: keys.releases,
    queryFn: api.releases,
    // Poll quickly while the tracker is being read.
    refetchInterval: (q) => (q.state.data?.state?.running ? 3_000 : 30_000),
  })
export const useMissingSeries = () =>
  useQuery({
    queryKey: keys.missingSeries,
    queryFn: api.missingSeries,
    refetchInterval: (q) => (q.state.data?.state?.running ? 3_000 : 60_000),
  })
export const useMissingSagas = () =>
  useQuery({
    queryKey: keys.missingSagas,
    queryFn: api.missingSagas,
    refetchInterval: (q) => (q.state.data?.state?.running ? 3_000 : 60_000),
  })
export const useMissingLists = () =>
  useQuery({
    queryKey: keys.missingLists,
    queryFn: api.missingLists,
    refetchInterval: (q) => (q.state.data?.state?.running ? 3_000 : 60_000),
  })
export const usePlan = () =>
  useQuery({
    queryKey: keys.plan,
    queryFn: api.plan,
    // Poll quickly while films are being searched or their torrents downloaded.
    refetchInterval: (q) => {
      const d = q.state.data
      if (d?.entries.some((e) => e.searching) || d?.best?.running) return 3_000
      return d?.entries.some((e) => e.torrents.some(torrentPending)) ? 10_000 : 60_000
    },
  })
export const useLogFiles = () => useQuery({ queryKey: keys.logFiles, queryFn: api.logFiles, refetchInterval: 60_000 })
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
        r.added > 0 && `found: ${r.added}`,
        r.removed > 0 && `gone: ${r.removed}`,
        r.alerts > 0 && `new alerts: ${r.alerts}`,
      ].filter(Boolean)
      const unmatched = r.disk.s3?.unmatched ? `unrecognized: ${r.disk.s3.unmatched}` : ""
      toast.success(`Bucket "${r.disk.label}" read`, {
        description: [changes.length ? `Torrent ${changes.join(", ")}` : "Nothing new", unmatched].filter(Boolean).join(" · "),
      })
      refresh()
    },
    onError: (err: Error) => {
      toast.error("Reading the bucket failed", { description: err.message })
      refresh()
    },
  })
}
