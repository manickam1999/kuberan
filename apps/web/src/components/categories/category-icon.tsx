import { cn } from "@/lib/utils";
import { isSingleEmoji, namedCategoryIcon } from "@/lib/category-icons";

interface CategoryIconProps {
  /** The category's stored icon: a named icon, an emoji, or legacy free text. */
  icon?: string;
  /** Used for the first-letter fallback. */
  name?: string;
  className?: string;
}

/**
 * Renders a category icon at the surrounding text size. Named icons render as
 * Lucide glyphs and emoji as text. Anything else (free text saved before icons
 * were restricted) falls back to the category's first letter, so it can never
 * overflow the tile it sits in.
 */
export function CategoryIcon({ icon, name, className }: CategoryIconProps) {
  const Named = namedCategoryIcon(icon);
  if (Named) {
    return <Named className={cn("size-[1.15em] shrink-0", className)} aria-hidden="true" />;
  }
  const text = icon && isSingleEmoji(icon) ? icon : name?.charAt(0).toUpperCase() || "?";
  return (
    <span className={cn("leading-none", className)} aria-hidden="true">
      {text}
    </span>
  );
}
