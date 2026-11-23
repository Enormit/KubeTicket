"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import QRCode from "qrcode";
import { Copy, QrCode, Ticket as TicketIcon } from "lucide-react";
import { api, APIError } from "@/lib/api";
import type { Ticket } from "@/lib/types";

interface TicketWithQR {
  ticket: Ticket;
  qrToken?: string;
  qrDataUrl?: string;
}

export default function MyTicketsPage() {
  const [items, setItems] = useState<TicketWithQR[]>([]);
  const [loading, setLoading] = useState(true);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    api.tickets
      .list()
      .then(async (res) => {
        const list = res.tickets || [];
        const hydrated = await Promise.all(
          list.map(async (t): Promise<TicketWithQR> => {
            if (t.status !== "PAID" && t.status !== "CHECKED_IN") {
              return { ticket: t };
            }
            try {
              const qr = await api.tickets.getQR(t.id);
              const dataUrl = await QRCode.toDataURL(qr.qr_token, {
                width: 240,
                margin: 1,
                color: { dark: "#121212", light: "#FFFFFF" },
              });
              return {
                ticket: t,
                qrToken: qr.qr_token,
                qrDataUrl: dataUrl,
              };
            } catch {
              return { ticket: t };
            }
          })
        );
        if (active) setItems(hydrated);
      })
      .catch((err) => {
        if (active) {
          setError(
            err instanceof APIError ? err.message : "Không thể tải danh sách vé"
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

  const copyToken = async (ticketId: string, token: string) => {
    try {
      await navigator.clipboard.writeText(token);
      setCopiedId(ticketId);
      setTimeout(() => setCopiedId(null), 1500);
    } catch {
      // Ignore clipboard failures
    }
  };

  if (loading) {
    return (
      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        {[1, 2, 3].map((n) => (
          <div
            key={n}
            className="h-72 animate-pulse rounded-[4px] border-2 border-kt-rule bg-kt-surface"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b-2 border-kt-ink pb-4">
        <div className="flex items-center gap-2.5">
          <TicketIcon className="h-6 w-6 text-kt-ink" />
          <h1 className="font-display text-4xl font-extrabold uppercase tracking-tight text-kt-ink">
            Ví vé điện tử QR
          </h1>
        </div>
        <span className="font-mono text-xs font-bold uppercase text-kt-muted kt-tabular">
          {items.length} vé đã phát hành
        </span>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-[3px] border-2 border-kt-danger bg-kt-surface p-4 text-sm font-medium text-kt-danger"
        >
          {error}
        </div>
      )}

      {items.length === 0 ? (
        <div className="kt-card p-10 text-center">
          <p className="text-sm font-medium text-kt-muted">
            Bạn chưa sở hữu vé nào. Chọn sự kiện đang mở để bắt đầu đặt chỗ.
          </p>
          <Link
            href="/"
            className="mt-4 inline-block rounded-[3px] border-2 border-kt-ink bg-kt-accent px-5 py-2.5 text-xs font-extrabold uppercase text-kt-ink hover:bg-kt-ink hover:text-kt-surface"
          >
            Xem lịch sự kiện
          </Link>
        </div>
      ) : (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {items.map(({ ticket, qrToken, qrDataUrl }) => (
            <article
              key={ticket.id}
              className="kt-card flex flex-col justify-between overflow-hidden"
            >
              <div className="space-y-3 p-5">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-xs font-bold text-kt-muted">
                    {ticket.ticket_code}
                  </span>
                  <span
                    className={`rounded-[2px] px-2.5 py-0.5 font-mono text-[11px] font-bold uppercase ${
                      ticket.status === "PAID"
                        ? "bg-kt-emerald text-white"
                        : ticket.status === "CHECKED_IN"
                          ? "bg-kt-ink text-kt-surface"
                          : "bg-kt-amber text-white"
                    }`}
                  >
                    {ticket.status}
                  </span>
                </div>

                <h2 className="font-display text-2xl font-extrabold uppercase leading-tight text-kt-ink">
                  {ticket.event_title || ticket.event_id}
                </h2>

                <div className="inline-block rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3 py-1 font-mono text-sm font-bold text-kt-ink">
                  GHẾ {ticket.seat_label || ticket.seat_id}
                </div>
              </div>

              {qrDataUrl && qrToken ? (
                <div className="kt-perforated-top bg-kt-ground p-5 text-center">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={qrDataUrl}
                    alt={`Mã QR vé ${ticket.ticket_code}`}
                    className="mx-auto h-44 w-44 rounded-[3px] border-2 border-kt-ink bg-white p-2"
                  />
                  <button
                    type="button"
                    onClick={() => void copyToken(ticket.id, qrToken)}
                    className="mt-3 inline-flex items-center gap-1.5 rounded-[3px] border-2 border-kt-ink bg-kt-surface px-3.5 py-1.5 text-xs font-bold text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface"
                  >
                    <Copy className="h-3.5 w-3.5" />
                    {copiedId === ticket.id
                      ? "Đã sao chép QR Token"
                      : "Sao chép QR Token"}
                  </button>
                </div>
              ) : (
                <div className="kt-perforated-top flex items-center justify-center gap-2 bg-kt-ground p-5 text-xs font-medium text-kt-muted">
                  <QrCode className="h-4 w-4" />
                  QR mở khóa sau khi thanh toán hoàn tất
                </div>
              )}
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
