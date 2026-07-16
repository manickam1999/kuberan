import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { apiClient } from "@/lib/api-client";
import type { Attachment } from "@/types/models";
import type {
  AttachmentResponse,
  AttachmentsResponse,
} from "@/types/api";
import { transactionKeys } from "./use-transactions";

export const attachmentKeys = {
  all: ["attachments"] as const,
  lists: () => [...attachmentKeys.all, "list"] as const,
  list: (txId: string) => [...attachmentKeys.lists(), txId] as const,
  blobs: () => [...attachmentKeys.all, "blob"] as const,
  blob: (txId: string, aid: string) =>
    [...attachmentKeys.blobs(), txId, aid] as const,
};

// List the receipt attachment metadata for a transaction (no bytes).
export function useTransactionAttachments(txId: string) {
  return useQuery({
    queryKey: attachmentKeys.list(txId),
    queryFn: async () => {
      const res = await apiClient.get<AttachmentsResponse>(
        `/api/v1/transactions/${txId}/attachments`
      );
      return res.attachments;
    },
    enabled: !!txId,
  });
}

// Upload a single receipt file to a transaction (multipart).
export function useUploadAttachment(txId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append("file", file);
      const res = await apiClient.upload<AttachmentResponse>(
        `/api/v1/transactions/${txId}/attachments`,
        form
      );
      return res.attachment;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: attachmentKeys.list(txId) });
      queryClient.invalidateQueries({
        queryKey: transactionKeys.detail(txId),
      });
      // The paperclip indicator lives on list rows.
      queryClient.invalidateQueries({ queryKey: transactionKeys.lists() });
      queryClient.invalidateQueries({ queryKey: transactionKeys.userLists() });
    },
  });
}

// Delete a receipt attachment from a transaction.
export function useDeleteAttachment(txId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (attachmentId: string) => {
      await apiClient.del<void>(
        `/api/v1/transactions/${txId}/attachments/${attachmentId}`
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: attachmentKeys.list(txId) });
      queryClient.invalidateQueries({
        queryKey: transactionKeys.detail(txId),
      });
      queryClient.invalidateQueries({ queryKey: transactionKeys.lists() });
      queryClient.invalidateQueries({ queryKey: transactionKeys.userLists() });
    },
  });
}

// Fetch an attachment's bytes as an authenticated Blob. The token never appears
// in a URL. The Blob is cached (and shared across observers of the same
// attachment) so multiple previews reuse a single download. Gated by `enabled`
// so the fetch only fires when a thumbnail/lightbox actually mounts.
//
// Do NOT create an object URL here: the cache is shared across every consumer of
// the same key, but object-URL lifetime is per-consumer. Caching the URL and
// revoking it on unmount would let one consumer (e.g. a closing lightbox) revoke
// the URL another consumer (e.g. the still-mounted thumbnail) is showing, and
// leave a dead URL in the cache. Derive object URLs per-consumer via
// `useAttachmentObjectUrl` instead.
export function useAttachmentBlob(
  txId: string,
  attachmentId: string,
  enabled = true
) {
  return useQuery({
    queryKey: attachmentKeys.blob(txId, attachmentId),
    queryFn: () =>
      apiClient.getBlob(
        `/api/v1/transactions/${txId}/attachments/${attachmentId}`
      ),
    enabled: enabled && !!txId && !!attachmentId,
    staleTime: Infinity, // the bytes are immutable for the attachment's lifetime
  });
}

// Derive a per-consumer object URL from the shared cached Blob. Each mounting
// component owns its own object URL and revokes exactly that URL on unmount (or
// when the underlying blob/id changes), so consumers can't invalidate each
// other's previews. Returns `{ url, isLoading }`.
export function useAttachmentObjectUrl(
  txId: string,
  attachmentId: string,
  enabled = true
) {
  const { data: blob, isLoading } = useAttachmentBlob(
    txId,
    attachmentId,
    enabled
  );
  const [url, setUrl] = useState<string>();

  useEffect(() => {
    if (!blob) {
      setUrl(undefined);
      return;
    }
    const objectUrl = URL.createObjectURL(blob);
    setUrl(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
  }, [blob]);

  return { url, isLoading };
}

export type { Attachment };
