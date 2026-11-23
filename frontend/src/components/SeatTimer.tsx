"use client";

import { useEffect, useState } from "react";
import { Timer } from "lucide-react";
import { useSeatStore } from "@/lib/seat-store";
import { computeRemainingSeconds, formatCountdown } from "@/lib/timer";

interface SeatTimerProps {
  onExpired?: () => void;
}

export function SeatTimer({ onExpired }: SeatTimerProps) {
  const expiresAt = useSeatStore((s) => s.myHoldExpiresAt);
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
      className={`flex items-center justify-between rounded-lg border px-3 py-2 text-sm font-semibold ${
        urgent
          ? "border-rose-500/50 bg-rose-950/40 text-rose-200"
          : "border-amber-500/40 bg-amber-950/30 text-amber-200"
      }`}
    >
      <span className="flex items-center gap-1.5 text-xs font-medium">
        <Timer className="h-4 w-4" />
        Thời gian giữ ghế (đồng bộ server)
      </span>
      <span className="font-mono text-base">{formatCountdown(remaining)}</span>
    </div>
  );
}
