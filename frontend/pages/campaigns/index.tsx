import { Megaphone, Plus } from 'lucide-react';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useState } from 'react';
import CampaignForm from '../../components/CampaignForm';
import DataTable, { Columns, Strong } from '../../components/DataTable';
import { Button, Card, PageHeader } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import type { Campaign } from '../../lib/types';
import { useLoad } from '../../lib/useLoad';

export default function Campaigns() {
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const [state] = useLoad(() => api<{ campaigns: Campaign[] }>('GET', '/api/campaigns'), []);

  useEffect(() => {
    if (router.query.new) {
      setCreating(true);
      router.replace('/campaigns/', undefined, { shallow: true });
    }
  }, [router.query.new]);

  const columns = useMemo<Columns<Campaign>>(() => [
    {
      accessorKey: 'name', header: 'Campaign',
      cell: ({ row }) => (
        <div className="flex items-center gap-3">
          <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-accent-soft text-accent"><Megaphone className="size-4" /></span>
          <Strong sub={row.original.description || undefined}>{row.original.name}</Strong>
        </div>
      ),
    },
    { accessorKey: 'emails', header: 'Emails', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'sent', header: 'Recipients', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'last_email_at', header: 'Last email', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);

  const newButton = <Button variant="primary" icon={Plus} onClick={() => setCreating(true)}>New campaign</Button>;
  return (
    <>
      <PageHeader title="Broadcasts" description="Campaigns group related broadcasts. Open one to write and send emails to your contacts." actions={newButton} />
      <Card flush>
        <DataTable
          columns={columns}
          data={state.data ? state.data.campaigns : null}
          error={state.error}
          noun="campaigns"
          onRowClick={(c) => router.push(`/campaigns/view/?id=${c.id}`)}
          empty={{ title: 'No campaigns yet', hint: 'Create a campaign like "Blog" or "Changelog" to start sending.', action: newButton }}
        />
      </Card>
      {creating ? <CampaignForm onClose={() => setCreating(false)} onSaved={(c) => router.push(`/campaigns/view/?id=${c.id}`)} /> : null}
    </>
  );
}
