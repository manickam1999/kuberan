"use client";

import {
  createContext,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { User } from "@/types/models";
import type {
  AuthResponse,
  LoginRequest,
  RegisterRequest,
  ProfileResponse,
} from "@/types/api";
import { apiClient } from "@/lib/api-client";
import {
  bootstrapSession,
  clearTokens,
  endSession,
  revokeSession,
  setAuthCookie,
  startSession,
} from "@/lib/auth";
import { broadcastAuthEvent, subscribeAuthEvents } from "@/lib/auth-channel";

/**
 * - loading: the app-start refresh has not settled yet
 * - authenticated / unauthenticated: the session state is known
 * - error: the server could not be reached; the session may still be valid
 */
export type SessionStatus =
  | "loading"
  | "authenticated"
  | "unauthenticated"
  | "error";

export interface AuthContextValue {
  user: User | null;
  status: SessionStatus;
  isLoading: boolean;
  isAuthenticated: boolean;
  hideBalances: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (data: RegisterRequest) => Promise<void>;
  logout: () => void;
  toggleHideBalances: () => Promise<void>;
  /** Account preselected for new transactions, or null when none is set. */
  defaultAccountId: string | null;
  /**
   * Merges server-confirmed profile fields into the session user. Only the
   * given fields change, so an in-flight optimistic update to another field
   * is not overwritten.
   */
  patchUser: (fields: Partial<User>) => void;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [user, setUser] = useState<User | null>(null);
  const [status, setStatus] = useState<SessionStatus>("loading");
  const userIdRef = useRef<string | null>(null);

  const isLoading = status === "loading";
  const isAuthenticated = status === "authenticated" && user !== null;
  // Mask balances until the user's preference is known, so a user who hides
  // balances never sees them flash while the session loads.
  const hideBalances = user ? user.hide_balances : isLoading;
  const defaultAccountId = user?.default_account_id ?? null;

  useEffect(() => {
    userIdRef.current = user?.id ?? null;
  }, [user]);

  // On mount: restore the session from the refresh cookie. Page queries wait
  // on the same bootstrap inside the API client, so they run in parallel with
  // this instead of after it.
  useEffect(() => {
    let cancelled = false;

    async function init() {
      const session = await bootstrapSession();
      if (cancelled) return;

      if (session.status === "unauthenticated") {
        setStatus("unauthenticated");
        return;
      }
      if (session.status === "error") {
        // Keep the session cookie so a reload can restore it once the server
        // is reachable again.
        toast.error("Could not reach the server");
        setStatus("error");
        return;
      }

      let sessionUser = session.user;
      if (!sessionUser) {
        // Older API without `user` in the refresh response.
        try {
          const res = await apiClient.get<ProfileResponse>("/api/v1/profile");
          sessionUser = res.user;
        } catch {
          if (cancelled) return;
          toast.error("Could not load your profile");
          setStatus("error");
          return;
        }
        if (cancelled) return;
      }
      setUser(sessionUser);
      setAuthCookie();
      setStatus("authenticated");
    }

    void init();
    return () => {
      cancelled = true;
    };
  }, []);

  // Another tab logged in or out. Drop everything cached for the previous
  // session: a logout ends this tab's session too (they share the cookie),
  // and a login as a different user replaces it.
  useEffect(() => {
    return subscribeAuthEvents((event) => {
      const currentUserId = userIdRef.current;
      if (event.type === "logout") {
        if (currentUserId === null) return;
        queryClient.clear();
        clearTokens();
        window.location.href = "/login";
        return;
      }
      if (event.userId !== currentUserId) {
        queryClient.clear();
        clearTokens();
        window.location.reload();
      }
    });
  }, [queryClient]);

  const establishSession = useCallback(
    (res: AuthResponse) => {
      // Nothing cached for a previous session may leak into this one.
      queryClient.clear();
      startSession(res.access_token, res.user);
      setAuthCookie();
      setUser(res.user);
      setStatus("authenticated");
      broadcastAuthEvent("login", res.user.id);
    },
    [queryClient]
  );

  const login = useCallback(
    async (email: string, password: string) => {
      const data: LoginRequest = { email, password };
      const res = await apiClient.post<AuthResponse>(
        "/api/v1/auth/login",
        data
      );
      establishSession(res);
    },
    [establishSession]
  );

  const register = useCallback(
    async (data: RegisterRequest) => {
      const res = await apiClient.post<AuthResponse>(
        "/api/v1/auth/register",
        data
      );
      establishSession(res);
    },
    [establishSession]
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

  const patchUser = useCallback((fields: Partial<User>) => {
    setUser((prev) => (prev ? { ...prev, ...fields } : prev));
  }, []);

  const logout = useCallback(() => {
    // revokeSession captures what it needs synchronously, before endSession
    // clears local state.
    void revokeSession();
    endSession();
    queryClient.clear();
    broadcastAuthEvent("logout", null);
    setUser(null);
    setStatus("unauthenticated");
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
  }, [queryClient]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      status,
      isLoading,
      isAuthenticated,
      hideBalances,
      login,
      register,
      logout,
      toggleHideBalances,
      defaultAccountId,
      patchUser,
    }),
    [
      user,
      status,
      isLoading,
      isAuthenticated,
      hideBalances,
      login,
      register,
      logout,
      toggleHideBalances,
      defaultAccountId,
      patchUser,
    ]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
