"use client";

import React, { memo, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, Layers, Sparkles } from "lucide-react";
import {
  useIsLockingSeat,
  useIsMyHeldSeat,
  useSeatStatus,
  useSeatStore,
} from "@/lib/seat-store";
import type { SeatState, SeatStatus, Zone } from "@/lib/types";

interface SeatMapProps {
  onSeatClick: (seat: SeatState) => void;
  conflictSeatId: string | null;
}

const CANVAS_THRESHOLD = 800;

function seatFillColor(
  status: SeatStatus,
  isMine: boolean,
  isLocking: boolean
): string {
  if (isLocking) return "#38bdf8";
  if (isMine) return "#6366f1";
  switch (status) {
    case "AVAILABLE":
      return "#10b981";
    case "HELD":
      return "#f59e0b";
    case "CHECKOUT":
      return "#a855f7";
    case "SOLD":
    case "BLOCKED":
    default:
      return "#334155";
  }
}

const SVGSeatNode = memo(function SVGSeatNode({
  seat,
  isConflict,
  onSelect,
}: {
  seat: SeatState;
  isConflict: boolean;
  onSelect: (seat: SeatState) => void;
}) {
  const status = useSeatStatus(seat.id);
  const isMine = useIsMyHeldSeat(seat.id);
  const isLocking = useIsLockingSeat(seat.id);

  const clickable =
    !isLocking && (isMine || status === "AVAILABLE");

  return (
    <g
      onClick={() => clickable && onSelect(seat)}
      className={clickable ? "cursor-pointer" : "cursor-not-allowed"}
    >
      <circle
        cx={seat.pos_x}
        cy={seat.pos_y}
        r={11}
        fill={seatFillColor(status, isMine, isLocking)}
        stroke={
          isConflict
            ? "#fb7185"
            : isMine
              ? "#e0e7ff"
              : "rgba(15,23,42,0.85)"
        }
        strokeWidth={isConflict || isMine ? 2.5 : 1.2}
        className={isLocking ? "animate-pulse" : "transition-colors"}
      />
      <text
        x={seat.pos_x}
        y={seat.pos_y + 3.5}
        textAnchor="middle"
        fontSize="7"
        fill="#ffffff"
        className="pointer-events-none select-none font-semibold"
      >
        {seat.row_label}
        {seat.seat_number}
      </text>
      <title>
        {seat.label} — {isMine ? "Bạn đang giữ" : status}
      </title>
    </g>
  );
});

function CanvasSeatRenderer({
  seatIds,
  onSeatClick,
}: {
  seatIds: string[];
  onSeatClick: (seat: SeatState) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const seatsById = useSeatStore((s) => s.seatsById);
  const myHeldSeatIds = useSeatStore((s) => s.myHeldSeatIds);
  const lockingSeatIds = useSeatStore((s) => s.lockingSeatIds);

  const seats = useMemo(
    () => seatIds.map((id) => seatsById[id]).filter(Boolean),
    [seatIds, seatsById]
  );

  const bounds = useMemo(() => {
    let maxX = 800;
    let maxY = 420;
    for (const s of seats) {
      if (s.pos_x + 40 > maxX) maxX = s.pos_x + 40;
      if (s.pos_y + 40 > maxY) maxY = s.pos_y + 40;
    }
    return { width: maxX, height: maxY };
  }, [seats]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    ctx.clearRect(0, 0, canvas.width, canvas.height);
    for (const seat of seats) {
      const isMine = myHeldSeatIds.includes(seat.id);
      const isLocking = Boolean(lockingSeatIds[seat.id]);
      ctx.beginPath();
      ctx.arc(seat.pos_x, seat.pos_y, 10, 0, Math.PI * 2);
      ctx.fillStyle = seatFillColor(seat.status, isMine, isLocking);
      ctx.fill();
      if (isMine) {
        ctx.lineWidth = 2;
        ctx.strokeStyle = "#e0e7ff";
        ctx.stroke();
      }
    }
  }, [seats, myHeldSeatIds, lockingSeatIds]);

  const handleClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const scaleX = canvas.width / rect.width;
    const scaleY = canvas.height / rect.height;
    const x = (e.clientX - rect.left) * scaleX;
    const y = (e.clientY - rect.top) * scaleY;

    for (const seat of seats) {
      if (Math.hypot(x - seat.pos_x, y - seat.pos_y) <= 12) {
        onSeatClick(seat);
        return;
      }
    }
  };

  return (
    <canvas
      ref={canvasRef}
      width={bounds.width}
      height={bounds.height}
      onClick={handleClick}
      className="h-auto w-full cursor-pointer rounded-xl bg-slate-950/70"
    />
  );
}

export function SeatMap({ onSeatClick, conflictSeatId }: SeatMapProps) {
  const zones = useSeatStore((s) => s.zones);
  const seatsById = useSeatStore((s) => s.seatsById);
  const seatIdsByZone = useSeatStore((s) => s.seatIdsByZone);

  const [activeZoneId, setActiveZoneId] = useState<string | null>(null);
  const [forceCanvas, setForceCanvas] = useState(false);

  const activeZone: Zone | undefined = useMemo(
    () => zones.find((z) => z.id === activeZoneId),
    [zones, activeZoneId]
  );

  const activeSeatIds = useMemo(
    () => (activeZoneId ? seatIdsByZone[activeZoneId] || [] : []),
    [activeZoneId, seatIdsByZone]
  );

  const sectionSeats = useMemo(
    () => activeSeatIds.map((id) => seatsById[id]).filter(Boolean),
    [activeSeatIds, seatsById]
  );

  const svgViewBox = useMemo(() => {
    let maxX = 800;
    let maxY = 400;
    for (const s of sectionSeats) {
      if (s.pos_x + 40 > maxX) maxX = s.pos_x + 40;
      if (s.pos_y + 40 > maxY) maxY = s.pos_y + 40;
    }
    return `0 0 ${maxX} ${maxY}`;
  }, [sectionSeats]);

  const useCanvasMode =
    forceCanvas || activeSeatIds.length > CANVAS_THRESHOLD;

  return (
    <div className="space-y-4 rounded-2xl border border-slate-800 bg-slate-900/75 p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        {activeZone ? (
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => setActiveZoneId(null)}
              className="flex items-center gap-1.5 rounded-lg border border-slate-700 px-3 py-1.5 text-xs font-medium text-slate-200 transition hover:border-slate-500"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              Sơ đồ tổng quan Zone
            </button>
            <div>
              <h2 className="text-base font-bold text-white">
                {activeZone.name} ({activeZone.code})
              </h2>
              <p className="text-xs text-slate-400">
                ${(activeZone.price_cents / 100).toFixed(2)} •{" "}
                {activeSeatIds.length} ghế
              </p>
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <Layers className="h-4 w-4 text-indigo-400" />
            <h2 className="text-base font-bold text-white">
              Chọn khu vực (Tier 1 — Zone Overview)
            </h2>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-3 text-xs text-slate-300">
          <span className="flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-full bg-emerald-500" /> Trống
          </span>
          <span className="flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-full bg-indigo-500" /> Bạn đang giữ
          </span>
          <span className="flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-full bg-amber-500" /> Người khác giữ
          </span>
          <span className="flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-full bg-purple-500" /> Đang thanh toán
          </span>
          <span className="flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-full bg-slate-700" /> Đã bán
          </span>
          {activeZone && (
            <button
              type="button"
              onClick={() => setForceCanvas((v) => !v)}
              className="flex items-center gap-1 rounded border border-slate-700 px-2 py-1 text-[11px] text-slate-300 hover:border-slate-500"
            >
              <Sparkles className="h-3 w-3" />
              {useCanvasMode ? "Canvas Mode" : "SVG Mode"}
            </button>
          )}
        </div>
      </div>

      {!activeZone ? (
        <div className="overflow-hidden rounded-xl border border-slate-800 bg-slate-950/80 p-4">
          <svg viewBox="0 0 800 680" className="h-auto w-full">
            <rect
              x={220}
              y={16}
              width={360}
              height={36}
              rx={8}
              fill="#1e293b"
              stroke="#475569"
            />
            <text
              x={400}
              y={39}
              textAnchor="middle"
              fill="#e2e8f0"
              fontSize="14"
              fontWeight="bold"
            >
              MAIN STAGE
            </text>

            {zones.map((z, idx) => {
              const x = z.svg_bounds?.x ?? 100;
              const y = z.svg_bounds?.y ?? 80 + idx * 140;
              const w = z.svg_bounds?.width ?? 600;
              const h = z.svg_bounds?.height ?? 110;

              const ids = seatIdsByZone[z.id] || [];
              let avail = 0;
              for (const id of ids) {
                if (seatsById[id]?.status === "AVAILABLE") avail += 1;
              }

              return (
                <g
                  key={z.id}
                  onClick={() => setActiveZoneId(z.id)}
                  className="cursor-pointer transition-opacity hover:opacity-90"
                >
                  <rect
                    x={x}
                    y={y}
                    width={w}
                    height={h}
                    rx={14}
                    fill={z.color || "#3B82F6"}
                    fillOpacity={0.2}
                    stroke={z.color || "#3B82F6"}
                    strokeWidth={2}
                  />
                  <text
                    x={x + w / 2}
                    y={y + h / 2 - 8}
                    textAnchor="middle"
                    fill="#ffffff"
                    fontSize="18"
                    fontWeight="bold"
                  >
                    {z.name} ({z.code})
                  </text>
                  <text
                    x={x + w / 2}
                    y={y + h / 2 + 16}
                    textAnchor="middle"
                    fill="#cbd5e1"
                    fontSize="13"
                  >
                    ${(z.price_cents / 100).toFixed(2)} — Còn {avail}/{ids.length}{" "}
                    ghế trống (Bấm để chọn ghế)
                  </text>
                </g>
              );
            })}
          </svg>
        </div>
      ) : useCanvasMode ? (
        <CanvasSeatRenderer
          seatIds={activeSeatIds}
          onSeatClick={onSeatClick}
        />
      ) : (
        <div className="overflow-auto rounded-xl border border-slate-800 bg-slate-950/80 p-3">
          <svg viewBox={svgViewBox} className="h-auto min-w-[640px] w-full">
            {sectionSeats.map((seat) => (
              <SVGSeatNode
                key={seat.id}
                seat={seat}
                isConflict={conflictSeatId === seat.id}
                onSelect={onSeatClick}
              />
            ))}
          </svg>
        </div>
      )}
    </div>
  );
}
