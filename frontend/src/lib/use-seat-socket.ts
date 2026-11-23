"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import { useSeatStore } from "./seat-store";
import type { WebSocketEnvelope } from "./types";
import { computeBackoffWithJitter, resolveWebSocketURL } from "./ws-backoff";

export type WSConnectionState =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "offline";

interface UseSeatSocketOptions {
  eventId: string;
  onOrderConfirmed?: (orderId: string) => void;
}

export function useSeatSocket({
  eventId,
  onOrderConfirmed,
}: UseSeatSocketOptions) {
  const [connectionState, setConnectionState] =
    useState<WSConnectionState>("connecting");
  const setSnapshot = useSeatStore((s) => s.setSnapshot);
  const applyDeltas = useSeatStore((s) => s.applyDeltas);

  const onOrderConfirmedRef = useRef(onOrderConfirmed);
  onOrderConfirmedRef.current = onOrderConfirmed;

  const resyncSnapshot = useCallback(async () => {
    try {
      const snap = await api.events.snapshot(eventId);
      setSnapshot(snap);
    } catch {
      // Snapshot errors are handled on initial page load or next reconnect
    }
  }, [eventId, setSnapshot]);

  useEffect(() => {
    if (!eventId) return;

    let cancelled = false;
    let attempt = 0;
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let pingTimer: ReturnType<typeof setInterval> | null = null;
    let runtimeWsUrl = "";

    const cleanupSocket = () => {
      if (pingTimer) {
        clearInterval(pingTimer);
        pingTimer = null;
      }
      if (ws) {
        ws.onopen = null;
        ws.onmessage = null;
        ws.onclose = null;
        ws.onerror = null;
        if (
          ws.readyState === WebSocket.OPEN ||
          ws.readyState === WebSocket.CONNECTING
        ) {
          ws.close();
        }
        ws = null;
      }
    };

    const connect = async () => {
      if (cancelled) return;
      if (typeof navigator !== "undefined" && !navigator.onLine) {
        setConnectionState("offline");
        return;
      }

      if (!runtimeWsUrl) {
        try {
          const res = await fetch("/api/runtime-config", { cache: "no-store" });
          if (res.ok) {
            const data = (await res.json()) as { wsUrl?: string };
            runtimeWsUrl = data.wsUrl || "";
          }
        } catch {
          runtimeWsUrl = "";
        }
      }

      if (cancelled) return;
      setConnectionState(attempt === 0 ? "connecting" : "reconnecting");

      const url = resolveWebSocketURL(eventId, runtimeWsUrl);
      const sock = new WebSocket(url);
      ws = sock;

      sock.onopen = () => {
        attempt = 0;
        setConnectionState("connected");
        pingTimer = setInterval(() => {
          if (sock.readyState === WebSocket.OPEN) {
            sock.send(JSON.stringify({ type: "PING" }));
          }
        }, 20_000);
      };

      sock.onmessage = (ev: MessageEvent<string>) => {
        try {
          const msg = JSON.parse(ev.data) as WebSocketEnvelope;
          if (msg.type === "SYNC") {
            const currentSeq = useSeatStore.getState().seq;
            if (msg.seq > currentSeq) {
              void resyncSnapshot();
            }
          } else if (msg.type === "DELTA" && msg.seats) {
            applyDeltas(msg.seats, msg.seq, msg.server_now);
          } else if (msg.type === "ORDER_CONFIRMED" && msg.order_id) {
            if (msg.seats) {
              applyDeltas(msg.seats, msg.seq, msg.server_now);
            }
            onOrderConfirmedRef.current?.(msg.order_id);
          }
        } catch {
          // Ignore malformed WS frame
        }
      };

      sock.onclose = () => {
        cleanupSocket();
        if (cancelled) return;
        setConnectionState("reconnecting");
        const delay = computeBackoffWithJitter(attempt);
        attempt += 1;
        reconnectTimer = setTimeout(() => {
          void connect();
        }, delay);
      };

      sock.onerror = () => {
        sock.close();
      };
    };

    const handleVisibilityOrOnline = () => {
      if (document.visibilityState === "visible" && navigator.onLine) {
        void resyncSnapshot();
        if (!ws || ws.readyState === WebSocket.CLOSED) {
          if (reconnectTimer) {
            clearTimeout(reconnectTimer);
            reconnectTimer = null;
          }
          attempt = 0;
          void connect();
        }
      } else if (!navigator.onLine) {
        setConnectionState("offline");
      }
    };

    void connect();
    document.addEventListener("visibilitychange", handleVisibilityOrOnline);
    window.addEventListener("online", handleVisibilityOrOnline);
    window.addEventListener("offline", handleVisibilityOrOnline);

    return () => {
      cancelled = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
      window.removeEventListener("online", handleVisibilityOrOnline);
      window.removeEventListener("offline", handleVisibilityOrOnline);
      cleanupSocket();
    };
  }, [applyDeltas, eventId, resyncSnapshot]);

  return { connectionState, resyncSnapshot };
}
