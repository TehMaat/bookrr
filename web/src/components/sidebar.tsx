import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { logoUrl } from "@/lib/accent"
import { useInfo } from "@/lib/queries"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export type NavItem = { id: string; label: string; icon: React.ElementType; count?: number }

/** Logo, version and the menu of the pages, on the left side. */
export function Sidebar({ items, active, onSelect }: { items: NavItem[]; active: string; onSelect: (id: string) => void }) {
  const { accent } = useTheme()
  const info = useInfo()

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-16 shrink-0 items-center gap-2 border-b px-4 font-semibold">
        <img src={logoUrl(accent)} alt="" className="size-8" />
        <span className="text-lg">bookrr</span>
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
    </div>
  )
}
