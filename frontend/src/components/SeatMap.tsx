"use client";

import React, { memo, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, Grid2X2, Layers } from "lucide-react";
import {
  useIsLockingSeat,
  useIsMyHeldSeat,
  useSeatStatus,
  useSeatStore,
} from "@/lib/seat-store";
import type { SeatState, SeatStatus, Zone } from "@/lib/types";
import { getZoneTurnstileColor } from "@/lib/zone-colors";

interface SeatMapProps {
  onSeatClick: (seat: SeatState) => void;
  conflictSeatId: string | null;
}

const CANVAS_THRESHOLD = 800;

function seatPalette(
  status: SeatStatus,
  isMine: boolean,
  isLocking: boolean
): { fill: string; text: string; stroke: string } {
  if (isLocking) {
    return { fill: "#121212", text: "#FAF8F4", stroke: "#E85D04" };
  }
  if (isMine) {
    return { fill: "#E85D04", text: "#121212", stroke: "#121212" };
  }
  switch (status) {
    case "AVAILABLE":
      return { fill: "#0F766E", text: "#FFFFFF", stroke: "#121212" };
    case "HELD":
      return { fill: "#B45309", text: "#FFFFFF", stroke: "#121212" };
    case "CHECKOUT":
      return { fill: "#52525B", text: "#FFFFFF", stroke: "#121212" };
    case "SOLD":
    case "BLOCKED":
    default:
      return { fill: "#D6D0C4", text: "#4A463F", stroke: "#A8A29E" };
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

  const clickable = !isLocking && (isMine || status === "AVAILABLE");
  const colors = seatPalette(status, isMine, isLocking);

  const handleKeyDown = (e: React.KeyboardEvent<SVGGElement>) => {
    if (!clickable) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onSelect(seat);
    }
  };

  return (
    <g
      role="button"
      tabIndex={clickable ? 0 : -1}
      aria-label={`Ghế ${seat.label}, trạng thái ${isMine ? "Bạn đang giữ" : status}`}
      aria-disabled={!clickable}
      onClick={() => clickable && onSelect(seat)}
      onKeyDown={handleKeyDown}
      className={`${clickable ? "cursor-pointer" : "cursor-not-allowed"} ${
        isConflict ? "kt-shake" : ""
      }`}
    >
      <rect
        x={seat.pos_x - 11}
        y={seat.pos_y - 11}
        width={22}
        height={22}
        rx={3}
        fill={colors.fill}
        stroke={isConflict ? "#B91C1C" : colors.stroke}
        strokeWidth={isConflict || isMine ? 2.2 : 1.2}
        className={
          isLocking ? "animate-pulse" : "transition-colors duration-150"
        }
      />
      <text
        x={seat.pos_x}
        y={seat.pos_y + 3}
        textAnchor="middle"
        fontSize="7.5"
        fill={colors.text}
        className="pointer-events-none select-none font-mono font-bold"
      >
        {status === "SOLD" ? "✕" : `${seat.row_label}${seat.seat_number}`}
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
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.font = "bold 7.5px monospace";

    for (const seat of seats) {
      const isMine = myHeldSeatIds.includes(seat.id);
      const isLocking = Boolean(lockingSeatIds[seat.id]);
      const colors = seatPalette(seat.status, isMine, isLocking);

      ctx.fillStyle = colors.fill;
      ctx.strokeStyle = colors.stroke;
      ctx.lineWidth = isMine ? 2.2 : 1.2;
      ctx.fillRect(seat.pos_x - 11, seat.pos_y - 11, 22, 22);
      ctx.strokeRect(seat.pos_x - 11, seat.pos_y - 11, 22, 22);

      ctx.fillStyle = colors.text;
      const caption =
        seat.status === "SOLD" ? "✕" : `${seat.row_label}${seat.seat_number}`;
      ctx.fillText(caption, seat.pos_x, seat.pos_y);
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
      if (
        Math.abs(x - seat.pos_x) <= 12 &&
        Math.abs(y - seat.pos_y) <= 12
      ) {
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
      className="h-auto w-full cursor-pointer rounded-[3px] border-2 border-kt-ink bg-kt-ground"
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
    <div className="kt-card space-y-4 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b-2 border-dashed border-kt-rule pb-3.5">
        {activeZone ? (
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => setActiveZoneId(null)}
              className="inline-flex items-center gap-1.5 rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3 py-1.5 text-xs font-bold uppercase text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              Sơ đồ Zone
            </button>
            <div>
              <h2 className="font-display text-2xl font-extrabold uppercase tracking-tight text-kt-ink">
                {activeZone.name} ({activeZone.code})
              </h2>
              <p className="font-mono text-xs text-kt-muted kt-tabular">
                ${(activeZone.price_cents / 100).toFixed(2)} •{" "}
                {activeSeatIds.length} ghế
              </p>
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <Layers className="h-4 w-4 text-kt-ink" />
            <h2 className="font-display text-2xl font-extrabold uppercase tracking-tight text-kt-ink">
              Chọn khu vực khán đài (Tier 1)
            </h2>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-3 text-xs font-semibold text-kt-ink">
          <span className="inline-flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-[2px] border border-kt-ink bg-kt-emerald" />
            Trống
          </span>
          <span className="inline-flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-[2px] border border-kt-ink bg-kt-accent" />
            Bạn giữ
          </span>
          <span className="inline-flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-[2px] border border-kt-ink bg-kt-amber" />
            Đang giữ
          </span>
          <span className="inline-flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-[2px] border border-kt-ink bg-zinc-600" />
            Thanh toán
          </span>
          <span className="inline-flex items-center gap-1.5">
            <i className="h-3 w-3 rounded-[2px] border border-stone-400 bg-stone-300" />
            Đã bán (✕)
          </span>
          {activeZone && (
            <button
              type="button"
              onClick={() => setForceCanvas((v) => !v)}
              className="inline-flex items-center gap-1 rounded-[3px] border-2 border-kt-ink bg-kt-ground px-2.5 py-1 font-mono text-[11px] font-bold uppercase text-kt-ink hover:bg-kt-alt"
            >
              <Grid2X2 className="h-3 w-3" />
              {useCanvasMode ? "Canvas" : "SVG"}
            </button>
          )}
        </div>
      </div>

      {!activeZone ? (
        <div className="overflow-hidden rounded-[3px] border-2 border-kt-ink bg-kt-ground p-3 sm:p-4">
          <svg viewBox="0 0 800 680" className="h-auto w-full">
            <rect
              x={200}
              y={16}
              width={400}
              height={40}
              rx={3}
              fill="#121212"
              stroke="#121212"
            />
            <text
              x={400}
              y={41}
              textAnchor="middle"
              fill="#FAF8F4"
              fontSize="14"
              fontWeight="800"
              letterSpacing="2"
            >
              SÂN KHẤU CHÍNH / MAIN STAGE
            </text>

            {zones.map((z, idx) => {
              const x = z.svg_bounds?.x ?? 100;
              const y = z.svg_bounds?.y ?? 80 + idx * 140;
              const w = z.svg_bounds?.width ?? 600;
              const h = z.svg_bounds?.height ?? 110;
              const tierColor = getZoneTurnstileColor(z.code, idx);

              const ids = seatIdsByZone[z.id] || [];
              let avail = 0;
              for (const id of ids) {
                if (seatsById[id]?.status === "AVAILABLE") avail += 1;
              }

              return (
                <g
                  key={z.id}
                  role="button"
                  tabIndex={0}
                  aria-label={`Mở khu vực ${z.name}, giá $${(z.price_cents / 100).toFixed(2)}, còn ${avail} ghế trống`}
                  onClick={() => setActiveZoneId(z.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setActiveZoneId(z.id);
                    }
                  }}
                  className="cursor-pointer transition-opacity hover:opacity-85"
                >
                  <rect
                    x={x}
                    y={y}
                    width={w}
                    height={h}
                    rx={4}
                    fill="#FAF8F4"
                    stroke="#121212"
                    strokeWidth={2.5}
                  />
                  <rect
                    x={x + 16}
                    y={y + 14}
                    width={64}
                    height={22}
                    rx={2}
                    fill={tierColor}
                  />
                  <text
                    x={x + 48}
                    y={y + 29}
                    textAnchor="middle"
                    fill={tierColor === "#E85D04" ? "#121212" : "#FFFFFF"}
                    fontSize="11"
                    fontWeight="800"
                  >
                    {z.code}
                  </text>
                  <text
                    x={x + w / 2}
                    y={y + h / 2 - 4}
                    textAnchor="middle"
                    fill="#121212"
                    fontSize="19"
                    fontWeight="800"
                  >
                    {z.name}
                  </text>
                  <text
                    x={x + w / 2}
                    y={y + h / 2 + 22}
                    textAnchor="middle"
                    fill="#4A463F"
                    fontSize="13.5"
                    fontWeight="600"
                  >
                    ${(z.price_cents / 100).toFixed(2)} — Còn {avail}/{ids.length}{" "}
                    ghế trống (Bấm để chọn)
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
        <div className="overflow-auto rounded-[3px] border-2 border-kt-ink bg-kt-ground p-3">
          <svg viewBox={svgViewBox} className="h-auto min-w-[620px] w-full">
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
