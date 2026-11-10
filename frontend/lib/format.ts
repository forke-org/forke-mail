export function formatDate(value: string | null | undefined): string {
  if (!value) return '—';
  const date = new Date(value);
  const sameYear = date.getFullYear() === new Date().getFullYear();
  const day = date.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: sameYear ? undefined : 'numeric' });
  const time = date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  return `${day}, ${time}`;
}

export function timeAgo(value: string | null | undefined): string {
  if (!value) return 'Never';
  const seconds = Math.round((Date.now() - new Date(value).getTime()) / 1000);
  if (seconds < 60) return 'Just now';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} min ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} h ago`;
  return formatDate(value);
}

export function number(value: number | null | undefined): string {
  return Number(value || 0).toLocaleString();
}

export function compact(value: number): string {
  return Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value || 0);
}

export function fullName(contact: { first_name?: string; last_name?: string }): string {
  return [contact.first_name, contact.last_name].filter(Boolean).join(' ') || '—';
}

export function plural(n: number, word: string, many = `${word}s`): string {
  return `${number(n)} ${n === 1 ? word : many}`;
}

export function humanize(value: string | null | undefined): string {
  return String(value || '').replace(/_/g, ' ');
}

export function percent(part: number, whole: number): string {
  if (!whole) return '—';
  return `${Math.round((part / whole) * 1000) / 10}%`;
}
