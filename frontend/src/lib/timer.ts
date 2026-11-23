export function computeRemainingSeconds(
  expiresAtIso: string | null | undefined,
  serverOffsetMs: number,
  clientNowMs: number = Date.now()
): number {
  if (!expiresAtIso) return 0;
  const targetMs = Date.parse(expiresAtIso);
  if (Number.isNaN(targetMs)) return 0;
  const syncedNowMs = clientNowMs + serverOffsetMs;
  return Math.max(0, Math.floor((targetMs - syncedNowMs) / 1000));
}

export function formatCountdown(totalSeconds: number): string {
  const safe = Math.max(0, Math.floor(totalSeconds));
  const mins = Math.floor(safe / 60);
  const secs = safe % 60;
  return `${String(mins).padStart(2, "0")}:${String(secs).padStart(2, "0")}`;
}
