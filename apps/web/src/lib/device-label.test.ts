import { describe, expect, it } from "vitest";
import { parseDeviceLabel } from "./device-label";

const MAC_SAFARI =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15";
const IPHONE_SAFARI =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1";
const IPHONE_CHROME =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/123.0 Mobile/15E148 Safari/604.1";
const WIN_EDGE =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36 Edg/123.0";
const ANDROID_CHROME =
  "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Mobile Safari/537.36";
const LINUX_FIREFOX =
  "Mozilla/5.0 (X11; Linux x86_64; rv:124.0) Gecko/20100101 Firefox/124.0";

describe("parseDeviceLabel", () => {
  it.each([
    [MAC_SAFARI, "Mac · Safari", "desktop"],
    [IPHONE_SAFARI, "iPhone · Safari", "phone"],
    [IPHONE_CHROME, "iPhone · Chrome", "phone"],
    [WIN_EDGE, "Windows PC · Edge", "desktop"],
    [ANDROID_CHROME, "Android phone · Chrome", "phone"],
    [LINUX_FIREFOX, "Linux PC · Firefox", "desktop"],
  ])("labels %#", (ua, label, kind) => {
    expect(parseDeviceLabel(ua)).toEqual({ label, kind });
  });

  it("handles empty and unknown agents", () => {
    expect(parseDeviceLabel("")).toEqual({
      label: "Unknown device",
      kind: "unknown",
    });
    expect(parseDeviceLabel(undefined).label).toBe("Unknown device");
    expect(parseDeviceLabel("curl/8.0").label).toBe("Unknown device");
  });

  it("keeps the browser when the OS is unknown", () => {
    expect(parseDeviceLabel("Foo/1.0 Firefox/100").label).toBe(
      "Unknown device · Firefox"
    );
  });
});
