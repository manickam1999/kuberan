import {
  useQuery,
  useMutation,
  useQueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import {
  optimisticId,
  patchInPage,
  removeFromPage,
  restoreSnapshot,
  snapshotQueries,
  updatePages,
  type OptimisticContext,
} from "@/lib/optimistic";
import type { Category, CategoryType } from "@/types/models";
import type {
  PageResponse,
  PaginationParams,
  CategoryResponse,
  CreateCategoryRequest,
  UpdateCategoryRequest,
  DeleteResponse,
} from "@/types/api";

type CategoryListParams = PaginationParams & { type?: CategoryType };

export const categoryKeys = {
  all: ["categories"] as const,
  lists: () => [...categoryKeys.all, "list"] as const,
  list: (params?: CategoryListParams) =>
    [...categoryKeys.lists(), params] as const,
  details: () => [...categoryKeys.all, "detail"] as const,
  detail: (id: string) => [...categoryKeys.details(), id] as const,
};

export function useCategories(params?: CategoryListParams) {
  return useQuery({
    queryKey: categoryKeys.list(params),
    queryFn: () =>
      apiClient.get<PageResponse<Category>>("/api/v1/categories", {
        ...params,
      }),
    // Keep the current page on screen while the next page loads. Not across
    // type filters: a parent picker must never offer the other type's
    // categories, even briefly.
    placeholderData: (previous, previousQuery) =>
      previousQuery && paramsFromKey(previousQuery.queryKey)?.type === params?.type
        ? previous
        : undefined,
  });
}

export function useCategory(id: string) {
  return useQuery({
    queryKey: categoryKeys.detail(id),
    queryFn: async () => {
      const res = await apiClient.get<CategoryResponse>(
        `/api/v1/categories/${id}`
      );
      return res.category;
    },
    enabled: !!id,
  });
}

/** List params stored in a category list key (see categoryKeys.list). */
function paramsFromKey(queryKey: QueryKey): CategoryListParams | undefined {
  return (queryKey as readonly unknown[])[2] as CategoryListParams | undefined;
}

/**
 * The server returns categories in creation order, so a new one belongs at
 * the end of the last page of every list whose type filter it matches.
 */
function appendToLastPage(
  page: PageResponse<Category>,
  params: CategoryListParams | undefined,
  category: Category
): PageResponse<Category> {
  if (params?.type && params.type !== category.type) return page;
  const isLastPage = page.total_pages <= 1 || page.page >= page.total_pages;
  const hasRoom = page.page_size <= 0 || page.data.length < page.page_size;
  if (!isLastPage || !hasRoom) {
    return { ...page, total_items: page.total_items + 1 };
  }
  return {
    ...page,
    data: [...page.data, category],
    total_items: page.total_items + 1,
  };
}

function applyCategoryUpdate(
  category: Category,
  data: UpdateCategoryRequest
): Category {
  return {
    ...category,
    ...(data.name !== undefined && { name: data.name }),
    ...(data.description !== undefined && { description: data.description }),
    ...(data.icon !== undefined && { icon: data.icon }),
    ...(data.color !== undefined && { color: data.color }),
    ...(data.parent_id !== undefined && { parent_id: data.parent_id }),
  };
}

export function useCreateCategory() {
  const queryClient = useQueryClient();
  return useMutation<Category, Error, CreateCategoryRequest, OptimisticContext>(
    {
      mutationFn: async (data) => {
        const res = await apiClient.post<CategoryResponse>(
          "/api/v1/categories",
          data
        );
        return res.category;
      },
      onMutate: async (data) => {
        const snapshot = await snapshotQueries(queryClient, [
          categoryKeys.lists(),
        ]);
        const now = new Date().toISOString();
        const optimistic: Category = {
          id: optimisticId(),
          created_at: now,
          updated_at: now,
          user_id: "",
          name: data.name,
          type: data.type,
          description: data.description ?? "",
          icon: data.icon ?? "",
          color: data.color ?? "",
          parent_id: data.parent_id ?? null,
        };
        updatePages<Category>(queryClient, categoryKeys.lists(), (page, key) =>
          appendToLastPage(page, paramsFromKey(key), optimistic)
        );
        return { snapshot };
      },
      onError: (_err, _data, context) => restoreSnapshot(queryClient, context),
      onSettled: () => {
        queryClient.invalidateQueries({ queryKey: categoryKeys.lists() });
      },
    }
  );
}

export function useUpdateCategory(id: string) {
  const queryClient = useQueryClient();
  return useMutation<Category, Error, UpdateCategoryRequest, OptimisticContext>(
    {
      mutationFn: async (data) => {
        const res = await apiClient.put<CategoryResponse>(
          `/api/v1/categories/${id}`,
          data
        );
        return res.category;
      },
      onMutate: async (data) => {
        const snapshot = await snapshotQueries(queryClient, [
          categoryKeys.lists(),
          categoryKeys.detail(id),
        ]);
        updatePages<Category>(queryClient, categoryKeys.lists(), (page) =>
          patchInPage(page, id, (c) => applyCategoryUpdate(c, data))
        );
        queryClient.setQueryData<Category>(categoryKeys.detail(id), (c) =>
          c ? applyCategoryUpdate(c, data) : c
        );
        return { snapshot };
      },
      onError: (_err, _data, context) => restoreSnapshot(queryClient, context),
      onSettled: () => {
        queryClient.invalidateQueries({ queryKey: categoryKeys.lists() });
        queryClient.invalidateQueries({ queryKey: categoryKeys.detail(id) });
      },
    }
  );
}

export function useDeleteCategory() {
  const queryClient = useQueryClient();
  return useMutation<DeleteResponse, Error, string, OptimisticContext>({
    mutationFn: async (id) => {
      const res = await apiClient.del<DeleteResponse>(
        `/api/v1/categories/${id}`
      );
      return res;
    },
    onMutate: async (id) => {
      const snapshot = await snapshotQueries(queryClient, [
        categoryKeys.lists(),
      ]);
      updatePages<Category>(queryClient, categoryKeys.lists(), (page) =>
        removeFromPage(page, id)
      );
      return { snapshot };
    },
    onError: (_err, _id, context) => restoreSnapshot(queryClient, context),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: categoryKeys.lists() });
    },
  });
}
