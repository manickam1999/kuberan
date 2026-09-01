"use client";

import { useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  sankey,
  sankeyLinkHorizontal,
  type SankeyLink as D3SankeyLink,
  type SankeyNode as D3SankeyNode,
} from "d3-sankey";
import { DitherFill } from "@/components/charts/dither-fill";
import { PALETTE, rgb, type DitherColor } from "@/components/dither-kit/palette";
import { useChartTheme } from "@/providers/chart-theme-provider";
import { cn } from "@/lib/utils";

/** A node in the flow graph. `align` controls which side its label sits on. */
export type SankeyNodeInput = {
  id: string;
  label: string;
  /** Clean-theme fill colour (any CSS colour). */
  cleanColor: string;
  /** Dither-theme palette hue. */
  ditherColor: DitherColor;
  /** Label placement. `center` nodes (e.g. the hub) render no label. */
  align: "left" | "center" | "right";
};

/** A weighted flow from one node to another (value shares the caller's unit). */
export type SankeyLinkInput = { source: string; target: string; value: number };

export type ThemedSankeyProps = {
  nodes: SankeyNodeInput[];
  links: SankeyLinkInput[];
  /** Formats a flow/node magnitude for labels and tooltips. */
  valueFormatter: (value: number) => string;
  /** Chart height in px. */
  height?: number;
  className?: string;
};

type LayoutNode = D3SankeyNode<SankeyNodeInput, SankeyLinkInput>;
type LayoutLink = D3SankeyLink<SankeyNodeInput, SankeyLinkInput>;

const NODE_WIDTH = 14;
const NODE_PADDING = 22;
const TOP_PAD = 10;
const BOTTOM_PAD = 10;

type Hover = { title: string; value: string; x: number; y: number } | null;

/**
 * Income-to-expense cashflow Sankey. Layout is computed with `d3-sankey`; the
 * ribbons render as SVG while the node bars render as HTML so the `dither` theme
 * can paint them with the same ordered-dither texture as the other charts. In
 * `clean` mode nodes/links use the caller's per-node CSS colours; in `dither`
 * mode they use the matching palette hue. Both themes read the same props.
 */
export function ThemedSankey({
  nodes,
  links,
  valueFormatter,
  height = 360,
  className,
}: ThemedSankeyProps) {
  const { chartTheme } = useChartTheme();
  const isDither = chartTheme === "dither";
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [hover, setHover] = useState<Hover>(null);
  const [activeLink, setActiveLink] = useState<string | null>(null);

  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setWidth(el.clientWidth);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const hasLeft = nodes.some((n) => n.align === "left");
  const hasRight = nodes.some((n) => n.align === "right");

  const graph = useMemo(() => {
    if (width <= 0 || nodes.length === 0 || links.length === 0) return null;
    const gutter = (present: boolean) =>
      present ? Math.min(150, Math.max(84, width * 0.2)) : 8;
    const left = gutter(hasLeft);
    const right = gutter(hasRight);
    try {
      const generator = sankey<SankeyNodeInput, SankeyLinkInput>()
        .nodeId((d) => d.id)
        .nodeWidth(NODE_WIDTH)
        .nodePadding(NODE_PADDING)
        .extent([
          [left, TOP_PAD],
          [Math.max(left + NODE_WIDTH * 3, width - right), height - BOTTOM_PAD],
        ]);
      return generator({
        nodes: nodes.map((n) => ({ ...n })),
        links: links.map((l) => ({ ...l })),
      });
    } catch {
      return null;
    }
  }, [nodes, links, width, height, hasLeft, hasRight]);

  const linkPath = useMemo(() => sankeyLinkHorizontal<SankeyNodeInput, SankeyLinkInput>(), []);

  const colorOf = (node: LayoutNode) =>
    isDither ? rgb(PALETTE[node.ditherColor].fill) : node.cleanColor;

  const linkKey = (l: LayoutLink) =>
    `${(l.source as LayoutNode).id}->${(l.target as LayoutNode).id}`;

  // The categorical (non-hub) endpoint drives a ribbon's colour.
  const ribbonColor = (l: LayoutLink) => {
    const src = l.source as LayoutNode;
    const tgt = l.target as LayoutNode;
    return colorOf(src.align === "center" ? tgt : src);
  };

  const moveTooltip = (e: React.MouseEvent, title: string, value: number) => {
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) return;
    setHover({
      title,
      value: valueFormatter(value),
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
    });
  };

  return (
    <div
      ref={containerRef}
      className={cn("relative w-full", className)}
      style={{ height }}
    >
      {graph && (
        <>
          {/* Ribbons */}
          <svg
            className="absolute inset-0"
            width={width}
            height={height}
            aria-hidden
          >
            {graph.links.map((l) => {
              const key = linkKey(l as LayoutLink);
              const dimmed = activeLink !== null && activeLink !== key;
              return (
                <path
                  key={key}
                  d={linkPath(l) ?? undefined}
                  fill="none"
                  stroke={ribbonColor(l as LayoutLink)}
                  strokeOpacity={dimmed ? 0.12 : activeLink === key ? 0.72 : 0.42}
                  strokeWidth={Math.max(1, l.width ?? 1)}
                  className="transition-[stroke-opacity] duration-150"
                  style={{ pointerEvents: "stroke" }}
                  onMouseEnter={() => setActiveLink(key)}
                  onMouseMove={(e) => {
                    const src = (l.source as LayoutNode).label;
                    const tgt = (l.target as LayoutNode).label;
                    moveTooltip(e, `${src} → ${tgt}`, l.value);
                  }}
                  onMouseLeave={() => {
                    setActiveLink(null);
                    setHover(null);
                  }}
                />
              );
            })}
          </svg>

          {/* Node bars (HTML so the dither texture can paint them) */}
          {graph.nodes.map((n) => {
            const node = n as LayoutNode;
            const w = (node.x1 ?? 0) - (node.x0 ?? 0);
            const h = Math.max(1, (node.y1 ?? 0) - (node.y0 ?? 0));
            return (
              <div
                key={node.id}
                className="absolute overflow-hidden rounded-[3px]"
                style={{
                  left: node.x0,
                  top: node.y0,
                  width: w,
                  height: h,
                  backgroundColor: isDither ? undefined : node.cleanColor,
                }}
                onMouseMove={(e) => moveTooltip(e, node.label, node.value ?? 0)}
                onMouseLeave={() => setHover(null)}
              >
                {isDither && <DitherFill color={node.ditherColor} solid />}
              </div>
            );
          })}

          {/* Labels */}
          {graph.nodes.map((n) => {
            const node = n as LayoutNode;
            if (node.align === "center") return null;
            const mid = ((node.y0 ?? 0) + (node.y1 ?? 0)) / 2;
            const isLeft = node.align === "left";
            const style: React.CSSProperties = isLeft
              ? {
                  right: width - (node.x0 ?? 0) + 8,
                  top: mid,
                  transform: "translateY(-50%)",
                  textAlign: "right",
                  maxWidth: (node.x0 ?? 0) - 12,
                }
              : {
                  left: (node.x1 ?? 0) + 8,
                  top: mid,
                  transform: "translateY(-50%)",
                  textAlign: "left",
                  maxWidth: width - (node.x1 ?? 0) - 12,
                };
            return (
              <div
                key={`label-${node.id}`}
                className="pointer-events-none absolute flex flex-col gap-0.5 leading-tight"
                style={style}
              >
                <span className="truncate text-[11px] font-medium text-foreground">
                  {node.label}
                </span>
                <span className="money truncate text-[11px] text-muted-foreground">
                  {valueFormatter(node.value ?? 0)}
                </span>
              </div>
            );
          })}

          {/* Tooltip */}
          {hover && (
            <div
              className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full whitespace-nowrap rounded-lg border border-border/50 bg-popover px-3 py-2 text-xs shadow-xl"
              style={{ left: hover.x, top: hover.y - 8 }}
            >
              <div className="font-medium">{hover.title}</div>
              <div className="money mt-0.5 text-muted-foreground">{hover.value}</div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
