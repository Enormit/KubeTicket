"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Calendar, Clock, MapPin, Sparkles, ArrowRight } from "lucide-react";
import { api } from "@/lib/api";
import type { Event } from "@/lib/types";

export default function HomePage() {
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    api.events
      .list()
      .then((res) => {
        if (active) setEvents(res.events || []);
      })
      .catch((err: unknown) => {
        if (active) {
          setError(
            err instanceof Error ? err.message : "Không thể tải danh sách sự kiện"
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

  return (
    <div className="space-y-10">
      <section className="relative overflow-hidden rounded-2xl border border-slate-800 bg-gradient-to-br from-indigo-950/60 via-slate-900 to-slate-950 p-8 sm:p-12">
        <div className="max-w-2xl space-y-4">
          <span className="inline-flex items-center gap-1.5 rounded-full border border-indigo-500/40 bg-indigo-500/10 px-3 py-1 text-xs font-medium text-indigo-300">
            <Sparkles className="h-3.5 w-3.5" />
            Zero Overselling • Real-time Seat Map • Virtual Waiting Room
          </span>
          <h1 className="text-3xl font-extrabold tracking-tight text-white sm:text-5xl">
            Săn vé sự kiện Flash Sale mượt mà & công bằng
          </h1>
          <p className="text-slate-300">
            Khóa ghế nguyên tử bằng Redis Lua, cập nhật sơ đồ ghế theo thời gian
            thực qua WebSocket và xác thực vé điện tử QR HMAC-SHA256.
          </p>
        </div>
      </section>

      <section className="space-y-5">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold text-white">Sự kiện đang & sắp mở bán</h2>
          <span className="text-xs text-slate-400">
            Cập nhật trực tiếp từ Gateway API
          </span>
        </div>

        {loading && (
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {[1, 2, 3].map((idx) => (
              <div
                key={idx}
                className="h-64 animate-pulse rounded-xl border border-slate-800 bg-slate-900/60"
              />
            ))}
          </div>
        )}

        {error && (
          <div className="rounded-xl border border-rose-500/40 bg-rose-950/30 p-4 text-sm text-rose-200">
            {error}
          </div>
        )}

        {!loading && !error && events.length === 0 && (
          <div className="rounded-xl border border-slate-800 bg-slate-900/50 p-8 text-center text-slate-400">
            Chưa có sự kiện nào được mở.
          </div>
        )}

        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {events.map((ev) => {
            const onSale = ev.status === "ON_SALE";
            return (
              <article
                key={ev.id}
                className="flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/70 p-6 transition hover:border-indigo-500/50"
              >
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span
                      className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${
                        onSale
                          ? "bg-emerald-500/15 text-emerald-300"
                          : "bg-amber-500/15 text-amber-300"
                      }`}
                    >
                      {ev.status}
                    </span>
                    <span className="text-xs text-slate-400">
                      Tối đa {ev.max_seats_per_order} ghế/đơn
                    </span>
                  </div>

                  <h3 className="text-lg font-bold text-white">{ev.title}</h3>
                  <p className="line-clamp-2 text-sm text-slate-400">
                    {ev.description}
                  </p>

                  <div className="space-y-1.5 pt-2 text-xs text-slate-300">
                    <p className="flex items-center gap-2">
                      <MapPin className="h-3.5 w-3.5 text-indigo-400" />
                      {ev.venue}
                    </p>
                    <p className="flex items-center gap-2">
                      <Calendar className="h-3.5 w-3.5 text-indigo-400" />
                      Diễn ra: {new Date(ev.starts_at).toLocaleString("vi-VN")}
                    </p>
                    <p className="flex items-center gap-2">
                      <Clock className="h-3.5 w-3.5 text-indigo-400" />
                      Mở bán:{" "}
                      {new Date(ev.sale_starts_at).toLocaleString("vi-VN")}
                    </p>
                  </div>
                </div>

                <div className="mt-6 pt-4 border-t border-slate-800">
                  {onSale ? (
                    <Link
                      href={`/waiting-room?event_id=${encodeURIComponent(ev.id)}`}
                      className="flex w-full items-center justify-center gap-2 rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-indigo-500"
                    >
                      Vào phòng chờ mua vé
                      <ArrowRight className="h-4 w-4" />
                    </Link>
                  ) : (
                    <span className="flex w-full cursor-not-allowed items-center justify-center rounded-lg border border-slate-700 bg-slate-800/60 px-4 py-2.5 text-sm font-medium text-slate-400">
                      Chưa mở bán
                    </span>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      </section>
    </div>
  );
}
