// Cross-tab auth signalling. Tabs share the refresh cookie, so when one tab
// logs in (possibly as a different user) or logs out, the others must drop
// their cached data and in-memory token or they would show one user's data
// under another user's session.

export type AuthEventType = "login" | "logout";

export interface AuthEvent {
  type: AuthEventType;
  /** The signed-in user after the event (null for logout). */
  userId: string | null;
  /** Sender tab, so a tab ignores its own events. */
  tabId: string;
}

const CHANNEL_NAME = "kuberan-auth";
// Fallback for browsers without BroadcastChannel: `storage` events fire in
// every other tab of the origin when this key changes.
const STORAGE_KEY = "kuberan_auth_event";

const tabId =
  typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random()}`;

function isAuthEvent(value: unknown): value is AuthEvent {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    (v.type === "login" || v.type === "logout") &&
    (typeof v.userId === "string" || v.userId === null) &&
    typeof v.tabId === "string"
  );
}

let channel: BroadcastChannel | null = null;

function getChannel(): BroadcastChannel | null {
  if (typeof window === "undefined" || typeof BroadcastChannel === "undefined") {
    return null;
  }
  if (!channel) channel = new BroadcastChannel(CHANNEL_NAME);
  return channel;
}

/** Tell other tabs that this tab logged in or out. */
export function broadcastAuthEvent(type: AuthEventType, userId: string | null): void {
  const event: AuthEvent = { type, userId, tabId };
  const ch = getChannel();
  if (ch) {
    ch.postMessage(event);
    return;
  }
  try {
    // A nonce makes every write a change, so repeated events still fire.
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...event, nonce: Date.now() }));
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Storage unavailable: other tabs notice on their next refresh instead.
  }
}

/** Subscribe to auth events from other tabs. Returns an unsubscribe function. */
export function subscribeAuthEvents(handler: (event: AuthEvent) => void): () => void {
  const ch = getChannel();
  if (ch) {
    const onMessage = (e: MessageEvent<unknown>) => {
      if (isAuthEvent(e.data) && e.data.tabId !== tabId) handler(e.data);
    };
    ch.addEventListener("message", onMessage);
    return () => ch.removeEventListener("message", onMessage);
  }
  if (typeof window === "undefined") return () => {};
  const onStorage = (e: StorageEvent) => {
    if (e.key !== STORAGE_KEY || !e.newValue) return;
    try {
      const parsed: unknown = JSON.parse(e.newValue);
      if (isAuthEvent(parsed) && parsed.tabId !== tabId) handler(parsed);
    } catch {
      // Ignore malformed values.
    }
  };
  window.addEventListener("storage", onStorage);
  return () => window.removeEventListener("storage", onStorage);
}
