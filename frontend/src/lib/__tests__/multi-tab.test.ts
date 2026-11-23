import { describe, expect, it, vi } from "vitest";
import { APIError } from "../api";
import { broadcastTabSync, subscribeTabSync } from "../multi-tab";

describe("APIError and BroadcastChannel sync", () => {
  it("constructs APIError with status and code", () => {
    const err = new APIError(409, "seat_conflict", "Seat already held");
    expect(err.status).toBe(409);
    expect(err.code).toBe("seat_conflict");
    expect(err.message).toBe("Seat already held");
  });

  it("handles BroadcastChannel gracefully when defined", () => {
    const postSpy = vi.fn();
    const closeSpy = vi.fn();

    class FakeBroadcastChannel {
      onmessage: ((ev: MessageEvent) => void) | null = null;
      postMessage = postSpy;
      close = closeSpy;
    }

    vi.stubGlobal("window", {});
    vi.stubGlobal("BroadcastChannel", FakeBroadcastChannel);

    const unsub = subscribeTabSync(() => {});
    broadcastTabSync({ type: "AUTH_CHANGED" });
    unsub();

    expect(postSpy).toHaveBeenCalledWith({ type: "AUTH_CHANGED" });
    expect(closeSpy).toHaveBeenCalledTimes(2);

    vi.unstubAllGlobals();
  });
});
