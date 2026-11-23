"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  AlertTriangle,
  ArrowRight,
  Trash2,
  Wifi,
  WifiOff,
} from "lucide-react";
import { api, APIError } from "@/lib/api";
import { loadAdmissionToken } from "@/lib/multi-tab";
import { useSeatStore } from "@/lib/seat-store";
import type { Event, SeatState } from "@/lib/types";
import { useSeatSocket } from "@/lib/use-seat-socket";
import { SeatMap } from "@/components/SeatMap";
import { SeatTimer } from "@/components/SeatTimer";

export default function EventSeatsPage() {
  const params = useParams<{ id: string }>();
  const eventId = params.id;
  const router = useRouter();

  const [event, setEvent] = useState<Event | null>(null);
  const [admissionToken, setAdmissionToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<string | null>(null);
  const [conflictSeatId, setConflictSeatId] = useState<string | null>(null);

  const zones = useSeatStore((s) => s.zones);
  const seatsById = useSeatStore((s) => s.seatsById);
  const myHeldSeatIds = useSeatStore((s) => s.myHeldSeatIds);
  const myHoldToken = useSeatStore((s) => s.myHoldToken);
  const setSnapshot = useSeatStore((s) => s.setSnapshot);
  const setLocking = useSeatStore((s) => s.setLocking);
  const addMyHold = useSeatStore((s) => s.addMyHold);
  const removeMyHeldSeat = useSeatStore((s) => s.removeMyHeldSeat);
  const applyDeltas = useSeatStore((s) => s.applyDeltas);

  const { connectionState, resyncSnapshot } = useSeatSocket({ eventId });

  useEffect(() => {
    if (!eventId) return;
    const tok = loadAdmissionToken(eventId);
    if (!tok) {
      router.replace(
        `/waiting-room?event_id=${encodeURIComponent(eventId)}`
      );
      return;
    }
    setAdmissionToken(tok);

    let active = true;
    Promise.all([api.events.get(eventId), api.events.snapshot(eventId)])
      .then(([evRes, snap]) => {
        if (!active) return;
        setEvent(evRes.event);
        setSnapshot(snap);
      })
      .catch((err: unknown) => {
        if (active) {
          setToast(
            err instanceof Error ? err.message : "Không thể tải sơ đồ ghế"
          );
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [eventId, router, setSnapshot]);

  const zoneMap = useMemo(() => {
    const m = new Map<string, (typeof zones)[number]>();
    for (const z of zones) m.set(z.id, z);
    return m;
  }, [zones]);

  const selectedSummary = useMemo(() => {
    let totalCents = 0;
    const items = myHeldSeatIds
      .map((id) => {
        const seat = seatsById[id];
        const zone = seat ? zoneMap.get(seat.zone_id) : undefined;
        if (zone) totalCents += zone.price_cents;
        return seat && zone ? { seat, zone } : null;
      })
      .filter((x): x is NonNullable<typeof x> => x !== null);
    return { items, totalCents };
  }, [myHeldSeatIds, seatsById, zoneMap]);

  const handleSeatClick = useCallback(
    async (seat: SeatState) => {
      if (!admissionToken) return;
      setToast(null);

      const isMine = myHeldSeatIds.includes(seat.id);
      if (isMine) {
        setLocking(seat.id, true);
        try {
          await api.holds.release(eventId, seat.id);
          removeMyHeldSeat(seat.id);
          applyDeltas([{ seat_id: seat.id, status: "AVAILABLE" }], 0);
        } catch (err) {
          setToast(
            err instanceof APIError ? err.message : "Không thể hủy giữ ghế"
          );
        } finally {
          setLocking(seat.id, false);
        }
        return;
      }

      const maxSeats = event?.max_seats_per_order || 4;
      if (myHeldSeatIds.length >= maxSeats) {
        setToast(`Mỗi đơn chỉ được giữ tối đa ${maxSeats} ghế.`);
        return;
      }

      setLocking(seat.id, true);
      try {
        const hold = await api.holds.acquire(
          eventId,
          [seat.id],
          admissionToken
        );
        addMyHold(hold.hold_token, hold.seat_ids, hold.expires_at);
        applyDeltas(
          hold.seat_ids.map((id) => ({
            seat_id: id,
            status: "HELD",
            expires_at: hold.expires_at,
          })),
          hold.seq,
          hold.server_now
        );
      } catch (err) {
        if (err instanceof APIError) {
          if (err.status === 409) {
            setConflictSeatId(seat.id);
            setToast(`Ghế ${seat.label} vừa có người khác giữ!`);
            setTimeout(() => setConflictSeatId(null), 1200);
            void resyncSnapshot();
          } else if (err.status === 403) {
            router.replace(
              `/waiting-room?event_id=${encodeURIComponent(eventId)}`
            );
          } else {
            setToast(err.message);
          }
        }
      } finally {
        setLocking(seat.id, false);
      }
    },
    [
      addMyHold,
      admissionToken,
      applyDeltas,
      event?.max_seats_per_order,
      eventId,
      myHeldSeatIds,
      removeMyHeldSeat,
      resyncSnapshot,
      router,
      setLocking,
    ]
  );

  const handleTimerExpired = useCallback(() => {
    setToast("Thời gian giữ ghế đã hết hạn. Đang đồng bộ lại sơ đồ...");
    useSeatStore.getState().clearMyHolds();
    void resyncSnapshot();
  }, [resyncSnapshot]);

  const handleProceedCheckout = () => {
    if (selectedSummary.items.length === 0 || !myHoldToken) return;
    if (typeof window !== "undefined") {
      window.sessionStorage.setItem(
        `kubeticket:checkout:${eventId}`,
        JSON.stringify({
          eventId,
          eventTitle: event?.title || "Sự kiện",
          holdToken: myHoldToken,
          seatIds: myHeldSeatIds,
          totalCents: selectedSummary.totalCents,
          items: selectedSummary.items.map((i) => ({
            id: i.seat.id,
            label: i.seat.label,
            zoneName: i.zone.name,
            priceCents: i.zone.price_cents,
          })),
        })
      );
    }
    router.push(`/checkout?event_id=${encodeURIComponent(eventId)}`);
  };

  if (loading) {
    return (
      <div className="h-96 animate-pulse rounded-2xl border border-slate-800 bg-slate-900/60" />
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-white">
            {event?.title || "Chọn ghế sự kiện"}
          </h1>
          <p className="text-sm text-slate-400">{event?.venue}</p>
        </div>

        <span
          className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium ${
            connectionState === "connected"
              ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300"
              : "border-amber-500/40 bg-amber-500/10 text-amber-300"
          }`}
        >
          {connectionState === "connected" ? (
            <Wifi className="h-3.5 w-3.5" />
          ) : (
            <WifiOff className="h-3.5 w-3.5" />
          )}
          WS: {connectionState}
        </span>
      </div>

      {toast && (
        <div className="flex items-center gap-2 rounded-xl border border-amber-500/40 bg-amber-950/40 px-4 py-3 text-sm text-amber-200">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{toast}</span>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-12">
        <div className="lg:col-span-8">
          <SeatMap
            onSeatClick={handleSeatClick}
            conflictSeatId={conflictSeatId}
          />
        </div>

        <aside className="space-y-4 lg:col-span-4">
          <div className="sticky top-24 space-y-4 rounded-2xl border border-slate-800 bg-slate-900/85 p-5">
            <h3 className="text-base font-bold text-white">
              Ghế đang giữ ({selectedSummary.items.length}/
              {event?.max_seats_per_order || 4})
            </h3>

            <SeatTimer onExpired={handleTimerExpired} />

            {selectedSummary.items.length === 0 ? (
              <p className="rounded-xl border border-dashed border-slate-700 p-4 text-center text-xs text-slate-400">
                Chọn khu vực bên trái rồi bấm vào ghế màu xanh để giữ chỗ
                nguyên tử trong 300s.
              </p>
            ) : (
              <ul className="space-y-2">
                {selectedSummary.items.map(({ seat, zone }) => (
                  <li
                    key={seat.id}
                    className="flex items-center justify-between rounded-lg border border-slate-800 bg-slate-950/70 px-3 py-2 text-sm"
                  >
                    <div>
                      <p className="font-semibold text-white">{seat.label}</p>
                      <p className="text-xs text-slate-400">{zone.name}</p>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-emerald-300">
                        ${(zone.price_cents / 100).toFixed(2)}
                      </span>
                      <button
                        type="button"
                        onClick={() => void handleSeatClick(seat)}
                        aria-label={`Hủy giữ ${seat.label}`}
                        className="text-slate-400 hover:text-rose-400"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}

            <div className="flex items-center justify-between border-t border-slate-800 pt-4 text-sm">
              <span className="text-slate-400">Tạm tính</span>
              <span className="text-xl font-extrabold text-white">
                ${(selectedSummary.totalCents / 100).toFixed(2)}
              </span>
            </div>

            <button
              type="button"
              disabled={selectedSummary.items.length === 0}
              onClick={handleProceedCheckout}
              className="flex w-full items-center justify-center gap-2 rounded-xl bg-indigo-600 py-3 text-sm font-semibold text-white transition hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Tiếp tục thanh toán
              <ArrowRight className="h-4 w-4" />
            </button>
          </div>
        </aside>
      </div>
    </div>
  );
}
