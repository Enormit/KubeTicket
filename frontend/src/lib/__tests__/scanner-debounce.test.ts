import { describe, expect, it } from "vitest";
import { shouldProcessQRScan } from "../scanner-debounce";

describe("admin QR scanner debounce", () => {
  it("debounces identical QR scans within 1500ms cooldown", () => {
    expect(shouldProcessQRScan("tok-1", "", 0, 1000)).toBe(true);
    expect(shouldProcessQRScan("tok-1", "tok-1", 1000, 1800, 1500)).toBe(false);
    expect(shouldProcessQRScan("tok-1", "tok-1", 1000, 2600, 1500)).toBe(true);
    expect(shouldProcessQRScan("tok-2", "tok-1", 1000, 1200, 1500)).toBe(true);
  });
});
