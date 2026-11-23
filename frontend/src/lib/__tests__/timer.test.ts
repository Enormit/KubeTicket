import { describe, expect, it } from "vitest";
import { computeRemainingSeconds, formatCountdown } from "../timer";

describe("server-synced countdown timer", () => {
  it("compensates for skewed client clocks using serverOffsetMs", () => {
    const serverNow = Date.parse("2026-11-23T10:00:00.000Z");
    const expiresAt = "2026-11-23T10:05:00.000Z"; // 300s remaining on server

    // Suppose user's laptop clock is 10 minutes fast
    const clientNowSkewed = serverNow + 10 * 60 * 1000;
    const serverOffsetMs = serverNow - clientNowSkewed; // -10 min

    expect(
      computeRemainingSeconds(expiresAt, serverOffsetMs, clientNowSkewed)
    ).toBe(300);
    expect(formatCountdown(300)).toBe("05:00");
    expect(formatCountdown(65)).toBe("01:05");
  });
});
