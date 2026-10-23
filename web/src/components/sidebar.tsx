import { useState } from "react"
import { Check, Laptop, Moon, Palette, RefreshCw, Settings, Sun } from "lucide-react"
import { toast } from "sonner"

import { SettingsDialog } from "@/components/settings-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { accents, logoUrl } from "@/lib/accent"
import { api } from "@/lib/api"
import { formatRelative } from "@/lib/format"
import { useAction, useInfo, useSyncState } from "@/lib/queries"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export type NavItem = { id: string; label: string; icon: React.ElementType; count?: number }

/** Logo and menu of the pages on the left side; sync, preferences and version at the bottom. */
export function Sidebar({ items, active, onSelect }: { items: NavItem[]; active: string; onSelect: (id: string) => void }) {
  const { accent } = useTheme()

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-16 shrink-0 items-center gap-2 border-b px-4 font-semibold">
        <img src={logoUrl(accent)} alt="" className="size-8" />
        <span className="text-lg">bookrr</span>
      </div>
      <nav className="grid gap-1 overflow-y-auto p-3">
        {items.map((it) => (
          <button
            key={it.id}
            type="button"
            onClick={() => onSelect(it.id)}
            aria-current={it.id === active ? "page" : undefined}
            className={cn(
              "text-muted-foreground hover:bg-accent hover:text-foreground flex h-9 cursor-pointer items-center gap-2.5 rounded-md px-3 text-sm font-medium transition-colors [&_svg]:size-4 [&_svg]:shrink-0",
              it.id === active && "bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary"
            )}
          >
            <it.icon />
            {it.label}
            {!!it.count && (
              <Badge variant="warning" className="ml-auto px-1.5">
                {it.count}
              </Badge>
            )}
          </button>
        ))}
      </nav>
      <SidebarFooter />
    </div>
  )
}

function SidebarFooter() {
  const { setTheme, accent, setAccent } = useTheme()
  const info = useInfo()
  const sync = useSyncState()
  const syncNow = useAction(() => api.syncNow())
  const running = sync.data?.running || syncNow.isPending
  const [settingsOpen, setSettingsOpen] = useState(false)

  return (
    <div className="mt-auto grid gap-2 border-t p-3">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="text-muted-foreground px-1 text-xs">Sync {formatRelative(sync.data?.lastRunAt)}</span>
        </TooltipTrigger>
        <TooltipContent side="right">{sync.data?.lastResult || "Nessuna sincronizzazione ancora"}</TooltipContent>
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
        Sincronizza
      </Button>
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Impostazioni" onClick={() => setSettingsOpen(true)}>
              <Settings />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Impostazioni</TooltipContent>
        </Tooltip>
        <DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-sm" aria-label="Colore">
                  <Palette />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent>Colore</TooltipContent>
          </Tooltip>
          <DropdownMenuContent side="top" align="start">
            <DropdownMenuLabel>Colore della spada</DropdownMenuLabel>
            {accents.map((a) => (
              <DropdownMenuItem key={a.id} onClick={() => setAccent(a.id)}>
                <img src={logoUrl(a)} alt="" className="size-5" />
                <span>{a.label}</span>
                <span className="text-muted-foreground text-xs">{a.saber}</span>
                {a.id === accent.id && <Check className="ml-auto" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Tema">
              <Sun className="dark:hidden" />
              <Moon className="hidden dark:block" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="start">
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
        {info.data && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge variant="brand" className="ml-auto font-mono">
                {info.data.version}
              </Badge>
            </TooltipTrigger>
            <TooltipContent>Versione di bookrr</TooltipContent>
          </Tooltip>
        )}
      </div>
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </div>
  )
}
