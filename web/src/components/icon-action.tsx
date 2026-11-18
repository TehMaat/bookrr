import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

/**
 * A small icon button with its name in a tooltip (and for screen readers): the actions of a row, always in sight.
 * With `href` it is a link; `danger` turns it red on hover; `active` colors it (a note there, marked complete).
 */
export function IconAction({
  label,
  onClick,
  href,
  download,
  disabled,
  danger,
  active,
  small,
  className,
  children,
}: {
  label: string
  onClick?: () => void
  href?: string
  download?: boolean
  disabled?: boolean
  danger?: boolean
  active?: boolean
  /** 24 px instead of 32: inside a cell, next to a torrent. */
  small?: boolean
  className?: string
  children: React.ReactNode
}) {
  const classes = cn(
    "text-muted-foreground hover:text-foreground",
    small && "size-6 [&_svg:not([class*='size-'])]:size-3.5",
    active && "text-primary bg-primary/10 hover:text-primary",
    danger && "hover:text-destructive hover:bg-destructive/10 dark:hover:bg-destructive/15",
    className,
  )
  const button = href ? (
    <Button variant="ghost" size="icon-sm" className={classes} aria-label={label} asChild>
      <a href={href} target={download ? undefined : "_blank"} rel="noreferrer" download={download || undefined}>
        {children}
      </a>
    </Button>
  ) : (
    <Button variant="ghost" size="icon-sm" className={classes} aria-label={label} disabled={disabled} onClick={onClick}>
      {children}
    </Button>
  )
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {/* A disabled button gets no pointer events: the span keeps the tooltip. */}
        {disabled ? <span className="inline-flex">{button}</span> : button}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

/**
 * The actions of a table row (a `group/row`): faded until the pointer is on the row or one of them has the focus, so
 * they do not compete with the content. Always full on touch screens, which have no hover.
 */
export const rowActions =
  "opacity-40 transition-opacity group-hover/row:opacity-100 focus-within:opacity-100 [@media(hover:none)]:opacity-100"

/** As rowActions, but hidden until then: the actions of the parts of a row (a .torrent of a film). */
export const partActions =
  "opacity-0 transition-opacity group-hover/row:opacity-100 focus-within:opacity-100 has-[[data-state=open]]:opacity-100 [@media(hover:none)]:opacity-100"

/** The thin line between the actions of a row and the one that removes. */
export function ActionSeparator() {
  return <span aria-hidden className="bg-border mx-0.5 h-4 w-px shrink-0" />
}
