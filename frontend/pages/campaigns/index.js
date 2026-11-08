import { useRouter } from 'next/router';
import { useMemo, useState } from 'react';
import CampaignForm from '../../components/CampaignForm';
import DataTable from '../../components/DataTable';
import Icon from '../../components/Icon';
import { Card, PageHead } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import { useLoad } from '../../lib/useLoad';

export default function Campaigns() {
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const [state] = useLoad(() => api('GET', '/api/campaigns'), []);

  const columns = useMemo(() => [
    { accessorKey: 'name', header: 'Campaign', cell: ({ row }) => <div><div className="cell-strong">{row.original.name}</div>{row.original.description ? <div className="small muted truncate">{row.original.description}</div> : null}</div> },
    { accessorKey: 'emails', header: 'Emails', meta: { className: 'num' }, cell: ({ getValue }) => number(getValue()) },
    { accessorKey: 'sent', header: 'Delivered to', meta: { className: 'num' }, cell: ({ getValue }) => number(getValue()) },
    { accessorKey: 'last_email_at', header: 'Last email', cell: ({ getValue }) => formatDate(getValue()) },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => formatDate(getValue()) },
  ], []);

  const newButton = <button className="btn btn-primary" type="button" onClick={() => setCreating(true)}><Icon name="plus" />New campaign</button>;
  return (
    <>
      <PageHead title="Campaigns" subtitle="Group related emails together, then send emails inside each campaign.">{newButton}</PageHead>
      <Card flush>
        <DataTable
          columns={columns}
          data={state.data ? state.data.campaigns : null}
          error={state.error}
          onRowClick={(c) => router.push(`/campaigns/view/?id=${c.id}`)}
          empty={{ title: 'No campaigns yet', hint: 'Create a campaign like "Product updates" or "Onboarding" to start sending.', action: newButton }}
        />
      </Card>
      {creating ? <CampaignForm onClose={() => setCreating(false)} onSaved={(c) => router.push(`/campaigns/view/?id=${c.id}`)} /> : null}
    </>
  );
}
