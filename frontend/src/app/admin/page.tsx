"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Camera, CheckCircle2, ShieldAlert, XCircle } from "lucide-react";
import { api, APIError } from "@/lib/api";
import { shouldProcessQRScan } from "@/lib/scanner-debounce";
import { useAuth } from "@/components/AuthProvider";

interface ScanLog {
  id: string;
  ok: boolean;
  message: string;
  gate: string;
  timestamp: string;
}

function playFeedbackTone(success: boolean): void {
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
    osc.type = success ? "sine" : "sawtooth";
    osc.frequency.setValueAtTime(success ? 880 : 210, ctx.currentTime);
    gain.gain.setValueAtTime(0.1, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.25);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.25);
  } catch {
    // Ignore browser audio blocks
  }
}

export default function AdminCheckinPage() {
  const { user, loading } = useAuth();
  const [gate, setGate] = useState("MAIN");
  const [manualToken, setManualToken] = useState("");
  const [scanning, setScanning] = useState(false);
  const [cameraError, setCameraError] = useState<string | null>(null);
  const [logs, setLogs] = useState<ScanLog[]>([]);

  const lastScanRef = useRef<{ token: string; atMs: number }>({
    token: "",
    atMs: 0,
  });
  const scannerCleanupRef = useRef<(() => Promise<void>) | null>(null);

  const submitCheckin = useCallback(
    async (qrToken: string) => {
      const now = Date.now();
      if (
        !shouldProcessQRScan(
          qrToken,
          lastScanRef.current.token,
          lastScanRef.current.atMs,
          now,
          1500
        )
      ) {
        return;
      }
      lastScanRef.current = { token: qrToken.trim(), atMs: now };

      try {
        const res = await api.admin.checkin(qrToken.trim(), gate);
        playFeedbackTone(true);
        setLogs((prev) => [
          {
            id: `${Date.now()}`,
            ok: true,
            message: `Hợp lệ — Vé ${res.checkin.ticket_id} qua cổng`,
            gate: res.checkin.gate,
            timestamp: new Date().toLocaleTimeString("vi-VN"),
          },
          ...prev.slice(0, 19),
        ]);
      } catch (err) {
        playFeedbackTone(false);
        const msg =
          err instanceof APIError ? err.message : "Lỗi xác thực QR check-in";
        setLogs((prev) => [
          {
            id: `${Date.now()}`,
            ok: false,
            message: msg,
            gate,
            timestamp: new Date().toLocaleTimeString("vi-VN"),
          },
          ...prev.slice(0, 19),
        ]);
      }
    },
    [gate]
  );

  const stopCamera = useCallback(async () => {
    if (scannerCleanupRef.current) {
      await scannerCleanupRef.current();
      scannerCleanupRef.current = null;
    }
    setScanning(false);
  }, []);

  const startCamera = async () => {
    setCameraError(null);
    try {
      const { Html5Qrcode } = await import("html5-qrcode");
      const scanner = new Html5Qrcode("qr-reader");
      await scanner.start(
        { facingMode: "environment" },
        { fps: 10, qrbox: { width: 240, height: 240 } },
        (decodedText) => {
          void submitCheckin(decodedText);
        },
        () => {}
      );
      scannerCleanupRef.current = async () => {
        try {
          await scanner.stop();
          scanner.clear();
        } catch {
          // Ignore cleanup errors
        }
      };
      setScanning(true);
    } catch (err) {
      setCameraError(
        err instanceof Error
          ? `Không bật được camera: ${err.message}`
          : "Không thể truy cập camera, hãy dùng ô nhập mã thủ công."
      );
      setScanning(false);
    }
  };

  useEffect(() => {
    return () => {
      if (scannerCleanupRef.current) {
        void scannerCleanupRef.current();
      }
    };
  }, []);

  if (loading) {
    return (
      <div className="h-56 animate-pulse rounded-[4px] border-2 border-kt-rule bg-kt-surface" />
    );
  }

  if (!user || user.role !== "ADMIN") {
    return (
      <div className="mx-auto max-w-lg kt-card p-6 text-sm text-kt-danger">
        <p className="flex items-center gap-2 font-bold">
          <ShieldAlert className="h-5 w-5" />
          Chỉ tài khoản ADMIN mới được truy cập cổng soát vé sự kiện.
        </p>
      </div>
    );
  }

  const latest = logs[0];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4 border-b-2 border-kt-ink pb-4">
        <h1 className="font-display text-4xl font-extrabold uppercase tracking-tight text-kt-ink">
          Cổng soát vé QR (Turnstile)
        </h1>
        <label className="flex items-center gap-2 text-xs font-bold uppercase text-kt-ink">
          <span>Cổng:</span>
          <select
            value={gate}
            onChange={(e) => setGate(e.target.value)}
            className="rounded-[3px] border-2 border-kt-ink bg-kt-surface px-3 py-1.5 font-mono text-xs font-bold text-kt-ink"
          >
            <option value="MAIN">MAIN</option>
            <option value="NORTH-1">NORTH-1</option>
            <option value="VIP-GATE">VIP-GATE</option>
          </select>
        </label>
      </div>

      {latest && (
        <div
          role="status"
          className={`flex items-center gap-3 rounded-[4px] border-2 border-kt-ink px-5 py-4 text-base font-extrabold text-white ${
            latest.ok ? "bg-kt-emerald" : "bg-kt-danger"
          }`}
        >
          {latest.ok ? (
            <CheckCircle2 className="h-6 w-6 shrink-0" />
          ) : (
            <XCircle className="h-6 w-6 shrink-0" />
          )}
          <span className="flex-1">{latest.message}</span>
          <span className="font-mono text-xs opacity-90 kt-tabular">
            {latest.gate} • {latest.timestamp}
          </span>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-12">
        <section className="kt-card space-y-5 p-6 lg:col-span-6">
          <div
            id="qr-reader"
            className="min-h-[240px] overflow-hidden rounded-[3px] border-2 border-kt-ink bg-kt-ink"
          />

          {cameraError && (
            <p className="text-xs font-medium text-kt-amber">{cameraError}</p>
          )}

          <button
            type="button"
            onClick={() => (scanning ? void stopCamera() : void startCamera())}
            className="flex w-full items-center justify-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-accent py-3 text-sm font-extrabold uppercase tracking-wide text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface"
          >
            <Camera className="h-4 w-4" />
            {scanning ? "Tắt Camera Quét QR" : "Bật Camera Quét QR"}
          </button>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              void submitCheckin(manualToken);
            }}
            className="kt-perforated-top space-y-2.5 pt-4"
          >
            <label
              htmlFor="manual-qr-input"
              className="block text-xs font-bold uppercase tracking-wider text-kt-muted"
            >
              Nhập / Dán mã QR Token thủ công (Debounce 1.5s)
            </label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <input
                id="manual-qr-input"
                type="text"
                value={manualToken}
                onChange={(e) => setManualToken(e.target.value)}
                placeholder="Dán qr_token..."
                className="min-w-0 flex-1 rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3.5 py-2 font-mono text-xs text-kt-ink placeholder:text-kt-muted"
              />
              <button
                type="submit"
                className="shrink-0 rounded-[3px] border-2 border-kt-ink bg-kt-emerald px-4 py-2 text-xs font-extrabold uppercase text-white transition hover:bg-kt-ink"
              >
                Check-in
              </button>
            </div>
          </form>
        </section>

        <section className="kt-card space-y-4 p-6 lg:col-span-6">
          <h2 className="font-display text-2xl font-extrabold uppercase tracking-tight text-kt-ink">
            Nhật ký soát vé phiên hiện tại
          </h2>
          {logs.length === 0 ? (
            <p className="rounded-[3px] border-2 border-dashed border-kt-rule bg-kt-ground p-6 text-center text-sm text-kt-muted">
              Sẵn sàng quét vé. Kết quả hợp lệ hoặc từ chối sẽ ghi nhận tức thì
              tại đây.
            </p>
          ) : (
            <ul className="space-y-2.5">
              {logs.map((entry) => (
                <li
                  key={entry.id}
                  className={`flex items-start gap-2.5 rounded-[3px] border-2 p-3 text-sm ${
                    entry.ok
                      ? "border-kt-emerald bg-kt-ground text-kt-ink"
                      : "border-kt-danger bg-kt-ground text-kt-danger"
                  }`}
                >
                  {entry.ok ? (
                    <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-kt-emerald" />
                  ) : (
                    <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-kt-danger" />
                  )}
                  <div className="flex-1">
                    <p className="font-semibold">{entry.message}</p>
                    <p className="font-mono text-xs text-kt-muted kt-tabular">
                      {entry.gate} • {entry.timestamp}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}
