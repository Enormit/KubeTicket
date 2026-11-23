export function computeBackoffWithJitter(
  attempt: number,
  baseMs = 500,
  capMs = 30_000,
  rand: () => number = Math.random
): number {
  const exp = Math.min(capMs, baseMs * 2 ** Math.max(0, attempt));
  return Math.max(100, Math.floor(rand() * exp));
}

export function computeServerOffsetMs(
  serverNowIso: string,
  clientNowMs: number = Date.now()
): number {
  const parsed = Date.parse(serverNowIso);
  if (Number.isNaN(parsed)) {
    return 0;
  }
  return parsed - clientNowMs;
}

export function resolveWebSocketURL(
  eventId: string,
  configuredWsUrl: string
): string {
  const encodedId = encodeURIComponent(eventId);
  const trimmed = configuredWsUrl.trim().replace(/\/+$/, "");
  if (trimmed) {
    // Support both ws://host and ws://host/ws formats cleanly
    const base = trimmed.endsWith("/ws") ? trimmed.slice(0, -3) : trimmed;
    return `${base}/ws/events/${encodedId}`;
  }
  if (typeof window !== "undefined") {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${window.location.host}/ws/events/${encodedId}`;
  }
  return `ws://localhost:8080/ws/events/${encodedId}`;
}
