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

export type Info = {
  version: string
  releaseTag: string
  syncInterval: string
  s3ScanInterval: string
  webhookTokenRequired: boolean
  /** A UNIT3D tracker is configured to find missing info hashes. */
  unit3d: boolean
}

/** Options changed from the web UI. */
export type Settings = {
  /** Address of bookrr from other devices, used in the disk QR codes; empty = the browser's address. */
  publicUrl: string
}

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
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": text ? "text/plain" : "application/json" } : undefined,
    body: body !== undefined ? (text ? body : JSON.stringify(body)) : undefined,
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
  /** Looks up on the UNIT3D tracker the hash of a torrent added without one; returns it under the new hash. */
  lookupHash: (hash: string) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/lookup-hash`),
  trackerCandidates: (hash: string, q = "") =>
    request<TrackerCandidate[]>("GET", `/api/torrents/${enc(hash)}/tracker-candidates?q=${enc(q)}`),
  /** Gives the torrent the hash of the tracker torrent picked by the user; returns it under the new hash. */
  trackerMatch: (hash: string, id: string) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/tracker-match`, { id }),
  torrentFileUrl: (hash: string) => `/api/torrents/${enc(hash)}/torrent-file`,
  removeFromClient: (hash: string, clientId: number, deleteFiles: boolean) =>
    request<Torrent>("DELETE", `/api/torrents/${enc(hash)}/locations/${clientId}?deleteFiles=${deleteFiles ? 1 : 0}`),
  addArchive: (hash: string, a: ArchiveInput) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/archives`, a),
  updateArchive: (id: number, a: ArchiveInput) => request<void>("PUT", `/api/archives/${id}`, a),
  deleteArchive: (id: number) => request<void>("DELETE", `/api/archives/${id}`),
  addAdoption: (hash: string, a: AdoptionInput) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/adoptions`, a),
  deleteAdoption: (id: number) => request<void>("DELETE", `/api/adoptions/${id}`),

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

  settings: () => request<Settings>("GET", "/api/settings"),
  saveSettings: (st: Settings) => request<Settings>("PUT", "/api/settings", st),

  syncState: () => request<SyncState>("GET", "/api/sync"),
  syncNow: () => request<{ result: string }>("POST", "/api/sync"),
}
