import type {
  CheckinResponse,
  CheckoutResult,
  Event,
  HoldResult,
  Order,
  QRPayload,
  SeatSnapshotResponse,
  Ticket,
  User,
  WaitingRoomStatus,
  Zone,
} from "./types";

export class APIError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(path, {
    ...init,
    headers,
    credentials: "include",
  });

  const text = await res.text();
  let parsed: Record<string, unknown> = {};
  if (text) {
    try {
      parsed = JSON.parse(text) as Record<string, unknown>;
    } catch {
      parsed = { message: text };
    }
  }

  if (!res.ok) {
    const code = typeof parsed.error === "string" ? parsed.error : "api_error";
    const msg =
      typeof parsed.message === "string"
        ? parsed.message
        : `Request failed with status ${res.status}`;
    throw new APIError(res.status, code, msg);
  }

  return parsed as unknown as T;
}

export function newIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export const api = {
  auth: {
    register: (email: string, password: string, full_name: string) =>
      request<{ user: User; token: string }>("/api/v1/auth/register", {
        method: "POST",
        body: JSON.stringify({ email, password, full_name }),
      }),
    login: (email: string, password: string) =>
      request<{ user: User; token: string }>("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password }),
      }),
    logout: () =>
      request<{ status: string }>("/api/v1/auth/logout", {
        method: "POST",
      }),
    me: () => request<{ user: User }>("/api/v1/auth/me"),
  },

  events: {
    list: () => request<{ events: Event[] }>("/api/v1/events"),
    get: (id: string) =>
      request<{ event: Event; zones: Zone[] }>(`/api/v1/events/${id}`),
    snapshot: (id: string) =>
      request<SeatSnapshotResponse>(`/api/v1/events/${id}/seats/snapshot`),
  },

  waitingRoom: {
    join: (event_id: string, turnstile_token = "") =>
      request<WaitingRoomStatus>("/api/v1/waiting-room/join", {
        method: "POST",
        body: JSON.stringify({ event_id, turnstile_token }),
      }),
    status: (event_id: string) =>
      request<WaitingRoomStatus>(
        `/api/v1/waiting-room/status?event_id=${encodeURIComponent(event_id)}`
      ),
  },

  holds: {
    acquire: (event_id: string, seat_ids: string[], admissionToken: string) =>
      request<HoldResult>("/api/v1/holds", {
        method: "POST",
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "X-Admission-Token": admissionToken,
        },
        body: JSON.stringify({ event_id, seat_ids }),
      }),
    release: (event_id: string, seat_id: string) =>
      request<{ status: string; seat_id: string }>(
        `/api/v1/holds/${encodeURIComponent(seat_id)}?event_id=${encodeURIComponent(event_id)}`,
        { method: "DELETE" }
      ),
  },

  checkout: {
    submit: (
      event_id: string,
      seat_ids: string[],
      hold_token: string,
      admissionToken: string
    ) =>
      request<CheckoutResult>("/api/v1/checkout/submit", {
        method: "POST",
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "X-Admission-Token": admissionToken,
        },
        body: JSON.stringify({
          event_id,
          seat_ids,
          hold_token,
          payment_provider: "stripe",
        }),
      }),
    sandboxComplete: (order_id: string, amount_cents: number) =>
      request<{ status: string; order_id: string }>(
        "/api/v1/payments/sandbox/complete",
        {
          method: "POST",
          body: JSON.stringify({ order_id, amount_cents }),
        }
      ),
  },

  orders: {
    list: () => request<{ orders: Order[] }>("/api/v1/orders"),
  },

  tickets: {
    list: () => request<{ tickets: Ticket[] }>("/api/v1/tickets"),
    getQR: (id: string) =>
      request<{ qr_token: string; payload: QRPayload }>(
        `/api/v1/tickets/${encodeURIComponent(id)}/qr`
      ),
  },

  admin: {
    checkin: (qr_token: string, gate = "MAIN") =>
      request<CheckinResponse>("/api/v1/admin/checkin", {
        method: "POST",
        body: JSON.stringify({ qr_token, gate }),
      }),
  },
};
