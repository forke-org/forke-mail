/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { Layers, Plus } from 'lucide-react';
import { useRouter } from 'next/router';
import { useMemo } from 'react';
import DataTable, { Columns, Strong } from '../../components/DataTable';
import { Button, Card, PageHeader } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import type { Segment } from '../../lib/types';
import { useLoad } from '../../lib/useLoad';

export default function Segments() {
  const router = useRouter();
  const [state] = useLoad(() => api<{ segments: Segment[] }>('GET', '/api/segments'), []);
  const columns = useMemo<Columns<Segment>>(() => [
    { accessorKey: 'name', header: 'Segment', cell: ({ row }) => <Strong sub={`${row.original.rules.length} ${row.original.rules.length === 1 ? 'rule' : 'rules'} · match ${row.original.match}`}>{row.original.name}</Strong> },
    { accessorKey: 'size', header: 'Contacts', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'refreshed_at', header: 'Counted', cell: ({ getValue }) => <span className="text-muted">{getValue() ? formatDate(getValue()) : '-'}</span> },
  ], []);
  return (
    <>
      <PageHeader title="Segments" description="Groups of contacts defined by rules: fields, tags, engagement and events. They update before every send."
        actions={<Button variant="primary" icon={Plus} onClick={() => router.push('/segments/edit/')}>Create segment</Button>} />
      <Card flush>
        <DataTable columns={columns} data={state.data ? state.data.segments : null} error={state.error} noun="segments" onRowClick={(s) => router.push(`/segments/edit/?id=${s.id}`)}
          empty={{ title: 'No segments yet', hint: 'Group contacts by what they are or what they did, then send broadcasts to them.', action: <Button size="sm" icon={Layers} onClick={() => router.push('/segments/edit/')}>Create segment</Button> }} />
      </Card>
    </>
  );
}
