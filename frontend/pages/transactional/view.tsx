import clsx from 'clsx';
import { useRouter } from 'next/router';
import { useEffect } from 'react';
import { Badge, Callout, Card, EmailFrame, Empty, Loading, PageHeader, Stat, Chip } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, humanize, number } from '../../lib/format';
import { ms, outcome } from '../../lib/transactional';
import type { TransactionalMessage } from '../../lib/types';
import { never, useLoad } from '../../lib/useLoad';

const failureReasons: Record<string, string> = {
  expired: 'It waited in the queue for over an hour, so it was dropped instead of sent late.',
  unknown_outcome: 'The email provider did not answer, so it was not retried to avoid sending it twice.',
  recipient_suppressed: 'The recipient hard bounced, complained or was suppressed manually.',
  workspace_sending_paused: 'Sending is paused for this workspace.',
};

const dotTone: Record<string, string> = {
  delivered: 'bg-success', sent: 'bg-success', opened: 'bg-accent', clicked: 'bg-accent',
  bounced: 'bg-danger', complained: 'bg-danger', failed: 'bg-danger', rejected: 'bg-danger',
};

function describe(ev: { type: string; detail: Record<string, any> | null }) {
  const d = ev.detail || {};
  switch (ev.type) {
    case 'sent': return `Handed off for delivery in ${ms(d.dispatch_ms)} (API call ${ms(d.send_ms)}, attempt ${d.attempts})`;
    case 'failed': return humanize(d.reason);
    case 'bounced': case 'soft_bounced': return [d.bounce_type, d.bounce_sub_type].filter(Boolean).join(' / ');
    case 'clicked': return d.url;
    case 'delivered': return d.smtp_response;
    default: return '';
  }
}

export default function TransactionalView() {
  const router = useRouter();
  const id = router.query.id as string | undefined;
  const [state, reload] = useLoad(() => (id ? api<TransactionalMessage>('GET', `/api/transactional/${id}`) : never<TransactionalMessage>()), [id]);
  const m = state.data;
  const live = m && (m.status === 'queued' || m.status === 'sending' || (m.status === 'sent' && !m.delivered_at && !m.bounced_at));
  useEffect(() => {
    if (!live) return undefined;
    const t = setTimeout(reload, 1500);
    return () => clearTimeout(t);
  }, [live, m]);

  if (state.error) return <Card><Empty title="Message not found" hint={state.error} /></Card>;
  if (!m) return <Loading />;
  const result = outcome(m);
  return (
    <>
      <PageHeader
        back={{ href: '/transactional/', label: 'Transactional' }}
        title={m.subject}
        meta={<><Badge value={m.status} />{result ? <Badge value={result} /> : null}</>}
        description={`To ${m.to} · From ${m.from_name ? `${m.from_name} <${m.from_address}>` : m.from_address} · ${formatDate(m.created_at)}`}
      />
      {m.status === 'failed' ? <div className="mb-5"><Callout tone="danger">{failureReasons[m.last_error || ''] || `Not sent: ${humanize(m.last_error || 'failed')}.`}</Callout></div> : null}
      {m.status === 'queued' && m.last_error ? <div className="mb-5"><Callout tone="warn">Waiting to retry: {humanize(m.last_error)}.</Callout></div> : null}
      <div className="mb-5 grid grid-cols-2 gap-px overflow-hidden rounded-[8px] bg-line shadow-card lg:grid-cols-4">
        <Stat label="Dispatch latency" value={ms(m.dispatch_ms)} hint="Accepted → handed off for delivery" />
        <Stat label="Attempts" value={number(m.attempts)} />
        <Stat label="Opens" value={m.track_opens ? number(m.open_count) : 'Off'} hint={m.track_opens ? 'Estimate' : 'Tracking disabled'} />
        <Stat label="Clicks" value={m.track_clicks ? number(m.click_count) : 'Off'} hint={m.track_clicks ? ' ' : 'Tracking disabled'} />
      </div>
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Timeline">
            <ol className="relative flex flex-col gap-4 before:absolute before:top-2 before:bottom-2 before:left-[5px] before:w-px before:bg-line-strong">
              {(m.events || []).map((ev, i) => (
                <li key={i} className="relative flex gap-3 pl-6">
                  <span className={clsx('absolute top-1.5 left-0 size-[11px] rounded-full ring-4 ring-surface', dotTone[ev.type] || 'bg-faint')} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium text-ink capitalize">{humanize(ev.type)}</span>
                      <span className="text-xs text-muted">{formatDate(ev.created_at)}</span>
                    </div>
                    {describe(ev) ? <div className="mt-0.5 truncate text-xs text-muted">{describe(ev)}</div> : null}
                  </div>
                </li>
              ))}
            </ol>
          </Card>
          {Object.keys(m.tags || {}).length ? (
            <Card title="Tags">
              <div className="flex flex-wrap gap-1.5">{Object.entries(m.tags || {}).map(([k, v]) => <Chip key={k}>{k}={v}</Chip>)}</div>
            </Card>
          ) : null}
        </div>
        <Card title="Content" flush>
          {m.html
            ? <div className="dot-grid bg-surface-2 p-4"><div className="overflow-hidden rounded-lg shadow-card"><EmailFrame html={m.html} /></div></div>
            : <pre className="overflow-x-auto p-5 font-mono text-xs whitespace-pre-wrap text-ink-2">{m.text}</pre>}
        </Card>
      </div>
    </>
  );
}
