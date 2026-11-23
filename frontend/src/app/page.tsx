"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ArrowRight, Calendar, MapPin, ShieldCheck } from "lucide-react";
import { api } from "@/lib/api";
import type { Event, SeatSnapshotResponse } from "@/lib/types";
import { getZoneTurnstileColor } from "@/lib/zone-colors";

function pickFlagshipEvent(events: Event[]): Event | undefined {
  if (events.length === 0) return undefined;
  const onSale = events.filter((e) => e.status === "ON_SALE");
  if (onSale.length === 0) return events[0];
  return (
    onSale.find((e) => e.title.toLowerCase().includes("kubefest")) || onSale[0]
  );
}

export default function HomePage() {
  const [events, setEvents] = useState<Event[]>([]);
  const [featuredSnapshot, setFeaturedSnapshot] =
    useState<SeatSnapshotResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    api.events
      .list()
      .then(async (res) => {
        const list = res.events || [];
        if (!active) return;
        setEvents(list);
        const primary = pickFlagshipEvent(list);
        if (primary) {
          try {
            const snap = await api.events.snapshot(primary.id);
            if (active) setFeaturedSnapshot(snap);
          } catch {
            // Snapshot preview is optional if primary event has no seats yet
          }
        }
      })
      .catch((err: unknown) => {
        if (active) {
          setError(
            err instanceof Error
              ? err.message
              : "Không thể tải lịch phát hành vé"
          );
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, []);

  const { featured, secondary } = useMemo(() => {
    const main = pickFlagshipEvent(events) || null;
    if (!main) return { featured: null, secondary: [] as Event[] };
    return {
      featured: main,
      secondary: events.filter((e) => e.id !== main.id),
    };
  }, [events]);

  const zoneAvailability = useMemo(() => {
    if (!featuredSnapshot) return [];
    const counts = new Map<string, { avail: number; total: number }>();
    for (const s of featuredSnapshot.seats) {
      const prev = counts.get(s.zone_id) || { avail: 0, total: 0 };
      prev.total += 1;
      if (s.status === "AVAILABLE") prev.avail += 1;
      counts.set(s.zone_id, prev);
    }
    return featuredSnapshot.zones.map((z) => ({
      zone: z,
      avail: counts.get(z.id)?.avail ?? 0,
      total: counts.get(z.id)?.total ?? 0,
    }));
  }, [featuredSnapshot]);

  const totalSeats = featuredSnapshot?.seats.length ?? 0;
  const availSeats = useMemo(
    () =>
      featuredSnapshot?.seats.filter((s) => s.status === "AVAILABLE").length ??
      0,
    [featuredSnapshot]
  );

  if (loading) {
    return (
      <div className="grid gap-6 lg:grid-cols-12">
        <div className="h-[460px] animate-pulse rounded-[4px] border-2 border-kt-rule bg-kt-surface lg:col-span-7" />
        <div className="h-[460px] animate-pulse rounded-[4px] border-2 border-kt-rule bg-kt-surface lg:col-span-5" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="kt-card p-6 text-sm text-kt-danger">
        <p className="font-bold">Lỗi kết nối phòng vé</p>
        <p className="mt-1">{error}</p>
      </div>
    );
  }

  return (
    <div className="space-y-10">
      {featured ? (
        <section
          aria-label="Sự kiện mở bán tâm điểm"
          className="gate-pass-opening grid gap-6 lg:grid-cols-[1.35fr_0.85fr] lg:items-stretch"
        >
          {/* Left: Flagship Gate Pass */}
          <div className="kt-card flex flex-col justify-between p-6 sm:p-8">
            <div>
              <h1 className="font-display text-4xl font-extrabold uppercase leading-[0.95] tracking-tight text-kt-ink sm:text-6xl">
                {featured.title}
              </h1>

              <div className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2 border-y-2 border-dashed border-kt-rule py-3 text-xs font-semibold text-kt-ink">
                <span className="inline-flex items-center gap-1.5 text-kt-emerald">
                  <span
                    className="h-2 w-2 rounded-full bg-kt-emerald"
                    aria-hidden="true"
                  />
                  {featured.status === "ON_SALE"
                    ? "CỔNG BÁN ĐANG MỞ — PHÒNG CHỜ FIFO"
                    : featured.status}
                </span>
                <span className="inline-flex items-center gap-1.5 text-kt-muted">
                  <MapPin className="h-3.5 w-3.5 text-kt-ink" />
                  {featured.venue}
                </span>
                <span className="inline-flex items-center gap-1.5 text-kt-muted kt-tabular">
                  <Calendar className="h-3.5 w-3.5 text-kt-ink" />
                  {new Date(featured.starts_at).toLocaleDateString("vi-VN")}
                </span>
              </div>

              <p className="mt-4 max-w-[65ch] text-sm leading-relaxed text-kt-muted sm:text-base">
                {featured.description} Khóa ghế nguyên tử 300 giây bằng Redis
                Lua, cập nhật trạng thái trực tiếp qua WebSocket và xác thực QR
                HMAC-SHA256.
              </p>

              {/* Real Zone Inventory Strip */}
              {zoneAvailability.length > 0 && (
                <div className="mt-6 grid gap-3 sm:grid-cols-3">
                  {zoneAvailability.map(({ zone, avail, total }) => (
                    <div
                      key={zone.id}
                      className="rounded-[3px] border-2 border-kt-ink bg-kt-ground p-3.5"
                    >
                      <div className="flex items-center justify-between text-xs font-bold uppercase text-kt-muted">
                        <span>{zone.code}</span>
                        <span className="font-mono kt-tabular">
                          {avail}/{total}
                        </span>
                      </div>
                      <p className="mt-1 font-mono text-xl font-bold text-kt-ink kt-tabular">
                        ${(zone.price_cents / 100).toFixed(2)}
                      </p>
                      <p className="mt-0.5 truncate text-xs text-kt-muted">
                        {zone.name}
                      </p>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="mt-8 space-y-3">
              {featured.status === "ON_SALE" ? (
                <Link
                  href={`/waiting-room?event_id=${encodeURIComponent(featured.id)}`}
                  className="group flex w-full items-center justify-between rounded-[3px] border-2 border-kt-ink bg-kt-accent px-5 py-4 text-base font-extrabold uppercase tracking-wide text-kt-ink transition duration-200 ease-expo-out hover:bg-kt-ink hover:text-kt-surface"
                >
                  <span>Vào phòng chờ mua vé ngay</span>
                  <span className="inline-flex items-center gap-2 font-mono text-sm kt-tabular">
                    CỔNG A
                    <ArrowRight className="h-5 w-5 transition-transform group-hover:translate-x-1" />
                  </span>
                </Link>
              ) : (
                <span className="flex w-full items-center justify-between rounded-[3px] border-2 border-kt-rule bg-kt-alt px-5 py-4 text-sm font-bold uppercase text-kt-muted">
                  <span>Đợt mở bán chưa bắt đầu</span>
                  <span>{featured.status}</span>
                </span>
              )}

              <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-kt-muted">
                <span className="inline-flex items-center gap-1.5">
                  <ShieldCheck className="h-4 w-4 text-kt-emerald" />
                  Tối đa {featured.max_seats_per_order} ghế/đơn • Zero
                  Overselling ACID
                </span>
                {featuredSnapshot && (
                  <span className="font-mono kt-tabular">
                    seq #{featuredSnapshot.seq} • {availSeats}/{totalSeats} ghế
                    trống
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Right: Obsidian Stadium Plate + Perforated Tear Strips */}
          <div className="flex flex-col gap-4">
            <div className="flex flex-1 flex-col justify-between rounded-[4px] border-2 border-kt-ink bg-kt-ink p-6 text-kt-surface">
              <div className="flex items-center justify-between border-b border-white/20 pb-3 text-xs font-bold uppercase tracking-wider text-kt-alt">
                <span>Cấu trúc khán đài</span>
                <span className="font-mono">{featured.venue}</span>
              </div>

              <div className="my-5 space-y-2.5">
                <div className="rounded-[2px] border border-white/30 bg-white/10 py-2 text-center font-mono text-xs font-bold tracking-widest text-kt-surface">
                  SÂN KHẤU CHÍNH
                </div>
                {zoneAvailability.map(({ zone, avail, total }, idx) => (
                  <div
                    key={zone.id}
                    className="flex items-center justify-between rounded-[2px] border-2 px-3.5 py-2.5 text-xs"
                    style={{
                      borderColor: getZoneTurnstileColor(zone.code, idx),
                    }}
                  >
                    <span className="font-bold text-kt-surface">
                      {zone.name}
                    </span>
                    <span className="font-mono text-kt-alt kt-tabular">
                      {avail}/{total} trống
                    </span>
                  </div>
                ))}
              </div>

              <p className="text-xs leading-relaxed text-kt-alt">
                Sơ đồ ghế cập nhật đồng bộ qua WebSocket và nâng cấp khóa lên
                900s ngay khi sang thanh toán.
              </p>
            </div>

            {secondary.map((ev) => {
              const onSale = ev.status === "ON_SALE";
              return (
                <article
                  key={ev.id}
                  className="kt-card flex flex-col justify-between gap-3 p-4 sm:flex-row sm:items-center"
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex items-center gap-2">
                      <span
                        className={`rounded-[2px] px-2 py-0.5 text-[11px] font-bold uppercase ${
                          onSale
                            ? "bg-kt-emerald text-white"
                            : "bg-kt-alt text-kt-muted"
                        }`}
                      >
                        {onSale ? "Đang mở bán" : "Sắp mở bán"}
                      </span>
                      <span className="truncate text-xs text-kt-muted">
                        {ev.venue}
                      </span>
                    </div>
                    <h2 className="truncate font-display text-2xl font-extrabold uppercase tracking-tight text-kt-ink">
                      {ev.title}
                    </h2>
                    <p className="text-xs text-kt-muted kt-tabular">
                      Diễn ra:{" "}
                      {new Date(ev.starts_at).toLocaleDateString("vi-VN")} • Tối
                      đa {ev.max_seats_per_order} ghế/đơn
                    </p>
                  </div>

                  {onSale ? (
                    <Link
                      href={`/waiting-room?event_id=${encodeURIComponent(ev.id)}`}
                      className="inline-flex shrink-0 items-center justify-center gap-1.5 rounded-[3px] border-2 border-kt-ink bg-kt-ink px-4 py-2.5 text-xs font-bold uppercase text-kt-surface transition hover:bg-kt-accent hover:text-kt-ink"
                    >
                      Vào hàng chờ
                      <ArrowRight className="h-3.5 w-3.5" />
                    </Link>
                  ) : (
                    <span className="shrink-0 rounded-[3px] border-2 border-kt-rule bg-kt-alt px-3.5 py-2 text-center text-xs font-bold uppercase text-kt-muted">
                      Mở{" "}
                      {new Date(ev.sale_starts_at).toLocaleDateString("vi-VN")}
                    </span>
                  )}
                </article>
              );
            })}
          </div>
        </section>
      ) : (
        <div className="kt-card p-8 text-center text-sm text-kt-muted">
          Hiện chưa có đợt phát hành vé nào được mở.
        </div>
      )}
    </div>
  );
}
