export type TabSyncEvent =
  | { type: "AUTH_CHANGED" }
  | { type: "ADMISSION_UPDATED"; eventId: string; token: string }
  | { type: "CART_UPDATED"; eventId: string };

const CHANNEL_NAME = "kubeticket_sync";

export function broadcastTabSync(event: TabSyncEvent): void {
  if (typeof window === "undefined" || typeof BroadcastChannel === "undefined") {
    return;
  }
  try {
    const ch = new BroadcastChannel(CHANNEL_NAME);
    ch.postMessage(event);
    ch.close();
  } catch {
    // Ignore unsupported browser environments
  }
}

export function subscribeTabSync(
  handler: (event: TabSyncEvent) => void
): () => void {
  if (typeof window === "undefined" || typeof BroadcastChannel === "undefined") {
    return () => {};
  }
  const ch = new BroadcastChannel(CHANNEL_NAME);
  ch.onmessage = (msg: MessageEvent<TabSyncEvent>) => {
    if (msg.data && typeof msg.data.type === "string") {
      handler(msg.data);
    }
  };
  return () => ch.close();
}

const ADMISSION_STORAGE_PREFIX = "kubeticket:admission:";

export function saveAdmissionToken(
  eventId: string,
  token: string,
  expiresAt?: string
): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(
      `${ADMISSION_STORAGE_PREFIX}${eventId}`,
      JSON.stringify({ token, expiresAt })
    );
    broadcastTabSync({ type: "ADMISSION_UPDATED", eventId, token });
  } catch {
    // Ignore storage quota issues
  }
}

export function loadAdmissionToken(eventId: string): string | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(
      `${ADMISSION_STORAGE_PREFIX}${eventId}`
    );
    if (!raw) return null;
    const parsed = JSON.parse(raw) as { token?: string; expiresAt?: string };
    if (!parsed.token) return null;
    if (parsed.expiresAt && new Date(parsed.expiresAt).getTime() <= Date.now()) {
      window.sessionStorage.removeItem(`${ADMISSION_STORAGE_PREFIX}${eventId}`);
      return null;
    }
    return parsed.token;
  } catch {
    return null;
  }
}
