import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
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

// Fetch an attachment's bytes as an authenticated object URL for use in <img>
// or a lightbox. The token never appears in a URL. The object URL is revoked
// when the consuming component unmounts (or the id changes). Gated by `enabled`
// so the fetch only fires when a thumbnail/lightbox actually mounts.
export function useAttachmentBlob(
  txId: string,
  attachmentId: string,
  enabled = true
) {
  const query = useQuery({
    queryKey: attachmentKeys.blob(txId, attachmentId),
    queryFn: async () => {
      const blob = await apiClient.getBlob(
        `/api/v1/transactions/${txId}/attachments/${attachmentId}`
      );
      return URL.createObjectURL(blob);
    },
    enabled: enabled && !!txId && !!attachmentId,
    staleTime: Infinity, // object URLs are stable for the blob's lifetime
  });

  // Revoke the object URL when it changes or the component unmounts, so we
  // don't leak blob references.
  const objectUrl = query.data;
  useEffect(() => {
    return () => {
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [objectUrl]);

  return query;
}

export type { Attachment };
