"use client";

import {
  createContext,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import type { User } from "@/types/models";
import type {
  AuthResponse,
  LoginRequest,
  RegisterRequest,
  ProfileResponse,
  UpdateProfileSettingsRequest,
} from "@/types/api";
import { apiClient, ApiClientError } from "@/lib/api-client";
import { profileKeys } from "@/hooks/use-profile";
import {
  clearTokens,
  setAccessToken,
  setRefreshToken,
  getAccessToken,
  setAuthCookie,
  clearAuthCookie,
} from "@/lib/auth";

export interface AuthContextValue {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  hideBalances: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (data: RegisterRequest) => Promise<void>;
  logout: () => void;
  toggleHideBalances: () => Promise<void>;
  /** Account preselected for new transactions, or null when none is set. */
  defaultAccountId: string | null;
  /** Saves (or, with null, clears) the default transaction account. */
  setDefaultAccount: (accountId: string | null) => Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const queryClient = useQueryClient();

  const isAuthenticated = user !== null;
  const hideBalances = user?.hide_balances ?? false;
  const defaultAccountId = user?.default_account_id ?? null;

  // Fetch user profile using stored tokens
  const fetchProfile = useCallback(async (): Promise<User | null> => {
    try {
      const res = await apiClient.get<ProfileResponse>("/api/v1/profile");
      return res.user;
    } catch {
      return null;
    }
  }, []);

  // On mount: try to restore session from stored tokens
  useEffect(() => {
    async function init() {
      const token = getAccessToken();
      if (!token) {
        setIsLoading(false);
        return;
      }

      const profile = await fetchProfile();
      if (profile) {
        setUser(profile);
        setAuthCookie();
      } else {
        clearTokens();
        clearAuthCookie();
      }
      setIsLoading(false);
    }

    init();
  }, [fetchProfile]);

  const login = useCallback(
    async (email: string, password: string) => {
      const data: LoginRequest = { email, password };
      const res = await apiClient.post<AuthResponse>(
        "/api/v1/auth/login",
        data
      );
      setAccessToken(res.access_token);
      setRefreshToken(res.refresh_token);
      setAuthCookie();
      setUser(res.user);
    },
    []
  );

  const register = useCallback(
    async (data: RegisterRequest) => {
      const res = await apiClient.post<AuthResponse>(
        "/api/v1/auth/register",
        data
      );
      setAccessToken(res.access_token);
      setRefreshToken(res.refresh_token);
      setAuthCookie();
      setUser(res.user);
    },
    []
  );

  const toggleHideBalances = useCallback(async () => {
    setUser((prev) => {
      if (!prev) return prev;
      const next = !prev.hide_balances;
      apiClient
        .patch<ProfileResponse>("/api/v1/profile", { hide_balances: next })
        .catch(() => {
          setUser((current) =>
            current ? { ...current, hide_balances: !next } : current
          );
          toast.error("Failed to update balance visibility setting");
        });
      return { ...prev, hide_balances: next };
    });
  }, []);

  const setDefaultAccount = useCallback(
    async (accountId: string | null) => {
      const body: UpdateProfileSettingsRequest = {
        default_account_id: accountId,
      };
      try {
        const res = await apiClient.patch<ProfileResponse>(
          "/api/v1/profile",
          body
        );
        setUser(res.user);
        queryClient.setQueryData(profileKeys.all, res.user);
      } catch (err) {
        toast.error(
          err instanceof ApiClientError
            ? err.message
            : "Failed to update default account"
        );
        throw err;
      }
    },
    [queryClient]
  );

  const logout = useCallback(() => {
    clearTokens();
    clearAuthCookie();
    setUser(null);
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      isLoading,
      isAuthenticated,
      hideBalances,
      login,
      register,
      logout,
      toggleHideBalances,
      defaultAccountId,
      setDefaultAccount,
    }),
    [
      user,
      isLoading,
      isAuthenticated,
      hideBalances,
      login,
      register,
      logout,
      toggleHideBalances,
      defaultAccountId,
      setDefaultAccount,
    ]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
