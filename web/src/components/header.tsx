import { BookCheck, Laptop, Moon, RefreshCw, Sun } from "lucide-react"
import { toast } from "sonner"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api } from "@/lib/api"
import { formatRelative } from "@/lib/format"
import { useAction, useInfo, useSyncState } from "@/lib/queries"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export function Header() {
  const { setTheme } = useTheme()
  const info = useInfo()
  const sync = useSyncState()
  const syncNow = useAction(() => api.syncNow())
  const running = sync.data?.running || syncNow.isPending

  return (
    <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
      <div className="mx-auto flex h-14 max-w-[1600px] items-center gap-3 px-4 sm:px-6">
        <div className="flex items-center gap-2 font-semibold">
          <div className="bg-violet-600 text-white flex size-8 items-center justify-center rounded-lg">
            <BookCheck className="size-5" />
          </div>
          <span className="text-lg">bookrr</span>
          {info.data && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="violet" className="font-mono">
                  {info.data.version}
                </Badge>
              </TooltipTrigger>
              <TooltipContent>Versione di bookrr</TooltipContent>
            </Tooltip>
          )}
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground hidden text-xs md:inline">
                Sync {formatRelative(sync.data?.lastRunAt)}
              </span>
            </TooltipTrigger>
            <TooltipContent>{sync.data?.lastResult || "Nessuna sincronizzazione ancora"}</TooltipContent>
          </Tooltip>
          <Button
            variant="outline"
            size="sm"
            disabled={running}
            onClick={() =>
              syncNow.mutate(undefined, { onSuccess: (r) => toast.success("Sincronizzazione completata", { description: r.result }) })
            }
          >
            <RefreshCw className={cn(running && "animate-spin")} />
            <span className="hidden sm:inline">Sincronizza</span>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Tema">
                <Sun className="dark:hidden" />
                <Moon className="hidden dark:block" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setTheme("light")}>
                <Sun /> Chiaro
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setTheme("dark")}>
                <Moon /> Scuro
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setTheme("system")}>
                <Laptop /> Sistema
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </header>
  )
}
