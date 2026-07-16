"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { FileText, Loader2, Plus, X } from "lucide-react";

import { cn } from "@/lib/utils";
import { ApiClientError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  useTransactionAttachments,
  useUploadAttachment,
  useDeleteAttachment,
  useAttachmentObjectUrl,
} from "@/hooks/use-attachments";
import type { Attachment } from "@/types/models";

// Mirrors the backend allowlist (SniffContentType) and per-file cap so we can
// reject obviously bad files before spending a round-trip. The server remains
// the source of truth.
const ACCEPT = "image/png,image/jpeg,image/webp,application/pdf";
const MAX_FILE_BYTES = 10 * 1024 * 1024; // 10 MiB, matches MAX_UPLOAD_BYTES
const MAX_ATTACHMENTS = 10; // matches MAX_ATTACHMENTS_PER_TX

function isImage(contentType: string): boolean {
  return contentType.startsWith("image/");
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Validate a candidate file against the client-side allowlist + size cap. */
function rejectionReason(file: File): string | null {
  if (file.size > MAX_FILE_BYTES) {
    return `${file.name} is larger than ${formatBytes(MAX_FILE_BYTES)}`;
  }
  const type = file.type;
  const ok =
    type === "image/png" ||
    type === "image/jpeg" ||
    type === "image/webp" ||
    type === "application/pdf";
  if (!ok) {
    return `${file.name} is not a supported image or PDF`;
  }
  return null;
}

// A tile that lazily fetches its own object URL and renders an image preview or
// a PDF placeholder. Clicking an image opens the lightbox; clicking a PDF opens
// it in a new tab.
function AttachmentTile({
  attachment,
  onOpenImage,
  onRemove,
  removing,
}: {
  attachment: Attachment;
  onOpenImage: (a: Attachment) => void;
  onRemove?: (a: Attachment) => void;
  removing?: boolean;
}) {
  const image = isImage(attachment.content_type);
  const { url, isLoading } = useAttachmentObjectUrl(
    attachment.transaction_id,
    attachment.id
  );

  function handleClick() {
    if (image) {
      onOpenImage(attachment);
    } else if (url) {
      window.open(url, "_blank", "noopener,noreferrer");
    }
  }

  return (
    <div className="group relative">
      <button
        type="button"
        onClick={handleClick}
        disabled={!url}
        title={attachment.file_name}
        className={cn(
          "flex size-20 items-center justify-center overflow-hidden rounded-lg border bg-muted/40 transition-colors hover:border-ring disabled:cursor-default"
        )}
      >
        {image && url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={url}
            alt={attachment.file_name}
            className="size-full object-cover"
          />
        ) : isLoading ? (
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        ) : (
          <FileText className="size-6 text-muted-foreground" />
        )}
      </button>
      {onRemove && (
        <button
          type="button"
          onClick={() => onRemove(attachment)}
          disabled={removing}
          aria-label={`Delete ${attachment.file_name}`}
          className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground opacity-0 shadow-sm transition-opacity group-hover:opacity-100 focus-visible:opacity-100 disabled:opacity-50"
        >
          {removing ? (
            <Loader2 className="size-3 animate-spin" />
          ) : (
            <X className="size-3" />
          )}
        </button>
      )}
    </div>
  );
}

// Full-size image preview sourced from the same cached object URL as the tile.
function AttachmentLightbox({
  attachment,
  onClose,
}: {
  attachment: Attachment | null;
  onClose: () => void;
}) {
  return (
    <Dialog open={!!attachment} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="truncate">
            {attachment?.file_name}
          </DialogTitle>
          <DialogDescription className="sr-only">
            Full-size preview of the receipt {attachment?.file_name}.
          </DialogDescription>
        </DialogHeader>
        {attachment && <LightboxImage attachment={attachment} />}
      </DialogContent>
    </Dialog>
  );
}

function LightboxImage({ attachment }: { attachment: Attachment }) {
  const { url, isLoading } = useAttachmentObjectUrl(
    attachment.transaction_id,
    attachment.id
  );
  if (isLoading || !url) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    );
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={url}
      alt={attachment.file_name}
      className="mx-auto max-h-[70vh] w-auto rounded-md object-contain"
    />
  );
}

// Hidden file input + trigger button shared by both the staged and existing
// attachment editors.
function AddButton({
  onFiles,
  disabled,
  label = "Add file",
}: {
  onFiles: (files: File[]) => void;
  disabled?: boolean;
  label?: string;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept={ACCEPT}
        multiple
        className="hidden"
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          if (files.length > 0) onFiles(files);
          // Reset so re-selecting the same file fires onChange again.
          e.target.value = "";
        }}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => inputRef.current?.click()}
      >
        <Plus className="size-4" />
        {label}
      </Button>
    </>
  );
}

/**
 * StagedAttachments stages File objects in local state before the transaction
 * exists (create flow). The parent owns the file list and uploads them after
 * the create mutation resolves with a transaction id.
 */
export function StagedAttachments({
  files,
  onChange,
  disabled,
}: {
  files: File[];
  onChange: (files: File[]) => void;
  disabled?: boolean;
}) {
  function addFiles(incoming: File[]) {
    const accepted: File[] = [];
    for (const file of incoming) {
      const reason = rejectionReason(file);
      if (reason) {
        toast.error(reason);
        continue;
      }
      accepted.push(file);
    }
    const next = [...files, ...accepted];
    if (next.length > MAX_ATTACHMENTS) {
      toast.error(`At most ${MAX_ATTACHMENTS} receipts per transaction`);
      onChange(next.slice(0, MAX_ATTACHMENTS));
      return;
    }
    onChange(next);
  }

  function removeAt(index: number) {
    onChange(files.filter((_, i) => i !== index));
  }

  return (
    <div className="flex flex-col gap-2">
      <Label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Receipts
      </Label>
      {files.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {files.map((file, index) => (
            <li
              key={`${file.name}-${index}`}
              className="flex items-center gap-2 rounded-md border bg-muted/30 px-2.5 py-1.5 text-sm"
            >
              <FileText className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">{file.name}</span>
              <span className="shrink-0 text-xs text-muted-foreground">
                {formatBytes(file.size)}
              </span>
              <button
                type="button"
                onClick={() => removeAt(index)}
                disabled={disabled}
                aria-label={`Remove ${file.name}`}
                className="shrink-0 text-muted-foreground transition-colors hover:text-destructive disabled:opacity-50"
              >
                <X className="size-4" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div>
        <AddButton
          onFiles={addFiles}
          disabled={disabled || files.length >= MAX_ATTACHMENTS}
          label={files.length > 0 ? "Add more" : "Attach receipt"}
        />
      </div>
    </div>
  );
}

/**
 * ExistingAttachments manages the receipts of a persisted transaction (edit
 * flow): it lists the current attachments as thumbnails, uploads new files
 * immediately, and deletes on demand.
 */
export function ExistingAttachments({ txId }: { txId: string }) {
  const { data: attachments, isLoading } = useTransactionAttachments(txId);
  const upload = useUploadAttachment(txId);
  const remove = useDeleteAttachment(txId);
  const [lightbox, setLightbox] = useState<Attachment | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const count = attachments?.length ?? 0;

  async function addFiles(incoming: File[]) {
    let remaining = MAX_ATTACHMENTS - count;
    for (const file of incoming) {
      if (remaining <= 0) {
        toast.error(`At most ${MAX_ATTACHMENTS} receipts per transaction`);
        break;
      }
      const reason = rejectionReason(file);
      if (reason) {
        toast.error(reason);
        continue;
      }
      try {
        await upload.mutateAsync(file);
        remaining -= 1;
      } catch (err) {
        toast.error(
          err instanceof ApiClientError
            ? err.message
            : `Failed to upload ${file.name}`
        );
      }
    }
  }

  function handleRemove(attachment: Attachment) {
    setDeletingId(attachment.id);
    remove.mutate(attachment.id, {
      onError: (err) =>
        toast.error(
          err instanceof ApiClientError
            ? err.message
            : "Failed to delete receipt"
        ),
      onSettled: () => setDeletingId(null),
    });
  }

  return (
    <div className="flex flex-col gap-2">
      <Label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Receipts
      </Label>
      {isLoading ? (
        <div className="flex h-20 items-center justify-center rounded-lg border border-dashed">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {(attachments ?? []).map((a) => (
            <AttachmentTile
              key={a.id}
              attachment={a}
              onOpenImage={setLightbox}
              onRemove={handleRemove}
              removing={deletingId === a.id}
            />
          ))}
          {count === 0 && (
            <p className="py-2 text-sm text-muted-foreground">
              No receipts attached yet.
            </p>
          )}
        </div>
      )}
      <div>
        <AddButton
          onFiles={addFiles}
          disabled={upload.isPending || count >= MAX_ATTACHMENTS}
          label={
            upload.isPending
              ? "Uploading…"
              : count > 0
                ? "Add more"
                : "Attach receipt"
          }
        />
      </div>
      <AttachmentLightbox
        attachment={lightbox}
        onClose={() => setLightbox(null)}
      />
    </div>
  );
}
