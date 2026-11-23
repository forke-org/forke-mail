/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { RefreshCw } from 'lucide-react';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { Badge, Button, Card, PageHeader, SearchInput, Select, Toolbar } from '../components/ui';
import { api } from '../lib/api';
import { formatDate } from '../lib/format';
import type { Delivery } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const pageSize = 50;
const statusOptions: [string, string][] = [
  ['problems', 'Problems only'], ['', 'All statuses'], ['bounced', 'Bounced'], ['complained', 'Spam reports'], ['failed', 'Failed'],
  ['skipped', 'Skipped'], ['delivered', 'Delivered'], ['sent', 'Sent, waiting'], ['pending', 'Pending'], ['test', 'Test sends'],
];

export default function Deliveries() {
  const router = useRouter();
  const [status, setStatus] = useState('problems');
  const [kind, setKind] = useState('');
  const [text, setText] = useState('');
  const [q, setQ] = useState('');
  const [page, setPage] = useState(0);

  useEffect(() => {
    if (typeof router.query.q === 'string') { setText(router.query.q); setQ(router.query.q); setStatus(''); }
  }, [router.query.q]);
  useEffect(() => {
    const id = setTimeout(() => { setQ(text.trim()); setPage(0); }, 300);
    return () => clearTimeout(id);
  }, [text]);

  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize) });
    if (status) params.set('status', status);
    if (kind) params.set('kind', kind);
    if (q) params.set('q', q);
    return api<{ deliveries: Delivery[]; total: number }>('GET', `/api/deliveries?${params}`);
  }, [status, kind, q, page]);

  const columns = useMemo<Columns<Delivery>>(() => [
    { accessorKey: 'to', header: 'Recipient', cell: ({ row }) => <Strong sub={row.original.subject}>{row.original.to}</Strong> },
    { accessorKey: 'kind', header: 'Type', cell: ({ getValue }) => <span className="text-muted capitalize">{getValue()}</span> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'reason', header: 'Reason', cell: ({ getValue }) => (getValue() ? <span className="font-mono text-xs text-muted break-all">{getValue()}</span> : <span className="text-faint">-</span>) },
    { accessorKey: 'at', header: 'Sent', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);

  return (
    <>
      <PageHeader title="Deliveries" description="Find any message by address or subject, across broadcasts and transactional mail. Starts on bounces, spam reports and failures." />
      <Card flush>
        <Toolbar>
          <SearchInput className="flex-1" placeholder="Search address or subject" value={text} onChange={setText} />
          <Select value={status} onChange={(e) => { setStatus(e.target.value); setPage(0); }} aria-label="Status" className="sm:w-44">
            {statusOptions.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </Select>
          <Select value={kind} onChange={(e) => { setKind(e.target.value); setPage(0); }} aria-label="Type" className="sm:w-44">
            <option value="">Broadcast and transactional</option>
            <option value="broadcast">Broadcast</option>
            <option value="transactional">Transactional</option>
          </Select>
          <Button className="sm:ml-auto" icon={RefreshCw} loading={state.loading} onClick={reload}>Refresh</Button>
        </Toolbar>
        <DataTable
          columns={columns}
          data={state.data ? state.data.deliveries : null}
          error={state.error}
          noun="messages"
          getRowId={(d) => `${d.kind}-${d.id}`}
          server={{ pageIndex: page, pageSize, total: state.data ? state.data.total : 0, onPageChange: setPage }}
          onRowClick={(d) => router.push(d.kind === 'transactional' ? `/transactional/view/?id=${d.id}` : `/emails/view/?id=${d.email_id}`)}
          empty={{ title: status === 'problems' ? 'No delivery problems' : 'No messages found', hint: status === 'problems' ? 'Nothing bounced, failed or was reported as spam.' : 'Try a different search or status.' }}
        />
      </Card>
    </>
  );
}
