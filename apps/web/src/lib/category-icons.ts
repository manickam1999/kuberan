import {
  Baby,
  Banknote,
  Bike,
  Book,
  Briefcase,
  Building2,
  Bus,
  Car,
  Coffee,
  Coins,
  CreditCard,
  Droplet,
  Dumbbell,
  Film,
  Fuel,
  Gamepad2,
  Gift,
  Globe,
  GraduationCap,
  HandCoins,
  HeartPulse,
  House,
  Landmark,
  Laptop,
  Music,
  PartyPopper,
  PawPrint,
  PiggyBank,
  Pill,
  Plane,
  Receipt,
  Scissors,
  Shirt,
  ShoppingBag,
  ShoppingCart,
  Smartphone,
  Sprout,
  Tag,
  TrainFront,
  TrendingUp,
  Tv,
  Utensils,
  Wallet,
  Wifi,
  Wrench,
  Zap,
  type LucideIcon,
} from "lucide-react";

/**
 * Named category icons, keyed by Lucide icon name. A category's `icon` is
 * either one of these keys or a single emoji. Keep in sync with
 * categoryIconNames in apps/api/internal/services/category_icons.go.
 * Ordered for the picker: everyday spending first, then money and income.
 */
export const CATEGORY_ICONS: ReadonlyArray<{ key: string; icon: LucideIcon }> = [
  { key: "utensils", icon: Utensils },
  { key: "coffee", icon: Coffee },
  { key: "shopping-cart", icon: ShoppingCart },
  { key: "shopping-bag", icon: ShoppingBag },
  { key: "car", icon: Car },
  { key: "fuel", icon: Fuel },
  { key: "bus", icon: Bus },
  { key: "train-front", icon: TrainFront },
  { key: "bike", icon: Bike },
  { key: "plane", icon: Plane },
  { key: "house", icon: House },
  { key: "zap", icon: Zap },
  { key: "droplet", icon: Droplet },
  { key: "wifi", icon: Wifi },
  { key: "smartphone", icon: Smartphone },
  { key: "tv", icon: Tv },
  { key: "film", icon: Film },
  { key: "music", icon: Music },
  { key: "gamepad-2", icon: Gamepad2 },
  { key: "book", icon: Book },
  { key: "graduation-cap", icon: GraduationCap },
  { key: "heart-pulse", icon: HeartPulse },
  { key: "pill", icon: Pill },
  { key: "dumbbell", icon: Dumbbell },
  { key: "shirt", icon: Shirt },
  { key: "scissors", icon: Scissors },
  { key: "gift", icon: Gift },
  { key: "party-popper", icon: PartyPopper },
  { key: "baby", icon: Baby },
  { key: "paw-print", icon: PawPrint },
  { key: "sprout", icon: Sprout },
  { key: "wrench", icon: Wrench },
  { key: "building-2", icon: Building2 },
  { key: "globe", icon: Globe },
  { key: "tag", icon: Tag },
  { key: "receipt", icon: Receipt },
  { key: "credit-card", icon: CreditCard },
  { key: "wallet", icon: Wallet },
  { key: "banknote", icon: Banknote },
  { key: "coins", icon: Coins },
  { key: "hand-coins", icon: HandCoins },
  { key: "piggy-bank", icon: PiggyBank },
  { key: "landmark", icon: Landmark },
  { key: "trending-up", icon: TrendingUp },
  { key: "briefcase", icon: Briefcase },
  { key: "laptop", icon: Laptop },
];

const ICON_BY_KEY = new Map<string, LucideIcon>(
  CATEGORY_ICONS.map(({ key, icon }) => [key, icon])
);
// Older name for "house", still accepted by the API.
ICON_BY_KEY.set("home", House);

/** Returns the Lucide icon for a named category icon, if it is one. */
export function namedCategoryIcon(icon: string | undefined): LucideIcon | undefined {
  return icon ? ICON_BY_KEY.get(icon) : undefined;
}

const PICTOGRAPHIC = /\p{Extended_Pictographic}|\p{Regional_Indicator}/u;

/** Reports whether `value` is exactly one emoji (one grapheme, pictographic). */
export function isSingleEmoji(value: string): boolean {
  if (!value || !PICTOGRAPHIC.test(value)) return false;
  const segments = Array.from(
    new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(value)
  );
  return segments.length === 1;
}

/** Reports whether the API will accept `value` as a category icon. */
export function isValidCategoryIcon(value: string): boolean {
  return value === "" || ICON_BY_KEY.has(value) || isSingleEmoji(value);
}
