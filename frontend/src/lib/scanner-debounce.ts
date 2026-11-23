export function shouldProcessQRScan(
  token: string,
  lastToken: string,
  lastScannedAtMs: number,
  nowMs: number = Date.now(),
  cooldownMs: number = 1500
): boolean {
  const trimmed = token.trim();
  if (!trimmed) return false;
  if (trimmed === lastToken && nowMs - lastScannedAtMs < cooldownMs) {
    return false;
  }
  return true;
}
