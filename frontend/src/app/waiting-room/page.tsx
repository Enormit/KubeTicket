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
    // Ignore browser autoplay blocks
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
            // Keep polling on transient network blips
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
      <div className="mx-auto max-w-lg kt-card p-8 text-center">
        <p className="text-sm font-medium text-kt-ink">
          Thiếu mã sự kiện cần xếp hàng.
        </p>
        <Link
          href="/"
          className="mt-4 inline-block text-sm font-bold text-kt-ink underline underline-offset-4"
        >
          Quay lại lịch sự kiện
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl py-4">
      <section className="kt-card overflow-hidden">
        <div className="border-b-2 border-kt-ink bg-kt-ink px-6 py-4 text-kt-surface">
          <div className="flex items-center justify-between text-xs font-bold uppercase tracking-wider">
            <span className="inline-flex items-center gap-2">
              <Users className="h-4 w-4 text-kt-accent" />
              Cổng điều tiết lưu lượng FIFO
            </span>
            <span className="font-mono text-kt-alt">TURNSTILE QUEUE</span>
          </div>
        </div>

        <div className="p-6 sm:p-8">
          <h1 className="font-display text-4xl font-extrabold uppercase tracking-tight text-kt-ink">
            Phòng chờ ảo trực tuyến
          </h1>
          <p className="mt-1 text-sm text-kt-muted">
            Mỗi tài khoản giữ duy nhất một lượt hợp lệ theo thứ tự vào hàng.
          </p>

          {error ? (
            <div
              role="alert"
              className="mt-6 flex items-start gap-2.5 rounded-[3px] border-2 border-kt-danger bg-kt-ground p-4 text-sm text-kt-danger"
            >
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
              <div>
                <p className="font-bold">{error}</p>
                <Link
                  href="/"
                  className="mt-2 inline-block text-xs font-bold text-kt-ink underline"
                >
                  Quay về trang chủ
                </Link>
              </div>
            </div>
          ) : (
            <div className="mt-6 space-y-6">
              <div className="grid grid-cols-2 gap-4">
                <div className="rounded-[3px] border-2 border-kt-ink bg-kt-ground p-4">
                  <p className="text-xs font-bold uppercase tracking-wider text-kt-muted">
                    Thứ tự hiện tại
                  </p>
                  <p className="mt-1 font-mono text-4xl font-extrabold text-kt-ink kt-tabular">
                    {status ? `#${Math.max(1, status.position)}` : "..."}
                  </p>
                </div>
                <div className="rounded-[3px] border-2 border-kt-ink bg-kt-ground p-4">
                  <p className="flex items-center gap-1 text-xs font-bold uppercase tracking-wider text-kt-muted">
                    <Clock className="h-3.5 w-3.5" />
                    Thời gian chờ
                  </p>
                  <p className="mt-1 font-mono text-4xl font-extrabold text-kt-ink kt-tabular">
                    {status ? `${status.estimated_wait_sec}s` : "..."}
                  </p>
                </div>
              </div>

              <div className="kt-perforated-top pt-5">
                <div className="flex items-center gap-3 rounded-[3px] border-2 border-kt-emerald bg-kt-emerald px-4 py-3.5 text-sm font-medium text-white">
                  <Loader2 className="h-5 w-5 shrink-0 animate-spin" />
                  <span>
                    Đang giữ vị trí — Không đóng trình duyệt. Hệ thống sẽ tự
                    chuyển sang sơ đồ chọn ghế ngay khi cổng mở cho bạn.
                  </span>
                </div>
              </div>

              <button
                type="button"
                onClick={enableNotifications}
                className="flex w-full items-center justify-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-surface py-2.5 text-xs font-bold uppercase tracking-wider text-kt-ink transition hover:bg-kt-alt"
              >
                <Bell className="h-4 w-4" />
                Bật thông báo trình duyệt khi tới lượt
              </button>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}

export default function WaitingRoomPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-xl kt-card p-8 text-sm text-kt-muted">
          Đang kết nối cổng phòng chờ...
        </div>
      }
    >
      <WaitingRoomInner />
    </Suspense>
  );
}
