import { describe, expect, it } from "vitest";
import { exportFileName } from "./export-file";

describe("exportFileName", () => {
  it("formats the local date as kuberan-export-YYYY-MM-DD.zip", () => {
    expect(exportFileName(new Date(2026, 0, 5))).toBe(
      "kuberan-export-2026-01-05.zip"
    );
    expect(exportFileName(new Date(2026, 10, 19))).toBe(
      "kuberan-export-2026-11-19.zip"
    );
  });
});
