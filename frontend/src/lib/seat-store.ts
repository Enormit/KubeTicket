"use client";

import { create } from "zustand";
import { computeServerOffsetMs } from "./ws-backoff";
import type {
  SeatDelta,
  SeatSnapshotResponse,
  SeatState,
  SeatStatus,
  Zone,
} from "./types";

interface SeatStoreState {
  eventId: string | null;
  zones: Zone[];
  seatsById: Record<string, SeatState>;
  seatIdsByZone: Record<string, string[]>;
  seq: number;
  serverOffsetMs: number;

  myHeldSeatIds: string[];
  myHoldToken: string | null;
  myHoldExpiresAt: string | null;
  lockingSeatIds: Record<string, boolean>;

  setSnapshot: (snap: SeatSnapshotResponse) => void;
  applyDeltas: (deltas: SeatDelta[], seq: number, serverNow?: string) => void;
  setLocking: (seatId: string, busy: boolean) => void;
  addMyHold: (holdToken: string, seatIds: string[], expiresAt: string) => void;
  removeMyHeldSeat: (seatId: string) => void;
  clearMyHolds: () => void;
  reset: () => void;
}

export const useSeatStore = create<SeatStoreState>((set) => ({
  eventId: null,
  zones: [],
  seatsById: {},
  seatIdsByZone: {},
  seq: 0,
  serverOffsetMs: 0,

  myHeldSeatIds: [],
  myHoldToken: null,
  myHoldExpiresAt: null,
  lockingSeatIds: {},

  setSnapshot: (snap) => {
    const seatsById: Record<string, SeatState> = {};
    const seatIdsByZone: Record<string, string[]> = {};
    for (const s of snap.seats) {
      seatsById[s.id] = s;
      if (!seatIdsByZone[s.zone_id]) {
        seatIdsByZone[s.zone_id] = [];
      }
      seatIdsByZone[s.zone_id].push(s.id);
    }
    set({
      eventId: snap.event_id,
      zones: snap.zones,
      seatsById,
      seatIdsByZone,
      seq: snap.seq,
      serverOffsetMs: computeServerOffsetMs(snap.server_now),
    });
  },

  applyDeltas: (deltas, seq, serverNow) =>
    set((state) => {
      if (seq > 0 && seq < state.seq) {
        return state;
      }
      const nextSeats = { ...state.seatsById };
      let nextHeld = state.myHeldSeatIds;
      let heldChanged = false;

      for (const d of deltas) {
        const cur = nextSeats[d.seat_id];
        if (cur) {
          nextSeats[d.seat_id] = {
            ...cur,
            status: d.status,
            expires_at: d.expires_at,
          };
        }
        if (
          (d.status === "AVAILABLE" || d.status === "SOLD") &&
          nextHeld.includes(d.seat_id)
        ) {
          nextHeld = nextHeld.filter((id) => id !== d.seat_id);
          heldChanged = true;
        }
      }

      return {
        seatsById: nextSeats,
        seq: seq > state.seq ? seq : state.seq,
        serverOffsetMs: serverNow
          ? computeServerOffsetMs(serverNow)
          : state.serverOffsetMs,
        myHeldSeatIds: heldChanged ? nextHeld : state.myHeldSeatIds,
        myHoldToken:
          heldChanged && nextHeld.length === 0 ? null : state.myHoldToken,
        myHoldExpiresAt:
          heldChanged && nextHeld.length === 0 ? null : state.myHoldExpiresAt,
      };
    }),

  setLocking: (seatId, busy) =>
    set((state) => {
      const next = { ...state.lockingSeatIds };
      if (busy) {
        next[seatId] = true;
      } else {
        delete next[seatId];
      }
      return { lockingSeatIds: next };
    }),

  addMyHold: (holdToken, seatIds, expiresAt) =>
    set((state) => {
      const merged = Array.from(new Set([...state.myHeldSeatIds, ...seatIds]));
      return {
        myHoldToken: holdToken,
        myHeldSeatIds: merged,
        myHoldExpiresAt: expiresAt,
      };
    }),

  removeMyHeldSeat: (seatId) =>
    set((state) => {
      const remaining = state.myHeldSeatIds.filter((id) => id !== seatId);
      return {
        myHeldSeatIds: remaining,
        myHoldToken: remaining.length === 0 ? null : state.myHoldToken,
        myHoldExpiresAt: remaining.length === 0 ? null : state.myHoldExpiresAt,
      };
    }),

  clearMyHolds: () =>
    set({
      myHeldSeatIds: [],
      myHoldToken: null,
      myHoldExpiresAt: null,
    }),

  reset: () =>
    set({
      eventId: null,
      zones: [],
      seatsById: {},
      seatIdsByZone: {},
      seq: 0,
      serverOffsetMs: 0,
      myHeldSeatIds: [],
      myHoldToken: null,
      myHoldExpiresAt: null,
      lockingSeatIds: {},
    }),
}));

export function useSeatStatus(seatId: string): SeatStatus {
  return useSeatStore((s) => s.seatsById[seatId]?.status ?? "AVAILABLE");
}

export function useIsMyHeldSeat(seatId: string): boolean {
  return useSeatStore((s) => s.myHeldSeatIds.includes(seatId));
}

export function useIsLockingSeat(seatId: string): boolean {
  return useSeatStore((s) => Boolean(s.lockingSeatIds[seatId]));
}
