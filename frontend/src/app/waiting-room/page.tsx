"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Bell, Clock, Loader2, ShieldAlert, Users } from "lucide-react";
import { api, APIError } from "@/lib/api";
import {
  loadAdmissionToken,
  saveAdmissionToken,
  subscribeTabSync,
} from "@/lib/multi-tab";
import type { WaitingRoomStatus } from "@/lib/types";

function playAdmissionChime(): void {
  if (typeof window === "undefined") return;
  try {
    const AudioCtx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext?: typeof AudioContext })
        .webkitAudioContext;
    if (!AudioCtx) return;
    const ctx = new AudioCtx();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.setValueAtTime(660, ctx.currentTime);
    osc.frequency.setValueAtTime(880, ctx.currentTime + 0.12);
    gain.gain.setValueAtTime(0.08, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.35);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.35);
  } catch {
    // Ignore audio context autoplay blocks
  }
}

function WaitingRoomInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const eventId = searchParams.get("event_id") || "";

  const [status, setStatus] = useState<WaitingRoomStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleAdmitted = useCallback(
    (st: WaitingRoomStatus) => {
      if (st.admitted && st.admission_token) {
        saveAdmissionToken(eventId, st.admission_token, st.expires_at);
        playAdmissionChime();
        if (
          typeof Notification !== "undefined" &&
          Notification.permission === "granted"
        ) {
          new Notification("KubeTicket: Đến lượt bạn chọn ghế!");
        }
        router.replace(`/events/${encodeURIComponent(eventId)}/seats`);
      }
    },
    [eventId, router]
  );

  useEffect(() => {
    if (!eventId) return;

    const existing = loadAdmissionToken(eventId);
    if (existing) {
      router.replace(`/events/${encodeURIComponent(eventId)}/seats`);
      return;
    }

    const unsub = subscribeTabSync((ev) => {
      if (ev.type === "ADMISSION_UPDATED" && ev.eventId === eventId) {
        router.replace(`/events/${encodeURIComponent(eventId)}/seats`);
      }
    });

    let cancelled = false;
    let pollTimer: ReturnType<typeof setInterval> | null = null;

    const enterQueue = async () => {
      try {
        const joined = await api.waitingRoom.join(eventId);
        if (cancelled) return;
        setStatus(joined);
        if (joined.admitted) {
          handleAdmitted(joined);
          return;
        }
        pollTimer = setInterval(async () => {
          try {
            const next = await api.waitingRoom.status(eventId);
            if (cancelled) return;
            setStatus(next);
            if (next.admitted) {
              if (pollTimer) clearInterval(pollTimer);
              handleAdmitted(next);
            }
          } catch {
            // Keep polling on transient blips
          }
        }, 2000);
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof APIError
              ? err.message
              : "Không thể tham gia phòng chờ ảo"
          );
        }
      }
    };

    void enterQueue();

    return () => {
      cancelled = true;
      unsub();
      if (pollTimer) clearInterval(pollTimer);
    };
  }, [eventId, handleAdmitted, router]);

  const enableNotifications = () => {
    if (
      typeof Notification !== "undefined" &&
      Notification.permission === "default"
    ) {
      void Notification.requestPermission();
    }
  };

  if (!eventId) {
    return (
      <div className="mx-auto max-w-lg rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-center">
        <p className="text-slate-300">Thiếu mã sự kiện (`event_id`).</p>
        <Link
          href="/"
          className="mt-4 inline-block text-indigo-400 hover:underline"
        >
          Quay lại danh sách sự kiện
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl rounded-2xl border border-slate-800 bg-slate-900/85 p-8 shadow-xl">
      <div className="flex items-center gap-3">
        <span className="flex h-11 w-11 items-center justify-center rounded-xl bg-indigo-500/20 text-indigo-400">
          <Users className="h-6 w-6" />
        </span>
        <div>
          <h1 className="text-xl font-bold text-white">
            Phòng chờ ảo (Virtual Waiting Room)
          </h1>
          <p className="text-xs text-slate-400">
            Hệ thống điều tiết lưu lượng công bằng theo thứ tự FIFO
          </p>
        </div>
      </div>

      {error ? (
        <div className="mt-6 flex items-start gap-2 rounded-xl border border-rose-500/40 bg-rose-950/40 p-4 text-sm text-rose-200">
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <p>{error}</p>
            <Link
              href="/"
              className="mt-2 inline-block text-xs text-indigo-300 underline"
            >
              Về trang chủ
            </Link>
          </div>
        </div>
      ) : (
        <div className="mt-8 space-y-6">
          <div className="grid grid-cols-2 gap-4">
            <div className="rounded-xl border border-slate-800 bg-slate-950/70 p-4">
              <p className="text-xs text-slate-400">Vị trí của bạn</p>
              <p className="mt-1 text-3xl font-extrabold text-indigo-400">
                {status ? `#${status.position}` : "..."}
              </p>
            </div>
            <div className="rounded-xl border border-slate-800 bg-slate-950/70 p-4">
              <p className="flex items-center gap-1 text-xs text-slate-400">
                <Clock className="h-3.5 w-3.5" />
                Dự kiến chờ
              </p>
              <p className="mt-1 text-3xl font-extrabold text-white">
                {status ? `${status.estimated_wait_sec}s` : "..."}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-3 rounded-xl border border-emerald-500/30 bg-emerald-950/20 p-4 text-sm text-emerald-200">
            <Loader2 className="h-5 w-5 animate-spin text-emerald-400" />
            <span>
              Đang giữ vị trí hàng chờ — Vui lòng không tắt trình duyệt. Hệ
              thống sẽ tự chuyển vào trang chọn ghế khi tới lượt.
            </span>
          </div>

          <button
            type="button"
            onClick={enableNotifications}
            className="flex w-full items-center justify-center gap-2 rounded-lg border border-slate-700 bg-slate-800/50 py-2.5 text-xs font-medium text-slate-300 transition hover:border-slate-500 hover:text-white"
          >
            <Bell className="h-4 w-4" />
            Bật thông báo trình duyệt khi tới lượt
          </button>
        </div>
      )}
    </div>
  );
}

export default function WaitingRoomPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-xl rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-sm text-slate-400">
          Đang kết nối phòng chờ...
        </div>
      }
    >
      <WaitingRoomInner />
    </Suspense>
  );
}
