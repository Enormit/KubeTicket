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
      // Order worker might still be consuming SQS/Stream message
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
          : "Không thể giả lập thanh toán (đợi worker tạo đơn xong rồi thử lại)"
      );
    } finally {
      setPaying(false);
    }
  };

  if (!orderId) {
    return (
      <div className="mx-auto max-w-lg rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-center">
        <p className="text-slate-300">Thiếu mã đơn hàng.</p>
        <Link href="/" className="mt-4 inline-block text-indigo-400 underline">
          Về trang chủ
        </Link>
      </div>
    );
  }

  const status = order?.status || "PENDING";

  return (
    <div className="mx-auto max-w-xl space-y-6 rounded-2xl border border-slate-800 bg-slate-900/85 p-8 shadow-xl">
      {status === "COMPLETED" ? (
        <div className="flex items-center gap-3 text-emerald-300">
          <CheckCircle2 className="h-8 w-8 shrink-0" />
          <div>
            <h1 className="text-xl font-bold text-white">
              Thanh toán thành công!
            </h1>
            <p className="text-sm text-slate-300">
              Vé điện tử QR đã sẵn sàng trong mục Vé của tôi.
            </p>
          </div>
        </div>
      ) : status === "EXPIRED" ||
        status === "CANCELLED" ||
        status === "REFUNDED_EXPIRED" ? (
        <div className="flex items-center gap-3 text-rose-300">
          <XCircle className="h-8 w-8 shrink-0" />
          <div>
            <h1 className="text-xl font-bold text-white">
              Đơn hàng đã hết hạn ({status})
            </h1>
            <p className="text-sm text-slate-300">
              {status === "REFUNDED_EXPIRED"
                ? "Thanh toán đến trễ sau khi hết giờ — hệ thống đã tự động hoàn tiền."
                : "Ghế đã được trả về trạng thái trống cho người dùng khác."}
            </p>
          </div>
        </div>
      ) : (
        <div className="flex items-center gap-3 text-amber-300">
          <Loader2 className="h-8 w-8 animate-spin shrink-0" />
          <div>
            <h1 className="text-xl font-bold text-white">
              Đang chờ xác nhận thanh toán...
            </h1>
            <p className="text-sm text-slate-300">
              Order Worker đang xử lý bất đồng bộ đơn <code className="text-xs">{orderId}</code>.
            </p>
          </div>
        </div>
      )}

      {order && (
        <div className="rounded-xl border border-slate-800 bg-slate-950/70 p-4 text-sm space-y-2">
          <div className="flex justify-between">
            <span className="text-slate-400">Mã đơn</span>
            <span className="font-mono text-xs text-slate-200">{order.id}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-400">Trạng thái</span>
            <span className="font-semibold text-white">{order.status}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-400">Tổng tiền</span>
            <span className="font-mono text-emerald-300">
              ${(order.total_cents / 100).toFixed(2)}
            </span>
          </div>
        </div>
      )}

      {error && (
        <div className="rounded-xl border border-rose-500/40 bg-rose-950/40 p-3 text-sm text-rose-200">
          {error}
        </div>
      )}

      {status === "PENDING" && (
        <div className="space-y-3 rounded-xl border border-indigo-500/30 bg-indigo-950/20 p-4">
          <p className="flex items-center gap-1.5 text-xs text-indigo-200">
            <Clock className="h-3.5 w-3.5" />
            Môi trường Local Sandbox: Bấm nút dưới để kích hoạt webhook thanh
            toán thành công về Order Worker.
          </p>
          <button
            type="button"
            disabled={paying || !order}
            onClick={handleSandboxPay}
            className="flex w-full items-center justify-center gap-2 rounded-xl bg-emerald-600 py-2.5 text-sm font-semibold text-white transition hover:bg-emerald-500 disabled:opacity-60"
          >
            <CreditCard className="h-4 w-4" />
            {!order
              ? "Đang chờ worker ghi đơn PENDING..."
              : paying
                ? "Đang gửi webhook..."
                : "Hoàn tất thanh toán Sandbox"}
          </button>
        </div>
      )}

      <div className="flex gap-3">
        <Link
          href="/tickets"
          className="flex-1 rounded-xl bg-indigo-600 py-2.5 text-center text-sm font-semibold text-white transition hover:bg-indigo-500"
        >
          Xem vé của tôi
        </Link>
        <Link
          href="/"
          className="flex-1 rounded-xl border border-slate-700 py-2.5 text-center text-sm font-semibold text-slate-300 transition hover:border-slate-500"
        >
          Về trang chủ
        </Link>
      </div>
    </div>
  );
}

export default function CheckoutResultPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-xl rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-sm text-slate-400">
          Đang kiểm tra kết quả đơn hàng...
        </div>
      }
    >
      <CheckoutResultInner />
    </Suspense>
  );
}
