import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"

/**
 * The card of a page's table, edge to edge: it takes the whole width of the page, without the space around it, and
 * its first and last columns line up with the rest of the page.
 */
export function TableCard({ className, ...props }: React.ComponentProps<typeof Card>) {
  return (
    <Card
      className={cn(
        "-mx-4 gap-0 overflow-hidden rounded-none border-x-0 py-0 shadow-none sm:-mx-6",
        "[&_tr>*:first-child]:pl-4 [&_tr>*:last-child]:pr-4 sm:[&_tr>*:first-child]:pl-6 sm:[&_tr>*:last-child]:pr-6",
        className
      )}
      {...props}
    />
  )
}
