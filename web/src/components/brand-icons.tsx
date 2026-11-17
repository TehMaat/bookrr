import { useState } from "react"

import mdblistLogo from "@/assets/mdblist.svg"
import tmdbLogo from "@/assets/tmdb.svg"
import { cn } from "@/lib/utils"

// The logos of the services bookrr links to or sends to: an action of bookrr has a Lucide icon, a service its logo.
// TMDB and MDBList are the marks of Simple Icons (CC0), in their brand color.

/** The Wayfinderr mark (a glass pyramid with a glowing core), in the color of the text like a Lucide icon. */
export function WayfinderrIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 100 102" fill="none" aria-hidden className={cn("size-4", className)}>
      <path d="M50 4 L6 76 L46 98 Z" fill="currentColor" fillOpacity={0.35} />
      <path d="M50 4 L46 98 L94 76 Z" fill="currentColor" fillOpacity={0.15} />
      <circle cx="48" cy="62" r="20" fill="currentColor" fillOpacity={0.25} />
      <path d="M50 4 L6 76 L46 98 L94 76 Z M50 4 L46 98" stroke="currentColor" strokeWidth={9} strokeLinejoin="round" />
      <circle cx="48" cy="62" r="9" fill="currentColor" />
    </svg>
  )
}

export function TmdbLogo({ className }: { className?: string }) {
  return <img src={tmdbLogo} alt="" className={cn("size-6", className)} />
}

export function MdbListLogo({ className }: { className?: string }) {
  return <img src={mdblistLogo} alt="" className={cn("size-4 rounded-[3px]", className)} />
}

/**
 * The favicon of the site of a link (the tracker of the user), or its first letter when the site has none: every
 * user sees their own tracker.
 */
export function SiteIcon({ url, className }: { url: string; className?: string }) {
  const [failed, setFailed] = useState(false)
  let origin = ""
  let host = ""
  try {
    const u = new URL(url)
    origin = u.origin
    host = u.hostname.replace(/^www\./, "")
  } catch {
    // Not a URL: the letter only.
  }
  if (!origin || failed)
    return (
      <span
        aria-hidden
        className={cn(
          "bg-primary text-primary-foreground inline-flex size-4 shrink-0 items-center justify-center rounded-[4px] text-[10px] leading-none font-bold uppercase",
          className,
        )}
      >
        {(host || "?").charAt(0)}
      </span>
    )
  return (
    <img
      src={`${origin}/favicon.ico`}
      alt=""
      className={cn("size-4 shrink-0 rounded-[3px]", className)}
      onError={() => setFailed(true)}
    />
  )
}
