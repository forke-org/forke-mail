import Link from 'next/link';
import { useRouter } from 'next/router';
import { useEffect } from 'react';
import { Badge, Card, EmailFrame, Empty, Loading, PageHead, Stat } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import { useLoad } from '../../lib/useLoad';
import { ms, outcome } from '../../lib/transactional';

const failureReasons = {
  expired: 'It waited in the queue for over an hour, so it was dropped instead of sent late.',
  unknown_outcome: 'Amazon SES did not answer, so it was not retried to avoid sending it twice.',
  recipient_suppressed: 'The recipient hard bounced, complained or was suppressed manually.',
  workspace_sending_paused: 'Sending is paused for this workspace.',
};

function describe(ev) {
  const d = ev.detail || {};
  switch (ev.type) {
    case 'sent': return `Handed to SES in ${ms(d.dispatch_ms)} (API call ${ms(d.send_ms)}, attempt ${d.attempts})`;
    case 'failed': return (d.reason || '').replace(/_/g, ' ');
    case 'bounced': case 'soft_bounced': return [d.bounce_type, d.bounce_sub_type].filter(Boolean).join(' / ');
    case 'clicked': return d.url;
    case 'delivered': return d.smtp_response;
    default: return '';
  }
}

export default function TransactionalView() {
  const router = useRouter();
  const id = router.query.id;
  const [state, reload] = useLoad(() => (id ? api('GET', `/api/transactional/${id}`) : new Promise(() => {})), [id]);
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
      <PageHead
        back={<Link className="back-link" href="/transactional/">← Transactional</Link>}
        title={m.subject}
        subtitle={`To ${m.to} · From ${m.from_name ? `${m.from_name} <${m.from_address}>` : m.from_address} · ${formatDate(m.created_at)}`}
      >
        <Badge value={m.status} />
        {result ? <Badge value={result} /> : null}
      </PageHead>
      {m.status === 'failed' ? <div className="callout">{failureReasons[m.last_error] || `Not sent: ${(m.last_error || 'failed').replace(/_/g, ' ')}.`}</div> : null}
      {m.status === 'queued' && m.last_error ? <div className="callout">Waiting to retry: {m.last_error.replace(/_/g, ' ')}.</div> : null}
      <div className="grid grid-4">
        <Stat label="Dispatch latency" value={ms(m.dispatch_ms)} hint="Accepted to handed to SES" />
        <Stat label="Attempts" value={number(m.attempts)} />
        <Stat label="Opens" value={m.track_opens ? number(m.open_count) : 'Off'} hint={m.track_opens ? 'Estimate' : null} />
        <Stat label="Clicks" value={m.track_clicks ? number(m.click_count) : 'Off'} />
      </div>
      <Card title="Timeline" flush>
        <ol className="timeline">
          {m.events.map((ev, i) => (
            <li key={i}>
              <Badge value={ev.type} />
              <span className="small muted truncate">{describe(ev)}</span>
              <span className="small muted">{formatDate(ev.created_at)}</span>
            </li>
          ))}
        </ol>
      </Card>
      {Object.keys(m.tags || {}).length ? (
        <Card title="Tags">
          <div className="card-body small">{Object.entries(m.tags).map(([k, v]) => <code key={k}>{k}={v} </code>)}</div>
        </Card>
      ) : null}
      <Card title="Content" flush>
        {m.html ? <EmailFrame html={m.html} /> : <pre className="card-body small">{m.text}</pre>}
      </Card>
    </>
  );
}
