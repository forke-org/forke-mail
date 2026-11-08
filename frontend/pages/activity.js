import Link from 'next/link';
import { useMemo, useState } from 'react';
import DataTable from '../components/DataTable';
import { Badge, Card, PageHead } from '../components/ui';
import { api } from '../lib/api';
import { formatDate } from '../lib/format';
import { useLoad } from '../lib/useLoad';

const types = ['sent', 'delivered', 'opened', 'clicked', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'rejected', 'delayed', 'failed', 'queued', 'sending_paused'];

export default function Activity() {
  const [type, setType] = useState('');
  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: 200 });
    if (type) params.set('type', type);
    return api('GET', `/api/events?${params}`);
  }, [type]);

  const columns = useMemo(() => [
    { accessorKey: 'type', header: 'Event', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'recipient', header: 'Recipient', cell: ({ getValue }) => getValue() || '—' },
    {
      accessorKey: 'subject', header: 'Email',
      cell: ({ row }) => {
        const ev = row.original;
        if (ev.email_id) return <Link href={`/emails/view/?id=${ev.email_id}`} onClick={(e) => e.stopPropagation()}><div className="cell-strong">{ev.subject}</div><div className="small muted">{ev.campaign_name}</div></Link>;
        if (ev.message_id) return <Link href={`/transactional/view/?id=${ev.message_id}`} onClick={(e) => e.stopPropagation()}><div className="cell-strong">{ev.subject}</div><div className="small muted">Transactional</div></Link>;
        return '—';
      },
    },
    { accessorKey: 'created_at', header: 'Time', cell: ({ getValue }) => formatDate(getValue()) },
  ], []);

  return (
    <>
      <PageHead title="Activity" subtitle="Every send, delivery, bounce and unsubscribe in your workspace.">
        <select value={type} onChange={(e) => setType(e.target.value)} aria-label="Event type">
          <option value="">All events</option>
          {types.map((t) => <option key={t} value={t}>{(t[0].toUpperCase() + t.slice(1)).replace(/_/g, ' ')}</option>)}
        </select>
        <button className="btn" type="button" onClick={reload}>Refresh</button>
      </PageHead>
      <Card flush>
        <DataTable columns={columns} data={state.data ? state.data.events : null} error={state.error} pageSize={25} empty={{ title: 'No activity yet', hint: 'Events appear here as your emails send.' }} />
      </Card>
    </>
  );
}
