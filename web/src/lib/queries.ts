import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/lib/api"

export const keys = {
  torrents: ["torrents"] as const,
  alerts: (open: boolean) => ["alerts", open] as const,
  disks: ["disks"] as const,
  clients: ["clients"] as const,
  info: ["info"] as const,
  sync: ["sync"] as const,
}

export const useTorrents = () => useQuery({ queryKey: keys.torrents, queryFn: api.torrents, refetchInterval: 30_000 })
export const useAlerts = (open = true) =>
  useQuery({ queryKey: keys.alerts(open), queryFn: () => api.alerts(open), refetchInterval: 30_000 })
export const useDisks = () => useQuery({ queryKey: keys.disks, queryFn: api.disks })
export const useClients = () => useQuery({ queryKey: keys.clients, queryFn: api.clients, refetchInterval: 30_000 })
export const useInfo = () => useQuery({ queryKey: keys.info, queryFn: api.info, staleTime: Infinity })
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
