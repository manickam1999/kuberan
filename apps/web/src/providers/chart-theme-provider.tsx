"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

/**
 * The visual style used for data visualizations, independent of the light/dark
 * color mode. `clean` is the default flat/vector look; `dither` renders the
 * ordered-dither (pixel-texture) charts and diagrams with a paired retro font.
 */
export type ChartTheme = "clean" | "dither";

export const CHART_THEME_STORAGE_KEY = "kuberan-chart-theme";
const DEFAULT_CHART_THEME: ChartTheme = "clean";

/**
 * Inline script that applies the persisted chart theme to <html> before first
 * paint, so a `dither` user never flashes the clean fonts/charts on load.
 * Injected in the document head (see the root layout).
 */
export const CHART_THEME_INIT_SCRIPT = `(function(){try{var t=localStorage.getItem('${CHART_THEME_STORAGE_KEY}');document.documentElement.dataset.chartTheme=(t==='dither'||t==='clean')?t:'${DEFAULT_CHART_THEME}';}catch(e){document.documentElement.dataset.chartTheme='${DEFAULT_CHART_THEME}';}})();`;

type ChartThemeContextValue = {
  chartTheme: ChartTheme;
  setChartTheme: (theme: ChartTheme) => void;
};

const ChartThemeContext = createContext<ChartThemeContextValue | null>(null);

export function ChartThemeProvider({ children }: { children: ReactNode }) {
  const [chartTheme, setChartThemeState] = useState<ChartTheme>(
    DEFAULT_CHART_THEME
  );

  // Hydrate from the attribute the init script already resolved (falling back
  // to localStorage), after mount so server and client first-render agree.
  useEffect(() => {
    const attr = document.documentElement.dataset.chartTheme;
    const stored =
      attr === "clean" || attr === "dither"
        ? attr
        : (localStorage.getItem(CHART_THEME_STORAGE_KEY) as ChartTheme | null);
    if (stored === "clean" || stored === "dither") {
      setChartThemeState(stored);
    }
  }, []);

  const setChartTheme = useCallback((theme: ChartTheme) => {
    setChartThemeState(theme);
    document.documentElement.dataset.chartTheme = theme;
    try {
      localStorage.setItem(CHART_THEME_STORAGE_KEY, theme);
    } catch {
      // Ignore storage failures (private mode / disabled) — the in-memory
      // state still drives the current session.
    }
  }, []);

  return (
    <ChartThemeContext.Provider value={{ chartTheme, setChartTheme }}>
      {children}
    </ChartThemeContext.Provider>
  );
}

/** Read/set the active chart theme. Must be used under {@link ChartThemeProvider}. */
export function useChartTheme(): ChartThemeContextValue {
  const ctx = useContext(ChartThemeContext);
  if (!ctx) {
    throw new Error("useChartTheme must be used within a ChartThemeProvider");
  }
  return ctx;
}
