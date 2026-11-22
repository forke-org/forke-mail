/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { FileText, Plus } from 'lucide-react';
import { useRouter } from 'next/router';
import { useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../../components/DataTable';
import { Badge, Button, Card, PageHeader, SearchInput, Toolbar } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate } from '../../lib/format';
import { useSession } from '../../lib/session';
import type { EmailTemplate } from '../../lib/types';
import { useLoad } from '../../lib/useLoad';

export default function Templates() {
  const router = useRouter();
  const { me } = useSession();
  const canEdit = Boolean(me?.user) || me?.key?.scope === 'full_access';
  const [q, setQ] = useState('');
  const [state] = useLoad(() => api<{ templates: EmailTemplate[] }>('GET', '/api/templates'), []);
  const list = useMemo(() => (state.data ? state.data.templates.filter((t) => !q || `${t.name} ${t.subject}`.toLowerCase().includes(q.toLowerCase())) : null), [state.data, q]);
  const columns = useMemo<Columns<EmailTemplate>>(() => [
    { accessorKey: 'name', header: 'Template', cell: ({ row }) => <Strong sub={row.original.subject}>{row.original.name}</Strong> },
    { accessorKey: 'kind', header: 'Type', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { id: 'variables', header: 'Variables', accessorFn: (t) => t.variables.join(', '), cell: ({ row }) => (row.original.variables.length ? <span className="font-mono text-xs text-muted">{row.original.variables.map((v) => `{{${v}}}`).join(' ')}</span> : <span className="text-faint">-</span>) },
    { accessorKey: 'updated_at', header: 'Updated', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);
  return (
    <>
      <PageHeader title="Templates" description="Reusable emails for transactional sends and broadcasts. Put {{variables}} in the subject or body and fill them per send."
        actions={canEdit ? <Button variant="primary" icon={Plus} onClick={() => router.push('/templates/edit/')}>Create template</Button> : null} />
      <Card flush>
        <Toolbar><SearchInput className="flex-1" placeholder="Search templates" value={q} onChange={setQ} /></Toolbar>
        <DataTable columns={columns} data={list} error={state.error} noun="templates" onRowClick={(t) => router.push(`/templates/edit/?id=${t.id}`)}
          empty={{ title: 'No templates yet', hint: 'Create reusable emails for receipts, sign-in codes or newsletters.', action: canEdit ? <Button size="sm" icon={FileText} onClick={() => router.push('/templates/edit/')}>Create template</Button> : undefined }} />
      </Card>
    </>
  );
}
