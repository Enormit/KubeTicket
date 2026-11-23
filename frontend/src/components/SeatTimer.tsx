"use client";

import { useEffect, useState } from "react";
import { Timer } from "lucide-react";
import { useSeatStore } from "@/lib/seat-store";
import { computeRemainingSeconds, formatCountdown } from "@/lib/timer";

interface SeatTimerProps {
  onExpired?: () => void;
  fallbackExpiresAt?: string | null;
}

export function SeatTimer({ onExpired, fallbackExpiresAt }: SeatTimerProps) {
  const storeExpiresAt = useSeatStore((s) => s.myHoldExpiresAt);
  const expiresAt = storeExpiresAt ?? fallbackExpiresAt ?? null;
  const serverOffsetMs = useSeatStore((s) => s.serverOffsetMs);
  const [remaining, setRemaining] = useState(() =>
    computeRemainingSeconds(expiresAt, serverOffsetMs)
  );

  useEffect(() => {
    if (!expiresAt) {
      setRemaining(0);
      return;
    }

    const update = () => {
      const secs = computeRemainingSeconds(expiresAt, serverOffsetMs);
      setRemaining(secs);
      if (secs === 0) {
        onExpired?.();
      }
    };

    update();
    const id = setInterval(update, 1000);
    return () => clearInterval(id);
  }, [expiresAt, serverOffsetMs, onExpired]);

  if (!expiresAt) return null;

  const urgent = remaining <= 60;

  return (
    <div
      className={`flex items-center justify-between rounded-[3px] border-2 px-3.5 py-2.5 text-sm font-bold ${
        urgent
          ? "border-kt-danger bg-kt-danger text-white"
          : "border-kt-ink bg-kt-ground text-kt-ink"
      }`}
    >
      <span className="inline-flex items-center gap-1.5 text-xs uppercase tracking-wider">
        <Timer className="h-4 w-4" />
        Hạn giữ ghế (Server)
      </span>
      <span className="font-mono text-lg kt-tabular">
        {formatCountdown(remaining)}
      </span>
    </div>
  );
}
