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
  const seq = useSeatStore((s) => s.seq);
  const seatsById = useSeatStore((s) => s.seatsById);
  const myHeldSeatIds = useSeatStore((s) => s.myHeldSeatIds);
  const myHoldToken = useSeatStore((s) => s.myHoldToken);
  const myHoldExpiresAt = useSeatStore((s) => s.myHoldExpiresAt);
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
            setToast(
              `Ghế ${seat.label} vừa có người khác giữ trước 1 mili-giây!`
            );
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
    setToast("Hết thời gian giữ ghế 300s. Đang đồng bộ lại sơ đồ...");
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
          expiresAt: myHoldExpiresAt,
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
      <div className="h-96 animate-pulse rounded-[4px] border-2 border-kt-rule bg-kt-surface" />
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4 border-b-2 border-kt-ink pb-4">
        <div>
          <h1 className="font-display text-3xl font-extrabold uppercase tracking-tight text-kt-ink sm:text-5xl">
            {event?.title || "Chọn ghế sự kiện"}
          </h1>
          <p className="mt-1 text-sm font-medium text-kt-muted">
            {event?.venue}
          </p>
        </div>

        <div className="flex items-center gap-3">
          <span className="font-mono text-xs font-bold text-kt-muted kt-tabular">
            seq #{seq}
          </span>
          <span
            className={`inline-flex items-center gap-1.5 rounded-[3px] border-2 border-kt-ink px-3 py-1 font-mono text-xs font-bold uppercase ${
              connectionState === "connected"
                ? "bg-kt-emerald text-white"
                : "bg-kt-amber text-white"
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
      </div>

      {toast && (
        <div
          role="status"
          className="flex items-center gap-2.5 rounded-[3px] border-2 border-kt-ink bg-kt-amber px-4 py-3 text-sm font-semibold text-white"
        >
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

        <aside className="lg:col-span-4">
          <div className="kt-card sticky top-24 space-y-4 p-5">
            <div className="flex items-center justify-between border-b-2 border-dashed border-kt-rule pb-3">
              <h3 className="font-display text-2xl font-extrabold uppercase tracking-tight text-kt-ink">
                Cuống vé đang giữ
              </h3>
              <span className="rounded-[2px] bg-kt-ink px-2 py-0.5 font-mono text-xs font-bold text-kt-surface kt-tabular">
                {selectedSummary.items.length}/
                {event?.max_seats_per_order || 4}
              </span>
            </div>

            <SeatTimer onExpired={handleTimerExpired} />

            {selectedSummary.items.length === 0 ? (
              <p className="rounded-[3px] border-2 border-dashed border-kt-rule bg-kt-ground p-4 text-center text-xs leading-relaxed text-kt-muted">
                Bấm vào một khu vực bên trái, sau đó chọn ghế màu xanh lục để
                khóa nguyên tử trong 300 giây.
              </p>
            ) : (
              <ul className="space-y-2">
                {selectedSummary.items.map(({ seat, zone }) => (
                  <li
                    key={seat.id}
                    className="flex items-center justify-between rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3 py-2.5 text-sm"
                  >
                    <div>
                      <p className="font-mono font-bold text-kt-ink">
                        {seat.label}
                      </p>
                      <p className="text-xs text-kt-muted">{zone.name}</p>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono font-bold text-kt-ink kt-tabular">
                        ${(zone.price_cents / 100).toFixed(2)}
                      </span>
                      <button
                        type="button"
                        onClick={() => void handleSeatClick(seat)}
                        aria-label={`Hủy giữ ghế ${seat.label}`}
                        className="rounded-[2px] p-1 text-kt-muted transition hover:bg-kt-danger hover:text-white"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}

            <div className="kt-perforated-top flex items-center justify-between pt-4 text-sm">
              <span className="font-bold uppercase tracking-wider text-kt-muted">
                Tạm tính
              </span>
              <span className="font-mono text-2xl font-extrabold text-kt-ink kt-tabular">
                ${(selectedSummary.totalCents / 100).toFixed(2)}
              </span>
            </div>

            <button
              type="button"
              disabled={selectedSummary.items.length === 0}
              onClick={handleProceedCheckout}
              className="flex w-full items-center justify-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-accent py-3.5 text-sm font-extrabold uppercase tracking-wide text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface disabled:cursor-not-allowed disabled:opacity-50"
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
