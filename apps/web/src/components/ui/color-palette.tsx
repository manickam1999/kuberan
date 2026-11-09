"use client";

import { cn } from "@/lib/utils";
import { PRESET_COLORS } from "@/lib/category-colors";

export { PRESET_COLORS };

function isPreset(color: string): boolean {
  return PRESET_COLORS.some((c) => c.toLowerCase() === color.toLowerCase());
}

interface ColorPaletteProps {
  value: string;
  onChange: (color: string) => void;
  disabled?: boolean;
}

export function ColorPalette({ value, onChange, disabled }: ColorPaletteProps) {
  // A stored colour that is not a preset is shown as an extra, selected swatch.
  const custom = value && !isPreset(value) ? value : null;
  const colors = custom ? [...PRESET_COLORS, custom] : PRESET_COLORS;

  return (
    <div className="flex flex-wrap gap-2">
      {colors.map((color) => (
        <button
          key={color}
          type="button"
          disabled={disabled}
          onClick={() => onChange(value.toLowerCase() === color.toLowerCase() ? "" : color)}
          className={cn(
            "h-7 w-7 rounded-full border-2 transition-all",
            value.toLowerCase() === color.toLowerCase()
              ? "ring-2 ring-offset-2 ring-primary border-primary"
              : "border-transparent hover:scale-110",
            disabled && "cursor-not-allowed opacity-50"
          )}
          style={{ backgroundColor: color }}
          aria-label={
            color === custom
              ? `Custom color ${color} (selected)`
              : `Select color ${color}`
          }
          aria-pressed={value.toLowerCase() === color.toLowerCase()}
        />
      ))}
    </div>
  );
}
