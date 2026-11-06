# API

[Documentation](README.md) · [Italiano](../it/api.md)

Everything the interface does is available as a JSON REST API under `/api`, behind the same login as the interface (`BOOKRR_AUTH_USER` / `BOOKRR_AUTH_PASSWORD`, HTTP Basic). Errors answer `{"error": "…"}` with a 4xx or 5xx status. The routes are in [`internal/api/server.go`](../../internal/api/server.go).

| Area | Routes |
|---|---|
| Info and sync | `GET /api/info`, `GET /api/sync`, `POST /api/sync` (read the clients now) |
| Torrents | `GET`/`POST /api/torrents`, `POST /api/torrents/import` (`{"items": […], "dryRun": true}`), `PATCH`/`DELETE /api/torrents/{hash}`, `POST /api/torrents/{hash}/archives`, `POST /api/torrents/{hash}/adoptions`, `DELETE /api/torrents/{hash}/locations/{clientId}?deleteFiles=1`, `POST /api/torrents/never-archive` |
| Tracker | `POST /api/torrents/{hash}/lookup-hash`, `GET /api/torrents/{hash}/tracker-candidates?q=…`, `POST /api/torrents/{hash}/tracker-match`, `GET /api/torrents/{hash}/torrent-file` |
| Releases | `GET /api/releases`, `POST /api/releases/sync`, `POST /api/releases/add` (`{"ids": [123]}` or `{"all": true}`), `POST /api/releases/unflag` (`{"hashes": ["…"]}` or `{"all": true}`) |
| Alerts | `GET /api/alerts?open=1`, `POST /api/alerts/{id}/resolve`, `POST /api/alerts/resolve` (several at once) |
| Disks | `GET`/`POST /api/disks`, `PUT`/`DELETE /api/disks/{id}`, `POST /api/disks/smart`, `POST /api/disks/test-s3`, `POST /api/disks/{id}/scan`, `GET /api/disks/{id}/bucket-entries` |
| Archives and adoptions | `PUT`/`DELETE /api/archives/{id}`, `DELETE /api/adoptions/{id}` |
| Adopters | `GET`/`POST /api/adopters`, `PUT`/`DELETE /api/adopters/{id}` |
| Clients | `GET`/`POST /api/clients`, `PUT`/`DELETE /api/clients/{id}`, `POST /api/clients/test` |
| Missing | see [Missing seasons and films](missing.md#when-it-runs) |
| Plan | see [Release plan](prowlarr-and-plan.md#release-plan) |
| Logs | see [Logs](logs.md) |
| Settings | `GET`/`PUT /api/settings` (archivable thresholds, `archiveMinSeeders`, `archiveMinCompleted`, `archiveDays`, and the address used by the QR codes, `publicUrl`) |
| Configuration | `GET /api/config` (the options, without the keys), `PUT /api/config` with `{"values": {"BOOKRR_UNIT3D_URL": "https://…", "BOOKRR_UNIT3D_API_KEY": null}}` (`null` keeps the value, `""` goes back to the default), `POST /api/config/test` with `{"service": "unit3d" \| "tmdb" \| "mdblist" \| "prowlarr", "values": {…}}` |
| Webhook | `POST /api/webhook/qbit`, see [Clients and alerts](clients-and-alerts.md#removal-webhook) |

Every torrent and every release has the `archivable` field (`null` when not on a client), with `neverArchive` for those never to archive.
