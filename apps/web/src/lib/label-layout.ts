/**
 * Spread vertically stacked labels so none overlap.
 *
 * Each label wants to sit centred on `wanted[i]` (e.g. the middle of a Sankey
 * node). Labels keep their order, stay at least `gap` apart centre to centre,
 * and stay within [min, max] (centres). Labels that already have room don't
 * move; crowded runs are pushed apart as little as possible.
 *
 * @returns the centre for each label, in the input order
 */
export function spreadLabels(
  wanted: readonly number[],
  gap: number,
  min: number,
  max: number
): number[] {
  const order = wanted.map((y, i) => ({ y, i })).sort((a, b) => a.y - b.y);
  const placed = order.map((o) => Math.min(Math.max(o.y, min), max));

  // Downward pass: push each label below the previous one.
  for (let k = 1; k < placed.length; k++) {
    placed[k] = Math.max(placed[k], placed[k - 1] + gap);
  }
  // Upward pass: if the stack ran off the bottom, pull it back up.
  if (placed.length > 0 && placed[placed.length - 1] > max) {
    placed[placed.length - 1] = max;
    for (let k = placed.length - 2; k >= 0; k--) {
      placed[k] = Math.min(placed[k], placed[k + 1] - gap);
    }
  }
  // If there isn't room for every label, start at the top and keep the gap
  // (the last ones may then exceed max; better than overlapping).
  if (placed.length > 0 && placed[0] < min) {
    placed[0] = min;
    for (let k = 1; k < placed.length; k++) {
      placed[k] = Math.max(placed[k], placed[k - 1] + gap);
    }
  }

  const out = new Array<number>(wanted.length);
  order.forEach((o, k) => {
    out[o.i] = placed[k];
  });
  return out;
}
