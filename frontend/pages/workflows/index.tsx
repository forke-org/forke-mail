import { Plus, Workflow as WorkflowIcon } from 'lucide-react';
import { useRouter } from 'next/router';
import { useMemo } from 'react';
import DataTable, { Columns, Strong } from '../../components/DataTable';
import { Badge, Button, Card, PageHeader } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import type { Workflow } from '../../lib/types';
import { useLoad } from '../../lib/useLoad';

export default function Workflows() {
  const router = useRouter();
  const [state] = useLoad(() => api<{ workflows: Workflow[] }>('GET', '/api/workflows'), []);
  const columns = useMemo<Columns<Workflow>>(() => [
    { accessorKey: 'name', header: 'Workflow', cell: ({ row }) => <Strong sub={<span className="font-mono">on {row.original.trigger_event}</span>}>{row.original.name}</Strong> },
    { id: 'steps', header: 'Steps', accessorFn: (w) => w.steps.length, cell: ({ row }) => <span className="text-muted">{row.original.steps.map((s) => (s.type === 'wait' ? 'wait' : 'send')).join(' → ') || '-'}</span> },
    { accessorKey: 'enabled', header: 'Status', cell: ({ getValue }) => <Badge value={getValue() ? 'live' : 'off'} tone={getValue() ? 'green' : 'neutral'} /> },
    { accessorKey: 'running', header: 'In progress', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'completed', header: 'Completed', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'updated_at', header: 'Updated', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);
  return (
    <>
      <PageHeader title="Workflows" description="Automated emails triggered by events from your app, like user.signed-up. Send events with POST /api/events."
        actions={<Button variant="primary" icon={Plus} onClick={() => router.push('/workflows/edit/')}>Create workflow</Button>} />
      <Card flush>
        <DataTable columns={columns} data={state.data ? state.data.workflows : null} error={state.error} noun="workflows" onRowClick={(w) => router.push(`/workflows/edit/?id=${w.id}`)}
          empty={{ title: 'No workflows yet', hint: 'Welcome series, trial reminders or receipts that go out when something happens in your app.', action: <Button size="sm" icon={WorkflowIcon} onClick={() => router.push('/workflows/edit/')}>Create workflow</Button> }} />
      </Card>
    </>
  );
}
