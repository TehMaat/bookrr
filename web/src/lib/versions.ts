/**
 * The versions of a film for its release name on Wayfinderr: bookrr's own copy of
 * Wayfinderr's list (moverr's). Free text is fine too.
 */
export const VERSIONS = [
  "Theatrical",
  "Extended",
  "Director's Cut",
  "Unrated",
  "Special Edition",
  "Remastered",
  "IMAX",
  "Final Cut",
  "Uncut",
  "Anniversary Edition",
]

/** "" is the standard version, chosen on purpose. */
export const versionLabel = (v: string) => v || "None"

export const MAX_VERSION = 60
