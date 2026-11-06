import { useEffect, useMemo, useState } from "react"
import { useMutation, useQuery } from "@tanstack/react-query"
import { CircleCheck, CircleX, Download, Upload } from "lucide-react"
import { toast } from "sonner"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { api, type Adopter, type Disk, type ImportResult, type TorrentInput } from "@/lib/api"
import { parseCsv, toCsv } from "@/lib/csv"
import { formatBytes, parseBytes, plural } from "@/lib/format"
import { buildTorrentInput, isValidHash, type ManualFields } from "@/lib/manual"
import { useAdopters, useDisks, useInfo, useRefreshAll } from "@/lib/queries"

/**
 * Accepted header names for each field, compared without case, accents, spaces and punctuation. The Italian ones are
 * from before the interface was in English, so that older files still import.
 */
const columns = {
  name: ["name", "title", "release", "nome", "titolo"],
  hash: ["hash", "infohash", "infohashv1", "infohashv2"],
  size: ["size", "dimensione", "peso"],
  tags: ["tag", "tags"],
  personal: ["personalrelease", "personal", "pr"],
  notes: ["notes", "note"],
  disk: ["disk", "serial", "sn", "disco", "seriale"],
  path: ["path", "folder", "percorso", "cartella"],
  archiveNotes: ["archivenotes", "disknotes", "notearchivio", "notedisco"],
  adopter: ["adoptedby", "adopter", "adottatoda", "adottatore"],
} as const

type Column = keyof typeof columns

const template = [
  ["Name", "Hash", "Size", "Tags", "Personal Release", "Notes", "Disk", "Path", "Archive notes", "Adopted by"],
  ["My.Release.2024", "", "12.5 GB", "Personal Release", "yes", "", "Archive 1", "/Release/2024", "", ""],
]

const normalizeHeader = (h: string) =>
  h
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]/g, "")

const truthy = new Set(["si", "sì", "s", "yes", "y", "true", "vero", "1", "x"])
const falsy = new Set(["no", "n", "false", "falso", "0"])

type ParsedRow = {
  line: number
  fields: ManualFields
  diskLabel: string
  errors: string[]
}

type Parsed = { rows: ParsedRow[]; error?: string }

function findDisk(disks: Disk[], v: string): Disk[] {
  const k = v.toLowerCase()
  return disks.filter((d) => d.label.toLowerCase() === k || (d.serial !== "" && d.serial.toLowerCase() === k))
}

function parse(text: string, disks: Disk[], adopters: Adopter[], defaultDisk: number | null, defaultPersonal: boolean): Parsed {
  const table = parseCsv(text)
  if (table.length === 0) return { rows: [] }
  const idx: Partial<Record<Column, number>> = {}
  table[0].forEach((h, i) => {
    const n = normalizeHeader(h)
    for (const [col, names] of Object.entries(columns) as [Column, readonly string[]][]) {
      if (idx[col] === undefined && names.includes(n)) idx[col] = i
    }
  })
  if (idx.name === undefined) {
    return { rows: [], error: 'The first line must hold the column headers, at least "Name".' }
  }

  const rows = table.slice(1).map((cells, i): ParsedRow => {
    const get = (c: Column) => (idx[c] === undefined ? "" : (cells[idx[c]!] ?? "").trim())
    const errors: string[] = []

    let diskId = defaultDisk
    let diskLabel = defaultDisk ? (disks.find((d) => d.id === defaultDisk)?.label ?? "") : ""
    const diskValue = get("disk")
    if (diskValue) {
      const found = findDisk(disks, diskValue)
      if (found.length === 1) {
        diskId = found[0].id
        diskLabel = found[0].label
      } else {
        diskId = null
        diskLabel = diskValue
        errors.push(
          found.length === 0
            ? `disk "${diskValue}" not found: create it first in the Disks page`
            : `"${diskValue}" matches several disks: use the serial number`
        )
      }
    }

    let adopterId: number | null = null
    const adopterValue = get("adopter")
    if (adopterValue) {
      const a = adopters.find((a) => a.name.toLowerCase() === adopterValue.toLowerCase())
      if (a) adopterId = a.id
      else errors.push(`adopter "${adopterValue}" not found: create it first in the Adopters page`)
    }

    let personal = defaultPersonal
    const personalValue = get("personal").toLowerCase()
    if (truthy.has(personalValue)) personal = true
    else if (falsy.has(personalValue)) personal = false
    else if (personalValue) errors.push(`Personal Release "${get("personal")}" not recognized (use yes/no)`)

    const fields: ManualFields = {
      name: get("name"),
      hash: get("hash"),
      size: get("size"),
      tags: get("tags"),
      personal,
      notes: get("notes"),
      adopterId,
      archive: { diskId, path: get("path"), notes: get("archiveNotes") },
    }
    if (!fields.name) errors.push("name missing")
    if (!isValidHash(fields.hash)) errors.push("invalid info hash: 40 or 64 hex characters needed")
    if (fields.size && parseBytes(fields.size) === 0) errors.push(`size "${fields.size}" not readable`)
    if (!diskId && !fields.archive.path && !diskValue) errors.push("disk or path missing")

    return { line: i + 2, fields, diskLabel, errors }
  })
  return { rows }
}

function downloadTemplate() {
  const blob = new Blob(["﻿" + toCsv(template)], { type: "text/csv;charset=utf-8" })
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = "bookrr-archived.csv"
  a.click()
  URL.revokeObjectURL(url)
}

const PREVIEW_LIMIT = 500

export function ImportDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const disks = useDisks()
  const adopters = useAdopters()
  const refresh = useRefreshAll()
  const info = useInfo()
  const [text, setText] = useState("")
  const [defaultDisk, setDefaultDisk] = useState<number | null>(null)
  const [defaultPersonal, setDefaultPersonal] = useState(true)
  const [rejected, setRejected] = useState<ImportResult | null>(null)

  useEffect(() => {
    if (!open) return
    setText("")
    setDefaultDisk(null)
    setDefaultPersonal(true)
    setRejected(null)
  }, [open])

  const parsed = useMemo(
    () => parse(text, disks.data ?? [], adopters.data ?? [], defaultDisk, defaultPersonal),
    [text, disks.data, adopters.data, defaultDisk, defaultPersonal]
  )
  const items: TorrentInput[] = useMemo(() => parsed.rows.map((r) => buildTorrentInput(r.fields)), [parsed])
  // A rejected import only describes the data it was sent.
  useEffect(() => setRejected(null), [items])

  // Rows that pass the checks done here are sent to the server, which
  // repeats the checks it would do on import (hash already known, repeated
  // in the file, …) without writing anything.
  const validRows = useMemo(() => parsed.rows.flatMap((r, i) => (r.errors.length === 0 ? [i] : [])), [parsed])
  const toCheck = useMemo(() => validRows.map((i) => items[i]), [validRows, items])
  const check = useQuery({
    queryKey: ["import-check", toCheck],
    queryFn: () => api.importTorrents(toCheck, true),
    enabled: open && toCheck.length > 0,
    gcTime: 0,
  })
  // A rejected import was sent every row, so its rows line up with the file.
  const server = rejected ?? (check.isFetching ? null : (check.data ?? null))
  const serverRows = rejected ? parsed.rows.map((_, i) => i) : validRows

  const serverErrors = new Map<number, string>()
  server?.rows.forEach((r, j) => r.error && serverErrors.set(serverRows[j], r.error))
  const rowErrors = (i: number) => {
    const local = parsed.rows[i].errors
    if (local.length > 0) return local
    const e = serverErrors.get(i)
    return e ? [e] : []
  }
  const errorCount = parsed.rows.filter((_, i) => rowErrors(i).length > 0).length

  const run = useMutation({
    mutationFn: () => api.importTorrents(items, false),
    onSuccess: (res) => {
      if (res.imported > 0) {
        toast.success(`${plural(res.imported, "torrent")} imported`)
        refresh()
        onOpenChange(false)
      } else {
        setRejected(res)
        toast.error("Import cancelled: fix the flagged rows")
      }
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const loadFile = async (f: File | undefined) => {
    if (!f) return
    setText(await f.text())
  }

  const ready = items.length > 0 && errorCount === 0 && server !== null && !rejected

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Import archived torrents</DialogTitle>
          <DialogDescription>
            Load or paste a CSV file (also exported from Excel): each row is saved exactly as if you entered it with
            "Add". Columns: Name (required), Hash, Size, Tags, Personal Release, Notes, Disk (name or serial number),
            Path, Archive notes, Adopted by.
            {info.data?.unit3d &&
              " If the hash is missing, after the import bookrr looks it up by name on the configured UNIT3D tracker."}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="import-file">CSV file</Label>
            <Input
              id="import-file"
              type="file"
              accept=".csv,.tsv,.txt,text/csv,text/tab-separated-values"
              onChange={(e) => loadFile(e.target.files?.[0])}
            />
          </div>
          <div className="flex items-end">
            <Button type="button" variant="outline" onClick={downloadTemplate}>
              <Download /> Download template
            </Button>
          </div>
          <div className="grid gap-2 sm:col-span-2">
            <Label htmlFor="import-text">Or paste here</Label>
            <Textarea
              id="import-text"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder={toCsv(template)}
              className="max-h-40 font-mono text-xs"
            />
          </div>
          <div className="grid gap-2">
            <Label>Disk for rows without a disk</Label>
            <Select
              value={defaultDisk ? String(defaultDisk) : "none"}
              onValueChange={(v) => setDefaultDisk(v === "none" ? null : Number(v))}
            >
              <SelectTrigger className="w-full min-w-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">None (given in the file)</SelectItem>
                {disks.data?.map((d) => (
                  <SelectItem key={d.id} value={String(d.id)}>
                    {d.label}
                    {d.serial && <span className="text-muted-foreground font-mono text-xs">SN {d.serial}</span>}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2 pt-6">
            <Switch
              id="import-personal"
              checked={defaultPersonal}
              onCheckedChange={setDefaultPersonal}
            />
            <Label htmlFor="import-personal">Personal Release when the column is empty</Label>
          </div>
        </div>

        {parsed.error && <p className="text-destructive text-sm">{parsed.error}</p>}

        {parsed.rows.length > 0 && (
          <div className="max-h-72 overflow-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/40 hover:bg-muted/40">
                  <TableHead>Row</TableHead>
                  <TableHead className="w-full">Name</TableHead>
                  <TableHead className="text-right">Size</TableHead>
                  <TableHead>Archived on</TableHead>
                  <TableHead className="min-w-[260px]">Result</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {parsed.rows.slice(0, PREVIEW_LIMIT).map((r, i) => {
                  const errs = rowErrors(i)
                  return (
                    <TableRow key={r.line} className={errs.length > 0 ? "bg-destructive/5" : undefined}>
                      <TableCell className="text-muted-foreground tabular-nums">{r.line}</TableCell>
                      <TableCell className="max-w-0 min-w-[200px] truncate font-medium" title={r.fields.name}>
                        {r.fields.name || "—"}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{formatBytes(items[i].size ?? 0)}</TableCell>
                      <TableCell className="max-w-[220px] truncate" title={r.fields.archive.path}>
                        {[r.diskLabel, r.fields.archive.path].filter(Boolean).join(" · ") || "—"}
                      </TableCell>
                      <TableCell className="whitespace-normal">
                        {errs.length > 0 ? (
                          <span className="text-destructive flex items-start gap-1 text-xs">
                            <CircleX className="mt-px size-3.5 shrink-0" /> {errs.join("; ")}
                          </span>
                        ) : server ? (
                          <Badge variant="success">
                            <CircleCheck /> OK
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground text-xs">checking…</span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
            {parsed.rows.length > PREVIEW_LIMIT && (
              <p className="text-muted-foreground border-t px-3 py-2 text-xs">
                Preview of the first {PREVIEW_LIMIT} rows of {parsed.rows.length}
                {errorCount > 0 && ` · rows with errors in total: ${errorCount}`}.
              </p>
            )}
          </div>
        )}
        {check.isError && <p className="text-destructive text-sm">Check failed: {check.error.message}</p>}

        <DialogFooter className="items-center">
          {parsed.rows.length > 0 && (
            <span className="text-muted-foreground mr-auto text-sm">
              {plural(parsed.rows.length, "row")}{errorCount > 0 && <span className="text-destructive"> · {errorCount} with errors</span>}
            </span>
          )}
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" disabled={!ready || run.isPending} onClick={() => run.mutate()}>
            <Upload /> Import {items.length > 0 ? items.length : ""}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
