/**
 * Maps database zone codes into the Stadium Turnstile & Gate Pass palette so
 * legacy seed hex values (for example #6366F1 indigo) never leak into UI plates.
 */
const TURNSTILE_TIER_COLORS = ["#E85D04", "#0F766E", "#B45309"] as const;

export function getZoneTurnstileColor(code: string, index: number): string {
  const upper = code.toUpperCase();
  if (upper === "VIP" || upper === "COURTSIDE") return "#E85D04";
  if (upper === "CAT1" || upper === "MID") return "#0F766E";
  if (upper === "GA" || upper === "UPPER" || upper === "ORCH") return "#B45309";
  return TURNSTILE_TIER_COLORS[index % TURNSTILE_TIER_COLORS.length];
}
