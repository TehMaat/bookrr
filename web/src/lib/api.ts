export type Location = {
  clientId: number
  clientName: string
  savePath: string
  state: string
  progress: number
  ratio: number
  tags: string
  category: string
  addedOn: number
  seenAt: string
}

export type Archive = {
  id: number
  diskId: number | null
  diskLabel: string
  diskSerial: string
  diskKind: string
  path: string
  notes: string
  createdAt: string
  /** "s3" when found in the disk's bucket: kept in sync with it, not edited by hand. */
  source: string
}

export type Adoption = {
  id: number
  adopterId: number
  adopterName: string
  notes: string
  createdAt: string
}

export type Alert = {
  id: number
  hash: string
  torrentName: string
  /** "s3": the torrent is no longer in the bucket of a cloud archive. */
  source: "sync" | "webhook" | "s3"
  clientName: string
  message: string
  createdAt: string
  resolvedAt: string | null
  resolution: string
}

export type TorrentStatus = "client" | "missing" | "archived" | "adopted" | "unknown"

/** Thresholds of the archive rule, from the settings. */
export type ArchiveRule = { minOthers: number; minCompleted: number; days: number }

/** How an upload still on a client stands against the archive rule. */
export type Archivability = {
  releaseId: number
  /** Seeders on the tracker, the user included. */
  seeders: number
  /** Clients of bookrr seeding it complete. */
  mine: number
  /** seeders − mine, never below zero. */
  others: number
  timesCompleted: number
  /** Last time the tracker gave these numbers. */
  trackerAt: string
  /** The thresholds are met now. */
  met: boolean
  /** Since when they are met without a break. */
  since: string | null
  /** Whole days since `since`. */
  days: number
  /** Met for at least the days of the rule: it can be taken off the clients. */
  archivable: boolean
  /** When it becomes archivable if the thresholds stay met. */
  readyAt: string | null
  rule: ArchiveRule
  /** The user keeps it on the clients: never archivable. */
  neverArchive: boolean
}

/** What the tracker counts of an upload. */
export type Peers = {
  releaseId: number
  seeders: number
  leechers: number
  timesCompleted: number
  /** Last time the tracker gave these numbers. */
  trackerAt: string
}

export type Torrent = {
  hash: string
  name: string
  size: number
  tags: string[]
  category: string
  tracker: string
  personalRelease: boolean
  manual: boolean
  notes: string
  firstSeenAt: string
  lastSeenAt: string | null
  updatedAt: string
  /** Last failed lookup of the hash on the UNIT3D tracker. */
  hashLookupAt: string | null
  hashLookupError: string
  /** Page of the torrent on the UNIT3D tracker, once found there. */
  trackerUrl: string
  /** The .torrent file downloaded from the tracker is saved in bookrr. */
  hasTorrentFile: boolean
  locations: Location[]
  archives: Archive[]
  adoptions: Adoption[]
  openAlert: Alert | null
  duplicate: boolean
  status: TorrentStatus
  /** Null when it is not one of the user's uploads still on a client. */
  archivable: Archivability | null
  /** Null when it is not one of the user's uploads still on the tracker. */
  peers: Peers | null
}

export type Disk = {
  id: number
  label: string
  kind: string
  serial: string
  model: string
  capacity: number
  place: string
  notes: string
  createdAt: string
  archiveCount: number
  archivedSize: number
  firmware: string
  health: string
  powerOnHours: number
  smartAt: string | null
  /** The S3 bucket behind a cloud archive, read by bookrr (never written). */
  s3: DiskS3 | null
}

export type DiskS3 = {
  endpoint: string
  region: string
  bucket: string
  /** Folder of the bucket that is scanned, ending with "/"; empty = the whole bucket. */
  prefix: string
  accessKey: string
  hasSecret: boolean
  scanAt: string | null
  scanError: string
  objects: number
  size: number
  /** Files and folders of the bucket that match no torrent. */
  unmatched: number
}

/** Bucket settings sent by the form; no secretKey keeps the stored one. */
export type S3Input = {
  endpoint: string
  region: string
  bucket: string
  prefix: string
  accessKey: string
  secretKey?: string
}

export type DiskInput = Partial<Omit<Disk, "s3">> & { s3: S3Input | null }

/** A file or folder of a bucket that matches no torrent. */
export type BucketEntry = { path: string; name: string; size: number; files: number; modifiedAt: string | null }
export type ScanResult = { added: number; removed: number; alerts: number; disk: Disk }

export type SmartInfo = {
  model: string
  serial: string
  firmware: string
  capacity: number
  kind: string
  health: string
  powerOnHours: number
}

/** Target of a SMART import: a disk id, a new disk, or (by default) the disk with the same serial. */
export type SmartTarget = { id?: number; newLabel?: string; dryRun?: boolean }
export type SmartResult = { created: boolean; disk: Disk; smart: SmartInfo }

export type Adopter = {
  id: number
  name: string
  contact: string
  notes: string
  createdAt: string
  adoptedCount: number
  adoptedSize: number
}

export type Client = {
  id: number
  name: string
  url: string
  username: string
  hasPassword: boolean
  skipTlsVerify: boolean
  enabled: boolean
  lastSyncAt: string | null
  lastError: string
  torrentCount: number
  createdAt: string
}

/** A tracker torrent that may be the one a torrent without hash refers to. */
export type TrackerCandidate = {
  id: string
  name: string
  size: number
  createdAt: string
  detailsLink: string
  /** Share of words in common with the torrent name, 0-100. */
  score: number
  /** Same release written differently: title, formats and group agree. */
  match: boolean
}

/** Status of an upload on the tracker: the status of the linked torrent, or "untracked" when bookrr doesn't know it. */
export type ReleaseStatus = TorrentStatus | "untracked"

/** One of the user's uploads on the UNIT3D tracker. */
export type TrackerRelease = {
  id: number
  name: string
  category: string
  type: string
  typeId: number
  resolution: string
  resolutionId: number
  size: number
  seeders: number
  leechers: number
  timesCompleted: number
  tmdbId: number
  imdbId: number
  tvdbId: number
  personalRelease: boolean
  createdAt: string
  detailsLink: string
  /** Info hash computed from the .torrent; empty until it is downloaded. */
  hash: string
  hashError: string
  hasTorrentFile: boolean
  seenAt: string
  /** No longer among the uploads on the tracker (deleted or replaced). */
  removedAt: string | null
  updatedAt: string
  /** Hash of the linked torrent of bookrr, empty if none. */
  torrentHash: string
  status: ReleaseStatus
  /** Null when the linked torrent is on no client. */
  archivable: Archivability | null
  /** For films, the best resolution found with Prowlarr; null when not searched yet or Prowlarr is not configured. */
  best: BestResolution | null
}

/** The best release found with Prowlarr for a film (by type, then resolution), whatever the language. */
export type BestResolution = {
  film: string
  /** "" when no release of the film with a resolution was found. */
  resolution: string
  /** Type of the release found, from its name ("Remux", "WEB-DL"); "" if unknown. */
  type: string
  name: string
  indexer: string
  url: string
  results: number
  /** "" when it was not searched: the user already has the film in a 2160p Remux. */
  checkedAt: string
  /** Resolution and type of the best of the user's uploads of the film. */
  mine: string
  mineType: string
  /** The release found is better than the user's: a better type, or the same type in a higher resolution. */
  better: boolean
}

/** The daily search of the best resolutions with Prowlarr. */
export type BestState = {
  running: boolean
  lastRunAt: string | null
  lastResult: string
  lastError: string
  nextRunAt: string | null
  done: number
  total: number
  /** How often each film is searched again ("7d"). */
  recheck: string
}

export type ReleasesState = {
  running: boolean
  lastRunAt: string | null
  lastResult: string
  lastError: string
  nextRunAt: string | null
  username: string
  pending: number
}

export type Releases = {
  /** A UNIT3D tracker is configured. */
  enabled: boolean
  state: ReleasesState | null
  /** Last time the uploads were read in full. */
  syncedAt: string | null
  releases: TrackerRelease[]
  /** Personal releases of bookrr missing from the uploads on the tracker. */
  notOnTracker: { hash: string; name: string; tracker: string }[]
  /** Null when Prowlarr is not configured. */
  best: BestState | null
}

/**
 * How a season, or a film of a saga or a collection, stands in one of the user's qualities. banned: uploaded by others
 * in this quality, but only by release groups the user banned (still missing, with less priority).
 */
/** Where a film of the release plan stands. */
export type PlanStatus = "todo" | "doing" | "downloaded" | "done" | "dropped"

/** A film the user plans to release, from TMDB. */
export type PlanEntry = {
  id: number
  tmdbId: number
  imdbId: number
  title: string
  originalTitle: string
  /** "1999-03-31", "" when unknown. */
  releaseDate: string
  posterPath: string
  status: PlanStatus
  /** Where it was added from: "" by hand, "saga:collection:119", "list:mdblist:user/slug". */
  source: string
  /** Name of the saga or list it was added from. */
  sourceTitle: string
  note: string
  /** The upload of the user that completed it, 0 if none. */
  releaseId: number
  createdAt: string
  doneAt: string | null
  /** When its .torrent files were complete on a client; null if never, or not since they were last sent. */
  downloadedAt: string | null
}

/** A .torrent of a film of the plan on a client, as its last read found it. */
export type PlanDownload = { clientId: number; clientName: string; state: string; progress: number; seenAt: string }

/** A .torrent file loaded on a film of the plan: one, or two for a release merged from two sources. */
export type PlanTorrent = {
  /** 1 or 2. */
  position: number
  /** v1 info hash: the hash of the torrent on the clients. */
  hash: string
  name: string
  size: number
  /** What to take from it ("video", "audio ITA"…), "" if none. */
  label: string
  /** The client it was last sent to, 0 if never. */
  sentClientId: number
  sentClientName: string
  sentAt: string | null
  createdAt: string
  /** Where it is on the clients. */
  downloads: PlanDownload[]
  /** Sent, but the client no longer has it. */
  gone: boolean
}

/** How the .torrent files of a film are sent to a client; the empty options leave the client's defaults. */
export type PlanSendInput = {
  /** 0 or none for the default client of the plan. */
  clientId?: number
  /** The positions of the torrents to send, none for all. */
  positions?: number[]
  category?: string
  /** Comma separated. */
  tags?: string
  savePath?: string
}

export type PlanSendResult = {
  client: string
  results: { position: number; name: string; ok: boolean; already: boolean; error: string }[]
}

/** An upload of the user of a film of the plan, still on the tracker. */
export type PlanUpload = { id: number; name: string; url: string; quality: string }

export type PlanItem = PlanEntry & {
  /** Out on the day: the films not out yet are not searched. */
  released: boolean
  /** The best release found with Prowlarr, null when not searched yet. */
  best: BestResolution | null
  /** Being searched on Prowlarr now. */
  searching: boolean
  uploads: PlanUpload[]
  /** Torrents of others on the tracker, from the last check of sagas and lists. */
  others: GapRef[]
  /** The film on the tracker, "" without tracker. */
  trackerUrl: string
  /** The .torrent files loaded on the film, by position. */
  torrents: PlanTorrent[]
}

export type Plan = {
  /** Films can be added (a TMDB key is set). */
  enabled: boolean
  disabledReason: string
  /** The films are searched on Prowlarr. */
  searchEnabled: boolean
  searchDisabledReason: string
  best: BestState | null
  entries: PlanItem[]
}

export type PlanLookup = { movie: PlanEntry; inPlan: number }
export type PlanBulkResult = { added: PlanEntry[]; skipped: number; errors: string[] }
export type PlanSource = { source: string; sourceTitle: string }

export type GapState = "mine" | "available" | "banned" | "other_quality" | "absent"

/** Quality of a release: type, resolution, HDR formats and audio languages. */
export type Quality = {
  key: string
  /** "2160p WEB-DL DV HDR ITA ENG" */
  label: string
  type: string
  resolution: string
  hdr: string[]
  langs: string[]
}

/** A torrent on the tracker, with its release group ("" if the name has none) and whether the user banned it. */
export type GapRef = { id: number; name: string; url: string; quality: string; mine: boolean; group: string; banned: boolean }

/** A torrent that holds a part, with its quality. */
export type GapVersion = GapRef & { q: Quality }

/** The user's choice on a cell or a part. */
export type GapMarkState = "" | "accepted" | "ignored" | "complete"

export type GapCell = {
  /** Number of the season, or of the film in the saga (GapUnit.number). */
  unit: number
  state: GapState
  /** In this quality for mine/available, in the other qualities for other_quality. */
  torrents: GapRef[]
  /** Labels of the qualities found, for other_quality. */
  qualities: string[]
  mark: GapMarkState
  /** The torrent of others accepted, for mark "accepted". */
  accepted: number
  /** The row the choice was made for, "" if for every row. */
  markQuality: string
}

export type GapRow = {
  quality: Quality
  cells: GapCell[]
  missing: number
  available: number
  otherQuality: number
  absent: number
  banned: number
  /** Hidden by the user: it does not count. */
  hidden: boolean
}

/** How a part stands against the minimum quality, whatever the rows (banned: only by banned groups). */
export type GapPartState = "mine" | "available" | "banned" | "below" | "absent"

/** A season, or a film, in the view by part. */
export type GapPart = {
  unit: number
  state: GapPartState
  /**
   * The user's best version reaching the minimum, else the best of others that does (of banned groups last), else
   * the best of all.
   */
  best: GapVersion | null
  mark: GapMarkState
  accepted: number
}

/** The lowest quality a part counts as present in; "" for any. */
export type GapThreshold = { type: string; resolution: string; lang: string }

/** Options of the view by part, for one kind of check. */
export type GapPrefs = GapThreshold & {
  /** What others uploaded counts as present. */
  othersCount: boolean
}

/**
 * A season of a series, or a film of a saga (numbered in order of release) or of a collection (in the order of the
 * list), with its TMDB id.
 */
export type GapUnit = {
  number: number
  name: string
  airDate: string
  episodes: number
  tmdbId?: number
  /** Stays the same across checks: TMDB id of a film, number of a season. */
  id: number
  /** Every torrent that holds it, best first (empty for results of an older check). */
  versions: GapVersion[]
}

export type GapKind = "series" | "saga" | "list"

/**
 * A series the user uploaded at least a season pack of, with its seasons in each of their qualities,
 * a saga the user uploaded at least a film of, with its films, or a collection the user follows
 * (a list of films on MDBList), with its films even if the user has none of them.
 */
export type GapGroup = {
  kind: GapKind
  key: string
  title: string
  year: string
  /** Of the series, or of the saga (TMDB collection); 0 for collections. */
  tmdbId: number
  tvdbId: number
  /** unverifiable: no TMDB id, or unknown to TMDB. */
  status: "ok" | "unverifiable"
  note: string
  /** The series or the saga on the tracker; "" for collections. */
  searchUrl: string
  /** The list on MDBList, for collections. */
  sourceUrl?: string
  units: GapUnit[]
  rows: GapRow[]
  /** The user's uploads of the series or the saga. */
  mine: GapRef[]
  missing: number
  available: number
  otherQuality: number
  absent: number
  banned: number
  checkedAt: string
  hidden: boolean
  /** Marked complete (and no part came out since). */
  completed: boolean
  /** The minimum quality used, and whether it is this group's own. */
  threshold: GapThreshold
  customThreshold: boolean
  /** The view by part, null for results of an older check (stale). */
  parts: GapPart[] | null
  partTotals: { missing: number; available: number; banned: number; below: number; absent: number }
  stale: boolean
}

/** A choice of the user on a series, a saga or a collection. */
export type GapMarkInput = {
  group: string
  action: "hide" | "complete" | "accept" | "ignore" | "threshold"
  /** Key of the quality row; "" for the whole group, or every row. */
  quality?: string
  /** GapUnit.id */
  unit?: number
  torrent?: number
  threshold?: GapThreshold
}

export type MissingCheckState = {
  running: boolean
  lastRunAt: string | null
  lastResult: string
  lastError: string
  nextRunAt: string | null
  /** What the running check is reading: series, then films, then sagas, then collections. */
  phase: "" | "series" | "movies" | "sagas" | "lists"
  /** Read so far in this phase, out of total. */
  done: number
  total: number
}

type MissingCheck = {
  /** A UNIT3D tracker and a TMDB key are configured. */
  enabled: boolean
  /** Why it is off, when it is. */
  disabledReason: string
  state: MissingCheckState | null
  /** End of the last complete check. */
  checkedAt: string | null
  /** Time of the nightly check, "04:00". */
  checkAt: string
  prefs: GapPrefs
  /** Release groups the user does not want, for every kind of check. */
  bannedGroups: string[]
  /** Types from best to worst, and resolutions, for the minimum quality. */
  types: string[]
  resolutions: string[]
}

export type MissingSeries = MissingCheck & { series: GapGroup[] }
export type MissingSagas = MissingCheck & { sagas: GapGroup[] }

/** A collection the user follows: a list of films on MDBList. */
export type GapListSource = {
  id: number
  source: "mdblist"
  /** "user/slug" */
  path: string
  title: string
  /** The list on MDBList. */
  url: string
  createdAt: string
}

export type MissingLists = MissingCheck & {
  lists: GapGroup[]
  /** Every collection followed, checked or not yet. */
  sources: GapListSource[]
  /** An MDBList key is configured (and the check is on). */
  listsEnabled: boolean
  listsDisabledReason: string
}

export type Info = {
  version: string
  releaseTag: string
  syncInterval: string
  s3ScanInterval: string
  webhookTokenRequired: boolean
  /** A UNIT3D tracker is configured to find missing info hashes. */
  unit3d: boolean
  /** How often the uploads on the tracker are read. */
  releasesInterval: string
}

/** Options changed from the web UI. */
export type Settings = {
  /** Address of bookrr from other devices, used in the disk QR codes; empty = the browser's address. */
  publicUrl: string
  /** An upload is archivable with at least this many other seeders… */
  archiveMinSeeders: number
  /** …and completed downloads… */
  archiveMinCompleted: number
  /** …for this many days in a row (0 = at once). */
  archiveDays: number
  /** The client the .torrent files of the plan are sent to without asking; 0 asks every time. */
  planClientId: number
}

/** Group of the options of the configuration, as shown in the settings. */
export type ConfigGroup = "general" | "unit3d" | "missing" | "prowlarr"

/** An option of the configuration that can be set from the web UI, unless its environment variable is set. */
export type ConfigOption = {
  /** The environment variable. */
  key: string
  group: ConfigGroup
  label: string
  default: string
  help: string
  /** Never sent back: only set says whether it has a value. */
  secret: boolean
  /** As typed in the UI, or the environment's when locked; always "" for secrets. */
  value: string
  set: boolean
  /** Set in the environment: shown, not changeable. */
  locked: boolean
}

/** A value for each option changed: null keeps it (a secret not retyped), "" goes back to the default. */
export type ConfigValues = Record<string, string | null>

/** Services whose connection can be tried from the settings. */
export type ConfigService = "unit3d" | "tmdb" | "mdblist" | "prowlarr"

export type SyncState = {
  running: boolean
  lastRunAt: string | null
  lastResult: string
  nextRunAt: string | null
}

export type ArchiveInput = { diskId: number | null; path: string; notes: string }
export type AdoptionInput = { adopterId: number; notes: string }

export type TorrentInput = {
  hash?: string
  name: string
  size?: number
  tags?: string[]
  category?: string
  personalRelease?: boolean
  notes?: string
  archive?: ArchiveInput
  adoption?: AdoptionInput
}

export type TorrentPatch = Partial<
  Pick<Torrent, "name" | "size" | "tags" | "category" | "personalRelease" | "notes">
>

export type ClientInput = {
  id?: number
  name: string
  url: string
  username: string
  password?: string
  skipTlsVerify: boolean
  enabled: boolean
}

export type ResolveInput = {
  resolution: string
  archive?: ArchiveInput
  adoption?: AdoptionInput
}

export type ImportRow = { row: number; hash: string; name: string; error?: string }
export type ImportResult = { rows: ImportRow[]; imported: number }

/** A log file in the data folder: one a day, more when a day passes the maximum size. */
export type LogFile = {
  name: string
  /** "2026-11-06", local time. */
  day: string
  /** 1 for the first file of the day, 2 for "bookrr-2026-11-06.2.log"… */
  part: number
  size: number
  modifiedAt: string
}

export type LogFiles = {
  /** False when the log files cannot be written: the messages only go to stderr. */
  enabled: boolean
  files: LogFile[]
  today: string
  dir: string
}

export type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR"

/** A line of a log file. */
export type LogEntry = {
  /** Position of the line in the day: unique within it. */
  id: number
  time: string
  /** As slog writes it: "INFO", also "WARN+2". */
  level: string
  msg: string
  attrs: Record<string, unknown>
}

export type LogQuery = { day: string; level?: string; q?: string; after?: number }

export type LogResult = {
  /** Oldest first. */
  entries: LogEntry[]
  /** Position past the last line read: `after` of the next read, for the new lines only. */
  next: number
  /** More lines matched than are returned: only the last ones are. */
  truncated: boolean
}

export class ApiError extends Error {
  status: number
  data: unknown
  constructor(status: number, message: string, data?: unknown) {
    super(message)
    this.status = status
    this.data = data
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const text = typeof body === "string"
  // A form (files) sets its own content type.
  const form = body instanceof FormData
  const res = await fetch(path, {
    method,
    headers: body !== undefined && !form ? { "Content-Type": text ? "text/plain" : "application/json" } : undefined,
    body: body !== undefined ? (text || form ? (body as string | FormData) : JSON.stringify(body)) : undefined,
  })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    let data: unknown
    try {
      data = await res.json()
      if (data && typeof data === "object" && "error" in data && data.error) msg = String(data.error)
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg, data)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

const enc = encodeURIComponent

const gapPath: Record<GapKind, string> = { series: "series", saga: "sagas", list: "lists" }

export const api = {
  info: () => request<Info>("GET", "/api/info"),

  torrents: () => request<Torrent[]>("GET", "/api/torrents"),
  createTorrent: (t: TorrentInput) => request<Torrent>("POST", "/api/torrents", t),
  /** Resolves with the per-row check also when some rows are rejected (HTTP 422). */
  importTorrents: (items: TorrentInput[], dryRun: boolean) =>
    request<ImportResult>("POST", "/api/torrents/import", { items, dryRun }).catch((err) => {
      if (err instanceof ApiError && err.status === 422) return err.data as ImportResult
      throw err
    }),
  updateTorrent: (hash: string, p: TorrentPatch) => request<Torrent>("PATCH", `/api/torrents/${enc(hash)}`, p),
  deleteTorrent: (hash: string) => request<void>("DELETE", `/api/torrents/${enc(hash)}`),
  /** Marks uploads the user keeps on the clients, never archivable; never false undoes it. */
  neverArchive: (hashes: string[], never: boolean) =>
    request<{ updated: number }>("POST", "/api/torrents/never-archive", { hashes, never }),
  /** Looks up on the UNIT3D tracker the hash of a torrent added without one; returns it under the new hash. */
  lookupHash: (hash: string) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/lookup-hash`),
  trackerCandidates: (hash: string, q = "") =>
    request<TrackerCandidate[]>("GET", `/api/torrents/${enc(hash)}/tracker-candidates?q=${enc(q)}`),
  /** Gives the torrent the hash of the tracker torrent picked by the user; returns it under the new hash. */
  trackerMatch: (hash: string, id: string) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/tracker-match`, { id }),
  torrentFileUrl: (hash: string) => `/api/torrents/${enc(hash)}/torrent-file`,

  logFiles: () => request<LogFiles>("GET", "/api/logs/files"),
  logs: (q: LogQuery) => {
    const p = new URLSearchParams({ day: q.day })
    if (q.level) p.set("level", q.level)
    if (q.q?.trim()) p.set("q", q.q.trim())
    if (q.after) p.set("after", String(q.after))
    return request<LogResult>("GET", `/api/logs?${p}`)
  },
  logFileUrl: (name: string) => `/api/logs/files/${enc(name)}`,
  removeFromClient: (hash: string, clientId: number, deleteFiles: boolean) =>
    request<Torrent>("DELETE", `/api/torrents/${enc(hash)}/locations/${clientId}?deleteFiles=${deleteFiles ? 1 : 0}`),
  addArchive: (hash: string, a: ArchiveInput) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/archives`, a),
  updateArchive: (id: number, a: ArchiveInput) => request<void>("PUT", `/api/archives/${id}`, a),
  deleteArchive: (id: number) => request<void>("DELETE", `/api/archives/${id}`),
  addAdoption: (hash: string, a: AdoptionInput) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/adoptions`, a),
  deleteAdoption: (id: number) => request<void>("DELETE", `/api/adoptions/${id}`),

  plan: () => request<Plan>("GET", "/api/plan"),
  /** Reads a film from TMDB, by id, link or IMDb id, before adding it. */
  lookupPlan: (q: string) => request<PlanLookup>("GET", `/api/plan/lookup?q=${enc(q)}`),
  addPlan: (tmdb: string, note = "", from?: PlanSource) => request<PlanEntry>("POST", "/api/plan", { tmdb, note, ...from }),
  addPlanBulk: (tmdbIds: number[], from: PlanSource) =>
    request<PlanBulkResult>("POST", "/api/plan/bulk", { tmdbIds, ...from }),
  updatePlan: (id: number, p: { status?: PlanStatus; note?: string }) => request<PlanEntry>("PATCH", `/api/plan/${id}`, p),
  deletePlan: (id: number) => request<void>("DELETE", `/api/plan/${id}`),
  /** Searches the film on Prowlarr now, in the background. */
  searchPlan: (id: number) => request<{ searching: boolean }>("POST", `/api/plan/${id}/search`),
  /** Loads a .torrent file on a film of the plan: at the first free position, or replacing the one at position. */
  uploadPlanTorrent: (id: number, file: File, label = "", position?: number) => {
    const form = new FormData()
    form.append("file", file)
    if (label.trim()) form.append("label", label.trim())
    return position
      ? request<PlanTorrent>("PUT", `/api/plan/${id}/torrents/${position}`, form)
      : request<PlanTorrent>("POST", `/api/plan/${id}/torrents`, form)
  },
  updatePlanTorrent: (id: number, position: number, label: string) =>
    request<PlanTorrent>("PATCH", `/api/plan/${id}/torrents/${position}`, { label }),
  /** Removes a .torrent from the film (not from the clients). */
  deletePlanTorrent: (id: number, position: number) => request<void>("DELETE", `/api/plan/${id}/torrents/${position}`),
  planTorrentFileUrl: (id: number, position: number) => `/api/plan/${id}/torrents/${position}/file`,
  /** Sends the .torrent files of a film to a client, to download them. */
  sendPlanTorrents: (id: number, p: PlanSendInput) => request<PlanSendResult>("POST", `/api/plan/${id}/torrents/send`, p),

  releases: () => request<Releases>("GET", "/api/releases"),
  /** Starts reading the uploads on the tracker now, in the background. */
  syncReleases: () => request<ReleasesState>("POST", "/api/releases/sync"),
  /** Adds the given uploads to bookrr as personal releases; with all, every unknown one. */
  addReleases: (ids: number[] | "all") =>
    request<{ added: number }>("POST", "/api/releases/add", ids === "all" ? { all: true } : { ids }),
  /**
   * Takes torrents out of the personal releases, removing the tag on the clients too; with all, every one not among
   * the uploads. errors lists the clients where the tag could not be removed.
   */
  unflagReleases: (hashes: string[] | "all") =>
    request<{ unflagged: number; errors: string[] }>(
      "POST",
      "/api/releases/unflag",
      hashes === "all" ? { all: true } : { hashes }
    ),

  missingSeries: () => request<MissingSeries>("GET", "/api/missing/series"),
  missingSagas: () => request<MissingSagas>("GET", "/api/missing/sagas"),
  missingLists: () => request<MissingLists>("GET", "/api/missing/lists"),
  /** Follows a list of films on MDBList (its address) and checks it now. */
  addGapList: (url: string) => request<GapListSource>("POST", "/api/missing/lists/sources", { url }),
  removeGapList: (id: number) => request<void>("DELETE", `/api/missing/lists/sources/${id}`),
  /** Starts the check of the missing seasons and films now, in the background. */
  checkMissing: () => request<MissingCheckState>("POST", "/api/missing/check"),
  saveBannedGroups: (groups: string[]) =>
    request<{ groups: string[] }>("PUT", "/api/missing/banned-groups", { groups }),
  saveMissingPrefs: (kind: GapKind, p: GapPrefs) => request<GapPrefs>("PUT", `/api/missing/${gapPath[kind]}/prefs`, p),
  setGapMark: (kind: GapKind, m: GapMarkInput) => request<unknown>("POST", `/api/missing/${gapPath[kind]}/marks`, m),
  removeGapMark: (kind: GapKind, m: GapMarkInput) => request<void>("DELETE", `/api/missing/${gapPath[kind]}/marks`, m),

  alerts: (open = true) => request<Alert[]>("GET", `/api/alerts?open=${open ? 1 : 0}`),
  resolveAlert: (id: number, r: ResolveInput) => request<void>("POST", `/api/alerts/${id}/resolve`, r),
  /** Records the same destination for several alerts at once: all or none are resolved. */
  resolveAlerts: (ids: number[], r: ResolveInput) => request<void>("POST", "/api/alerts/resolve", { ids, ...r }),

  disks: () => request<Disk[]>("GET", "/api/disks"),
  createDisk: (d: DiskInput) => request<Disk>("POST", "/api/disks", d),
  updateDisk: (id: number, d: DiskInput) => request<Disk>("PUT", `/api/disks/${id}`, d),
  deleteDisk: (id: number) => request<void>("DELETE", `/api/disks/${id}`),
  /** Reads the first page of the bucket; with id, the stored secret key is used if none is given. */
  testS3: (b: S3Input & { id?: number }) =>
    request<{ ok: boolean; error?: string; objects?: number; more?: boolean }>("POST", "/api/disks/test-s3", b),
  scanDisk: (id: number) => request<ScanResult>("POST", `/api/disks/${id}/scan`),
  bucketEntries: (id: number) => request<BucketEntry[]>("GET", `/api/disks/${id}/bucket-entries`),
  importSmart: (report: string, t: SmartTarget = {}) => {
    const q = new URLSearchParams()
    if (t.id) q.set("id", String(t.id))
    if (t.newLabel !== undefined) {
      q.set("new", "1")
      q.set("label", t.newLabel)
    }
    if (t.dryRun) q.set("dryRun", "1")
    return request<SmartResult>("POST", `/api/disks/smart?${q}`, report)
  },

  adopters: () => request<Adopter[]>("GET", "/api/adopters"),
  createAdopter: (a: Partial<Adopter>) => request<Adopter>("POST", "/api/adopters", a),
  updateAdopter: (id: number, a: Partial<Adopter>) => request<Adopter>("PUT", `/api/adopters/${id}`, a),
  deleteAdopter: (id: number) => request<void>("DELETE", `/api/adopters/${id}`),

  clients: () => request<Client[]>("GET", "/api/clients"),
  createClient: (c: ClientInput) => request<Client>("POST", "/api/clients", c),
  updateClient: (id: number, c: ClientInput) => request<Client>("PUT", `/api/clients/${id}`, c),
  deleteClient: (id: number) => request<void>("DELETE", `/api/clients/${id}`),
  testClient: (c: ClientInput) =>
    request<{ ok: boolean; error?: string; version?: string; torrents?: number }>("POST", "/api/clients/test", c),

  config: () => request<{ options: ConfigOption[] }>("GET", "/api/config"),
  /** Saves the options and starts again, without restarting bookrr, the services whose options changed. */
  saveConfig: (values: ConfigValues) => request<{ options: ConfigOption[] }>("PUT", "/api/config", { values }),
  testConfig: (service: ConfigService, values: ConfigValues) =>
    request<{ ok: boolean; message?: string; error?: string }>("POST", "/api/config/test", { service, values }),

  settings: () => request<Settings>("GET", "/api/settings"),
  /** Options left out keep their value. */
  saveSettings: (st: Partial<Settings>) => request<Settings>("PUT", "/api/settings", st),

  syncState: () => request<SyncState>("GET", "/api/sync"),
  syncNow: () => request<{ result: string }>("POST", "/api/sync"),
}
