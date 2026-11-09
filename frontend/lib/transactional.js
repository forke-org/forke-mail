export function ms(value) {
  if (value === null || value === undefined) return '—';
  if (value < 1000) return `${Math.round(value)} ms`;
  return `${(value / 1000).toFixed(1)} s`;
}

export function outcome(m) {
  if (m.complained_at) return 'complained';
  if (m.bounced_at) return 'bounced';
  if (m.clicked_at) return 'clicked';
  if (m.opened_at) return 'opened';
  if (m.delivered_at) return 'delivered';
  return '';
}
