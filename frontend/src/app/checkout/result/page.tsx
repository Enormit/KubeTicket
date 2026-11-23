"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import {
  CheckCircle2,
  Clock,
  CreditCard,
  Loader2,
  XCircle,
} from "lucide-react";
import { api, APIError } from "@/lib/api";
import type { Order } from "@/lib/types";
import { useSeatSocket } from "@/lib/use-seat-socket";

function CheckoutResultInner() {
  const searchParams = useSearchParams();
  const orderId = searchParams.get("order_id") || "";
  const eventId = searchParams.get("event_id") || "";
  const amountParam = Number(searchParams.get("amount") || "0");

  const [order, setOrder] = useState<Order | null>(null);
  const [paying, setPaying] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchOrder = useCallback(async () => {
    if (!orderId) return;
    try {
      const res = await api.orders.list();
      const found = (res.orders || []).find((o) => o.id === orderId) || null;
      if (found) setOrder(found);
    } catch {
      // Order worker might still be consuming message
    }
  }, [orderId]);

  useSeatSocket({
    eventId,
    onOrderConfirmed: (confirmedOrderId) => {
      if (confirmedOrderId === orderId) {
        void fetchOrder();
      }
    },
  });

  useEffect(() => {
    if (!orderId) return;
    void fetchOrder();
    const id = setInterval(() => {
      void fetchOrder();
    }, 1500);
    return () => clearInterval(id);
  }, [fetchOrder, orderId]);

  const handleSandboxPay = async () => {
    if (!orderId) return;
    setError(null);
    setPaying(true);
    try {
      await api.checkout.sandboxComplete(
        orderId,
        order?.total_cents || amountParam
      );
      await fetchOrder();
    } catch (err) {
      setError(
        err instanceof APIError
          ? err.message
          : "Không thể giả lập thanh toán (chờ worker ghi đơn rồi thử lại)"
      );
    } finally {
      setPaying(false);
    }
  };

  if (!orderId) {
    return (
      <div className="mx-auto max-w-lg kt-card p-8 text-center">
        <p className="text-sm text-kt-ink">Thiếu mã đơn hàng.</p>
        <Link
          href="/"
          className="mt-4 inline-block text-sm font-bold text-kt-ink underline"
        >
          Về trang chủ
        </Link>
      </div>
    );
  }

  const status = order?.status || "PENDING";

  return (
    <div className="mx-auto max-w-xl py-4">
      <section className="kt-card space-y-6 p-6 sm:p-8">
        {status === "COMPLETED" ? (
          <div className="flex items-start gap-3 rounded-[3px] border-2 border-kt-emerald bg-kt-emerald p-4 text-white">
            <CheckCircle2 className="mt-0.5 h-7 w-7 shrink-0" />
            <div>
              <h1 className="font-display text-3xl font-extrabold uppercase tracking-tight">
                Thanh toán thành công
              </h1>
              <p className="mt-0.5 text-sm">
                Vé điện tử QR HMAC-SHA256 đã phát hành vào ví vé của bạn.
              </p>
            </div>
          </div>
        ) : status === "EXPIRED" ||
          status === "CANCELLED" ||
          status === "REFUNDED_EXPIRED" ? (
          <div className="flex items-start gap-3 rounded-[3px] border-2 border-kt-danger bg-kt-danger p-4 text-white">
            <XCircle className="mt-0.5 h-7 w-7 shrink-0" />
            <div>
              <h1 className="font-display text-3xl font-extrabold uppercase tracking-tight">
                Đơn đã đóng ({status})
              </h1>
              <p className="mt-0.5 text-sm">
                {status === "REFUNDED_EXPIRED"
                  ? "Thanh toán đến trễ sau khi hết hạn giữ ghế — hệ thống đã hoàn tiền tự động."
                  : "Ghế đã được trả về trạng thái trống cho người dùng khác."}
              </p>
            </div>
          </div>
        ) : (
          <div className="flex items-start gap-3 rounded-[3px] border-2 border-kt-ink bg-kt-ground p-4 text-kt-ink">
            <Loader2 className="mt-1 h-6 w-6 shrink-0 animate-spin text-kt-accent" />
            <div>
              <h1 className="font-display text-3xl font-extrabold uppercase tracking-tight">
                Đang chờ xác nhận thanh toán
              </h1>
              <p className="mt-1 text-xs text-kt-muted">
                Order Worker đang xử lý bất đồng bộ đơn{" "}
                <span className="font-mono font-bold text-kt-ink">
                  {orderId}
                </span>
                .
              </p>
            </div>
          </div>
        )}

        {order && (
          <dl className="space-y-2 rounded-[3px] border-2 border-kt-ink bg-kt-ground p-4 text-sm">
            <div className="flex justify-between gap-2">
              <dt className="text-kt-muted">Mã đơn</dt>
              <dd className="truncate font-mono text-xs font-bold text-kt-ink">
                {order.id}
              </dd>
            </div>
            <div className="flex justify-between gap-2">
              <dt className="text-kt-muted">Trạng thái</dt>
              <dd className="font-mono font-bold text-kt-ink">
                {order.status}
              </dd>
            </div>
            <div className="flex justify-between gap-2">
              <dt className="text-kt-muted">Tổng thanh toán</dt>
              <dd className="font-mono font-extrabold text-kt-ink kt-tabular">
                ${(order.total_cents / 100).toFixed(2)}
              </dd>
            </div>
          </dl>
        )}

        {error && (
          <div
            role="alert"
            className="rounded-[3px] border-2 border-kt-danger bg-kt-ground p-3 text-sm font-medium text-kt-danger"
          >
            {error}
          </div>
        )}

        {status === "PENDING" && (
          <div className="kt-perforated-top space-y-3 pt-5">
            <p className="flex items-center gap-1.5 text-xs font-medium text-kt-muted">
              <Clock className="h-3.5 w-3.5 text-kt-ink" />
              Môi trường Sandbox: bấm nút dưới để gửi Webhook xác nhận thanh
              toán tới Order Worker.
            </p>
            <button
              type="button"
              disabled={paying || !order}
              onClick={handleSandboxPay}
              className="flex w-full items-center justify-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-emerald py-3 text-sm font-extrabold uppercase tracking-wide text-white transition hover:bg-kt-ink disabled:opacity-60"
            >
              <CreditCard className="h-4 w-4" />
              {!order
                ? "Đang chờ worker tạo đơn PENDING..."
                : paying
                  ? "Đang xử lý webhook..."
                  : "Hoàn tất thanh toán Sandbox"}
            </button>
          </div>
        )}

        <div className="grid grid-cols-2 gap-3">
          <Link
            href="/tickets"
            className="rounded-[3px] border-2 border-kt-ink bg-kt-accent py-2.5 text-center text-sm font-extrabold uppercase text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface"
          >
            Xem vé của tôi
          </Link>
          <Link
            href="/"
            className="rounded-[3px] border-2 border-kt-ink bg-kt-surface py-2.5 text-center text-sm font-bold uppercase text-kt-ink transition hover:bg-kt-alt"
          >
            Về trang chủ
          </Link>
        </div>
      </section>
    </div>
  );
}

export default function CheckoutResultPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-xl kt-card p-8 text-sm text-kt-muted">
          Đang kiểm tra trạng thái đơn hàng...
        </div>
      }
    >
      <CheckoutResultInner />
    </Suspense>
  );
}
