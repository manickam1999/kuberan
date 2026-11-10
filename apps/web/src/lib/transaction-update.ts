import type { UpdateTransactionRequest } from "@/types/api";

/**
 * The wire form of an update. The API reads `category_id: null` (like an
 * omitted field) as "leave the category alone" and an empty string as "clear
 * it", so the UI's null ("no category") must be sent as "". Without this,
 * removing a category, or switching a transaction's type, silently kept the
 * old category (and the type switch was then refused as a category mismatch).
 */
export function toUpdateBody(data: UpdateTransactionRequest): UpdateTransactionRequest {
  if (data.category_id === null) return { ...data, category_id: "" };
  return data;
}
