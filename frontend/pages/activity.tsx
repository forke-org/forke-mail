import { RefreshCw } from 'lucide-react';
import Link from 'next/link';
import { useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { Badge, Button, Card, PageHeader, Select, Toolbar } from '../components/ui';
import { api } from '../lib/api';
import { formatDate, humanize, timeAgo } from '../lib/format';
import type { EventRow } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const types = ['sent', 'delivered', 'opened', 'clicked', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'rejected', 'delayed', 'failed', 'queued', 'sending_paused'];

export default function Activity() {
  const [type, setType] = useState('');
  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: '200' });
    if (type) params.set('type', type);
    return api<{ events: EventRow[] }>('GET', `/api/events?${params}`);
  }, [type]);

  const columns = useMemo<Columns<EventRow>>(() => [
    { accessorKey: 'type', header: 'Event', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'recipient', header: 'Recipient', cell: ({ getValue }) => (getValue() ? <span className="text-ink">{getValue()}</span> : <span className="text-faint">—</span>) },
    {
      accessorKey: 'subject', header: 'Email',
      cell: ({ row }) => {
        const ev = row.original;
        if (ev.email_id) return <Link href={`/emails/view/?id=${ev.email_id}`} className="hover:underline"><Strong sub={ev.campaign_name}>{ev.subject}</Strong></Link>;
        if (ev.message_id) return <Link href={`/transactional/view/?id=${ev.message_id}`} className="hover:underline"><Strong sub="Transactional">{ev.subject}</Strong></Link>;
        return <span className="text-faint">—</span>;
      },
    },
    { accessorKey: 'created_at', header: 'Time', cell: ({ getValue }) => <span className="text-muted" title={formatDate(getValue())}>{timeAgo(getValue())}</span> },
  ], []);

  return (
    <>
      <PageHeader title="Activity" description="Every send, delivery, open, click, bounce and unsubscribe in this workspace." />
      <Card flush>
        <Toolbar>
          <Select value={type} onChange={(e) => setType(e.target.value)} aria-label="Event type" className="sm:w-52">
            <option value="">All events</option>
            {types.map((t) => <option key={t} value={t}>{humanize(t[0].toUpperCase() + t.slice(1))}</option>)}
          </Select>
          <Button className="sm:ml-auto" icon={RefreshCw} loading={state.loading} onClick={reload}>Refresh</Button>
        </Toolbar>
        <DataTable columns={columns} data={state.data ? state.data.events : null} error={state.error} pageSize={25} noun="events" empty={{ title: 'No activity yet', hint: 'Events appear here as your emails send.' }} />
      </Card>
    </>
  );
}
