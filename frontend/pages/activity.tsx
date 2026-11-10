import { RefreshCw } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/router';
import { useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { Badge, Button, Card, PageHeader, Select, Toolbar } from '../components/ui';
import { api } from '../lib/api';
import { formatDate, humanize, timeAgo } from '../lib/format';
import type { EventRow } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const stage: Record<string, number> = {
  queued: 0, sending_paused: 1, delayed: 1, sent: 2, delivered: 3, opened: 4, clicked: 5,
  soft_bounced: 6, unsubscribed: 6, rejected: 7, failed: 7, bounced: 7, complained: 8,
};

type Thread = EventRow & { events: number };

function latestPerMessage(events: EventRow[]): Thread[] {
  const threads = new Map<string, Thread>();
  for (const ev of events) {
    const key = ev.message_id ? `m:${ev.message_id}` : ev.email_id ? `e:${ev.email_id}:${ev.recipient || ''}` : `w:${ev.id}`;
    const seen = threads.get(key);
    if (!seen) { threads.set(key, { ...ev, events: 1 }); continue; }
    seen.events += 1;
    const newer = new Date(ev.created_at).getTime() - new Date(seen.created_at).getTime();
    if (newer > 0 || (newer === 0 && (stage[ev.type] ?? 0) > (stage[seen.type] ?? 0))) {
      threads.set(key, { ...ev, events: seen.events });
    }
  }
  return [...threads.values()].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
}

const types = ['sent', 'delivered', 'opened', 'clicked', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'rejected', 'delayed', 'failed', 'queued', 'sending_paused'];

export default function Activity() {
  const router = useRouter();
  const [type, setType] = useState('');
  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: '200' });
    if (type) params.set('type', type);
    return api<{ events: EventRow[] }>('GET', `/api/events?${params}`);
  }, [type]);

  const threads = useMemo(() => (state.data ? latestPerMessage(state.data.events) : null), [state.data]);

  const columns = useMemo<Columns<Thread>>(() => [
    {
      accessorKey: 'type', header: 'Status',
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-2">
          <Badge value={row.original.type} />
          {row.original.events > 1 ? <span className="font-mono text-[11px] text-faint tabular-nums">{row.original.events} events</span> : null}
        </span>
      ),
    },
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
    { accessorKey: 'created_at', header: 'Updated', cell: ({ getValue }) => <span className="text-muted" title={formatDate(getValue())}>{timeAgo(getValue())}</span> },
  ], []);

  return (
    <>
      <PageHeader title="Activity" description="The latest status of every message in this workspace. Each row updates as the message is sent, delivered, opened or bounced." />
      <Card flush>
        <Toolbar>
          <Select value={type} onChange={(e) => setType(e.target.value)} aria-label="Status" className="sm:w-52">
            <option value="">All statuses</option>
            {types.map((t) => <option key={t} value={t}>{humanize(t[0].toUpperCase() + t.slice(1))}</option>)}
          </Select>
          <Button className="sm:ml-auto" icon={RefreshCw} loading={state.loading} onClick={reload}>Refresh</Button>
        </Toolbar>
        <DataTable
          columns={columns}
          data={threads}
          error={state.error}
          pageSize={25}
          noun="messages"
          onRowClick={(t) => {
            if (t.message_id) router.push(`/transactional/view/?id=${t.message_id}`);
            else if (t.email_id) router.push(`/emails/view/?id=${t.email_id}`);
          }} empty={{ title: 'No activity yet', hint: 'Events appear here as your emails send.' }} />
      </Card>
    </>
  );
}
