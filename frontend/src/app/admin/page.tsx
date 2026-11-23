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
            message: `Hợp lệ — Vé ${res.checkin.ticket_id} check-in thành công`,
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
          // Ignore stop cleanup issues
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
      <div className="h-48 animate-pulse rounded-2xl border border-slate-800 bg-slate-900/60" />
    );
  }

  if (!user || user.role !== "ADMIN") {
    return (
      <div className="mx-auto max-w-lg rounded-2xl border border-rose-500/40 bg-rose-950/30 p-6 text-sm text-rose-200">
        <p className="flex items-center gap-2 font-semibold">
          <ShieldAlert className="h-5 w-5" />
          Chỉ tài khoản ADMIN mới được phép soát vé tại cổng sự kiện.
        </p>
      </div>
    );
  }

  return (
    <div className="grid gap-6 lg:grid-cols-12">
      <section className="space-y-5 rounded-2xl border border-slate-800 bg-slate-900/85 p-6 lg:col-span-6">
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-bold text-white">
            Soát vé QR Cổng Sự Kiện
          </h1>
          <select
            value={gate}
            onChange={(e) => setGate(e.target.value)}
            className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-1.5 text-xs text-white"
          >
            <option value="MAIN">Cổng MAIN</option>
            <option value="NORTH-1">Cổng NORTH-1</option>
            <option value="VIP-GATE">Cổng VIP</option>
          </select>
        </div>

        <div className="space-y-3">
          <div
            id="qr-reader"
            className="min-h-[240px] overflow-hidden rounded-xl border border-slate-800 bg-slate-950"
          />

          {cameraError && (
            <p className="text-xs text-amber-300">{cameraError}</p>
          )}

          <button
            type="button"
            onClick={() => (scanning ? void stopCamera() : void startCamera())}
            className="flex w-full items-center justify-center gap-2 rounded-xl bg-indigo-600 py-2.5 text-sm font-semibold text-white transition hover:bg-indigo-500"
          >
            <Camera className="h-4 w-4" />
            {scanning ? "Tắt Camera Quét QR" : "Bật Camera Quét QR"}
          </button>
        </div>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            void submitCheckin(manualToken);
          }}
          className="space-y-2 border-t border-slate-800 pt-4"
        >
          <label className="block text-xs font-medium text-slate-400">
            Nhập / Dán mã QR Token thủ công (Debounce 1.5s chống quét trùng)
          </label>
          <div className="flex gap-2">
            <input
              type="text"
              value={manualToken}
              onChange={(e) => setManualToken(e.target.value)}
              placeholder="Dán qr_token..."
              className="flex-1 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white focus:border-indigo-500 focus:outline-none"
            />
            <button
              type="submit"
              className="rounded-lg bg-emerald-600 px-4 py-2 text-xs font-semibold text-white hover:bg-emerald-500"
            >
              Check-in
            </button>
          </div>
        </form>
      </section>

      <section className="space-y-4 rounded-2xl border border-slate-800 bg-slate-900/85 p-6 lg:col-span-6">
        <h2 className="text-base font-bold text-white">Lịch sử quét gần đây</h2>
        {logs.length === 0 ? (
          <p className="text-sm text-slate-400">
            Chưa có lượt soát vé nào trong phiên này.
          </p>
        ) : (
          <ul className="space-y-2.5">
            {logs.map((entry) => (
              <li
                key={entry.id}
                className={`flex items-start gap-2.5 rounded-xl border p-3 text-sm ${
                  entry.ok
                    ? "border-emerald-500/40 bg-emerald-950/30 text-emerald-200"
                    : "border-rose-500/40 bg-rose-950/30 text-rose-200"
                }`}
              >
                {entry.ok ? (
                  <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
                ) : (
                  <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
                )}
                <div className="flex-1">
                  <p className="font-medium">{entry.message}</p>
                  <p className="text-xs opacity-75">
                    {entry.gate} • {entry.timestamp}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
