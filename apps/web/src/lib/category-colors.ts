/**
 * Category colour presets (plan 023, 2.1). Ten hexes picked so each reads as a
 * tile fill (>= 3:1 against the card colour) in light and dark across all
 * eight palettes in lib/palettes.ts (category-colors.test.ts enforces it).
 * Stored category colours are user data and are never migrated: one outside
 * this set renders as a "custom" swatch and stays selectable.
 */
export const PRESET_COLORS = [
  "#DB5757", // red
  "#D2642D", // orange
  "#AD7B25", // amber
  "#20975C", // green
  "#1F938A", // teal
  "#4683D8", // blue
  "#7D74E2", // indigo
  "#A368DF", // purple
  "#DA4E94", // pink
  "#7C8290", // gray
];

