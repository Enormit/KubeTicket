export type SeatStatus = "AVAILABLE" | "HELD" | "CHECKOUT" | "SOLD" | "BLOCKED";

export interface User {
  id: string;
  email: string;
  full_name: string;
  role: "USER" | "ADMIN";
  created_at: string;
}

export interface Event {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  starts_at: string;
  sale_starts_at: string;
  sale_ends_at?: string;
  max_seats_per_order: number;
  status: "DRAFT" | "UPCOMING" | "ON_SALE" | "SOLD_OUT" | "ENDED";
}

export interface SVGBounds {
  x?: number;
  y?: number;
  width?: number;
  height?: number;
}

export interface Zone {
  id: string;
  event_id: string;
  code: string;
  name: string;
  price_cents: number;
  currency: string;
  color: string;
  svg_bounds: SVGBounds;
}

export interface SeatState {
  id: string;
  zone_id: string;
  row_label: string;
  seat_number: number;
  label: string;
  pos_x: number;
  pos_y: number;
  status: SeatStatus;
  expires_at?: string;
}

export interface SeatSnapshotResponse {
  event_id: string;
  seq: number;
  server_now: string;
  zones: Zone[];
  seats: SeatState[];
}

export interface WaitingRoomStatus {
  event_id: string;
  position: number;
  ahead_count: number;
  estimated_wait_sec: number;
  admitted: boolean;
  admission_token?: string;
  expires_at?: string;
}

export interface HoldResult {
  hold_token: string;
  event_id: string;
  seat_ids: string[];
  seq: number;
  server_now: string;
  expires_at: string;
}

export interface CheckoutResult {
  order_id: string;
  event_id: string;
  seat_ids: string[];
  status: string;
  seq: number;
  expires_at: string;
}

export interface OrderItem {
  seat_id: string;
  seat_label?: string;
  zone_id: string;
  unit_price_cents: number;
}

export interface Order {
  id: string;
  user_id: string;
  event_id: string;
  status: "PENDING" | "COMPLETED" | "EXPIRED" | "CANCELLED" | "REFUNDED_EXPIRED";
  total_cents: number;
  currency: string;
  payment_provider: string;
  expires_at: string;
  completed_at?: string;
  created_at: string;
  items?: OrderItem[];
}

export interface Ticket {
  id: string;
  order_id: string;
  event_id: string;
  event_title?: string;
  seat_id: string;
  seat_label?: string;
  ticket_code: string;
  status: "PENDING" | "PAID" | "CHECKED_IN" | "CANCELLED" | "REFUNDED";
  created_at: string;
}

export interface QRPayload {
  ticket_id: string;
  event_id: string;
  exp: number;
  nonce: string;
  sig: string;
}

export interface SeatDelta {
  seat_id: string;
  status: SeatStatus;
  expires_at?: string;
}

export interface WebSocketEnvelope {
  type: "SYNC" | "DELTA" | "ORDER_CONFIRMED" | "PONG";
  event_id?: string;
  order_id?: string;
  seq: number;
  server_now: string;
  seats?: SeatDelta[];
}

export interface CheckinResponse {
  status: string;
  checkin: {
    checkin_id: string;
    ticket_id: string;
    event_id: string;
    gate: string;
    checked_in_at: string;
  };
}
