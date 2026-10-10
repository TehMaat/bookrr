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
  source: "sync" | "webhook"
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
}

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

export type Info = {
  version: string
  releaseTag: string
  syncInterval: string
  webhookTokenRequired: boolean
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

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
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
    try {
      const data = await res.json()
      if (data?.error) msg = data.error
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

const enc = encodeURIComponent

export const api = {
  info: () => request<Info>("GET", "/api/info"),

  torrents: () => request<Torrent[]>("GET", "/api/torrents"),
  createTorrent: (t: TorrentInput) => request<Torrent>("POST", "/api/torrents", t),
  updateTorrent: (hash: string, p: TorrentPatch) => request<Torrent>("PATCH", `/api/torrents/${enc(hash)}`, p),
  deleteTorrent: (hash: string) => request<void>("DELETE", `/api/torrents/${enc(hash)}`),
  removeFromClient: (hash: string, clientId: number, deleteFiles: boolean) =>
    request<Torrent>("DELETE", `/api/torrents/${enc(hash)}/locations/${clientId}?deleteFiles=${deleteFiles ? 1 : 0}`),
  addArchive: (hash: string, a: ArchiveInput) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/archives`, a),
  updateArchive: (id: number, a: ArchiveInput) => request<void>("PUT", `/api/archives/${id}`, a),
  deleteArchive: (id: number) => request<void>("DELETE", `/api/archives/${id}`),
  addAdoption: (hash: string, a: AdoptionInput) => request<Torrent>("POST", `/api/torrents/${enc(hash)}/adoptions`, a),
  deleteAdoption: (id: number) => request<void>("DELETE", `/api/adoptions/${id}`),

  alerts: (open = true) => request<Alert[]>("GET", `/api/alerts?open=${open ? 1 : 0}`),
  resolveAlert: (id: number, r: ResolveInput) => request<void>("POST", `/api/alerts/${id}/resolve`, r),

  disks: () => request<Disk[]>("GET", "/api/disks"),
  createDisk: (d: Partial<Disk>) => request<Disk>("POST", "/api/disks", d),
  updateDisk: (id: number, d: Partial<Disk>) => request<Disk>("PUT", `/api/disks/${id}`, d),
  deleteDisk: (id: number) => request<void>("DELETE", `/api/disks/${id}`),
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

  syncState: () => request<SyncState>("GET", "/api/sync"),
  syncNow: () => request<{ result: string }>("POST", "/api/sync"),
}
