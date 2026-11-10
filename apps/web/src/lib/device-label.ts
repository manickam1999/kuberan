export type DeviceKind = "desktop" | "phone" | "tablet" | "unknown";

export interface DeviceLabel {
  /** "Mac · Safari", "iPhone · Chrome", "Unknown device". */
  label: string;
  kind: DeviceKind;
}

function detectBrowser(ua: string): string | null {
  // Order matters: Edge, Opera and Chrome-on-iOS all also say "Safari"/"Chrome".
  if (/Edg(e|A|iOS)?\//.test(ua)) return "Edge";
  if (/OPR\/|Opera/.test(ua)) return "Opera";
  if (/Firefox\/|FxiOS\//.test(ua)) return "Firefox";
  if (/CriOS\/|Chrome\//.test(ua)) return "Chrome";
  if (/Safari\//.test(ua)) return "Safari";
  return null;
}

function detectDevice(ua: string): { name: string; kind: DeviceKind } | null {
  if (/iPhone/.test(ua)) return { name: "iPhone", kind: "phone" };
  if (/iPad/.test(ua)) return { name: "iPad", kind: "tablet" };
  if (/Android/.test(ua)) {
    return /Mobile/.test(ua)
      ? { name: "Android phone", kind: "phone" }
      : { name: "Android tablet", kind: "tablet" };
  }
  if (/Windows/.test(ua)) return { name: "Windows PC", kind: "desktop" };
  if (/Macintosh|Mac OS X/.test(ua)) return { name: "Mac", kind: "desktop" };
  if (/CrOS/.test(ua)) return { name: "Chromebook", kind: "desktop" };
  if (/Linux|X11/.test(ua)) return { name: "Linux PC", kind: "desktop" };
  return null;
}

/**
 * Turns a raw User-Agent header into a short device label for the signed-in
 * devices list. Never throws; an empty or unrecognised agent is "Unknown device".
 */
export function parseDeviceLabel(
  userAgent: string | null | undefined
): DeviceLabel {
  const ua = (userAgent ?? "").trim();
  if (!ua) return { label: "Unknown device", kind: "unknown" };
  const device = detectDevice(ua);
  const browser = detectBrowser(ua);
  if (!device && !browser) return { label: "Unknown device", kind: "unknown" };
  const parts = [device?.name ?? "Unknown device", browser].filter(Boolean);
  return { label: parts.join(" · "), kind: device?.kind ?? "unknown" };
}
