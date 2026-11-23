import { describe, expect, it } from "vitest";
import { useSeatStore } from "../seat-store";
import {
  computeBackoffWithJitter,
  computeServerOffsetMs,
  resolveWebSocketURL,
} from "../ws-backoff";

describe("ws-backoff helpers", () => {
  it("caps exponential backoff with full jitter", () => {
    expect(computeBackoffWithJitter(0, 500, 30_000, () => 0)).toBe(100);
    expect(computeBackoffWithJitter(0, 500, 30_000, () => 0.5)).toBe(250);
    expect(computeBackoffWithJitter(10, 500, 30_000, () => 0.99)).toBeLessThanOrEqual(
      30_000
    );
  });

  it("computes server-client clock skew offset", () => {
    const serverTime = "2026-11-23T10:05:00.000Z";
    const clientSkewedEarlier = Date.parse("2026-11-23T10:00:00.000Z");
    expect(computeServerOffsetMs(serverTime, clientSkewedEarlier)).toBe(
      5 * 60 * 1000
    );
  });

  it("resolves WebSocket URL cleanly", () => {
    expect(resolveWebSocketURL("ev-1", "ws://localhost:8080/ws")).toBe(
      "ws://localhost:8080/ws/events/ev-1"
    );
    expect(resolveWebSocketURL("ev-1", "wss://tickets.example.com")).toBe(
      "wss://tickets.example.com/ws/events/ev-1"
    );
  });
});

describe("useSeatStore selective updates", () => {
  it("loads snapshot, applies newer deltas, and ignores stale sequence frames", () => {
    const store = useSeatStore.getState();
    store.reset();

    store.setSnapshot({
      event_id: "ev-1",
      seq: 10,
      server_now: new Date().toISOString(),
      zones: [
        {
          id: "z-1",
          event_id: "ev-1",
          code: "VIP",
          name: "VIP",
          price_cents: 25000,
          currency: "USD",
          color: "#F59E0B",
          svg_bounds: {},
        },
      ],
      seats: [
        {
          id: "s-1",
          zone_id: "z-1",
          row_label: "A",
          seat_number: 1,
          label: "VIP-A1",
          pos_x: 10,
          pos_y: 10,
          status: "AVAILABLE",
        },
      ],
    });

    store.addMyHold("hold-1", ["s-1"], new Date(Date.now() + 60_000).toISOString());

    // Stale seq (5 < 10) must be ignored
    useSeatStore.getState().applyDeltas([{ seat_id: "s-1", status: "SOLD" }], 5);
    expect(useSeatStore.getState().seatsById["s-1"].status).toBe("AVAILABLE");

    // Newer seq (11 > 10) updates seat and clears myHeldSeatIds when SOLD
    useSeatStore.getState().applyDeltas([{ seat_id: "s-1", status: "SOLD" }], 11);
    expect(useSeatStore.getState().seatsById["s-1"].status).toBe("SOLD");
    expect(useSeatStore.getState().seq).toBe(11);
    expect(useSeatStore.getState().myHeldSeatIds).toEqual([]);
  });
});
