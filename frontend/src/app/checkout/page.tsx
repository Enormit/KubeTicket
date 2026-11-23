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
  expiresAt?: string | null;
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
      <div className="mx-auto max-w-lg kt-card p-8 text-center">
        <p className="text-sm font-medium text-kt-ink">
          Không tìm thấy ghế đang giữ để thanh toán.
        </p>
        <Link
          href="/"
          className="mt-4 inline-block text-sm font-bold text-kt-ink underline underline-offset-4"
        >
          Quay về lịch sự kiện
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl py-4">
      <section className="kt-card space-y-6 p-6 sm:p-8">
        <div className="border-b-2 border-kt-ink pb-4">
          <h1 className="font-display text-4xl font-extrabold uppercase tracking-tight text-kt-ink">
            Xác nhận đơn đặt vé
          </h1>
          <p className="mt-1 text-sm font-medium text-kt-muted">
            {summary.eventTitle}
          </p>
        </div>

        <SeatTimer fallbackExpiresAt={summary.expiresAt} />

        <ul className="space-y-2">
          {summary.items.map((item) => (
            <li
              key={item.id}
              className="flex items-center justify-between rounded-[3px] border-2 border-kt-ink bg-kt-ground px-4 py-3 text-sm"
            >
              <div>
                <p className="font-mono font-bold text-kt-ink">{item.label}</p>
                <p className="text-xs text-kt-muted">{item.zoneName}</p>
              </div>
              <span className="font-mono font-bold text-kt-ink kt-tabular">
                ${(item.priceCents / 100).toFixed(2)}
              </span>
            </li>
          ))}
        </ul>

        <div className="kt-perforated-top flex items-center justify-between pt-4">
          <span className="text-xs font-bold uppercase tracking-wider text-kt-muted">
            Tổng thanh toán
          </span>
          <span className="font-mono text-3xl font-extrabold text-kt-ink kt-tabular">
            ${(summary.totalCents / 100).toFixed(2)}
          </span>
        </div>

        {error && (
          <div
            role="alert"
            className="rounded-[3px] border-2 border-kt-danger bg-kt-ground p-3 text-sm font-medium text-kt-danger"
          >
            {error}
          </div>
        )}

        <div className="flex items-start gap-2.5 rounded-[3px] border-2 border-kt-emerald bg-kt-ground p-3.5 text-xs font-medium text-kt-ink">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-kt-emerald" />
          <span>
            Khi xác nhận, Redis khóa ghế được nâng hạn từ 300s lên 900s và đưa
            vào hàng đợi xử lý giao dịch ACID.
          </span>
        </div>

        <button
          type="button"
          disabled={submitting}
          onClick={handleConfirmOrder}
          className="flex w-full items-center justify-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-accent py-3.5 text-sm font-extrabold uppercase tracking-wide text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface disabled:opacity-60"
        >
          <CreditCard className="h-4 w-4" />
          {submitting
            ? "Đang tạo đơn hàng..."
            : "Tạo đơn & Thanh toán (Stripe Sandbox)"}
        </button>
      </section>
    </div>
  );
}

export default function CheckoutPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-xl kt-card p-8 text-sm text-kt-muted">
          Đang tải chi tiết thanh toán...
        </div>
      }
    >
      <CheckoutInner />
    </Suspense>
  );
}
