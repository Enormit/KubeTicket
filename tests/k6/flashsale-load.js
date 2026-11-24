import http from "k6/http";
import ws from "k6/ws";
import { check, sleep } from "k6";
import { Counter } from "k6/metrics";

const BASE_URL = (__ENV.BASE_URL || "http://localhost:8080").replace(/\/+$/, "");
const WS_URL = (__ENV.WS_URL || BASE_URL.replace(/^http/, "ws")).replace(/\/+$/, "");
const EVENT_ID = __ENV.EVENT_ID || "11111111-1111-1111-1111-111111111101";
const HOT_POOL_SIZE = Number(__ENV.HOT_POOL_SIZE || 20);
const CHECKOUT_RATIO = Number(__ENV.CHECKOUT_RATIO || 0.1);
const RUN_ID = __ENV.RUN_ID || String(Date.now());

// Custom counters to verify real flash-sale contention and queue submissions.
const seatHoldAcquired = new Counter("kubeticket_seat_hold_acquired");
const seatHoldConflict = new Counter("kubeticket_seat_hold_conflict");
const checkoutSubmitted = new Counter("kubeticket_checkout_submitted");

// Treat 409 Conflict (seat already held) as an expected flash-sale outcome, not an HTTP error.
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 409));

export const options = {
  scenarios: {
    browse_and_snapshot: {
      executor: "constant-vus",
      exec: "browseFlow",
      vus: Number(__ENV.BROWSE_VUS || 5),
      duration: __ENV.DURATION || "30s",
    },
    ws_spectators: {
      executor: "constant-vus",
      exec: "wsFlow",
      vus: Number(__ENV.WS_VUS || 4),
      duration: __ENV.DURATION || "30s",
    },
    flashsale_rush: {
      executor: "ramping-vus",
      exec: "flashSaleFlow",
      startVUs: 0,
      stages: [
        { duration: "5s", target: Number(__ENV.RUSH_VUS || 8) },
        { duration: __ENV.DURATION || "20s", target: Number(__ENV.RUSH_VUS || 8) },
        { duration: "5s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.05"],
    http_req_duration: ["p(95)<1000"],
  },
};

// Cached per-VU auth session so each VU acts as one distinct user across iterations.
let vuSession = null;
let vuAdmissionToken = null;

function vuHeaders(extra = {}) {
  const oct3 = Math.floor(__VU / 250) % 250;
  const oct4 = (__VU % 250) + 1;
  return Object.assign(
    {
      "Content-Type": "application/json",
      "X-Forwarded-For": `10.99.${oct3}.${oct4}`,
    },
    extra
  );
}

export function setup() {
  const evRes = http.get(`${BASE_URL}/api/v1/events/`);
  check(evRes, { "setup list events 200": (r) => r.status === 200 });

  const snapRes = http.get(`${BASE_URL}/api/v1/events/${EVENT_ID}/seats/snapshot`);
  check(snapRes, { "setup seat snapshot 200": (r) => r.status === 200 });

  const body = snapRes.json();
  const availableSeats = (body.seats || [])
    .filter((s) => s.status === "AVAILABLE")
    .slice(0, HOT_POOL_SIZE)
    .map((s) => s.id);

  if (availableSeats.length === 0) {
    throw new Error(`No AVAILABLE seats found for event ${EVENT_ID}`);
  }

  return { availableSeats };
}

function ensureAuth() {
  if (vuSession) {
    return vuSession;
  }
  const email = `k6_${RUN_ID}_vu${__VU}@kubeticket.local`;
  const password = "LoadTest@123";
  let res = http.post(
    `${BASE_URL}/api/v1/auth/register`,
    JSON.stringify({
      email,
      password,
      full_name: `K6 Load User ${__VU}`,
    }),
    { headers: vuHeaders(), tags: { name: "auth_register" } }
  );

  if (res.status === 409) {
    res = http.post(
      `${BASE_URL}/api/v1/auth/login`,
      JSON.stringify({ email, password }),
      { headers: vuHeaders(), tags: { name: "auth_login" } }
    );
  }

  check(res, { "auth succeeded": (r) => r.status === 200 || r.status === 201 });
  const data = res.json();
  vuSession = { token: data.token, userId: data.user?.id };
  return vuSession;
}

function ensureAdmitted(authToken) {
  if (vuAdmissionToken) {
    return vuAdmissionToken;
  }
  const headers = vuHeaders({ Authorization: `Bearer ${authToken}` });
  const joinRes = http.post(
    `${BASE_URL}/api/v1/waiting-room/join`,
    JSON.stringify({ event_id: EVENT_ID, turnstile_token: "" }),
    { headers, tags: { name: "waiting_room_join" } }
  );
  check(joinRes, { "waiting-room join 200": (r) => r.status === 200 });

  let statusData = joinRes.json();
  for (let i = 0; i < 10 && !statusData.admitted; i++) {
    sleep(0.5);
    const stRes = http.get(
      `${BASE_URL}/api/v1/waiting-room/status?event_id=${EVENT_ID}`,
      { headers, tags: { name: "waiting_room_status" } }
    );
    if (stRes.status === 200) {
      statusData = stRes.json();
    }
  }
  vuAdmissionToken = statusData.admission_token || null;
  return vuAdmissionToken || "";
}

export function browseFlow() {
  const r1 = http.get(`${BASE_URL}/api/v1/events/`, {
    headers: vuHeaders(),
    tags: { name: "events_list" },
  });
  check(r1, { "list events 200": (r) => r.status === 200 });

  const r2 = http.get(`${BASE_URL}/api/v1/events/${EVENT_ID}/seats/snapshot`, {
    headers: vuHeaders(),
    tags: { name: "seats_snapshot" },
  });
  check(r2, { "seat snapshot 200": (r) => r.status === 200 });
  sleep(0.5);
}

export function wsFlow() {
  const url = `${WS_URL}/ws/events/${EVENT_ID}`;
  const res = ws.connect(url, { headers: { Origin: "http://localhost:3000" } }, function (socket) {
    socket.on("open", () => {
      socket.setTimeout(() => socket.close(), 5000);
    });
  });
  check(res, { "ws upgraded 101": (r) => r && r.status === 101 });
  sleep(0.5);
}

export function flashSaleFlow(data) {
  const session = ensureAuth();
  if (!session || !session.token) {
    sleep(1);
    return;
  }

  const admissionToken = ensureAdmitted(session.token);
  if (!admissionToken) {
    sleep(1);
    return;
  }

  const seatId = data.availableSeats[Math.floor(Math.random() * data.availableSeats.length)];
  const idemBase = `${RUN_ID}-vu${__VU}-iter${__ITER}`;

  const holdRes = http.post(
    `${BASE_URL}/api/v1/holds`,
    JSON.stringify({ event_id: EVENT_ID, seat_ids: [seatId] }),
    {
      headers: vuHeaders({
        Authorization: `Bearer ${session.token}`,
        "X-Admission-Token": admissionToken,
        "Idempotency-Key": `hold-${idemBase}`,
      }),
      tags: { name: "holds_acquire" },
    }
  );

  check(holdRes, {
    "hold 200 or 409 conflict": (r) => r.status === 200 || r.status === 409,
  });

  if (holdRes.status === 403) {
    vuAdmissionToken = null;
    sleep(0.5);
    return;
  }

  if (holdRes.status === 409) {
    seatHoldConflict.add(1);
    sleep(1.5);
    return;
  }

  if (holdRes.status !== 200) {
    sleep(1.5);
    return;
  }

  seatHoldAcquired.add(1);
  const holdToken = holdRes.json().hold_token;

  if (Math.random() < CHECKOUT_RATIO) {
    const chkRes = http.post(
      `${BASE_URL}/api/v1/checkout/submit`,
      JSON.stringify({
        event_id: EVENT_ID,
        hold_token: holdToken,
        seat_ids: [seatId],
        payment_provider: "stripe",
      }),
      {
        headers: vuHeaders({
          Authorization: `Bearer ${session.token}`,
          "X-Admission-Token": admissionToken,
          "Idempotency-Key": `chk-${idemBase}`,
        }),
        tags: { name: "checkout_submit" },
      }
    );
    check(chkRes, { "checkout accepted 202": (r) => r.status === 202 });
    if (chkRes.status === 202) {
      checkoutSubmitted.add(1);
    }
  } else {
    const relRes = http.del(
      `${BASE_URL}/api/v1/holds/${seatId}?event_id=${EVENT_ID}`,
      null,
      {
        headers: vuHeaders({ Authorization: `Bearer ${session.token}` }),
        tags: { name: "holds_release" },
      }
    );
    check(relRes, { "hold released 200": (r) => r.status === 200 });
  }

  sleep(0.5);
}
