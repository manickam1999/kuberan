import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { transactionKeys } from "@/hooks/use-transactions";
import type { TransactionRule } from "@/types/models";
import type {
  ApplyRuleRequest,
  ApplyRuleResult,
  CreateRuleRequest,
  DeleteResponse,
  ReorderRulesRequest,
  RuleMatchPreview,
  RulePreviewRequest,
  RuleResponse,
  RulesResponse,
  UpdateRuleRequest,
} from "@/types/api";

export const ruleKeys = {
  all: ["rules"] as const,
  lists: () => [...ruleKeys.all, "list"] as const,
  list: () => [...ruleKeys.lists()] as const,
  details: () => [...ruleKeys.all, "detail"] as const,
  detail: (id: string) => [...ruleKeys.details(), id] as const,
};

export function useRules() {
  return useQuery({
    queryKey: ruleKeys.list(),
    queryFn: async () => {
      const res = await apiClient.get<RulesResponse>("/api/v1/rules");
      return res.rules;
    },
  });
}

export function useRule(id: string) {
  return useQuery({
    queryKey: ruleKeys.detail(id),
    queryFn: async () => {
      const res = await apiClient.get<RuleResponse>(`/api/v1/rules/${id}`);
      return res.rule;
    },
    enabled: !!id,
  });
}

export function useCreateRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateRuleRequest) => {
      const res = await apiClient.post<RuleResponse>("/api/v1/rules", data);
      return res.rule;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ruleKeys.lists() });
    },
  });
}

export function useUpdateRule(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: UpdateRuleRequest) => {
      const res = await apiClient.put<RuleResponse>(`/api/v1/rules/${id}`, data);
      return res.rule;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ruleKeys.lists() });
      queryClient.invalidateQueries({ queryKey: ruleKeys.detail(id) });
    },
  });
}

export function useDeleteRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      return apiClient.del<DeleteResponse>(`/api/v1/rules/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ruleKeys.lists() });
    },
  });
}

export function useReorderRules() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (ruleIds: string[]) => {
      const body: ReorderRulesRequest = { rule_ids: ruleIds };
      const res = await apiClient.post<RulesResponse>(
        "/api/v1/rules/reorder",
        body
      );
      return res.rules;
    },
    onSuccess: (rules) => {
      // The reordered list is authoritative; seed it so the UI doesn't flicker.
      queryClient.setQueryData<TransactionRule[]>(ruleKeys.list(), rules);
      queryClient.invalidateQueries({ queryKey: ruleKeys.lists() });
    },
  });
}

// Preview how many existing transactions match a set of unsaved conditions.
export function useRulePreview() {
  return useMutation({
    mutationFn: async (body: RulePreviewRequest) => {
      return apiClient.post<RuleMatchPreview>("/api/v1/rules/preview", body);
    },
  });
}

// Backfill a saved rule over existing transactions (dry-run by default).
export function useApplyRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      ruleId,
      ...body
    }: ApplyRuleRequest & { ruleId: string }) => {
      return apiClient.post<ApplyRuleResult>(
        `/api/v1/rules/${ruleId}/apply`,
        body
      );
    },
    onSuccess: (_result, variables) => {
      // A committed backfill changes transaction categories.
      if (variables.dry_run === false) {
        queryClient.invalidateQueries({ queryKey: transactionKeys.all });
      }
    },
  });
}
