"use client";

import { useIsFetching } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { isBackgroundRefetch } from "@/lib/refresh-activity";

/** Fetches quicker than this never show the bar (no flicker). */
const SHOW_DELAY_MS = 250;
/** Once shown, the bar stays at least this long so it registers. */
const MIN_VISIBLE_MS = 600;

/**
 * True while any query that already shows data is being refetched, with a
 * short delay before showing and a minimum time shown.
 */
function useBackgroundRefreshing(): boolean {
  const fetching = useIsFetching({ predicate: isBackgroundRefetch }) > 0;
  const [visible, setVisible] = useState(false);
  const [shownAt, setShownAt] = useState(0);

  useEffect(() => {
    if (fetching && !visible) {
      const t = setTimeout(() => {
        setShownAt(Date.now());
        setVisible(true);
      }, SHOW_DELAY_MS);
      return () => clearTimeout(t);
    }
    if (!fetching && visible) {
      const left = Math.max(0, MIN_VISIBLE_MS - (Date.now() - shownAt));
      const t = setTimeout(() => setVisible(false), left);
      return () => clearTimeout(t);
    }
  }, [fetching, visible, shownAt]);

  return visible;
}

/**
 * A thin indeterminate bar along the bottom edge of the app header while
 * on-screen data is being reloaded in the background (returning to the
 * installed app, refocusing the tab, reconnecting, or after a change). The
 * data stays visible underneath; the bar just says it is being refreshed.
 */
export function RefreshIndicator() {
  const visible = useBackgroundRefreshing();

  return (
    <>
      <div
        aria-hidden
        data-slot="refresh-indicator"
        data-state={visible ? "active" : "idle"}
        className="refresh-bar pointer-events-none absolute inset-x-0 -bottom-px h-0.5 overflow-hidden"
      />
      <span role="status" aria-live="polite" className="sr-only">
        {visible ? "Refreshing data" : ""}
      </span>
    </>
  );
}
