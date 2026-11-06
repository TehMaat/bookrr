import { useState } from "react"
import { Plus } from "lucide-react"

import { DiskFormDialog } from "@/components/disk-form-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { ArchiveInput } from "@/lib/api"
import { useDisks } from "@/lib/queries"

export const emptyArchive: ArchiveInput = { diskId: null, path: "", notes: "" }

export function ArchiveFields({ value, onChange }: { value: ArchiveInput; onChange: (v: ArchiveInput) => void }) {
  const disks = useDisks()
  const [newDisk, setNewDisk] = useState(false)

  return (
    <div className="grid gap-3">
      <div className="grid gap-2">
        <Label>Disk / archive</Label>
        <div className="flex gap-2">
          <Select
            value={value.diskId ? String(value.diskId) : "none"}
            onValueChange={(v) => onChange({ ...value, diskId: v === "none" ? null : Number(v) })}
          >
            <SelectTrigger className="w-full min-w-0">
              <SelectValue placeholder="Choose a disk" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">No disk (path only)</SelectItem>
              {disks.data?.map((d) => (
                <SelectItem key={d.id} value={String(d.id)}>
                  {d.label}
                  {d.serial && <span className="text-muted-foreground font-mono text-xs">SN {d.serial}</span>}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type="button" variant="outline" size="icon" onClick={() => setNewDisk(true)} aria-label="New disk">
            <Plus />
          </Button>
        </div>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="archive-path">Path / folder</Label>
        <Input
          id="archive-path"
          value={value.path}
          onChange={(e) => onChange({ ...value, path: e.target.value })}
          placeholder="/Release/2024/Release.Name"
          className="font-mono text-sm"
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="archive-notes">Archive notes</Label>
        <Input id="archive-notes" value={value.notes} onChange={(e) => onChange({ ...value, notes: e.target.value })} />
      </div>
      <DiskFormDialog open={newDisk} onOpenChange={setNewDisk} onSaved={(d) => onChange({ ...value, diskId: d.id })} />
    </div>
  )
}
