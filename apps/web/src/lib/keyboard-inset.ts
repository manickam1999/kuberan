/**
 * How much of the layout viewport the on-screen keyboard covers, in CSS px.
 *
 * iOS Safari does not resize the layout viewport for the keyboard (and ignores
 * `interactive-widget`), so a `position: fixed; bottom: 0` sheet stays behind
 * it. The visual viewport does shrink: whatever lies below its bottom edge is
 * keyboard. Pinch zoom also shrinks it, so a zoomed-in viewport reports 0.
 */
export function keyboardInset(
  layoutHeight: number,
  viewport: { height: number; offsetTop: number; scale: number }
): number {
  if (viewport.scale > 1.01) return 0;
  const covered = layoutHeight - viewport.height - viewport.offsetTop;
  // Small differences are browser chrome (URL bar) settling, not a keyboard.
  return covered > 80 ? Math.round(covered) : 0;
}
