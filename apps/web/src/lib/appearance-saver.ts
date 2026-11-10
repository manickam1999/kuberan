import type { Appearance } from "@/types/models";

/** What the server confirmed: the stored appearance and the palette it resolves to. */
export interface ConfirmedAppearance {
  appearance: Appearance;
  resolved_palette?: string;
}

export type SaveStatus = "idle" | "saving" | "saved" | "error";

export interface SaveHandlers {
  /** Persists the full desired appearance; resolves with what the server stored. */
  send: (appearance: Appearance) => Promise<ConfirmedAppearance>;
  /** The latest save landed. */
  onSaved: (confirmed: ConfirmedAppearance) => void;
  /** The latest save failed: put this (the last confirmed state) back. */
  onFailed: (confirmed: ConfirmedAppearance) => void;
}

/**
 * Saves appearance changes in the background so the UI can apply them at once.
 *
 * - Changes made in quick succession are merged and sent as one request after
 *   `delayMs` of quiet.
 * - Every request carries every field changed this session, so the newest
 *   request alone describes the whole desired state.
 * - Only the newest request's outcome counts: an older reply arriving late
 *   never re-applies a choice the user has already replaced.
 * - When the newest request fails, the last confirmed state is restored.
 */
export class AppearanceSaver {
  private desired: Appearance = {};
  private confirmed: ConfirmedAppearance | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private seq = 0;
  private handlers: SaveHandlers | null = null;
  private status: SaveStatus = "idle";
  private readonly listeners = new Set<() => void>();

  constructor(private readonly delayMs = 350) {}

  /**
   * Queues a change. `current` is the state shown before any unsaved change
   * (used as the rollback point the first time).
   */
  schedule(
    patch: Appearance,
    current: ConfirmedAppearance,
    handlers: SaveHandlers
  ): void {
    if (this.confirmed === null) this.confirmed = current;
    this.desired = { ...this.desired, ...patch };
    this.handlers = handlers;
    this.setStatus("saving");
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = setTimeout(() => void this.flush(), this.delayMs);
  }

  /** Sends what is queued now (also used by tests). */
  async flush(): Promise<void> {
    this.timer = null;
    const handlers = this.handlers;
    if (!handlers) return;
    const mine = ++this.seq;
    const body = { ...this.desired };
    try {
      const saved = await handlers.send(body);
      if (mine !== this.seq || this.timer !== null) return; // superseded
      this.confirmed = saved;
      this.desired = {};
      this.setStatus("saved");
      handlers.onSaved(saved);
    } catch {
      if (mine !== this.seq || this.timer !== null) return; // superseded
      const back = this.confirmed ?? { appearance: {} };
      this.desired = {};
      this.setStatus("error");
      handlers.onFailed(back);
    }
  }

  getStatus(): SaveStatus {
    return this.status;
  }

  /** For useSyncExternalStore. */
  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private setStatus(status: SaveStatus): void {
    if (this.status === status) return;
    this.status = status;
    for (const l of this.listeners) l();
  }
}

/** The one saver the app shares (every settings control and the header toggle). */
export const appearanceSaver = new AppearanceSaver();
