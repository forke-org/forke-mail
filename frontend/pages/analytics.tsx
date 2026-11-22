/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import clsx from 'clsx';
import { useMemo, useState } from 'react';
import { Card, Loading, PageHeader, Select, Tabs } from '../components/ui';
import { api } from '../lib/api';
import { number } from '../lib/format';
import { useLoad } from '../lib/useLoad';

type Totals = { sent: number; delivered: number; opened: number; clicked: number; bounced: number; complained: number; unsubscribed: number };
type Report = { days: number; stream: string; current: Totals; previous: Totals; daily: { date: string; sent: number; delivered: number; opened: number; bounced: number; broadcast: number; transactional: number }[]; streams: (Totals & { stream: string })[] };

const ratio = (n: number, d: number) => (d ? n / d : null);
const pct = (v: number | null, digits = 1) => (v == null ? '—' : `${(v * 100).toFixed(digits).replace(/\.0+$/, '')}%`);

const metrics: { key: string; label: string; value: (t: Totals) => number | null; rate: boolean; good: 'up' | 'down'; digits?: number }[] = [
  { key: 'sent', label: 'Sent', value: (t) => t.sent, rate: false, good: 'up' },
  { key: 'delivery', label: 'Delivery rate', value: (t) => ratio(t.delivered, t.sent), rate: true, good: 'up' },
  { key: 'open', label: 'Open rate', value: (t) => ratio(t.opened, t.delivered), rate: true, good: 'up' },
  { key: 'click', label: 'Click rate', value: (t) => ratio(t.clicked, t.delivered), rate: true, good: 'up' },
  { key: 'bounce', label: 'Bounce rate', value: (t) => ratio(t.bounced, t.sent), rate: true, good: 'down', digits: 2 },
  { key: 'complaint', label: 'Spam reports', value: (t) => ratio(t.complained, t.sent), rate: true, good: 'down', digits: 3 },
  { key: 'unsub', label: 'Unsubscribes', value: (t) => ratio(t.unsubscribed, t.delivered), rate: true, good: 'down', digits: 2 },
];

function Delta({ now, before, rate, good }: { now: number | null; before: number | null; rate: boolean; good: 'up' | 'down' }) {
  if (now == null || before == null || before === 0) return <span className="text-xs text-faint">No earlier data</span>;
  const diff = rate ? (now - before) * 100 : ((now - before) / before) * 100;
  if (Math.abs(diff) < 0.05) return <span className="text-xs text-muted">Same as before</span>;
  const better = good === 'up' ? diff > 0 : diff < 0;
  return (
    <span className={clsx('text-xs tabular-nums', better ? 'text-success' : 'text-danger')}>
      {diff > 0 ? '+' : '−'}{Math.abs(diff).toFixed(1)}{rate ? ' pts' : '%'} vs previous
    </span>
  );
}

type Daily = Report['daily'];

const fmtDay = (d: string) => new Date(d + 'T00:00:00Z').toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });

function Swatch({ color }: { color: string }) {
  return <span className="size-2.5 shrink-0 rounded-[2px]" style={{ background: color }} />;
}

function Bars({ daily, series, height, empty }: {
  daily: Daily; series: { key: 'broadcast' | 'transactional'; label: string; color: string }[]; height: string; empty: string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const total = (d: Daily[number]) => series.reduce((n, s) => n + d[s.key], 0);
  const max = Math.max(1, ...daily.map(total));
  if (!daily.some((d) => total(d) > 0)) return <div className={clsx('grid place-items-center text-[13px] text-muted', height)}>{empty}</div>;
  const h = hover == null ? null : daily[hover];
  return (
    <div>
      <div className="mb-3 flex min-h-5 flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
        {h ? (
          <>
            <span className="text-ink">{fmtDay(h.date)}</span>
            {series.map((s) => <span key={s.key} className="inline-flex items-center gap-1.5"><Swatch color={s.color} />{s.label} <span className="text-ink tabular-nums">{number(h[s.key])}</span></span>)}
            {series.length > 1 ? <span>Total <span className="text-ink tabular-nums">{number(total(h))}</span></span> : null}
          </>
        ) : series.map((s) => <span key={s.key} className="inline-flex items-center gap-1.5"><Swatch color={s.color} />{s.label}</span>)}
      </div>
      <div className={clsx('relative flex items-end gap-[3px] border-b border-line', height)} onMouseLeave={() => setHover(null)}>
        {daily.map((d, i) => {
          const t = total(d);
          const shown = series.filter((s) => d[s.key] > 0);
          return (
            <div key={d.date} className="flex h-full flex-1 cursor-default flex-col justify-end" onMouseEnter={() => setHover(i)}>
              <div className={clsx('flex flex-col-reverse gap-[2px] transition-opacity', hover != null && hover !== i && 'opacity-45')} style={{ height: `${(t / max) * 100}%`, minHeight: t ? 3 : 0 }}>
                {shown.map((s, j) => (
                  <div key={s.key} className={j === shown.length - 1 ? 'rounded-t-[4px]' : ''} style={{ flexGrow: d[s.key], flexBasis: 0, minHeight: 2, background: s.color }} />
                ))}
              </div>
            </div>
          );
        })}
      </div>
      <div className="mt-2 flex justify-between text-[11px] text-faint tabular-nums">
        <span>{fmtDay(daily[0].date)}</span><span>max {number(max)} / day</span><span>{fmtDay(daily[daily.length - 1].date)}</span>
      </div>
    </div>
  );
}

const broadcastSeries = { key: 'broadcast' as const, label: 'Broadcasts', color: 'var(--series-broadcast)' };
const transactionalSeries = { key: 'transactional' as const, label: 'Transactional', color: 'var(--series-transactional)' };

export default function Analytics() {
  const [days, setDays] = useState<'1' | '7' | '30' | '90'>('30');
  const [stream, setStream] = useState('all');
  const [state] = useLoad(() => api<Report>('GET', `/api/analytics?days=${days}&stream=${stream}`), [days, stream]);
  const r = state.data;
  const label = days === '1' ? 'Today' : `Last ${days} days`;
  const streamRows = useMemo(() => {
    const names: [string, string][] = [['transactional', 'Transactional'], ['broadcast', 'Broadcasts']];
    return names.map(([key, name]) => ({ name, t: r?.streams.find((s) => s.stream === key) }));
  }, [r]);

  return (
    <>
      <PageHeader title="Analytics" description={`${label}, compared with the ${days === '1' ? 'day' : `${days} days`} before. Delivery, bounces and spam reports come from Amazon SES; opens are estimates.`}
        actions={<div className="flex items-center gap-2 whitespace-nowrap">
          <Select value={stream} onChange={(e) => setStream(e.target.value)} aria-label="Stream" className="w-36 shrink-0">
            <option value="all">All mail</option>
            <option value="transactional">Transactional</option>
            <option value="broadcast">Broadcasts</option>
          </Select>
          <Tabs className="h-9 shrink-0 items-center" value={days} onChange={setDays} options={[['1', 'Today'], ['7', '7 days'], ['30', '30 days'], ['90', '90 days']]} />
        </div>} />
      {!r ? <Loading /> : (
        <>
          <div className="mb-5 grid grid-cols-2 gap-px overflow-hidden rounded-[8px] bg-line shadow-card sm:grid-cols-4 xl:grid-cols-7">
            {metrics.map((m) => {
              const now = m.value(r.current);
              const before = m.value(r.previous);
              return (
                <div key={m.key} className="bg-surface px-4 py-3.5">
                  <div className="text-xs text-muted">{m.label}</div>
                  <div className="mt-1 text-xl font-semibold text-ink tabular-nums">{m.rate ? pct(now, m.digits) : number(now || 0)}</div>
                  <Delta now={now} before={before} rate={m.rate} good={m.good} />
                </div>
              );
            })}
          </div>
          <Card title="Sent by day" className="mb-5" description="Broadcasts stacked on transactional mail. Hover a day for the numbers.">
            <Bars daily={r.daily} height="h-56" empty="No emails were sent in this period."
              series={stream === 'broadcast' ? [broadcastSeries] : stream === 'transactional' ? [transactionalSeries] : [transactionalSeries, broadcastSeries]} />
          </Card>
          {stream === 'all' ? (
            <Card title="Transactional by day" className="mb-5" description="Its own scale, so sign-in codes and receipts stay readable next to big broadcasts.">
              <Bars daily={r.daily} height="h-28" empty="No transactional emails in this period." series={[transactionalSeries]} />
            </Card>
          ) : null}
          <Card title="By stream" description="Transactional mail should sit well above broadcasts on every line." flush>
            <div className="overflow-x-auto">
              <table className="w-full text-[13px]">
                <thead>
                  <tr className="border-b border-line text-left text-xs text-muted">
                    {['Stream', 'Sent', 'Delivered', 'Opened', 'Clicked', 'Bounced', 'Spam reports', 'Unsubscribed'].map((h, i) => <th key={h} className={clsx('px-5 py-2.5 font-normal', i > 0 && 'text-right')}>{h}</th>)}
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {streamRows.map(({ name, t }) => (
                    <tr key={name}>
                      <td className="px-5 py-3 text-ink">{name}</td>
                      <td className="px-5 py-3 text-right tabular-nums">{number(t?.sent || 0)}</td>
                      {(['delivered', 'opened', 'clicked', 'bounced', 'complained', 'unsubscribed'] as const).map((k) => (
                        <td key={k} className="px-5 py-3 text-right tabular-nums text-ink-2">{t && t.sent ? pct(ratio(t[k], k === 'bounced' || k === 'complained' || k === 'delivered' ? t.sent : t.delivered), k === 'complained' ? 3 : 1) : '—'}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
        </>
      )}
    </>
  );
}
