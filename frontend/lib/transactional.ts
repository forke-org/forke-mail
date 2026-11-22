/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

export function ms(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-';
  if (value < 1000) return `${Math.round(value)} ms`;
  return `${(value / 1000).toFixed(1)} s`;
}

export function outcome(m: { complained_at?: string | null; bounced_at?: string | null; clicked_at?: string | null; opened_at?: string | null; delivered_at?: string | null; unsubscribed_at?: string | null }): string {
  if (m.complained_at) return 'complained';
  if (m.bounced_at) return 'bounced';
  if (m.unsubscribed_at) return 'unsubscribed';
  if (m.clicked_at) return 'clicked';
  if (m.opened_at) return 'opened';
  if (m.delivered_at) return 'delivered';
  return '';
}
