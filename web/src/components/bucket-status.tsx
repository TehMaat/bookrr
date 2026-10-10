import type { Disk } from "@/lib/api"
import { formatBytes, formatRelative } from "@/lib/format"

/** Last read of a cloud archive's bucket. */
export function BucketStatus({ disk }: { disk: Disk }) {
  const b = disk.s3
  if (!b) return null
  return (
    <div className="grid gap-1 text-xs">
      <div className="text-muted-foreground">
        {b.scanAt ? (
          <>
            Bucket letto {formatRelative(b.scanAt)} · {b.objects} file · {formatBytes(b.size)}
            {b.unmatched > 0 && <> · {b.unmatched} non riconosciuti</>}
          </>
        ) : (
          "Bucket non ancora letto"
        )}
      </div>
      {b.scanError && <div className="text-destructive break-all">{b.scanError}</div>}
    </div>
  )
}
