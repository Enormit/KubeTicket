"use client";

import { useEffect, useState } from "react";
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
                width: 220,
                margin: 1,
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
      // Ignore clipboard failure
    }
  };

  if (loading) {
    return (
      <div className="h-64 animate-pulse rounded-2xl border border-slate-800 bg-slate-900/60" />
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <TicketIcon className="h-6 w-6 text-indigo-400" />
        <h1 className="text-2xl font-bold text-white">Vé điện tử của tôi</h1>
      </div>

      {error && (
        <div className="rounded-xl border border-rose-500/40 bg-rose-950/40 p-4 text-sm text-rose-200">
          {error}
        </div>
      )}

      {items.length === 0 ? (
        <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-8 text-center text-slate-400">
          Bạn chưa có vé nào. Hãy chọn một sự kiện để đặt chỗ.
        </div>
      ) : (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {items.map(({ ticket, qrToken, qrDataUrl }) => (
            <article
              key={ticket.id}
              className="flex flex-col justify-between rounded-2xl border border-slate-800 bg-slate-900/80 p-6"
            >
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="font-mono text-xs text-slate-400">
                    {ticket.ticket_code}
                  </span>
                  <span
                    className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${
                      ticket.status === "PAID"
                        ? "bg-emerald-500/15 text-emerald-300"
                        : ticket.status === "CHECKED_IN"
                          ? "bg-sky-500/15 text-sky-300"
                          : "bg-amber-500/15 text-amber-300"
                    }`}
                  >
                    {ticket.status}
                  </span>
                </div>
                <h2 className="text-lg font-bold text-white">
                  {ticket.event_title || ticket.event_id}
                </h2>
                <p className="text-sm text-indigo-300">
                  Ghế: {ticket.seat_label || ticket.seat_id}
                </p>
              </div>

              {qrDataUrl && qrToken ? (
                <div className="mt-5 space-y-3 border-t border-slate-800 pt-4 text-center">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={qrDataUrl}
                    alt={`QR ${ticket.ticket_code}`}
                    className="mx-auto h-44 w-44 rounded-lg bg-white p-2"
                  />
                  <button
                    type="button"
                    onClick={() => void copyToken(ticket.id, qrToken)}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-slate-700 px-3 py-1.5 text-xs text-slate-300 hover:border-slate-500 hover:text-white"
                  >
                    <Copy className="h-3.5 w-3.5" />
                    {copiedId === ticket.id
                      ? "Đã copy mã QR Token!"
                      : "Copy QR Token"}
                  </button>
                </div>
              ) : (
                <div className="mt-5 flex items-center justify-center gap-2 rounded-xl border border-dashed border-slate-700 p-4 text-xs text-slate-400">
                  <QrCode className="h-4 w-4" />
                  QR khả dụng sau khi thanh toán hoàn tất
                </div>
              )}
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
