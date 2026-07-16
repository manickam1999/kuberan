# Plan 017 — Transaction Receipt Attachments (+ off-host backup/DR)

Status: **planned** (design frozen 2026-07-16, not yet implemented)
Design report: `.lavish/transaction-receipts-plan.html` (gitignored, review artifact)

## Goal

Let users attach receipt images (and PDFs) to transactions. Store the bytes in
self-hosted **MinIO** (S3-compatible) behind a swappable `BlobStore` interface;
keep only metadata in Postgres. Serve them through an authenticated, ownership-checked
API proxy that keeps MinIO fully private. Ship the web UI first; Telegram/MCP later.

Bundled with this: **Phase 0** stands up an off-host, encrypted **R2 backup target**
for both the DB dumps and the MinIO bucket — this fixes a pre-existing single-host
backup risk and provides the receipts DR the MinIO decision requires.

## Locked decisions

| # | Decision | Choice |
|---|----------|--------|
| D1 | Storage backend | Self-hosted **MinIO** (S3 API) behind a `BlobStore` strategy interface; one `S3BlobStore` impl runs in dev + prod; in-memory store for unit tests. Swappable to R2/Supabase by changing endpoint. |
| D2 | Data model | Separate one-to-many `transaction_attachments` table. Postgres holds a `storage_key`, never the blob. |
| D3 | File types | JPEG, PNG, WebP, PDF. **HEIC: see Risk R1** — likely client-converted to JPEG or dropped. |
| D4 | Limits | 10 MiB per file, 10 attachments per transaction. |
| D5 | Serving | API-proxied, MinIO private, ownership-checked. Browser fetches bytes with the `Authorization` header then `createObjectURL` — token never in a URL. NOT presigned/signed-query URLs. Hardened response headers. |
| D6 | Upload privacy | Re-encode raster images server-side to strip EXIF (GPS) + defuse decompression bombs. |
| D7 | Backup/DR | Extend `apps/backup` with `rclone` + `crypt` to push DB dumps AND the MinIO bucket off-host to R2, client-side encrypted. Guard deletes with R2 versioning. Add failure alerting. Ships as Phase 0, independent of receipts. |
| D8 | Scope | Web UI first. Telegram bot + MCP upload are follow-ups (out of scope here). |

## Architecture

```
Browser ──JSON── POST /transactions ─────────────► create tx (existing)
   │
   └──multipart── POST /transactions/:id/attachments
                     │  sniff MIME · size cap · ownership · re-encode/strip EXIF
                     ▼
                  AttachmentService ──Put(key,bytes)──► MinIO (private, internal net)
                     │
                     └──INSERT metadata──► Postgres (transaction_attachments)

Browser <img> ──GET /transactions/:id/attachments/:aid (Bearer header)──►
                  AttachmentService ──Get(key)──► MinIO ──stream──► hardened response
```

MinIO is reachable only on the internal Docker network; it is **never** published
through the Cloudflare tunnel. The API is the sole authenticated gateway to bytes.

---

## Phase 0 — Off-host backup / DR  (~0.5 day, ships independently, not blocked by receipts)

Today `apps/backup/backup.sh` runs `pg_dump -Fc` into `/backups`, a **bind mount on
the same host** (`docker-compose.prod.yml:197`). Disk loss = DB + all backups gone.
Fix: push encrypted copies off-host to R2 with `rclone`.

### Files

- `apps/backup/Dockerfile` — add `rclone` and `curl` to the `apk add` line.
- `apps/backup/backup.sh` — after a successful `pg_dump` + prune, add:
  ```sh
  # 2. push the new dump off-host (client-side encrypted via crypt remote)
  rclone copy "${BACKUP_FILE}" r2crypt:db/ --s3-no-check-bucket

  # 3. mirror the MinIO receipts bucket off-host (skip if receipts not deployed yet)
  if [ -n "${RECEIPTS_BUCKET:-}" ]; then
      rclone sync "minio:${RECEIPTS_BUCKET}" r2crypt:receipts/ --s3-no-check-bucket
  fi

  # 5. dead-man's-switch: ping only on full success
  [ -n "${HEALTHCHECK_URL:-}" ] && curl -fsS -m 10 "${HEALTHCHECK_URL}" >/dev/null || true
  ```
- `docker-compose.prod.yml` — add the backup service's rclone env (see below); add
  `minio` to the `depends_on`/network once Phase 4 lands (the receipts mirror is a no-op
  until `RECEIPTS_BUCKET` is set).
- `.env.prod.example` — document the new vars.

### rclone config (via env vars, no config file needed)

```sh
# R2 target
RCLONE_CONFIG_R2_TYPE=s3
RCLONE_CONFIG_R2_PROVIDER=Cloudflare
RCLONE_CONFIG_R2_ACCESS_KEY_ID=<r2 token id>
RCLONE_CONFIG_R2_SECRET_ACCESS_KEY=<r2 token secret>
RCLONE_CONFIG_R2_ENDPOINT=https://<acct>.r2.cloudflarestorage.com
# Client-side encryption wrapper (filenames + contents encrypted; Cloudflare can't read)
RCLONE_CONFIG_R2CRYPT_TYPE=crypt
RCLONE_CONFIG_R2CRYPT_REMOTE=r2:kuberan-backups
RCLONE_CONFIG_R2CRYPT_PASSWORD=<rclone obscure output>
RCLONE_CONFIG_R2CRYPT_PASSWORD2=<rclone obscure output, salt>
# MinIO source (Phase 4)
RCLONE_CONFIG_MINIO_TYPE=s3
RCLONE_CONFIG_MINIO_PROVIDER=Minio
RCLONE_CONFIG_MINIO_ACCESS_KEY_ID=<minio svc key>
RCLONE_CONFIG_MINIO_SECRET_ACCESS_KEY=<minio svc secret>
RCLONE_CONFIG_MINIO_ENDPOINT=http://minio:9000
# Ops
RECEIPTS_BUCKET=kuberan-receipts   # unset until Phase 4
HEALTHCHECK_URL=https://hc-ping.com/<uuid>
```

### Operational requirements

- **Crypt passphrase custody:** the `PASSWORD`/`PASSWORD2` values are produced by
  `rclone obscure` locally and must ALSO be stored in a password manager off the VPS.
  Losing them makes the off-host copy unrecoverable. Document in the runbook.
- **R2 bucket versioning** enabled on `kuberan-backups` so a bad `sync` (or ransomware)
  can't erase history. Optional lifecycle rule to expire object versions > 90 days.
- **Restore runbook** (`plans/017-transaction-receipts/RUNBOOK.md`): decrypt-copy from
  R2, `pg_restore`, verify bucket. Drill it once before relying on it.

### Verification

- Run the backup container once; confirm a `.dump` lands locally AND appears (encrypted)
  under `r2:kuberan-backups/db/`.
- Kill/restore drill against a scratch DB.
- Confirm the healthcheck endpoint flips to "up".

---

## Phase 1 — Storage foundation  (~1 day)

### 1a. `BlobStore` interface + S3 implementation

New package `apps/api/internal/storage`:

- `blobstore.go`
  ```go
  package storage

  type BlobStore interface {
      Put(ctx context.Context, key string, r io.Reader, contentType string, size int64) error
      Get(ctx context.Context, key string) (io.ReadCloser, error)
      Delete(ctx context.Context, key string) error
  }
  ```
- `s3_blobstore.go` — `S3BlobStore` using `aws-sdk-go-v2` (`config`, `service/s3`) with:
  - a custom `BaseEndpoint` (MinIO endpoint) and `UsePathStyle = true`;
  - static credentials from config;
  - `NewS3BlobStore(cfg S3Config) (*S3BlobStore, error)`.
- `mem_blobstore.go` — in-memory `map[string][]byte` for unit tests.

Add deps: `go get github.com/aws-sdk-go-v2/...`. (No aws/minio libs present today.)

### 1b. Config

`apps/api/internal/config/config.go` — extend `Config` + `Load()` (follow the existing
`getEnv`/`os.Getenv` pattern) with a `Storage` sub-struct:

```go
// Storage (receipt attachments). See plans/017.
StorageEndpoint    string // e.g. http://minio:9000 (internal only)
StorageBucket      string // e.g. kuberan-receipts
StorageAccessKey   string
StorageSecretKey   string
StorageUsePathStyle bool  // true for MinIO
MaxUploadBytes     int64  // default 10*1024*1024
MaxAttachmentsPerTx int   // default 10
```

Env keys: `STORAGE_ENDPOINT`, `STORAGE_BUCKET`, `STORAGE_ACCESS_KEY`,
`STORAGE_SECRET_KEY`, `STORAGE_USE_PATH_STYLE` (default `true`),
`MAX_UPLOAD_BYTES` (default `10485760`), `MAX_ATTACHMENTS_PER_TX` (default `10`).

In `validateProduction()`: if `StorageBucket != ""`, require `StorageSecretKey != ""`.

### 1c. Migration

`apps/api/migrations/000025_create_transaction_attachments.{up,down}.sql`:

```sql
-- up
CREATE TABLE IF NOT EXISTS transaction_attachments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    user_id         UUID NOT NULL REFERENCES users(id),
    transaction_id  UUID NOT NULL REFERENCES transactions(id),
    storage_key     VARCHAR(512) NOT NULL,
    file_name       VARCHAR(255) NOT NULL DEFAULT '',
    content_type    VARCHAR(128) NOT NULL,
    byte_size       BIGINT NOT NULL,
    checksum_sha256 CHAR(64) NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tx_attachments_transaction_id ON transaction_attachments (transaction_id);
CREATE INDEX IF NOT EXISTS idx_tx_attachments_user_id       ON transaction_attachments (user_id);
CREATE INDEX IF NOT EXISTS idx_tx_attachments_deleted_at    ON transaction_attachments (deleted_at);

-- down
DROP TABLE IF EXISTS transaction_attachments;
```

### 1d. Model

`apps/api/internal/models/transaction_attachment.go` — embeds `Base` (matches soft-delete
+ UUID conventions), mirrors the column set. Add to `Transaction` a preloadable relation
`Attachments []TransactionAttachment` (preloaded only on detail queries, not list).

### 1e. Dev infra

`docker-compose.yml` — add a `minio` service (internal network, named volume, console
optional), plus a one-shot `mc` init to create the `kuberan-receipts` bucket + a scoped
service account. Wire the new `STORAGE_*` env into the `api` service.

### Verification (Phase 1)

`./scripts/check-go.sh apps/api` green; unit tests for `memBlobStore` + an integration
test for `S3BlobStore` against the dev MinIO (`Put`/`Get`/`Delete` round-trip).

---

## Phase 2 — Backend service, handlers, routes  (~1 day)

### 2a. Image processing helper

`apps/api/internal/storage/image.go` (or `internal/media`):
- `SniffContentType(head []byte) string` — wraps `http.DetectContentType`, maps to our allowlist.
- `NormalizeImage(r io.Reader, declaredType string) (io.Reader, contentType string, err error)`:
  - `image.DecodeConfig` first — reject if `W*H > MaxPixels` (e.g. 50 MP) → bomb defense.
  - JPEG/PNG: decode via stdlib, re-encode (drops EXIF). WebP: decode via
    `golang.org/x/image/webp`, re-encode to JPEG (x/image has no WebP encoder).
  - PDF: no re-encode; validate `%PDF-` magic + size only (metadata scrub is a later nicety).
  - HEIC: **see Risk R1** — not handled server-side in pure Go.

### 2b. Service

`apps/api/internal/services/interfaces.go` — add:
```go
type AttachmentServicer interface {
    Upload(userID, txID, fileName, declaredType string, size int64, data io.Reader) (*models.TransactionAttachment, error)
    List(userID, txID string) ([]models.TransactionAttachment, error)
    Open(userID, attachmentID string) (*models.TransactionAttachment, io.ReadCloser, error)
    Delete(userID, attachmentID string) error
}
```
`apps/api/internal/services/attachment_service.go` — `NewAttachmentService(db *gorm.DB, store storage.BlobStore, cfg AttachmentConfig) AttachmentServicer` (constructor returns the interface, concrete struct unexported — matches house style). Responsibilities:
- verify the transaction belongs to `userID` (reuse existing scoping);
- enforce `MaxAttachmentsPerTx` (count existing, non-deleted);
- sniff + normalize + checksum (sha256) + generate opaque key `userID/txID/<uuid>.<ext>`;
- **order:** `store.Put(...)` first, then DB `Create`; on DB error best-effort `store.Delete` the orphan;
- `Delete`: soft-delete row + `store.Delete` object;
- `Open`: ownership check, return metadata + `store.Get` stream.

Errors: reuse `apperrors` sentinels (`ErrInvalidInput`, `ErrForbidden`/`ErrNotFound`);
add `ErrUnsupportedMediaType`, `ErrPayloadTooLarge`, `ErrAttachmentLimit` if not present.

### 2c. Handler

`apps/api/internal/handlers/attachment_handler.go` — `NewAttachmentHandler(services.AttachmentServicer, services.AuditServicer)` (matches `NewTransactionHandler` shape). Methods:
- `Upload(c)` — `getUserID(c)`, `parsePathID(c,"id")`, cap body with
  `http.MaxBytesReader(c.Writer, c.Request.Body, cfg.MaxUploadBytes)`, `c.Request.FormFile("file")`,
  call service, `auditService.Log(userID,"UPLOAD_ATTACHMENT","attachment",att.ID,c.ClientIP(),...)`, `201`.
- `List(c)` — returns metadata array.
- `Download(c)` — ownership-checked stream; set hardened headers (see Security);
  `io.Copy(c.Writer, stream)`.
- `Delete(c)` — soft-delete + audit `DELETE_ATTACHMENT`; `204`.

Use existing helpers `getUserID`, `parsePathID`, `respondWithError` (`handlers/helpers.go`).

### 2d. Wiring (`cmd/api/main.go`)

- Construct the store + service near the other services (after `transactionService`):
  ```go
  blobStore, err := storage.NewS3BlobStore(appConfig.StorageConfig())
  if err != nil { return fmt.Errorf("blob store: %w", err) }
  attachmentService := services.NewAttachmentService(db, blobStore, appConfig.AttachmentConfig())
  attachmentHandler := handlers.NewAttachmentHandler(attachmentService, auditService)
  ```
- Register under the existing `transactions` group (note: static/collection routes are
  fine here; `:id` is already used):
  ```go
  transactions.POST("/:id/attachments", attachmentHandler.Upload)
  transactions.GET("/:id/attachments", attachmentHandler.List)
  transactions.GET("/:id/attachments/:aid", attachmentHandler.Download)
  transactions.DELETE("/:id/attachments/:aid", attachmentHandler.Delete)
  ```
  Gin note: `:id` is consistent across these; adding `:aid` as a second segment is fine.

### 2e. Swagger

Regenerate: `swag init -g cmd/api/main.go -d . --output internal/docs --parseDependency`.

### Verification (Phase 2)

Table-driven service tests (SQLite + `memBlobStore`): upload happy path, oversize,
bad type, limit exceeded, ownership rejection, delete removes object. Handler tests
(`httptest` + mock service). `./scripts/check-go.sh apps/api` green.

---

## Phase 3 — Frontend  (~1–1.5 days)

### 3a. API client (`apps/web/src/lib/api-client.ts`)

Refactor `request()` to detect `FormData` bodies: when the body is `FormData`, do NOT
set `Content-Type` (browser sets the multipart boundary) and pass the body as-is instead
of `JSON.stringify`. Keep the existing proactive-refresh + 401-retry logic (it re-sends
the same body object, which is fine for `FormData`). Add two public methods:
```ts
upload<T>(path: string, form: FormData): Promise<T>      // multipart POST
getBlob(path: string): Promise<Blob>                     // auth'd binary fetch (for <img>)
```
`getBlob` shares the token/refresh path but returns `res.blob()` instead of `res.json()`.

### 3b. Types

- `types/models.ts` — add `Attachment extends BaseModel { transaction_id; file_name; content_type; byte_size; }`.
  Add `attachments?: Attachment[]` and `attachments_count?: number` to `Transaction`.
- `types/api.ts` — response DTOs (`{ attachment }`, `{ attachments }`).

### 3c. Hooks (`apps/web/src/hooks/use-attachments.ts`)

Query-key factory `attachmentKeys`. Hooks:
- `useTransactionAttachments(txId)` — list metadata.
- `useUploadAttachment(txId)` — `apiClient.upload`, invalidate `transactionKeys.detail(txId)` + `attachmentKeys`.
- `useDeleteAttachment(txId)` — invalidate same.
- `useAttachmentBlob(txId, aid)` — `apiClient.getBlob`, wrap in `URL.createObjectURL`,
  revoke on unmount; `enabled` gated so it only fetches when a thumbnail/lightbox mounts.

### 3d. UI

- `components/transactions/create-transaction-dialog.tsx` — stage `File[]` in `useState`;
  after the create mutation resolves with the new tx id, upload staged files sequentially
  (two-step). Show pending file chips with remove.
- `components/transactions/edit-transaction-dialog.tsx` — list existing attachments
  (thumbnail grid), allow add + delete. Reuse the file-styled `Input`.
- Thumbnail + lightbox: small grid; clicking opens the existing `Dialog` at full size,
  image sourced from `useAttachmentBlob`.
- `app/(dashboard)/transactions/page.tsx` — `TransactionRow`: render a small `Paperclip`
  when `attachments_count > 0`.

### Verification (Phase 3)

End-to-end against dev MinIO: create tx with 2 receipts → row shows clip → open detail →
thumbnails render → delete one → count updates. `pnpm lint` + `pnpm build` clean.
Pixel check: thumbnails, empty state, mobile bottom-sheet dialog.

---

## Phase 4 — Production hardening & deploy  (~0.5–1 day)

- `docker-compose.prod.yml` — add `minio` (internal only, named volume, scoped service
  account, `MINIO_*` env), NOT published through cloudflared. Wire `STORAGE_*` into `api`.
- Set `RECEIPTS_BUCKET=kuberan-receipts` on the backup service so Phase 0's mirror activates.
- Enable MinIO bucket versioning.
- Confirm hardened serve headers (below) and EXIF-strip are on in prod build.
- Update `.env.prod.example`, `deploy/deploy.sh` (ensure MinIO volume + init run), and
  `docs/` (new receipts + backup runbook).

---

## API contract

All under `/api/v1`, Bearer auth, user-scoped.

| Method | Path | Body | Response |
|--------|------|------|----------|
| POST | `/transactions/:id/attachments` | multipart `file` | `201 { attachment }` |
| GET | `/transactions/:id/attachments` | — | `200 { attachments: [] }` |
| GET | `/transactions/:id/attachments/:aid` | — | `200` binary (image/pdf) |
| DELETE | `/transactions/:id/attachments/:aid` | — | `204` |

`Attachment` JSON: `{ id, transaction_id, file_name, content_type, byte_size, created_at }`
(never exposes `storage_key`). Errors follow the standard AppError envelope:
`400 INVALID_INPUT`, `413 PAYLOAD_TOO_LARGE`, `415 UNSUPPORTED_MEDIA_TYPE`,
`409 ATTACHMENT_LIMIT`, `403/404` on ownership.

## Security checklist (first untrusted-binary path in the codebase)

- [ ] Sniff magic bytes server-side; never trust client `Content-Type`/extension.
- [ ] Allowlist: image/jpeg, image/png, image/webp, application/pdf (HEIC per R1).
- [ ] `http.MaxBytesReader` cap + `MaxAttachmentsPerTx` cap.
- [ ] Ownership check on every read/write/delete (`user_id == getUserID(c)`).
- [ ] Re-encode raster images → strips EXIF/GPS + defuses decompression bombs.
- [ ] Opaque random storage keys (no user filename in path); sanitize display name.
- [ ] MinIO private (internal net only), scoped service account (not root), versioning on.
- [ ] Serve headers: sniffed `Content-Type`, `Content-Disposition: inline`,
      `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; sandbox`,
      `Cross-Origin-Resource-Policy: same-origin`, `Cache-Control: private`.
- [ ] Run the `security-auditor` agent pass before merge.

## Testing strategy

- **Storage:** `memBlobStore` unit tests; `S3BlobStore` integration test vs dev MinIO.
- **Service:** table-driven (SQLite + mem store) — happy path, oversize, bad type, limit,
  ownership, orphan cleanup on DB failure.
- **Handler:** `httptest` + mock service — multipart parse, headers on download, 4xx codes.
- **Frontend:** manual E2E vs dev MinIO + `pnpm build`/`lint`.
- Coverage target unchanged (80% overall; this path is auth-adjacent → aim 95%).

## Risks & open items

- **R1 — HEIC.** No reliable pure-Go HEIC decoder, and browsers can't render HEIC in
  `<img>` anyway. Recommendation: **drop HEIC from the initial allowlist** and instead
  convert to JPEG client-side on selection (canvas), or defer HEIC to a later cgo/libheif
  pass. This refines D3 — confirm before Phase 2.
- **R2 — Thumbnails.** MVP serves full images scaled by CSS (few per detail view; fine).
  Server-side thumbnail generation (stored under a `thumb/` key) is a later optimization.
- **R3 — Crypt passphrase custody** (Phase 0). Must live off-VPS in a password manager;
  a lost passphrase makes the off-host backup unrecoverable. Gate Phase 0 sign-off on this.
- **R4 — Two-step upload atomicity.** If create succeeds but an attachment upload fails,
  the tx exists without its receipt. Acceptable (user retries from edit dialog); surface a
  clear toast. Not worth a staging/pending-upload table for v1.

## Sequencing

Phase 0 can land first and alone (pure ops win). Phases 1→4 are ordered; each ends green on
`./scripts/check-go.sh apps/api` (backend) or `pnpm build`/`lint` (web). Total ~3.5–4.5 days.
