"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { CreditCard, ShieldCheck } from "lucide-react";
import { api, APIError } from "@/lib/api";
import { loadAdmissionToken } from "@/lib/multi-tab";
import { useSeatStore } from "@/lib/seat-store";
import { SeatTimer } from "@/components/SeatTimer";

interface CheckoutSummary {
  eventId: string;
  eventTitle: string;
  holdToken: string;
  seatIds: string[];
  totalCents: number;
  items: {
    id: string;
    label: string;
    zoneName: string;
    priceCents: number;
  }[];
}

function CheckoutInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const eventId = searchParams.get("event_id") || "";

  const [summary, setSummary] = useState<CheckoutSummary | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!eventId || typeof window === "undefined") return;
    const raw = window.sessionStorage.getItem(
      `kubeticket:checkout:${eventId}`
    );
    if (raw) {
      try {
        setSummary(JSON.parse(raw) as CheckoutSummary);
      } catch {
        setSummary(null);
      }
    }
  }, [eventId]);

  const handleConfirmOrder = async () => {
    if (!summary) return;
    setError(null);
    const admTok = loadAdmissionToken(summary.eventId);
    if (!admTok) {
      router.replace(
        `/waiting-room?event_id=${encodeURIComponent(summary.eventId)}`
      );
      return;
    }

    setSubmitting(true);
    try {
      const res = await api.checkout.submit(
        summary.eventId,
        summary.seatIds,
        summary.holdToken,
        admTok
      );
      useSeatStore.getState().clearMyHolds();
      if (typeof window !== "undefined") {
        window.sessionStorage.removeItem(
          `kubeticket:checkout:${summary.eventId}`
        );
      }
      router.replace(
        `/checkout/result?order_id=${encodeURIComponent(res.order_id)}&event_id=${encodeURIComponent(summary.eventId)}&amount=${summary.totalCents}`
      );
    } catch (err) {
      setError(
        err instanceof APIError
          ? err.message
          : "Không thể gửi yêu cầu thanh toán"
      );
    } finally {
      setSubmitting(false);
    }
  };

  if (!summary) {
    return (
      <div className="mx-auto max-w-lg rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-center">
        <p className="text-slate-300">Không tìm thấy giỏ ghế cần thanh toán.</p>
        <Link
          href="/"
          className="mt-4 inline-block text-sm text-indigo-400 hover:underline"
        >
          Về trang chủ
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl space-y-6 rounded-2xl border border-slate-800 bg-slate-900/85 p-8 shadow-xl">
      <div>
        <h1 className="text-2xl font-bold text-white">Xác nhận đơn đặt vé</h1>
        <p className="mt-1 text-sm text-slate-400">{summary.eventTitle}</p>
      </div>

      <SeatTimer />

      <ul className="space-y-2">
        {summary.items.map((item) => (
          <li
            key={item.id}
            className="flex items-center justify-between rounded-xl border border-slate-800 bg-slate-950/70 px-4 py-3 text-sm"
          >
            <div>
              <p className="font-semibold text-white">{item.label}</p>
              <p className="text-xs text-slate-400">{item.zoneName}</p>
            </div>
            <span className="font-mono text-emerald-300">
              ${(item.priceCents / 100).toFixed(2)}
            </span>
          </li>
        ))}
      </ul>

      <div className="flex items-center justify-between border-t border-slate-800 pt-4">
        <span className="text-slate-400">Tổng thanh toán</span>
        <span className="text-2xl font-extrabold text-white">
          ${(summary.totalCents / 100).toFixed(2)}
        </span>
      </div>

      {error && (
        <div className="rounded-xl border border-rose-500/40 bg-rose-950/40 p-3 text-sm text-rose-200">
          {error}
        </div>
      )}

      <div className="flex items-center gap-2 rounded-xl border border-indigo-500/30 bg-indigo-950/30 p-3 text-xs text-indigo-200">
        <ShieldCheck className="h-4 w-4 shrink-0 text-indigo-400" />
        <span>
          Khi bấm xác nhận, khóa ghế tự động nâng cấp từ 300s lên 900s để chờ
          xử lý giao dịch ACID an toàn.
        </span>
      </div>

      <button
        type="button"
        disabled={submitting}
        onClick={handleConfirmOrder}
        className="flex w-full items-center justify-center gap-2 rounded-xl bg-indigo-600 py-3 text-sm font-semibold text-white transition hover:bg-indigo-500 disabled:opacity-60"
      >
        <CreditCard className="h-4 w-4" />
        {submitting
          ? "Đang tạo đơn hàng..."
          : "Tạo đơn & Thanh toán (Stripe Sandbox)"}
      </button>
    </div>
  );
}

export default function CheckoutPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-xl rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-sm text-slate-400">
          Đang tải thông tin thanh toán...
        </div>
      }
    >
      <CheckoutInner />
    </Suspense>
  );
}
