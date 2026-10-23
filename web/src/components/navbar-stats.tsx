import { createContext, useContext, type ReactNode } from "react"
import { createPortal } from "react-dom"

import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

/** The spot in the navbar where the open page shows its counters. */
export const NavbarSlot = createContext<HTMLElement | null>(null)

/** Renders the page's stat cards in the navbar. */
export function NavbarStats({ children }: { children: ReactNode }) {
  const slot = useContext(NavbarSlot)
  return slot ? createPortal(children, slot) : null
}

/** A counter in the navbar; with onClick it filters the page and active marks the chosen one. */
export function StatCard({
  label,
  value,
  icon: Icon,
  tone,
  active,
  loading,
  onClick,
}: {
  label: string
  value: number
  icon: React.ElementType
  tone?: string
  active?: boolean
  loading?: boolean
  onClick?: () => void
}) {
  const Box = onClick ? "button" : "div"
  return (
    <Box
      type={onClick ? "button" : undefined}
      onClick={onClick}
      className={cn(
        "bg-card flex h-9 shrink-0 items-center gap-1.5 rounded-lg border px-3 text-sm whitespace-nowrap transition-colors",
        onClick && "hover:bg-accent/50 cursor-pointer",
        active && "ring-primary/40 border-primary/50 ring-2"
      )}
    >
      <Icon className={cn("text-muted-foreground size-4", tone)} />
      <span className="text-muted-foreground">{label}</span>
      <span className={cn("font-semibold tabular-nums", value > 0 && tone)}>
        {loading ? <Skeleton className="h-4 w-6" /> : value}
      </span>
    </Box>
  )
}
