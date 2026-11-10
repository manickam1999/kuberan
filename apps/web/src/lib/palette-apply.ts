import { PALETTES, isPaletteId, type PaletteId } from "./palettes";

/**
 * Applying a palette means setting `data-palette` on <html>; the CSS in
 * app/globals.css does the rest. A page with no attribute shows the built-in
 * palette (Catppuccin).
 *
 * The resolved palette (the user's choice, else the server default) is
 * mirrored into a cookie and into localStorage so the inline init script can
 * apply it before first paint:
 * - the cookie is the primary mirror, shared by every tab and readable early;
 * - localStorage is the fallback when cookies are blocked.
 * Neither holds anything sensitive: it is a public palette id.
 */
export const PALETTE_COOKIE = "kuberan_palette";
export const PALETTE_STORAGE_KEY = "kuberan-palette";

const COOKIE_MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

/**
 * Inline script that applies the mirrored palette to <html> before first
 * paint. Injected in the document head with the CSP nonce (see the root
 * layout), like the chart-theme script. Unknown or missing values leave the
 * attribute off, which renders the built-in palette.
 */
export const PALETTE_INIT_SCRIPT = `(function(){try{var ids=${JSON.stringify(
  PALETTES.map((p) => p.id)
)};var v=null;var m=document.cookie.match(/(?:^|; )${PALETTE_COOKIE}=([^;]*)/);if(m){v=decodeURIComponent(m[1]);}if(ids.indexOf(v)<0){try{v=localStorage.getItem('${PALETTE_STORAGE_KEY}');}catch(e){v=null;}}if(ids.indexOf(v)>=0){document.documentElement.dataset.palette=v;}}catch(e){}})();`;

/** Class that cross-fades colours while a palette change lands (globals.css). */
export const PALETTE_TRANSITION_CLASS = "palette-transition";
const TRANSITION_MS = 280;
let transitionTimer: ReturnType<typeof setTimeout> | null = null;

/**
 * Sets the palette on <html> for the current page. With `animate`, colours
 * cross-fade instead of snapping (skipped under prefers-reduced-motion, and
 * when the palette is unchanged).
 */
export function applyPalette(
  id: PaletteId,
  options: { animate?: boolean } = {}
): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  if (root.dataset.palette === id) return;
  const reduceMotion =
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (options.animate && !reduceMotion) {
    root.classList.add(PALETTE_TRANSITION_CLASS);
    if (transitionTimer !== null) clearTimeout(transitionTimer);
    transitionTimer = setTimeout(() => {
      root.classList.remove(PALETTE_TRANSITION_CLASS);
      transitionTimer = null;
    }, TRANSITION_MS);
  }
  root.dataset.palette = id;
}

/** Mirrors the palette for the next page load's init script. */
export function persistPalette(id: PaletteId): void {
  if (typeof document === "undefined") return;
  const secure = window.location.protocol === "https:" ? "; Secure" : "";
  try {
    document.cookie = `${PALETTE_COOKIE}=${encodeURIComponent(id)}; path=/; max-age=${COOKIE_MAX_AGE_SECONDS}; SameSite=Lax${secure}`;
  } catch {
    // Cookies blocked: the localStorage mirror below still works.
  }
  try {
    localStorage.setItem(PALETTE_STORAGE_KEY, id);
  } catch {
    // Storage blocked (private mode): the palette still applies this session.
  }
}

/** Applies a palette now and mirrors it for the next load. */
export function applyAndPersistPalette(
  id: PaletteId,
  options: { animate?: boolean } = {}
): void {
  applyPalette(id, options);
  persistPalette(id);
}

/** The palette currently mirrored for first paint, if any. */
export function readMirroredPalette(): PaletteId | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie.match(
    new RegExp(`(?:^|; )${PALETTE_COOKIE}=([^;]*)`)
  );
  const fromCookie = match ? decodeURIComponent(match[1]) : null;
  if (isPaletteId(fromCookie)) return fromCookie;
  try {
    const stored = localStorage.getItem(PALETTE_STORAGE_KEY);
    return isPaletteId(stored) ? stored : null;
  } catch {
    return null;
  }
}
