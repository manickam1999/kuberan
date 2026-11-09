import { useCallback, useSyncExternalStore } from "react";

/** The non-standard Chromium event that offers an install prompt. */
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

// `beforeinstallprompt` fires once, early. Capture it at module load (the
// dashboard layout imports this file) so the More page can use it later.
let deferred: BeforeInstallPromptEvent | null = null;
let installed = false;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

if (typeof window !== "undefined") {
  window.addEventListener("beforeinstallprompt", (e) => {
    e.preventDefault();
    deferred = e as BeforeInstallPromptEvent;
    emit();
  });
  window.addEventListener("appinstalled", () => {
    deferred = null;
    installed = true;
    emit();
  });
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export type InstallState =
  /** Already running as an installed app. */
  | "standalone"
  /** Android/desktop Chromium: `install()` shows the native prompt. */
  | "promptable"
  /** iOS Safari: no install event; show Share > Add to Home Screen steps. */
  | "ios"
  /** Anything else: show the generic instruction. */
  | "manual";

function isStandalone(): boolean {
  const nav = window.navigator as Navigator & { standalone?: boolean };
  return window.matchMedia("(display-mode: standalone)").matches || nav.standalone === true;
}

function isIos(): boolean {
  const ua = window.navigator.userAgent;
  // iPadOS 13+ reports as a Mac with touch points.
  return (
    /iPad|iPhone|iPod/.test(ua) ||
    (ua.includes("Macintosh") && window.navigator.maxTouchPoints > 1)
  );
}

function getState(): InstallState {
  if (installed || isStandalone()) return "standalone";
  if (deferred) return "promptable";
  return isIos() ? "ios" : "manual";
}

/**
 * How the app can be put on the Home Screen right now, plus `install()` for
 * the Chromium prompt. Returns null on the server and during hydration.
 */
export function useInstallPrompt(): {
  state: InstallState | null;
  install: () => Promise<void>;
} {
  const state = useSyncExternalStore<InstallState | null>(
    subscribe,
    getState,
    () => null
  );

  const install = useCallback(async () => {
    const event = deferred;
    if (!event) return;
    await event.prompt();
    await event.userChoice;
    deferred = null;
    emit();
  }, []);

  return { state, install };
}
